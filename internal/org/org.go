// Package org 组织模型：学科、班级、学生、任课关系。
package org

import (
	"database/sql"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"xyz/internal/auth"
	"xyz/internal/httpx"
)

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) Register(mux *http.ServeMux) {
	staff := []string{auth.RoleAdmin, auth.RoleTeacher}
	admin := auth.RoleAdmin

	mux.HandleFunc("GET /api/v1/school", auth.Require(s.handleSchool, staff...))

	mux.HandleFunc("GET /api/v1/subjects", auth.Require(s.handleSubjects, staff...))
	mux.HandleFunc("POST /api/v1/subjects", auth.Require(s.handleSubjectCreate, admin))
	mux.HandleFunc("PUT /api/v1/subjects/{id}", auth.Require(s.handleSubjectUpdate, admin))
	mux.HandleFunc("DELETE /api/v1/subjects/{id}", auth.Require(s.handleSubjectDelete, admin))

	mux.HandleFunc("GET /api/v1/classes", auth.Require(s.handleClasses, staff...))
	mux.HandleFunc("POST /api/v1/classes", auth.Require(s.handleClassCreate, admin))
	mux.HandleFunc("PUT /api/v1/classes/{id}", auth.Require(s.handleClassUpdate, admin))
	mux.HandleFunc("DELETE /api/v1/classes/{id}", auth.Require(s.handleClassDelete, admin))

	mux.HandleFunc("GET /api/v1/classes/{id}/students", auth.Require(s.handleStudents, staff...))
	mux.HandleFunc("POST /api/v1/classes/{id}/students", auth.Require(s.handleStudentCreate, staff...))
	mux.HandleFunc("POST /api/v1/classes/{id}/students/import", auth.Require(s.handleStudentImport, staff...))
	mux.HandleFunc("PUT /api/v1/students/{id}", auth.Require(s.handleStudentUpdate, staff...))
	mux.HandleFunc("DELETE /api/v1/students/{id}", auth.Require(s.handleStudentDelete, staff...))

	mux.HandleFunc("GET /api/v1/teaching", auth.Require(s.handleTeaching, staff...))
	mux.HandleFunc("PUT /api/v1/teaching", auth.Require(s.handleTeachingSet, admin))

	mux.HandleFunc("GET /api/v1/me/classes", auth.Require(s.handleMyClasses, staff...))
}

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func now() string { return time.Now().Format("2006-01-02 15:04:05") }

func (s *Service) handleSchool(w http.ResponseWriter, r *http.Request) {
	var name string
	s.db.QueryRow("SELECT name FROM schools WHERE id=?", auth.FromRequest(r).SchoolID).Scan(&name)
	httpx.JSON(w, map[string]interface{}{"id": auth.FromRequest(r).SchoolID, "name": name})
}

// ===== 学科 =====

type Subject struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

func (s *Service) handleSubjects(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT id,name,sort FROM subjects WHERE school_id=? ORDER BY sort,id", auth.FromRequest(r).SchoolID)
	if err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []Subject{}
	for rows.Next() {
		var x Subject
		rows.Scan(&x.ID, &x.Name, &x.Sort)
		list = append(list, x)
	}
	httpx.JSON(w, list)
}

func (s *Service) handleSubjectCreate(w http.ResponseWriter, r *http.Request) {
	var req Subject
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.Name = strings.TrimSpace(req.Name); req.Name == "" {
		httpx.Error(w, 400, "学科名不能为空")
		return
	}
	sid := auth.FromRequest(r).SchoolID
	if req.Sort == 0 {
		s.db.QueryRow("SELECT COALESCE(MAX(sort),0)+1 FROM subjects WHERE school_id=?", sid).Scan(&req.Sort)
	}
	res, err := s.db.Exec("INSERT INTO subjects(school_id,name,sort) VALUES(?,?,?)", sid, req.Name, req.Sort)
	if err != nil {
		httpx.Error(w, 400, "学科已存在")
		return
	}
	id, _ := res.LastInsertId()
	httpx.JSON(w, map[string]int64{"id": id})
}

func (s *Service) handleSubjectUpdate(w http.ResponseWriter, r *http.Request) {
	var req Subject
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpx.Error(w, 400, "学科名不能为空")
		return
	}
	if _, err := s.db.Exec("UPDATE subjects SET name=?, sort=? WHERE id=? AND school_id=?",
		req.Name, req.Sort, pathID(r), auth.FromRequest(r).SchoolID); err != nil {
		httpx.Error(w, 400, "学科名重复")
		return
	}
	httpx.OK(w)
}

func (s *Service) handleSubjectDelete(w http.ResponseWriter, r *http.Request) {
	s.db.Exec("DELETE FROM subjects WHERE id=? AND school_id=?", pathID(r), auth.FromRequest(r).SchoolID)
	httpx.OK(w)
}

// ===== 班级 =====

type Class struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Grade           string `json:"grade"`
	EnrollYear      int    `json:"enroll_year"`
	HeadTeacherID   int64  `json:"head_teacher_id"`
	HeadTeacherName string `json:"head_teacher_name"`
	Archived        bool   `json:"archived"`
	StudentCount    int    `json:"student_count"`
}

const classSelect = `SELECT c.id,c.name,c.grade,c.enroll_year,COALESCE(c.head_teacher_id,0),COALESCE(u.name,''),c.archived,
	(SELECT COUNT(*) FROM students st WHERE st.class_id=c.id)
	FROM classes c LEFT JOIN users u ON u.id=c.head_teacher_id`

func scanClasses(rows *sql.Rows) []Class {
	defer rows.Close()
	list := []Class{}
	for rows.Next() {
		var c Class
		var arch int
		rows.Scan(&c.ID, &c.Name, &c.Grade, &c.EnrollYear, &c.HeadTeacherID, &c.HeadTeacherName, &arch, &c.StudentCount)
		c.Archived = arch != 0
		list = append(list, c)
	}
	return list
}

// GET /api/v1/classes?archived=1 包含已归档（毕业）班级
func (s *Service) handleClasses(w http.ResponseWriter, r *http.Request) {
	q := classSelect + " WHERE c.school_id=?"
	if r.URL.Query().Get("archived") != "1" {
		q += " AND c.archived=0"
	}
	rows, err := s.db.Query(q+" ORDER BY c.archived, c.enroll_year DESC, c.name", auth.FromRequest(r).SchoolID)
	if err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	httpx.JSON(w, scanClasses(rows))
}

func nullID(id int64) interface{} {
	if id <= 0 {
		return nil
	}
	return id
}

func (s *Service) handleClassCreate(w http.ResponseWriter, r *http.Request) {
	var c Class
	if !httpx.Decode(w, r, &c) {
		return
	}
	if c.Name = strings.TrimSpace(c.Name); c.Name == "" {
		httpx.Error(w, 400, "班级名称不能为空")
		return
	}
	res, err := s.db.Exec("INSERT INTO classes(school_id,name,grade,enroll_year,head_teacher_id,created_at) VALUES(?,?,?,?,?,?)",
		auth.FromRequest(r).SchoolID, c.Name, strings.TrimSpace(c.Grade), c.EnrollYear, nullID(c.HeadTeacherID), now())
	if err != nil {
		httpx.Error(w, 400, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	httpx.JSON(w, map[string]int64{"id": id})
}

func (s *Service) handleClassUpdate(w http.ResponseWriter, r *http.Request) {
	var c Class
	if !httpx.Decode(w, r, &c) {
		return
	}
	if c.Name = strings.TrimSpace(c.Name); c.Name == "" {
		httpx.Error(w, 400, "班级名称不能为空")
		return
	}
	_, err := s.db.Exec("UPDATE classes SET name=?,grade=?,enroll_year=?,head_teacher_id=?,archived=? WHERE id=? AND school_id=?",
		c.Name, strings.TrimSpace(c.Grade), c.EnrollYear, nullID(c.HeadTeacherID), boolInt(c.Archived), pathID(r), auth.FromRequest(r).SchoolID)
	if err != nil {
		httpx.Error(w, 400, err.Error())
		return
	}
	httpx.OK(w)
}

// DELETE 只允许删除空班级；有学生的班级请「归档」。
func (s *Service) handleClassDelete(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM students WHERE class_id=?", id).Scan(&n)
	if n > 0 {
		httpx.Error(w, 400, "班级里还有学生，不能删除；毕业班请用「归档」")
		return
	}
	s.db.Exec("DELETE FROM classes WHERE id=? AND school_id=?", id, auth.FromRequest(r).SchoolID)
	httpx.OK(w)
}

// canManageClass 管理员或该班班主任可以改学生名单。
func (s *Service) canManageClass(u *auth.User, classID int64) bool {
	var school, head int64
	if s.db.QueryRow("SELECT school_id, COALESCE(head_teacher_id,0) FROM classes WHERE id=?", classID).Scan(&school, &head) != nil {
		return false
	}
	if school != u.SchoolID {
		return false
	}
	return u.IsAdmin() || (head != 0 && head == u.ID)
}

// ===== 学生 =====

type Student struct {
	ID        int64  `json:"id"`
	ClassID   int64  `json:"class_id"`
	Name      string `json:"name"`
	StudentNo string `json:"student_no"`
	Gender    string `json:"gender"`
	Note      string `json:"note"`
}

func (s *Service) handleStudents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT st.id,st.class_id,st.name,st.student_no,st.gender,st.note FROM students st
		JOIN classes c ON c.id=st.class_id WHERE st.class_id=? AND c.school_id=?
		ORDER BY CASE WHEN st.student_no='' THEN 1 ELSE 0 END, LENGTH(st.student_no), st.student_no, st.id`,
		pathID(r), auth.FromRequest(r).SchoolID)
	if err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []Student{}
	for rows.Next() {
		var x Student
		rows.Scan(&x.ID, &x.ClassID, &x.Name, &x.StudentNo, &x.Gender, &x.Note)
		list = append(list, x)
	}
	httpx.JSON(w, list)
}

func normGender(g string) string {
	switch strings.TrimSpace(g) {
	case "男", "M", "m", "male":
		return "男"
	case "女", "F", "f", "female":
		return "女"
	}
	return ""
}

func (s *Service) handleStudentCreate(w http.ResponseWriter, r *http.Request) {
	u := auth.FromRequest(r)
	cid := pathID(r)
	if !s.canManageClass(u, cid) {
		httpx.Error(w, 403, "只有管理员或本班班主任可以修改学生名单")
		return
	}
	var x Student
	if !httpx.Decode(w, r, &x) {
		return
	}
	if x.Name = strings.TrimSpace(x.Name); x.Name == "" {
		httpx.Error(w, 400, "姓名不能为空")
		return
	}
	res, err := s.db.Exec("INSERT INTO students(school_id,class_id,name,student_no,gender,note,created_at) VALUES(?,?,?,?,?,?,?)",
		u.SchoolID, cid, x.Name, strings.TrimSpace(x.StudentNo), normGender(x.Gender), x.Note, now())
	if err != nil {
		httpx.Error(w, 400, "学号重复")
		return
	}
	id, _ := res.LastInsertId()
	httpx.JSON(w, map[string]int64{"id": id})
}

var (
	reSplit = regexp.MustCompile(`[\t,，;；|]+|\s{2,}| `)
	reNo    = regexp.MustCompile(`^[0-9A-Za-z\-]{1,20}$`)
)

// ParseRoster 解析从 Excel/Word 粘贴的名单。每行一个学生，可含学号、姓名、性别，顺序不限。
func ParseRoster(text string) []Student {
	var out []Student
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var st Student
		header := false
		for _, f := range reSplit.Split(line, -1) {
			f = strings.TrimSpace(f)
			switch {
			case f == "姓名" || f == "学号" || f == "学生姓名" || f == "性别" || f == "序号":
				header = true
			case f == "":
			case normGender(f) != "" && st.Gender == "":
				st.Gender = normGender(f)
			case reNo.MatchString(f) && st.StudentNo == "":
				st.StudentNo = f
			case st.Name == "":
				st.Name = f
			}
		}
		// 跳过表头行
		if header || st.Name == "" {
			continue
		}
		out = append(out, st)
	}
	return out
}

// POST /api/v1/classes/{id}/students/import {"text": "...", "replace": false}
// 学号相同则更新姓名/性别；没有学号的按姓名去重。
func (s *Service) handleStudentImport(w http.ResponseWriter, r *http.Request) {
	u := auth.FromRequest(r)
	cid := pathID(r)
	if !s.canManageClass(u, cid) {
		httpx.Error(w, 403, "只有管理员或本班班主任可以修改学生名单")
		return
	}
	var req struct {
		Text    string `json:"text"`
		Replace bool   `json:"replace"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	list := ParseRoster(req.Text)
	if len(list) == 0 {
		httpx.Error(w, 400, "没有识别到学生，请每行粘贴一个学生（学号 姓名 性别）")
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	if req.Replace {
		tx.Exec("DELETE FROM students WHERE class_id=?", cid)
	}
	added, updated := 0, 0
	for _, st := range list {
		var id int64
		if st.StudentNo != "" {
			tx.QueryRow("SELECT id FROM students WHERE class_id=? AND student_no=?", cid, st.StudentNo).Scan(&id)
		} else {
			tx.QueryRow("SELECT id FROM students WHERE class_id=? AND name=?", cid, st.Name).Scan(&id)
		}
		if id > 0 {
			tx.Exec("UPDATE students SET name=?, gender=CASE WHEN ?='' THEN gender ELSE ? END WHERE id=?", st.Name, st.Gender, st.Gender, id)
			updated++
			continue
		}
		if _, err := tx.Exec("INSERT INTO students(school_id,class_id,name,student_no,gender,created_at) VALUES(?,?,?,?,?,?)",
			u.SchoolID, cid, st.Name, st.StudentNo, st.Gender, now()); err != nil {
			httpx.Error(w, 400, "导入失败："+st.Name+" "+err.Error())
			return
		}
		added++
	}
	if err := tx.Commit(); err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	httpx.JSON(w, map[string]int{"added": added, "updated": updated})
}

func (s *Service) studentClass(id int64) int64 {
	var cid int64
	s.db.QueryRow("SELECT class_id FROM students WHERE id=?", id).Scan(&cid)
	return cid
}

// PUT /api/v1/students/{id}：可改姓名、学号、性别、备注、转班（class_id）。
func (s *Service) handleStudentUpdate(w http.ResponseWriter, r *http.Request) {
	u := auth.FromRequest(r)
	id := pathID(r)
	cid := s.studentClass(id)
	if cid == 0 || !s.canManageClass(u, cid) {
		httpx.Error(w, 403, "只有管理员或本班班主任可以修改学生名单")
		return
	}
	var x Student
	if !httpx.Decode(w, r, &x) {
		return
	}
	if x.Name = strings.TrimSpace(x.Name); x.Name == "" {
		httpx.Error(w, 400, "姓名不能为空")
		return
	}
	if x.ClassID == 0 {
		x.ClassID = cid
	}
	if x.ClassID != cid && !s.canManageClass(u, x.ClassID) {
		httpx.Error(w, 403, "没有目标班级的权限")
		return
	}
	if _, err := s.db.Exec("UPDATE students SET name=?,student_no=?,gender=?,note=?,class_id=? WHERE id=?",
		x.Name, strings.TrimSpace(x.StudentNo), normGender(x.Gender), x.Note, x.ClassID, id); err != nil {
		httpx.Error(w, 400, "学号重复")
		return
	}
	httpx.OK(w)
}

func (s *Service) handleStudentDelete(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	cid := s.studentClass(id)
	if cid == 0 || !s.canManageClass(auth.FromRequest(r), cid) {
		httpx.Error(w, 403, "只有管理员或本班班主任可以修改学生名单")
		return
	}
	s.db.Exec("DELETE FROM students WHERE id=?", id)
	httpx.OK(w)
}

// ===== 任课 =====

type Teaching struct {
	ClassID     int64  `json:"class_id"`
	SubjectID   int64  `json:"subject_id"`
	TeacherID   int64  `json:"teacher_id"`
	TeacherName string `json:"teacher_name,omitempty"`
}

// GET /api/v1/teaching?class_id=  全校或某班的任课表
func (s *Service) handleTeaching(w http.ResponseWriter, r *http.Request) {
	q := `SELECT t.class_id,t.subject_id,t.teacher_id,u.name FROM teaching t
		JOIN classes c ON c.id=t.class_id JOIN users u ON u.id=t.teacher_id WHERE c.school_id=?`
	args := []interface{}{auth.FromRequest(r).SchoolID}
	if cid := r.URL.Query().Get("class_id"); cid != "" {
		q += " AND t.class_id=?"
		args = append(args, cid)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []Teaching{}
	for rows.Next() {
		var t Teaching
		rows.Scan(&t.ClassID, &t.SubjectID, &t.TeacherID, &t.TeacherName)
		list = append(list, t)
	}
	httpx.JSON(w, list)
}

// PUT /api/v1/teaching {class_id, subject_id, teacher_id}；teacher_id=0 表示取消。
func (s *Service) handleTeachingSet(w http.ResponseWriter, r *http.Request) {
	var t Teaching
	if !httpx.Decode(w, r, &t) {
		return
	}
	if t.ClassID == 0 || t.SubjectID == 0 {
		httpx.Error(w, 400, "缺少班级或学科")
		return
	}
	if t.TeacherID == 0 {
		s.db.Exec("DELETE FROM teaching WHERE class_id=? AND subject_id=?", t.ClassID, t.SubjectID)
	} else if _, err := s.db.Exec(`INSERT INTO teaching(class_id,subject_id,teacher_id) VALUES(?,?,?)
		ON CONFLICT(class_id,subject_id) DO UPDATE SET teacher_id=excluded.teacher_id`, t.ClassID, t.SubjectID, t.TeacherID); err != nil {
		httpx.Error(w, 400, err.Error())
		return
	}
	httpx.OK(w)
}

// GET /api/v1/me/classes：我当班主任或任课的班级，以及我教的学科。
func (s *Service) handleMyClasses(w http.ResponseWriter, r *http.Request) {
	u := auth.FromRequest(r)
	type item struct {
		Class
		IsHead   bool     `json:"is_head"`
		Subjects []string `json:"subjects"`
	}
	rows, err := s.db.Query(classSelect+` WHERE c.school_id=? AND c.archived=0 AND (c.head_teacher_id=? OR c.id IN (SELECT class_id FROM teaching WHERE teacher_id=?))
		ORDER BY c.enroll_year DESC, c.name`, u.SchoolID, u.ID, u.ID)
	if err != nil {
		httpx.Error(w, 500, err.Error())
		return
	}
	out := []item{}
	for _, c := range scanClasses(rows) {
		it := item{Class: c, IsHead: c.HeadTeacherID == u.ID, Subjects: []string{}}
		sr, _ := s.db.Query("SELECT sb.name FROM teaching t JOIN subjects sb ON sb.id=t.subject_id WHERE t.class_id=? AND t.teacher_id=? ORDER BY sb.sort", c.ID, u.ID)
		for sr != nil && sr.Next() {
			var n string
			sr.Scan(&n)
			it.Subjects = append(it.Subjects, n)
		}
		if sr != nil {
			sr.Close()
		}
		out = append(out, it)
	}
	httpx.JSON(w, out)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
