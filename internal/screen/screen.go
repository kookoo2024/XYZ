// Package screen 教室大屏：课前轮播、随机点名。
package screen

import (
	"database/sql"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"xyz/internal/auth"
	"xyz/internal/config"
	"xyz/internal/httpx"
	"xyz/internal/safepath"
	"xyz/internal/store"
)

type Service struct {
	db  *sql.DB
	cfg *config.Config
}

func New(db *sql.DB, cfg *config.Config) *Service { return &Service{db: db, cfg: cfg} }

func (s *Service) Register(mux *http.ServeMux) {
	staff := []string{auth.RoleAdmin, auth.RoleTeacher}
	mux.HandleFunc("GET /api/v1/screen/slides", auth.Require(s.handleSlides, staff...))
	mux.HandleFunc("GET /api/v1/screen/call", auth.Require(s.handleCallState, staff...))
	mux.HandleFunc("POST /api/v1/screen/call", auth.Require(s.handleCall, staff...))
	mux.HandleFunc("POST /api/v1/screen/call/reset", auth.Require(s.handleReset, staff...))
}

var mediaExt = map[string]string{
	".jpg": "image", ".jpeg": "image", ".png": "image", ".gif": "image", ".webp": "image", ".bmp": "image", ".svg": "image",
	".mp4": "video", ".webm": "video",
}

type Slide struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Type string `json:"type"`
}

func (s *Service) className(id int64, school int64) string {
	var n string
	s.db.QueryRow("SELECT name FROM classes WHERE id=? AND school_id=?", id, school).Scan(&n)
	return n
}

// listMedia 列出文件夹下（不含子文件夹）的图片和视频，按文件名自然排序。
func listMedia(root, rel string) []Slide {
	abs, ok := safepath.Join(root, rel)
	if !ok {
		return nil
	}
	ents, err := os.ReadDir(abs)
	if err != nil {
		return nil
	}
	var out []Slide
	for _, e := range ents {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		typ, ok := mediaExt[strings.ToLower(filepath.Ext(e.Name()))]
		if !ok {
			continue
		}
		p := strings.Trim(rel+"/"+e.Name(), "/")
		segs := strings.Split(p, "/")
		for i := range segs {
			segs[i] = url.PathEscape(segs[i])
		}
		out = append(out, Slide{Name: e.Name(), URL: "/files/" + strings.Join(segs, "/"), Type: typ})
	}
	sort.Slice(out, func(i, j int) bool { return naturalLess(out[i].Name, out[j].Name) })
	return out
}

// GET /api/v1/screen/slides?class_id=  优先用「轮播/班级名」子文件夹，没有则用「轮播」根目录。
func (s *Service) handleSlides(w http.ResponseWriter, r *http.Request) {
	u := auth.FromRequest(r)
	dir := safepath.CleanRel(s.cfg.SlidesDir)
	cid, _ := strconv.ParseInt(r.URL.Query().Get("class_id"), 10, 64)
	folder := dir
	var list []Slide
	if name := s.className(cid, u.SchoolID); name != "" {
		list = listMedia(s.cfg.DataDir, dir+"/"+name)
		if len(list) > 0 {
			folder = dir + "/" + name
		}
	}
	if len(list) == 0 {
		list = listMedia(s.cfg.DataDir, dir)
	}
	if list == nil {
		list = []Slide{}
	}
	httpx.JSON(w, map[string]interface{}{"folder": folder, "slides": list})
}

// naturalLess 文件名自然排序：2.jpg 排在 10.jpg 前。
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		ad, bd := isDigit(a[0]), isDigit(b[0])
		if ad && bd {
			na, ra := takeNum(a)
			nb, rb := takeNum(b)
			if na != nb {
				return na < nb
			}
			a, b = ra, rb
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func takeNum(s string) (int, string) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	n, _ := strconv.Atoi(s[:i])
	return n, s[i:]
}

// ===== 随机点名：一轮内每人只点一次，全班点完自动进入下一轮 =====

func roundKey(cid int64) string { return "screen.round." + strconv.FormatInt(cid, 10) }

func (s *Service) round(cid int64) int {
	v, _ := store.GetKV(s.db, roundKey(cid))
	n, _ := strconv.Atoi(v)
	if n < 1 {
		n = 1
	}
	return n
}

type pickStudent struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	StudentNo string `json:"student_no"`
	Gender    string `json:"gender"`
	Called    bool   `json:"called"` // 本轮已点过
	Times     int    `json:"times"`  // 累计被点次数
}

func (s *Service) classOK(u *auth.User, cid int64) bool {
	return s.className(cid, u.SchoolID) != ""
}

func (s *Service) students(cid int64, round int) []pickStudent {
	rows, err := s.db.Query(`SELECT st.id, st.name, st.student_no, st.gender,
		EXISTS(SELECT 1 FROM call_log c WHERE c.student_id=st.id AND c.class_id=st.class_id AND c.round=?),
		(SELECT COUNT(*) FROM call_log c WHERE c.student_id=st.id)
		FROM students st WHERE st.class_id=?
		ORDER BY CASE WHEN st.student_no='' THEN 1 ELSE 0 END, LENGTH(st.student_no), st.student_no, st.id`, round, cid)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []pickStudent{}
	for rows.Next() {
		var p pickStudent
		var called int
		rows.Scan(&p.ID, &p.Name, &p.StudentNo, &p.Gender, &called, &p.Times)
		p.Called = called != 0
		out = append(out, p)
	}
	return out
}

// GET /api/v1/screen/call?class_id=  名单及本轮点名情况
func (s *Service) handleCallState(w http.ResponseWriter, r *http.Request) {
	cid, _ := strconv.ParseInt(r.URL.Query().Get("class_id"), 10, 64)
	if !s.classOK(auth.FromRequest(r), cid) {
		httpx.Error(w, 404, "班级不存在")
		return
	}
	rd := s.round(cid)
	httpx.JSON(w, map[string]interface{}{"round": rd, "students": s.students(cid, rd)})
}

// POST /api/v1/screen/call {class_id, count, exclude:[缺席学生id]}
func (s *Service) handleCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClassID int64   `json:"class_id"`
		Count   int     `json:"count"`
		Exclude []int64 `json:"exclude"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	if !s.classOK(auth.FromRequest(r), req.ClassID) {
		httpx.Error(w, 404, "班级不存在")
		return
	}
	if req.Count < 1 {
		req.Count = 1
	}
	if req.Count > 10 {
		req.Count = 10
	}
	ex := map[int64]bool{}
	for _, id := range req.Exclude {
		ex[id] = true
	}
	rd := s.round(req.ClassID)
	all := s.students(req.ClassID, rd)
	var present []pickStudent
	for _, p := range all {
		if !ex[p.ID] {
			present = append(present, p)
		}
	}
	if len(present) == 0 {
		httpx.Error(w, 400, "名单为空，请先在管理后台导入学生")
		return
	}
	if req.Count > len(present) {
		req.Count = len(present)
	}
	var fresh []pickStudent
	for _, p := range present {
		if !p.Called {
			fresh = append(fresh, p)
		}
	}
	rand.Shuffle(len(fresh), func(i, j int) { fresh[i], fresh[j] = fresh[j], fresh[i] })
	picked := fresh
	newRound := false
	if len(picked) >= req.Count {
		picked = picked[:req.Count]
	} else {
		// 本轮剩下的人不够：先点完剩下的，再开新一轮补足
		newRound = true
		rd++
		store.SetKV(s.db, roundKey(req.ClassID), strconv.Itoa(rd))
		got := map[int64]bool{}
		for _, p := range picked {
			got[p.ID] = true
		}
		var rest []pickStudent
		for _, p := range present {
			if !got[p.ID] {
				rest = append(rest, p)
			}
		}
		rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
		picked = append(picked, rest[:req.Count-len(picked)]...)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for i := range picked {
		// 新一轮补进来的人记在新一轮里；上一轮剩下的人也记到当前轮，保证新一轮不会马上又点到同一批
		s.db.Exec("INSERT INTO call_log(class_id,student_id,round,at) VALUES(?,?,?,?)", req.ClassID, picked[i].ID, rd, now)
		picked[i].Times++
		picked[i].Called = true
	}
	left := 0
	for _, p := range s.students(req.ClassID, rd) {
		if !p.Called && !ex[p.ID] {
			left++
		}
	}
	httpx.JSON(w, map[string]interface{}{"picked": picked, "round": rd, "new_round": newRound, "left": left})
}

// POST /api/v1/screen/call/reset {class_id}  手动开始新一轮
func (s *Service) handleReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClassID int64 `json:"class_id"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	if !s.classOK(auth.FromRequest(r), req.ClassID) {
		httpx.Error(w, 404, "班级不存在")
		return
	}
	rd := s.round(req.ClassID) + 1
	store.SetKV(s.db, roundKey(req.ClassID), strconv.Itoa(rd))
	httpx.JSON(w, map[string]int{"round": rd})
}
