// Package files 云盘：浏览、上传、删除、H5 课件托管、文件直链、Markdown、二维码分享、视频书签与素材标签。
package files

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/skip2/go-qrcode"

	"xyz/internal/httpx"
	"xyz/internal/safepath"
)

// MaxUploadBytes 单个文件上传上限（4GB，足够放课堂视频）。
const MaxUploadBytes = 4 << 30

type Service struct {
	db        *sql.DB
	root      string
	publicURL string           // 配置的对外地址，留空则用请求 Host
	index     http.HandlerFunc // 文件管理界面（内嵌 index.html）
	status    func() map[string]string
	sign      func(rel string) string // 分享链接签名，扫码者免登录只能看这一个文件
}

func New(db *sql.DB, root, publicURL string, index http.HandlerFunc, status func() map[string]string, sign func(string) string) *Service {
	return &Service{db: db, root: root, publicURL: publicURL, index: index, status: status, sign: sign}
}

// Register 注册路由；"/" 兜底交给 HandleRoot。
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/share", s.handleShare)
	mux.HandleFunc("/api/list", s.handleList)
	mux.HandleFunc("/api/upload", s.handleUpload)
	mux.HandleFunc("/api/delete", s.handleDelete)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/markers/get", s.handleGetMarkers)
	mux.HandleFunc("/api/markers/save", s.handleSaveMarkers)
	mux.HandleFunc("/api/md", s.handleGetMD)
	mux.HandleFunc("/api/tags/getAll", s.handleGetAllTags)
	mux.HandleFunc("/api/tags/save", s.handleSaveFileTags)
	mux.HandleFunc("/api/tree", s.handleGetTree)
	mux.HandleFunc("/files/", s.handleFileServe)
	mux.HandleFunc("/", s.HandleRoot)
}

// ===== 数据结构 =====
type FileInfo struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}
type ListResponse struct {
	Files []FileInfo `json:"files"`
	Path  string     `json:"path"`
}

// Marker 视频书签
type Marker struct {
	Time  float64 `json:"time"`
	Label string  `json:"label"`
}
type MarkersResponse struct {
	Markers []Marker `json:"markers"`
}

// TreeNode 素材目录树节点（备课系统用）
type TreeNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"isDir"`
	Tags     []string   `json:"tags"`
	Markers  []Marker   `json:"markers,omitempty"`
	Children []TreeNode `json:"children,omitempty"`
}

// resolve 把相对路径映射到数据目录内；越界返回 ok=false。
func (s *Service) resolve(rel string) (string, string, bool) {
	clean := safepath.CleanRel(rel)
	abs, ok := safepath.Join(s.root, clean)
	return clean, abs, ok
}

// ===== 主路由：H5 课件优先，其次文件，最后管理界面 =====
func (s *Service) HandleRoot(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path
	manage := r.URL.Query().Get("manage") == "1"

	_, absPath, ok := s.resolve(urlPath)
	if !ok {
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}
	info, err := os.Stat(absPath)
	if err != nil {
		s.index(w, r)
		return
	}
	if info.IsDir() {
		// 目录必须以 / 结尾，否则 index.html 里的相对资源会定位错误
		if urlPath != "/" && !strings.HasSuffix(urlPath, "/") {
			http.Redirect(w, r, urlPath+"/", http.StatusMovedPermanently)
			return
		}
		if !manage {
			indexPath := filepath.Join(absPath, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				serveFileCached(w, r, indexPath)
				return
			}
		}
		s.index(w, r)
		return
	}
	serveFileCached(w, r, absPath)
}

// setCacheHeaders 按扩展名设置缓存：字体/图片/音视频长缓存，页面每次校验。
func setCacheHeaders(w http.ResponseWriter, name string) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		w.Header().Set("Cache-Control", "no-cache")
	case ".woff", ".woff2", ".ttf", ".otf", ".eot",
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".bmp",
		".mp4", ".webm", ".mkv", ".mov", ".m4v",
		".mp3", ".wav", ".ogg", ".m4a", ".flac":
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	default:
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
}

func serveFileCached(w http.ResponseWriter, r *http.Request, absPath string) {
	setCacheHeaders(w, absPath)
	http.ServeFile(w, r, absPath) // 自带 HTTP Range，视频可拖动
}

// ===== 文件 API =====
func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	relPath, absPath, ok := s.resolve(r.URL.Query().Get("path"))
	if !ok {
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		httpx.JSON(w, ListResponse{Files: []FileInfo{}, Path: relPath})
		return
	}
	files := []FileInfo{}
	for _, e := range entries {
		name := e.Name()
		// 过滤：.json 文件以及 . 开头的隐藏项（含 .xyz 系统目录）
		if strings.HasSuffix(strings.ToLower(name), ".json") || strings.HasPrefix(name, ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{Name: name, IsDir: e.IsDir(), Size: info.Size()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	httpx.JSON(w, ListResponse{Files: files, Path: relPath})
}

func (s *Service) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	relPath, absPath, ok := s.resolve(r.URL.Query().Get("path"))
	if relPath == "" {
		http.Error(w, "缺少路径参数", http.StatusBadRequest)
		return
	}
	if !ok || isSystemPath(relPath) {
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		http.Error(w, "创建目录失败", http.StatusInternalServerError)
		return
	}
	// 先写临时文件，成功后再改名：上传中断不会留下半截文件覆盖原文件
	tmp := absPath + ".uploading"
	out, err := os.Create(tmp)
	if err != nil {
		http.Error(w, "创建文件失败", http.StatusInternalServerError)
		return
	}
	_, err = io.Copy(out, http.MaxBytesReader(w, r.Body, MaxUploadBytes))
	out.Close()
	if err != nil {
		os.Remove(tmp)
		http.Error(w, "上传失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	os.Remove(absPath)
	if err := os.Rename(tmp, absPath); err != nil {
		os.Remove(tmp)
		http.Error(w, "保存文件失败", http.StatusInternalServerError)
		return
	}
	w.Write([]byte("OK"))
}

// handleDelete 删除单个文件（目录非空时不会删除）。
func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	relPath, absPath, ok := s.resolve(r.URL.Query().Get("path"))
	if relPath == "" {
		http.Error(w, "缺少路径参数", http.StatusBadRequest)
		return
	}
	if !ok || isSystemPath(relPath) {
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		http.Error(w, "删除失败", http.StatusInternalServerError)
		return
	}
	w.Write([]byte("OK"))
}

// isSystemPath 禁止通过网页改动程序自用的 .xyz 目录（数据库、备份）。
func isSystemPath(rel string) bool {
	first := strings.SplitN(rel, "/", 2)[0]
	return strings.EqualFold(first, ".xyz")
}

func (s *Service) handleFileServe(w http.ResponseWriter, r *http.Request) {
	relPath, absPath, ok := s.resolve(strings.TrimPrefix(r.URL.Path, "/files/"))
	if relPath == "" {
		http.Error(w, "路径无效", http.StatusBadRequest)
		return
	}
	if !ok || isSystemPath(relPath) {
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}
	serveFileCached(w, r, absPath)
}

func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, s.status())
}

func (s *Service) handleGetMD(w http.ResponseWriter, r *http.Request) {
	relPath, absPath, ok := s.resolve(r.URL.Query().Get("path"))
	if relPath == "" {
		http.Error(w, "缺少路径参数", http.StatusBadRequest)
		return
	}
	if !ok || isSystemPath(relPath) {
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		http.Error(w, "读取文件失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

// handleShare 生成文件直链与二维码。配置了 public_url 时用它，否则用访问者的 Host。
func (s *Service) handleShare(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Path required", 400)
		return
	}
	rel := safepath.CleanRel(path)
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	base := s.publicURL
	if base == "" {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https" // 经 Cloudflare 隧道访问时为 https
		}
		base = scheme + "://" + r.Host
	}
	fullURL := fmt.Sprintf("%s/files/%s", base, strings.Join(parts, "/"))
	if s.sign != nil {
		fullURL += "?s=" + s.sign(rel)
	}
	png, err := qrcode.Encode(fullURL, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, "QR Generation failed", 500)
		return
	}
	httpx.JSON(w, map[string]string{"url": fullURL, "qr": base64.StdEncoding.EncodeToString(png)})
}

// ===== 视频书签（存数据库，每个文件一行） =====
func (s *Service) loadMarkers() map[string][]Marker {
	out := map[string][]Marker{}
	rows, err := s.db.Query("SELECT path, markers FROM markers")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p, m string
		if rows.Scan(&p, &m) == nil {
			var ms []Marker
			json.Unmarshal([]byte(m), &ms)
			out[p] = ms
		}
	}
	return out
}

func (s *Service) loadTags() map[string][]string {
	out := map[string][]string{}
	rows, err := s.db.Query("SELECT path, tags FROM file_tags")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p, t string
		if rows.Scan(&p, &t) == nil {
			var ts []string
			json.Unmarshal([]byte(t), &ts)
			out[p] = ts
		}
	}
	return out
}

func (s *Service) handleGetMarkers(w http.ResponseWriter, r *http.Request) {
	relPath := safepath.CleanRel(r.URL.Query().Get("path"))
	if relPath == "" {
		http.Error(w, "缺少路径参数", http.StatusBadRequest)
		return
	}
	markers := []Marker{}
	var m string
	if s.db.QueryRow("SELECT markers FROM markers WHERE path=?", relPath).Scan(&m) == nil {
		json.Unmarshal([]byte(m), &markers)
		if markers == nil {
			markers = []Marker{}
		}
	}
	httpx.JSON(w, MarkersResponse{Markers: markers})
}

func (s *Service) handleSaveMarkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	relPath := safepath.CleanRel(r.URL.Query().Get("path"))
	if relPath == "" {
		http.Error(w, "缺少路径参数", http.StatusBadRequest)
		return
	}
	var req MarkersResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "解析标注格式错误", http.StatusBadRequest)
		return
	}
	if req.Markers == nil {
		req.Markers = []Marker{}
	}
	data, _ := json.Marshal(req.Markers)
	if _, err := s.db.Exec(`INSERT INTO markers(path, markers) VALUES(?,?)
		ON CONFLICT(path) DO UPDATE SET markers=excluded.markers`, relPath, string(data)); err != nil {
		http.Error(w, "写入数据库失败", http.StatusInternalServerError)
		return
	}
	w.Write([]byte("OK"))
}

// ===== 素材标签（备课系统） =====

// handleGetAllTags 返回全部标签；有书签的文件自动带「已标注」。
func (s *Service) handleGetAllTags(w http.ResponseWriter, r *http.Request) {
	db := s.loadTags()
	for path := range s.loadMarkers() {
		db[path] = append(db[path], "已标注")
	}
	httpx.JSON(w, db)
}

func (s *Service) handleGetTree(w http.ResponseWriter, r *http.Request) {
	tagDB := s.loadTags()
	markerDB := s.loadMarkers()
	for path := range markerDB {
		tagDB[path] = append(tagDB[path], "已标注")
	}
	httpx.JSON(w, buildTree(s.root, "", tagDB, markerDB))
}

func isMediaFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp",
		".mp4", ".webm", ".mkv", ".mov", ".avi":
		return true
	}
	return false
}

func buildTree(basePath, relPath string, tagDB map[string][]string, markerDB map[string][]Marker) []TreeNode {
	entries, err := os.ReadDir(filepath.Join(basePath, filepath.FromSlash(relPath)))
	if err != nil {
		return nil
	}
	var nodes []TreeNode
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		childRel := name
		if relPath != "" {
			childRel = relPath + "/" + name
		}
		if e.IsDir() {
			if children := buildTree(basePath, childRel, tagDB, markerDB); len(children) > 0 {
				nodes = append(nodes, TreeNode{Name: name, Path: childRel, IsDir: true, Tags: tagDB[childRel], Children: children})
			}
		} else if isMediaFile(name) {
			nodes = append(nodes, TreeNode{Name: name, Path: childRel, Tags: tagDB[childRel], Markers: markerDB[childRel]})
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
	return nodes
}

// handleSaveFileTags 合并保存标签；空数组表示删除该文件的标签。
func (s *Service) handleSaveFileTags(w http.ResponseWriter, r *http.Request) {
	var newTags map[string][]string
	if err := json.NewDecoder(r.Body).Decode(&newTags); err != nil {
		http.Error(w, "Bad JSON", 400)
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		http.Error(w, "数据库错误", 500)
		return
	}
	for k, v := range newTags {
		if len(v) == 0 {
			_, err = tx.Exec("DELETE FROM file_tags WHERE path=?", k)
		} else {
			data, _ := json.Marshal(v)
			_, err = tx.Exec(`INSERT INTO file_tags(path,tags) VALUES(?,?)
				ON CONFLICT(path) DO UPDATE SET tags=excluded.tags`, k, string(data))
		}
		if err != nil {
			tx.Rollback()
			http.Error(w, "写入失败", 500)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "写入失败", 500)
		return
	}
	w.Write([]byte("OK"))
}
