// Package pk 教室大屏双人 PK / 单人练习：从题库按学科、章节、知识点抽选择题，记录每题作答，统计成绩和错题。
package pk

import (
	"database/sql"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"xyz/internal/auth"
	"xyz/internal/httpx"
)

// DefaultSubject 旧题库没有学科字段，空值视为数学。
const DefaultSubject = "数学"

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) Register(mux *http.ServeMux) {
	staff := []string{auth.RoleAdmin, auth.RoleTeacher}
	mux.HandleFunc("GET /api/v1/pk/scope", auth.Require(s.handleScope, staff...))
	mux.HandleFunc("POST /api/v1/pk/questions", auth.Require(s.handleQuestions, staff...))
	mux.HandleFunc("POST /api/v1/pk/matches", auth.Require(s.handleSave, staff...))
	mux.HandleFunc("GET /api/v1/pk/stats", auth.Require(s.handleStats, staff...))
	mux.HandleFunc("DELETE /api/v1/pk/matches/{id}", auth.Require(s.handleDelete, staff...))
}

// PK 只用能自动判分的单选题（含判断题）：至少 2 个选项、答案是单个字母。
const usable = `questions.type IN ('单选','判断','选择') AND json_array_length(questions.options) >= 2 AND length(trim(questions.answer)) = 1`

func subjExpr() string {
	return "CASE WHEN questions.subject='' THEN '" + DefaultSubject + "' ELSE questions.subject END"
}

func scopeWhere(subject, chapter, section, tag string) (string, []interface{}) {
	w := []string{usable}
	var args []interface{}
	if subject != "" {
		w = append(w, subjExpr()+"=?")
		args = append(args, subject)
	}
	if chapter != "" {
		w = append(w, "questions.chapter=?")
		args = append(args, chapter)
	}
	if section != "" {
		w = append(w, "questions.section=?")
		args = append(args, section)
	}
	if tag != "" {
		w = append(w, "EXISTS(SELECT 1 FROM json_each(questions.tags) WHERE value=?)")
		args = append(args, tag)
	}
	return strings.Join(w, " AND "), args
}

type named struct {
	Name     string  `json:"name"`
	Count    int     `json:"count"`
	Children []named `json:"children,omitempty"`
}

// GET /api/v1/pk/scope?subject=&chapter=  可出题的学科、章节、小节、知识点及题量
func (s *Service) handleScope(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	subject, chapter := q.Get("subject"), q.Get("chapter")
	out := map[string]interface{}{}

	subs := []named{}
	if rows, err := s.db.Query("SELECT " + subjExpr() + " sj, COUNT(*) FROM questions WHERE " + usable + " GROUP BY sj ORDER BY COUNT(*) DESC"); err == nil {
		for rows.Next() {
			var n named
			rows.Scan(&n.Name, &n.Count)
			subs = append(subs, n)
		}
		rows.Close()
	}
	out["subjects"] = subs

	where, args := scopeWhere(subject, "", "", "")
	chs := []named{}
	idx := map[string]int{}
	if rows, err := s.db.Query("SELECT chapter, section, COUNT(*) FROM questions WHERE "+where+" GROUP BY chapter, section ORDER BY chapter, section", args...); err == nil {
		for rows.Next() {
			var c, sec string
			var n int
			rows.Scan(&c, &sec, &n)
			if c == "" {
				c = "（未分章节）"
			}
			i, ok := idx[c]
			if !ok {
				i = len(chs)
				idx[c] = i
				chs = append(chs, named{Name: c})
			}
			chs[i].Count += n
			if sec != "" {
				chs[i].Children = append(chs[i].Children, named{Name: sec, Count: n})
			}
		}
		rows.Close()
	}
	sort.SliceStable(chs, func(i, j int) bool { return natLess(chs[i].Name, chs[j].Name) })
	out["chapters"] = chs

	ch := chapter
	if ch == "（未分章节）" {
		ch = ""
	}
	where, args = scopeWhere(subject, ch, "", "")
	tags := []named{}
	if rows, err := s.db.Query("SELECT j.value, COUNT(*) FROM questions, json_each(questions.tags) j WHERE "+where+" GROUP BY j.value ORDER BY COUNT(*) DESC LIMIT 200", args...); err == nil {
		for rows.Next() {
			var n named
			rows.Scan(&n.Name, &n.Count)
			if strings.TrimSpace(n.Name) != "" {
				tags = append(tags, n)
			}
		}
		rows.Close()
	}
	out["tags"] = tags
	httpx.JSON(w, out)
}

// natLess 让「第二章」排在「第十章」前、「2.1」在「10.1」前（按首个数字比较）。
func natLess(a, b string) bool {
	na, nb := leadNum(a), leadNum(b)
	if na != nb {
		return na < nb
	}
	return a < b
}

var cnNum = map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9, '十': 10}

func leadNum(s string) int {
	n, seen := 0, false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n, seen = n*10+int(c-'0'), true
			continue
		}
		if seen {
			break
		}
	}
	if seen {
		return n
	}
	// 中文数字：第十二章
	rs := []rune(s)
	for i, c := range rs {
		if v, ok := cnNum[c]; ok {
			total := 0
			for _, d := range rs[i:] {
				dv, ok := cnNum[d]
				if !ok {
					break
				}
				if dv == 10 {
					if total == 0 {
						total = 1
					}
					total *= 10
				} else {
					total += dv
				}
			}
			_ = v
			return total
		}
	}
	return 1 << 30
}

// Question 发给大屏的题目。Answer 为正确选项在 Options 里的下标。
type Question struct {
	ID       int64    `json:"id"`
	Type     string   `json:"type"`
	Stem     string   `json:"stem"`
	Options  []string `json:"options"`
	Letters  []string `json:"letters"` // 每个选项原来的字母，便于对照题库
	Answer   int      `json:"answer"`
	Solution string   `json:"solution"`
	Images   []string `json:"images"`
	Chapter  string   `json:"chapter"`
	Section  string   `json:"section"`
}

// POST /api/v1/pk/questions {subject, chapter, section, tag, count, options, class_id}
// 随机抽题；本班最近答过的题往后排；options>0 时把选项裁到该数量（保留正确项，其余随机）。
func (s *Service) handleQuestions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject, Chapter, Section, Tag string
		Count, Options                 int
		ClassID                        int64 `json:"class_id"`
		Shuffle                        bool  // 打乱选项顺序
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.Count <= 0 || req.Count > 50 {
		req.Count = 6
	}
	ch := req.Chapter
	if ch == "（未分章节）" {
		ch = ""
		req.Chapter = "\x00empty"
	}
	where, args := scopeWhere(req.Subject, ch, req.Section, req.Tag)
	if req.Chapter == "\x00empty" {
		where += " AND questions.chapter=''"
	}
	// 最近被本班 PK 用过的题排后面，尽量不重复
	args = append([]interface{}{req.ClassID}, args...)
	rows, err := s.db.Query(`SELECT id, type, stem, options, answer, solution, images, chapter, section FROM questions
		LEFT JOIN (SELECT question_id, MAX(at) last FROM pk_answers WHERE class_id=? GROUP BY question_id) u ON u.question_id=questions.id
		WHERE `+where+` ORDER BY u.last IS NOT NULL, u.last, RANDOM() LIMIT ?`, append(args, req.Count)...)
	if err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []Question{}
	for rows.Next() {
		var q Question
		var opts, ans, imgs string
		rows.Scan(&q.ID, &q.Type, &q.Stem, &opts, &ans, &q.Solution, &imgs, &q.Chapter, &q.Section)
		json.Unmarshal([]byte(opts), &q.Options)
		json.Unmarshal([]byte(imgs), &q.Images)
		if q.Images == nil {
			q.Images = []string{}
		}
		ai := int(strings.ToUpper(strings.TrimSpace(ans))[0]) - 'A'
		if ai < 0 || ai >= len(q.Options) {
			continue
		}
		q.Answer = ai
		q.Letters = make([]string, len(q.Options))
		for i := range q.Options {
			q.Letters[i] = string(rune('A' + i))
			q.Options[i] = stripLetter(q.Options[i])
		}
		trimOptions(&q, req.Options, req.Shuffle)
		out = append(out, q)
	}
	// 结果再打乱一次，避免「旧题排后」导致总是同样的开局顺序
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	httpx.JSON(w, out)
}

// stripLetter 去掉选项开头的「A.」「A、」「(A)」等。
func stripLetter(o string) string {
	t := strings.TrimSpace(o)
	for _, p := range []string{"(", "（"} {
		if strings.HasPrefix(t, p) && len(t) > 3 {
			rest := strings.TrimPrefix(t, p)
			if rest[0] >= 'A' && rest[0] <= 'H' {
				r2 := rest[1:]
				if strings.HasPrefix(r2, ")") || strings.HasPrefix(r2, "）") {
					return strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(r2, ")"), "）"), " "))
				}
			}
		}
	}
	if len(t) >= 2 && t[0] >= 'A' && t[0] <= 'H' {
		r := t[1:]
		for _, sep := range []string{".", "．", "、", ":", "：", " "} {
			if strings.HasPrefix(r, sep) {
				return strings.TrimSpace(strings.TrimPrefix(r, sep))
			}
		}
	}
	return t
}

func trimOptions(q *Question, n int, shuffle bool) {
	idx := make([]int, len(q.Options))
	for i := range idx {
		idx[i] = i
	}
	if n >= 2 && n < len(q.Options) {
		others := []int{}
		for _, i := range idx {
			if i != q.Answer {
				others = append(others, i)
			}
		}
		rand.Shuffle(len(others), func(i, j int) { others[i], others[j] = others[j], others[i] })
		idx = append([]int{q.Answer}, others[:n-1]...)
		sort.Ints(idx)
	}
	if shuffle && q.Type != "判断" {
		rand.Shuffle(len(idx), func(i, j int) { idx[i], idx[j] = idx[j], idx[i] })
	}
	opts, letters := make([]string, len(idx)), make([]string, len(idx))
	ans := 0
	for k, i := range idx {
		opts[k], letters[k] = q.Options[i], q.Letters[i]
		if i == q.Answer {
			ans = k
		}
	}
	q.Options, q.Letters, q.Answer = opts, letters, ans
}

func (s *Service) classOK(u *auth.User, cid int64) bool {
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM classes WHERE id=? AND school_id=?", cid, u.SchoolID).Scan(&n)
	return n > 0
}

type player struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Score int    `json:"score"`
}

type answer struct {
	StudentID  int64  `json:"student_id"`
	QuestionID int64  `json:"question_id"`
	Choice     string `json:"choice"`
	Correct    bool   `json:"correct"`
	MS         int    `json:"ms"`
}

// POST /api/v1/pk/matches  保存一局
func (s *Service) handleSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClassID int64    `json:"class_id"`
		Subject string   `json:"subject"`
		Mode    string   `json:"mode"`
		Total   int      `json:"total"`
		Players []player `json:"players"`
		Answers []answer `json:"answers"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	u := auth.FromRequest(r)
	if !s.classOK(u, req.ClassID) || len(req.Players) == 0 || len(req.Players) > 2 {
		httpx.Error(w, 400, "班级或选手不对")
		return
	}
	// 选手必须是本班学生
	for _, p := range req.Players {
		var n int
		s.db.QueryRow("SELECT COUNT(*) FROM students WHERE id=? AND class_id=?", p.ID, req.ClassID).Scan(&n)
		if n == 0 {
			httpx.Error(w, 400, "选手不在本班名单里")
			return
		}
	}
	a := req.Players[0]
	b := player{}
	if len(req.Players) == 2 {
		b = req.Players[1]
	}
	var winner int64
	if b.ID != 0 && a.Score != b.Score {
		winner = a.ID
		if b.Score > a.Score {
			winner = b.ID
		}
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	tx, err := s.db.Begin()
	if err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO pk_matches(school_id,class_id,subject,mode,total,a_id,a_name,a_score,b_id,b_name,b_score,winner,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, u.SchoolID, req.ClassID, req.Subject, req.Mode, req.Total,
		a.ID, a.Name, a.Score, b.ID, b.Name, b.Score, winner, now)
	if err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	mid, _ := res.LastInsertId()
	used := map[int64]bool{}
	for _, an := range req.Answers {
		if an.StudentID != a.ID && an.StudentID != b.ID {
			continue
		}
		c := 0
		if an.Correct {
			c = 1
		}
		tx.Exec(`INSERT INTO pk_answers(match_id,class_id,student_id,question_id,choice,correct,ms,at) VALUES(?,?,?,?,?,?,?,?)`,
			mid, req.ClassID, an.StudentID, an.QuestionID, an.Choice, c, an.MS, now)
		used[an.QuestionID] = true
	}
	for qid := range used {
		tx.Exec("UPDATE questions SET use_count=use_count+1, last_used=? WHERE id=?", now, qid)
	}
	if err := tx.Commit(); err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	httpx.JSON(w, map[string]int64{"id": mid})
}

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.db.Exec("DELETE FROM pk_answers WHERE match_id IN (SELECT id FROM pk_matches WHERE id=? AND school_id=?)", id, auth.FromRequest(r).SchoolID)
	s.db.Exec("DELETE FROM pk_matches WHERE id=? AND school_id=?", id, auth.FromRequest(r).SchoolID)
	httpx.OK(w)
}

// GET /api/v1/pk/stats?class_id=&subject=&student_id=  排行榜、最近对局、错题
func (s *Service) handleStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cid, _ := strconv.ParseInt(q.Get("class_id"), 10, 64)
	sid, _ := strconv.ParseInt(q.Get("student_id"), 10, 64)
	subject := q.Get("subject")
	if !s.classOK(auth.FromRequest(r), cid) {
		httpx.Error(w, 404, "班级不存在")
		return
	}
	subjW, subjArg := "1=1", []interface{}{}
	if subject != "" {
		subjW, subjArg = "m.subject=?", []interface{}{subject}
	}

	type rank struct {
		ID       int64   `json:"id"`
		Name     string  `json:"name"`
		Matches  int     `json:"matches"`
		Wins     int     `json:"wins"`
		Answered int     `json:"answered"`
		Correct  int     `json:"correct"`
		Acc      float64 `json:"acc"`
		AvgMS    int     `json:"avg_ms"`
	}
	ranks := []rank{}
	rows, err := s.db.Query(`SELECT st.id, st.name,
		(SELECT COUNT(*) FROM pk_matches m WHERE m.class_id=st.class_id AND (m.a_id=st.id OR m.b_id=st.id) AND `+subjW+`),
		(SELECT COUNT(*) FROM pk_matches m WHERE m.class_id=st.class_id AND m.winner=st.id AND `+subjW+`),
		COUNT(a.id), COALESCE(SUM(a.correct),0), COALESCE(AVG(CASE WHEN a.correct=1 THEN a.ms END),0)
		FROM students st
		LEFT JOIN pk_answers a ON a.student_id=st.id AND a.match_id IN (SELECT m.id FROM pk_matches m WHERE m.class_id=st.class_id AND `+subjW+`)
		WHERE st.class_id=? GROUP BY st.id`, append(append(append(subjArg, subjArg...), subjArg...), cid)...)
	if err == nil {
		for rows.Next() {
			var k rank
			var avg float64
			rows.Scan(&k.ID, &k.Name, &k.Matches, &k.Wins, &k.Answered, &k.Correct, &avg)
			k.AvgMS = int(avg)
			if k.Answered > 0 {
				k.Acc = float64(k.Correct) / float64(k.Answered)
			}
			if k.Matches > 0 {
				ranks = append(ranks, k)
			}
		}
		rows.Close()
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		if ranks[i].Wins != ranks[j].Wins {
			return ranks[i].Wins > ranks[j].Wins
		}
		return ranks[i].Acc > ranks[j].Acc
	})

	type match struct {
		ID      int64  `json:"id"`
		Mode    string `json:"mode"`
		Subject string `json:"subject"`
		Total   int    `json:"total"`
		AName   string `json:"a_name"`
		AScore  int    `json:"a_score"`
		BName   string `json:"b_name"`
		BScore  int    `json:"b_score"`
		Winner  string `json:"winner"`
		At      string `json:"at"`
	}
	recent := []match{}
	mw, ma := "m.class_id=? AND "+subjW, append([]interface{}{cid}, subjArg...)
	if sid > 0 {
		mw += " AND (m.a_id=? OR m.b_id=?)"
		ma = append(ma, sid, sid)
	}
	if rows, err := s.db.Query(`SELECT m.id, m.mode, m.subject, m.total, m.a_name, m.a_score, m.b_name, m.b_score,
		CASE WHEN m.winner=0 THEN '' WHEN m.winner=m.a_id THEN m.a_name ELSE m.b_name END, m.created_at
		FROM pk_matches m WHERE `+mw+` ORDER BY m.id DESC LIMIT 30`, ma...); err == nil {
		for rows.Next() {
			var m match
			rows.Scan(&m.ID, &m.Mode, &m.Subject, &m.Total, &m.AName, &m.AScore, &m.BName, &m.BScore, &m.Winner, &m.At)
			recent = append(recent, m)
		}
		rows.Close()
	}

	type wrong struct {
		ID       int64    `json:"id"`
		Stem     string   `json:"stem"`
		Options  []string `json:"options"`
		Answer   string   `json:"answer"`
		Solution string   `json:"solution"`
		Chapter  string   `json:"chapter"`
		Wrong    int      `json:"wrong"`
		Total    int      `json:"total"`
		Names    string   `json:"names"`
	}
	wrongs := []wrong{}
	ww, wa := "a.class_id=?", []interface{}{cid}
	if subject != "" {
		ww += " AND m.subject=?"
		wa = append(wa, subject)
	}
	if sid > 0 {
		ww += " AND a.student_id=?"
		wa = append(wa, sid)
	}
	if rows, err := s.db.Query(`SELECT q.id, q.stem, q.options, q.answer, q.solution, q.chapter,
		SUM(1-a.correct) w, COUNT(*),
		COALESCE((SELECT group_concat(name,'、') FROM (SELECT DISTINCT st.name FROM pk_answers a2 JOIN students st ON st.id=a2.student_id
			WHERE a2.question_id=q.id AND a2.class_id=a.class_id AND a2.correct=0 LIMIT 8)),'')
		FROM pk_answers a JOIN pk_matches m ON m.id=a.match_id JOIN questions q ON q.id=a.question_id
		WHERE `+ww+` GROUP BY q.id HAVING w>0 ORDER BY w DESC, MAX(a.at) DESC LIMIT 50`, wa...); err == nil {
		for rows.Next() {
			var k wrong
			var opts string
			rows.Scan(&k.ID, &k.Stem, &opts, &k.Answer, &k.Solution, &k.Chapter, &k.Wrong, &k.Total, &k.Names)
			json.Unmarshal([]byte(opts), &k.Options)
			for i := range k.Options {
				k.Options[i] = stripLetter(k.Options[i])
			}
			wrongs = append(wrongs, k)
		}
		rows.Close()
	}
	httpx.JSON(w, map[string]interface{}{"ranks": ranks, "recent": recent, "wrong": wrongs})
}
