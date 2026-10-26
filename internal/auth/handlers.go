package auth

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xyz/internal/httpx"
)

// Register 注册账号相关接口。
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth/status", s.handleStatus)
	mux.HandleFunc("POST /api/v1/auth/setup", s.handleSetup)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/v1/auth/me", s.handleMe)
	mux.HandleFunc("POST /api/v1/auth/password", s.handleChangePassword)

	mux.HandleFunc("GET /api/v1/users", Require(s.handleUsersList, RoleAdmin, RoleTeacher))
	mux.HandleFunc("POST /api/v1/users", Require(s.handleUserCreate, RoleAdmin))
	mux.HandleFunc("PUT /api/v1/users/{id}", Require(s.handleUserUpdate, RoleAdmin))
	mux.HandleFunc("DELETE /api/v1/users/{id}", Require(s.handleUserDelete, RoleAdmin))
}

func (s *Service) userCount() int {
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM users WHERE role='admin'").Scan(&n)
	return n
}

// GET /api/v1/auth/status：登录页用，告诉前端是否需要初始化、是否本机。
func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	var school string
	s.db.QueryRow("SELECT name FROM schools WHERE id=1").Scan(&school)
	httpx.JSON(w, map[string]interface{}{
		"setup_needed": s.userCount() == 0,
		"local":        IsLocal(r),
		"user":         s.Current(r),
		"school":       school,
	})
}

// POST /api/v1/auth/setup：首次创建管理员，只允许在服务器本机操作。
func (s *Service) handleSetup(w http.ResponseWriter, r *http.Request) {
	if !IsLocal(r) {
		httpx.Error(w, http.StatusForbidden, "请在服务器本机打开 http://localhost/setup 完成初始化")
		return
	}
	if s.userCount() > 0 {
		httpx.Error(w, http.StatusConflict, "已经初始化过了")
		return
	}
	var req struct{ Username, Name, Password, School string }
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.School = strings.TrimSpace(req.School); req.School != "" {
		s.db.Exec("UPDATE schools SET name=? WHERE id=1", req.School)
		s.cfg.SchoolName = req.School
		s.cfg.Save()
	}
	id, err := s.createUser(1, req.Username, req.Name, RoleAdmin, req.Password)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	s.createSession(w, r, id)
	httpx.OK(w)
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := ClientIP(r)
	if s.limiter.blocked(ip) {
		httpx.Error(w, http.StatusTooManyRequests, "尝试次数太多，请 15 分钟后再试")
		return
	}
	var req struct{ Username, Password string }
	if !httpx.Decode(w, r, &req) {
		return
	}
	var id int64
	var hash string
	var disabled int
	err := s.db.QueryRow("SELECT id,password_hash,disabled FROM users WHERE username=?", strings.TrimSpace(req.Username)).
		Scan(&id, &hash, &disabled)
	if err != nil || !CheckPassword(hash, req.Password) {
		s.limiter.addFail(ip)
		httpx.Error(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if disabled != 0 {
		httpx.Error(w, http.StatusForbidden, "账号已停用，请联系管理员")
		return
	}
	s.limiter.reset(ip)
	if err := s.createSession(w, r, id); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "登录失败")
		return
	}
	httpx.OK(w)
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w, r)
	httpx.OK(w)
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, FromRequest(r))
}

func (s *Service) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := FromRequest(r)
	if u == nil || u.Local {
		httpx.Error(w, http.StatusBadRequest, "本机免登录状态下没有个人账号，请先登录再修改密码")
		return
	}
	var req struct{ Old, New string }
	if !httpx.Decode(w, r, &req) {
		return
	}
	var hash string
	s.db.QueryRow("SELECT password_hash FROM users WHERE id=?", u.ID).Scan(&hash)
	if !CheckPassword(hash, req.Old) {
		httpx.Error(w, http.StatusBadRequest, "原密码不对")
		return
	}
	if !ValidPassword(req.New) {
		httpx.Error(w, http.StatusBadRequest, "新密码至少 6 位")
		return
	}
	nh, _ := HashPassword(req.New)
	s.db.Exec("UPDATE users SET password_hash=? WHERE id=?", nh, u.ID)
	httpx.OK(w)
}

// ===== 账号管理 =====

type userRow struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"created_at"`
	LastLogin string `json:"last_login"`
}

func (s *Service) createUser(schoolID int64, username, name, role, password string) (int64, error) {
	username = strings.TrimSpace(username)
	name = strings.TrimSpace(name)
	if username == "" || strings.ContainsAny(username, " /\\?#") {
		return 0, errf("用户名不能为空，也不能含空格和 / \\ ? #")
	}
	if name == "" {
		name = username
	}
	if !ValidRole(role) {
		return 0, errf("角色不正确")
	}
	if !ValidPassword(password) {
		return 0, errf("密码至少 6 位")
	}
	h, err := HashPassword(password)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec("INSERT INTO users(school_id,username,name,password_hash,role,created_at) VALUES(?,?,?,?,?,?)",
		schoolID, username, name, h, role, time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, errf("用户名「" + username + "」已存在")
		}
		return 0, err
	}
	return res.LastInsertId()
}

// GET /api/v1/users?role=teacher：教师也能看（选任课教师用），学生账号不列出给教师。
func (s *Service) handleUsersList(w http.ResponseWriter, r *http.Request) {
	u := FromRequest(r)
	q := "SELECT id,username,name,role,disabled,created_at,last_login FROM users WHERE school_id=?"
	args := []interface{}{u.SchoolID}
	if role := r.URL.Query().Get("role"); role != "" {
		q += " AND role=?"
		args = append(args, role)
	} else if !u.IsAdmin() {
		q += " AND role!='student'"
	}
	q += " ORDER BY CASE role WHEN 'admin' THEN 0 WHEN 'teacher' THEN 1 ELSE 2 END, name"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []userRow{}
	for rows.Next() {
		var x userRow
		var dis int
		rows.Scan(&x.ID, &x.Username, &x.Name, &x.Role, &dis, &x.CreatedAt, &x.LastLogin)
		x.Disabled = dis != 0
		list = append(list, x)
	}
	httpx.JSON(w, list)
}

func (s *Service) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Name, Role, Password string }
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.Role == "" {
		req.Role = RoleTeacher
	}
	id, err := s.createUser(FromRequest(r).SchoolID, req.Username, req.Name, req.Role, req.Password)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, map[string]int64{"id": id})
}

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

// PUT /api/v1/users/{id}：改姓名、角色、停用、重置密码（只传要改的字段）。
func (s *Service) handleUserUpdate(w http.ResponseWriter, r *http.Request) {
	me := FromRequest(r)
	id := pathID(r)
	var req struct {
		Name     *string `json:"name"`
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
		Password *string `json:"password"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	var curRole string
	if err := s.db.QueryRow("SELECT role FROM users WHERE id=? AND school_id=?", id, me.SchoolID).Scan(&curRole); err == sql.ErrNoRows {
		httpx.Error(w, 404, "账号不存在")
		return
	}
	// 防止把最后一个管理员降级/停用，导致没人能管理
	if curRole == RoleAdmin && ((req.Role != nil && *req.Role != RoleAdmin) || (req.Disabled != nil && *req.Disabled)) && s.activeAdmins() <= 1 {
		httpx.Error(w, 400, "至少要保留一个可用的管理员")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		s.db.Exec("UPDATE users SET name=? WHERE id=?", strings.TrimSpace(*req.Name), id)
	}
	if req.Role != nil {
		if !ValidRole(*req.Role) {
			httpx.Error(w, 400, "角色不正确")
			return
		}
		s.db.Exec("UPDATE users SET role=? WHERE id=?", *req.Role, id)
	}
	if req.Disabled != nil {
		s.db.Exec("UPDATE users SET disabled=? WHERE id=?", boolInt(*req.Disabled), id)
		if *req.Disabled {
			s.db.Exec("DELETE FROM sessions WHERE user_id=?", id)
		}
	}
	if req.Password != nil {
		if !ValidPassword(*req.Password) {
			httpx.Error(w, 400, "密码至少 6 位")
			return
		}
		h, _ := HashPassword(*req.Password)
		s.db.Exec("UPDATE users SET password_hash=? WHERE id=?", h, id)
		s.db.Exec("DELETE FROM sessions WHERE user_id=?", id) // 重置后旧登录全部失效
	}
	httpx.OK(w)
}

func (s *Service) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	me := FromRequest(r)
	id := pathID(r)
	if id == me.ID {
		httpx.Error(w, 400, "不能删除自己")
		return
	}
	var role string
	if err := s.db.QueryRow("SELECT role FROM users WHERE id=? AND school_id=?", id, me.SchoolID).Scan(&role); err != nil {
		httpx.Error(w, 404, "账号不存在")
		return
	}
	if role == RoleAdmin && s.activeAdmins() <= 1 {
		httpx.Error(w, 400, "至少要保留一个可用的管理员")
		return
	}
	s.db.Exec("DELETE FROM users WHERE id=?", id)
	httpx.OK(w)
}

func (s *Service) activeAdmins() int {
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM users WHERE role='admin' AND disabled=0").Scan(&n)
	return n
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type errf string

func (e errf) Error() string { return string(e) }
