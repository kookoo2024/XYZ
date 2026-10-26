// Package auth 登录、会话、角色与权限中间件，以及免登录的分享链接签名。
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"xyz/internal/config"
	"xyz/internal/store"
)

const (
	RoleAdmin   = "admin"   // 管理员：管全校账号、班级、学科、任课
	RoleTeacher = "teacher" // 教师：使用全部教学功能；班主任可管本班学生
	RoleStudent = "student" // 学生：后续练习/PK 使用，目前只能登录
	CookieName  = "xyz_session"
)

// ValidRole 判断角色名是否合法。
func ValidRole(r string) bool { return r == RoleAdmin || r == RoleTeacher || r == RoleStudent }

// User 当前请求的登录者。
type User struct {
	ID       int64  `json:"id"`
	SchoolID int64  `json:"school_id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Local    bool   `json:"local"` // 服务器本机免登录
}

func (u *User) IsAdmin() bool { return u != nil && u.Role == RoleAdmin }
func (u *User) IsStaff() bool { return u != nil && (u.Role == RoleAdmin || u.Role == RoleTeacher) }

type ctxKey struct{}

// FromRequest 取当前用户（未登录为 nil）。
func FromRequest(r *http.Request) *User {
	u, _ := r.Context().Value(ctxKey{}).(*User)
	return u
}

// WithUser 把用户放进 context（测试也会用到）。
func WithUser(r *http.Request, u *User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
}

type Service struct {
	db      *sql.DB
	cfg     *config.Config
	secret  []byte
	limiter *limiter
}

func New(db *sql.DB, cfg *config.Config) *Service {
	return &Service{db: db, cfg: cfg, secret: loadSecret(db), limiter: newLimiter()}
}

// loadSecret 读取（或首次生成）用于签名分享链接的密钥。
func loadSecret(db *sql.DB) []byte {
	const key = "auth.share_secret"
	if v, ok := store.GetKV(db, key); ok {
		if b, err := hex.DecodeString(v); err == nil && len(b) >= 32 {
			return b
		}
	}
	b := make([]byte, 32)
	rand.Read(b)
	store.SetKV(db, key, hex.EncodeToString(b))
	return b
}

// ===== 本机判定 =====

// IsLocal 请求来自服务器本机，且不是经 Cloudflare/反向代理转发进来的。
func IsLocal(r *http.Request) bool {
	if r.Header.Get("Cf-Connecting-Ip") != "" || r.Header.Get("X-Forwarded-For") != "" ||
		r.Header.Get("X-Real-Ip") != "" || r.Header.Get("Cf-Ray") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ClientIP 尽量取真实客户端 IP（用于登录限流与记录）。
func ClientIP(r *http.Request) string {
	if v := r.Header.Get("Cf-Connecting-Ip"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") ||
		strings.Contains(r.Header.Get("Cf-Visitor"), "https")
}

// ===== 会话 =====

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func (s *Service) createSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	ttl := time.Duration(s.cfg.Auth.SessionDays) * 24 * time.Hour
	now := time.Now()
	_, err := s.db.Exec("INSERT INTO sessions(token_hash,user_id,expires_at,created_at,ip,user_agent) VALUES(?,?,?,?,?,?)",
		hashToken(token), userID, now.Add(ttl).Unix(), now.Unix(), ClientIP(r), truncate(r.UserAgent(), 200))
	if err != nil {
		return err
	}
	s.db.Exec("UPDATE users SET last_login=datetime('now','localtime') WHERE id=?", userID)
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: isHTTPS(r), Expires: now.Add(ttl),
	})
	return nil
}

func (s *Service) clearSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		s.db.Exec("DELETE FROM sessions WHERE token_hash=?", hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
}

// sessionUser 根据 cookie 查用户；快过期时自动续期。
func (s *Service) sessionUser(r *http.Request) *User {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	th := hashToken(c.Value)
	var u User
	var exp int64
	err = s.db.QueryRow(`SELECT u.id,u.school_id,u.username,u.name,u.role,s.expires_at
		FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND u.disabled=0`, th).
		Scan(&u.ID, &u.SchoolID, &u.Username, &u.Name, &u.Role, &exp)
	if err != nil {
		return nil
	}
	now := time.Now()
	if exp < now.Unix() {
		s.db.Exec("DELETE FROM sessions WHERE token_hash=?", th)
		return nil
	}
	ttl := time.Duration(s.cfg.Auth.SessionDays) * 24 * time.Hour
	if time.Unix(exp, 0).Sub(now) < ttl/2 {
		s.db.Exec("UPDATE sessions SET expires_at=? WHERE token_hash=?", now.Add(ttl).Unix(), th)
	}
	return &u
}

// Current 解析请求者：先看登录会话，再看是否本机免登录。
func (s *Service) Current(r *http.Request) *User {
	if u := s.sessionUser(r); u != nil {
		return u
	}
	if s.cfg.Auth.TrustLocalhost && IsLocal(r) {
		return &User{ID: 0, SchoolID: 1, Username: "localhost", Name: "本机", Role: RoleAdmin, Local: true}
	}
	return nil
}

// ===== 分享链接签名：/files/xxx?s=签名，学生扫码免登录，只能看这一个文件 =====

// SignPath 为相对路径生成签名。
func (s *Service) SignPath(rel string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte("files:" + rel))
	return hex.EncodeToString(m.Sum(nil)[:12])
}

func (s *Service) validShare(r *http.Request) bool {
	sig := r.URL.Query().Get("s")
	if sig == "" || !strings.HasPrefix(r.URL.Path, "/files/") {
		return false
	}
	rel, err := url.PathUnescape(strings.TrimPrefix(r.URL.EscapedPath(), "/files/"))
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(s.SignPath(cleanSlash(rel))))
}

func cleanSlash(p string) string { return strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/") }

// ===== 中间件 =====

// 不需要登录的路径
func isPublic(p string) bool {
	switch p {
	case "/login", "/setup", "/favicon.ico",
		"/api/v1/auth/login", "/api/v1/auth/status", "/api/v1/auth/setup", "/api/v1/auth/logout":
		return true
	}
	return strings.HasPrefix(p, "/assets/")
}

// 学生目前只能访问账号相关接口（练习/PK 上线后再开放）
func studentAllowed(p string) bool {
	return strings.HasPrefix(p, "/api/v1/auth/") || p == "/login" || strings.HasPrefix(p, "/assets/") || p == "/me"
}

// Middleware 给每个请求识别用户；未登录的页面请求跳转登录页，接口返回 401。
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := s.Current(r)
		if u != nil {
			r = WithUser(r, u)
		}
		p := r.URL.Path
		switch {
		case isPublic(p):
		case u == nil && r.Method == http.MethodGet && s.validShare(r):
		case u == nil:
			if strings.HasPrefix(p, "/api/") || r.Method != http.MethodGet {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"请先登录","login":"/login"}`))
				return
			}
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		case u.Role == RoleStudent && !studentAllowed(p):
			http.Error(w, "学生账号暂无此功能权限", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Require 包装处理函数：只允许指定角色。
func Require(h http.HandlerFunc, roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := FromRequest(r)
		if u == nil {
			http.Error(w, `{"error":"请先登录"}`, http.StatusUnauthorized)
			return
		}
		for _, role := range roles {
			if u.Role == role {
				h(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"没有权限"}`))
	}
}

// ===== 登录限流：同一 IP 15 分钟内失败 8 次，锁 15 分钟 =====

type limiter struct {
	mu   sync.Mutex
	fail map[string][]time.Time
}

func newLimiter() *limiter { return &limiter{fail: map[string][]time.Time{}} }

const (
	limitWindow = 15 * time.Minute
	limitMax    = 8
)

func (l *limiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(ip)) >= limitMax
}

func (l *limiter) recent(ip string) []time.Time {
	cut := time.Now().Add(-limitWindow)
	ts := l.fail[ip][:0]
	for _, t := range l.fail[ip] {
		if t.After(cut) {
			ts = append(ts, t)
		}
	}
	if len(ts) == 0 {
		delete(l.fail, ip)
		return nil
	}
	l.fail[ip] = ts
	return ts
}

func (l *limiter) addFail(ip string) {
	l.mu.Lock()
	l.fail[ip] = append(l.recent(ip), time.Now())
	l.mu.Unlock()
}

func (l *limiter) reset(ip string) {
	l.mu.Lock()
	delete(l.fail, ip)
	l.mu.Unlock()
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
