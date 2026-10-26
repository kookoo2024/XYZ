// Package update 在线更新：从 GitHub Releases 检查新版本，下载、校验 SHA256、替换 exe 并重启。
//
// 流程：检查 → 下载到 XYZ.exe.new → 校验 → 当前 exe 改名为 XYZ.exe.old → 新文件改名为 XYZ.exe → 重启。
// 旧版保留为 XYZ.exe.old，可一键回退。国内直连 GitHub 下载慢时依次尝试镜像。
package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"xyz/internal/auth"
	"xyz/internal/httpx"
)

// Options 由配置传入。
type Options struct {
	Repo    string   // 如 kookoo2024/XYZ
	Mirrors []string // 下载前缀，"" 表示直连 GitHub
	Current string   // 当前版本号
	Asset   string   // 发布包里的文件名，默认 XYZ.exe
}

// RestartFunc 由 main 提供：关闭服务和数据库，启动 exePath，然后退出当前进程。
type RestartFunc func(exePath string)

type Release struct {
	Version string `json:"version"`
	Tag     string `json:"tag"`
	Notes   string `json:"notes"`
	URL     string `json:"url"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Date    string `json:"date"`
	Page    string `json:"page"`
}

type Status struct {
	Current   string   `json:"current"`
	Latest    *Release `json:"latest"`
	HasUpdate bool     `json:"has_update"`
	CheckedAt string   `json:"checked_at"`
	State     string   `json:"state"` // idle / checking / downloading / restarting / error
	Progress  int      `json:"progress"`
	Message   string   `json:"message"`
	CanApply  bool     `json:"can_apply"`
	CanBack   bool     `json:"can_rollback"`
	Platform  string   `json:"platform"`
}

type Service struct {
	opt     Options
	restart RestartFunc
	client  *http.Client
	mu      sync.Mutex
	st      Status
}

func New(opt Options, restart RestartFunc) *Service {
	if opt.Asset == "" {
		opt.Asset = "XYZ.exe"
	}
	s := &Service{opt: opt, restart: restart, client: &http.Client{Timeout: 20 * time.Second}}
	s.st = Status{Current: opt.Current, State: "idle", Platform: runtime.GOOS + "/" + runtime.GOARCH}
	return s
}

// StartAutoCheck 启动后 1 分钟检查一次，之后每 12 小时一次；只提示，不自动安装（避免上课时重启）。
func (s *Service) StartAutoCheck() {
	go func() {
		time.Sleep(time.Minute)
		for {
			if _, err := s.Check(); err != nil {
				log.Println("检查更新失败:", err)
			}
			time.Sleep(12 * time.Hour)
		}
	}()
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/update/status", auth.Require(s.handleStatus, auth.RoleAdmin, auth.RoleTeacher))
	mux.HandleFunc("POST /api/v1/update/check", auth.Require(s.handleCheck, auth.RoleAdmin))
	mux.HandleFunc("POST /api/v1/update/apply", auth.Require(s.handleApply, auth.RoleAdmin))
	mux.HandleFunc("POST /api/v1/update/rollback", auth.Require(s.handleRollback, auth.RoleAdmin))
}

func (s *Service) snapshot() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.st
	exe, err := os.Executable()
	st.CanApply = runtime.GOOS == "windows" && err == nil && s.restart != nil
	if err == nil {
		if _, e := os.Stat(exe + ".old"); e == nil && st.CanApply {
			st.CanBack = true
		}
	}
	return st
}

func (s *Service) set(f func(*Status)) {
	s.mu.Lock()
	f(&s.st)
	s.mu.Unlock()
}

// ===== 检查 =====

type ghRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	Assets      []struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

var apiBase = "https://api.github.com" // 测试时替换

var reSHA = regexp.MustCompile(`(?i)sha256[^0-9a-f]{0,20}([0-9a-f]{64})`)

// Check 查询 GitHub 最新正式版本。
func (s *Service) Check() (*Release, error) {
	s.set(func(st *Status) {
		if st.State == "idle" || st.State == "error" {
			st.State, st.Message = "checking", "正在检查…"
		}
	})
	rel, err := s.fetchLatest()
	s.set(func(st *Status) {
		st.CheckedAt = time.Now().Format("2006-01-02 15:04")
		if st.State == "checking" {
			st.State, st.Message = "idle", ""
		}
		if err != nil {
			if st.State == "idle" {
				st.State, st.Message = "error", err.Error()
			}
			return
		}
		st.Latest = rel
		st.HasUpdate = Newer(rel.Version, s.opt.Current)
	})
	return rel, err
}

func (s *Service) fetchLatest() (*Release, error) {
	req, _ := http.NewRequest("GET", apiBase+"/repos/"+s.opt.Repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "XYZ-updater")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连不上 GitHub：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, errors.New("还没有发布任何版本（或仓库不是公开的）")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub 返回 %d", resp.StatusCode)
	}
	var g ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		return nil, err
	}
	rel := &Release{Tag: g.TagName, Version: strings.TrimPrefix(g.TagName, "v"), Notes: g.Body, Date: g.PublishedAt, Page: g.HTMLURL}
	for _, a := range g.Assets {
		if strings.EqualFold(a.Name, s.opt.Asset) {
			rel.URL, rel.Size = a.URL, a.Size
			if strings.HasPrefix(a.Digest, "sha256:") {
				rel.SHA256 = strings.ToLower(strings.TrimPrefix(a.Digest, "sha256:"))
			}
		}
	}
	if rel.SHA256 == "" {
		if m := reSHA.FindStringSubmatch(g.Body); m != nil {
			rel.SHA256 = strings.ToLower(m[1])
		}
	}
	if rel.URL == "" {
		return nil, fmt.Errorf("版本 %s 里没有 %s", g.TagName, s.opt.Asset)
	}
	return rel, nil
}

// Newer 判断 a 是否比 b 新（按 主.次.修订 比较；b 为 dev 视为最旧）。
func Newer(a, b string) bool {
	pa, pb := parseVer(a), parseVer(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parseVer(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

// ===== 下载并安装 =====

type progressWriter struct {
	total, n int64
	on       func(int)
	last     int
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if p.total > 0 {
		if pc := int(p.n * 100 / p.total); pc != p.last {
			p.last = pc
			p.on(pc)
		}
	}
	return len(b), nil
}

// download 依次尝试各镜像，返回校验通过的临时文件路径。
func (s *Service) download(rel *Release, dst string) error {
	mirrors := s.opt.Mirrors
	if len(mirrors) == 0 {
		mirrors = []string{""}
	}
	var lastErr error
	for _, m := range mirrors {
		src := m + rel.URL
		label := "GitHub"
		if m != "" {
			label = "镜像 " + strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(m, "https://"), "http://"), "/")
		}
		s.set(func(st *Status) { st.Progress, st.Message = 0, "正在从 "+label+" 下载…" })
		err := s.downloadOne(src, dst, rel, label)
		if err == nil {
			return nil
		}
		log.Printf("更新下载失败（%s）: %v", label, err)
		lastErr = fmt.Errorf("%s：%v", label, err)
		os.Remove(dst)
	}
	return lastErr
}

func (s *Service) downloadOne(src, dst string, rel *Release, label string) error {
	c := &http.Client{Timeout: 10 * time.Minute}
	req, _ := http.NewRequest("GET", src, nil)
	req.Header.Set("User-Agent", "XYZ-updater")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	h := sha256.New()
	total := resp.ContentLength
	if total <= 0 {
		total = rel.Size
	}
	pw := &progressWriter{total: total, on: func(p int) {
		s.set(func(st *Status) { st.Progress, st.Message = p, fmt.Sprintf("正在从 %s 下载… %d%%", label, p) })
	}}
	n, err := io.Copy(io.MultiWriter(f, h, pw), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if rel.Size > 0 && n != rel.Size {
		return fmt.Errorf("文件大小不对（%d / %d）", n, rel.Size)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if rel.SHA256 != "" && got != rel.SHA256 {
		return errors.New("SHA256 校验失败，文件可能被篡改")
	}
	if n < 1<<20 {
		return errors.New("下载的文件太小，不是有效程序")
	}
	return nil
}

// Apply 后台下载并安装最新版，完成后重启。
func (s *Service) Apply() error {
	st := s.snapshot()
	if !st.CanApply {
		return errors.New("当前系统不支持在线更新，请手动替换程序")
	}
	if st.State == "downloading" || st.State == "restarting" {
		return errors.New("正在更新中")
	}
	rel, err := s.Check()
	if err != nil {
		return err
	}
	if !Newer(rel.Version, s.opt.Current) {
		return errors.New("已经是最新版本")
	}
	s.set(func(st *Status) { st.State, st.Progress, st.Message = "downloading", 0, "准备下载…" })
	go func() {
		if err := s.install(rel); err != nil {
			log.Println("在线更新失败:", err)
			s.set(func(st *Status) { st.State, st.Message = "error", "更新失败："+err.Error() })
		}
	}()
	return nil
}

func (s *Service) install(rel *Release) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	newPath, oldPath := exe+".new", exe+".old"
	if err := s.download(rel, newPath); err != nil {
		return err
	}
	// Windows 不能覆盖正在运行的 exe，但可以改名
	os.Remove(oldPath)
	if err := os.Rename(exe, oldPath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("无法替换程序（没有写权限？）：%v", err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		os.Rename(oldPath, exe) // 还原
		return err
	}
	log.Printf("已更新到 %s，旧版保存为 %s，正在重启", rel.Version, filepath.Base(oldPath))
	s.set(func(st *Status) {
		st.State, st.Progress, st.Message = "restarting", 100, "安装完成，正在重启…"
	})
	go func() {
		time.Sleep(800 * time.Millisecond) // 让前端收到最后状态
		s.restart(exe)
	}()
	return nil
}

// Rollback 回退到 XYZ.exe.old 并重启。
func (s *Service) Rollback() error {
	st := s.snapshot()
	if !st.CanBack {
		return errors.New("没有可回退的旧版本")
	}
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	oldPath, tmp := exe+".old", exe+".rollback"
	os.Remove(tmp)
	if err := os.Rename(exe, tmp); err != nil {
		return err
	}
	if err := os.Rename(oldPath, exe); err != nil {
		os.Rename(tmp, exe)
		return err
	}
	os.Rename(tmp, oldPath) // 新版变成 .old，可再切回去
	log.Println("已回退到上一版本，正在重启")
	s.set(func(st *Status) { st.State, st.Message = "restarting", "已回退，正在重启…" })
	go func() {
		time.Sleep(800 * time.Millisecond)
		s.restart(exe)
	}()
	return nil
}

// Cleanup 启动时删掉上次残留的临时文件。
func Cleanup() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".new")
		os.Remove(exe + ".rollback")
	}
}

// ===== 接口 =====

func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) { httpx.JSON(w, s.snapshot()) }

func (s *Service) handleCheck(w http.ResponseWriter, r *http.Request) {
	if _, err := s.Check(); err != nil {
		httpx.Error(w, http.StatusBadGateway, err.Error())
		return
	}
	httpx.JSON(w, s.snapshot())
}

func (s *Service) handleApply(w http.ResponseWriter, r *http.Request) {
	if err := s.Apply(); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, s.snapshot())
}

func (s *Service) handleRollback(w http.ResponseWriter, r *http.Request) {
	if err := s.Rollback(); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, s.snapshot())
}
