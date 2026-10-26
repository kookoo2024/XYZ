package tiku

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var tikuDB *sql.DB

// Question 题目完整结构
type Question struct {
	ID         int64    `json:"id"`
	Type       string   `json:"type"`
	Stem       string   `json:"stem"`
	Options    []string `json:"options"`
	Answer     string   `json:"answer"`
	Solution   string   `json:"solution"`
	Chapter    string   `json:"chapter"`
	Section    string   `json:"section"`
	Difficulty string   `json:"difficulty"`
	Tags       []string `json:"tags"`
	Images     []string `json:"images"`
	Source     string   `json:"source"`
	UseCount   int      `json:"use_count"`
	LastUsed   string   `json:"last_used"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// 题库选项列表项（用于筛选）
type OptionItem struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type TikuStats struct {
	Total     int          `json:"total"`
	ByChapter []OptionItem `json:"byChapter"`
	ByType    []OptionItem `json:"byType"`
	ByDiff    []OptionItem `json:"byDifficulty"`
}

// Service 题库模块：题目、组卷、教材目录树、导出。
type Service struct {
	db   *sql.DB
	root string // 数据根目录（解析题目图片）
}

// New 创建题库服务。数据库表由 store 包统一建好。
func New(db *sql.DB, root string) *Service { return &Service{db: db, root: root} }

// Register 注册路由（路径与原 FireCloud 完全一致，前端无需改动）。
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/tiku/list", s.handleTikuList)
	mux.HandleFunc("/api/tiku/get", s.handleTikuGet)
	mux.HandleFunc("/api/tiku/create", s.handleTikuCreate)
	mux.HandleFunc("/api/tiku/update", s.handleTikuUpdate)
	mux.HandleFunc("/api/tiku/delete", s.handleTikuDelete)
	mux.HandleFunc("/api/tiku/stats", s.handleTikuStats)
	mux.HandleFunc("/api/tiku/use", s.handleTikuUse)
	mux.HandleFunc("/api/tiku/export", s.handleTikuExport)
	mux.HandleFunc("/api/tiku/export/docx", s.handleTikuExportDocx)
	mux.HandleFunc("/api/tiku/papers", s.handleTikuPapersList)
	mux.HandleFunc("/api/tiku/papers/get", s.handleTikuPapersGet)
	mux.HandleFunc("/api/tiku/papers/save", s.handleTikuPapersSave)
	mux.HandleFunc("/api/tiku/papers/delete", s.handleTikuPapersDelete)
	mux.HandleFunc("/api/tiku/tree", s.handleTikuTreeGet)
	mux.HandleFunc("/api/tiku/tree/save", s.handleTikuTreeSave)
}

// ===== API 处理函数 =====

// 查询题目列表（分页+筛选）
func (s *Service) handleTikuList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 构建查询条件
	where := []string{"1=1"}
	args := []interface{}{}

	if v := r.URL.Query().Get("keyword"); v != "" {
		where = append(where, "(stem LIKE ? OR solution LIKE ? OR tags LIKE ?)")
		args = append(args, "%"+v+"%", "%"+v+"%", "%"+v+"%")
	}
	// 多选筛选（逗号分隔）
	if v := r.URL.Query().Get("type"); v != "" {
		vals := strings.Split(v, ",")
		ph := make([]string, len(vals))
		for i, val := range vals {
			ph[i] = "?"
			args = append(args, strings.TrimSpace(val))
		}
		where = append(where, fmt.Sprintf("type IN (%s)", strings.Join(ph, ",")))
	}
	var chapterCond, sectionCond string
	if v := r.URL.Query().Get("chapter"); v != "" {
		vals := strings.Split(v, ",")
		ph := make([]string, len(vals))
		for i, val := range vals {
			ph[i] = "?"
			args = append(args, strings.TrimSpace(val))
		}
		chapterCond = fmt.Sprintf("chapter IN (%s)", strings.Join(ph, ","))
	}
	if v := r.URL.Query().Get("section"); v != "" {
		vals := strings.Split(v, ",")
		ph := make([]string, len(vals))
		for i, val := range vals {
			ph[i] = "?"
			args = append(args, strings.TrimSpace(val))
		}
		sectionCond = fmt.Sprintf("section IN (%s)", strings.Join(ph, ","))
	}
	if chapterCond != "" && sectionCond != "" {
		// 同时勾选章与节时为并集：章内题目 OR 节内题目
		where = append(where, "("+chapterCond+" OR "+sectionCond+")")
	} else if chapterCond != "" {
		where = append(where, chapterCond)
	} else if sectionCond != "" {
		where = append(where, sectionCond)
	}
	if v := r.URL.Query().Get("difficulty"); v != "" {
		vals := strings.Split(v, ",")
		ph := make([]string, len(vals))
		for i, val := range vals {
			ph[i] = "?"
			args = append(args, strings.TrimSpace(val))
		}
		where = append(where, fmt.Sprintf("difficulty IN (%s)", strings.Join(ph, ",")))
	}
	if v := r.URL.Query().Get("source"); v != "" {
		where = append(where, "source LIKE ?")
		args = append(args, "%"+v+"%")
	}
	if v := r.URL.Query().Get("tag"); v != "" {
		where = append(where, "tags LIKE ?")
		args = append(args, "%\""+v+"\"%")
	}

	whereClause := strings.Join(where, " AND ")
	offset := (page - 1) * pageSize

	// 查总数
	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM questions WHERE %s", whereClause)
	// 安全检查：args 数量必须匹配
	s.db.QueryRow(countSQL, args...).Scan(&total)

	// 查列表（含全部字段，展开时零延迟）
	querySQL := fmt.Sprintf(`
		SELECT id, type, stem, options, answer, solution, chapter, section, difficulty, 
		       tags, images, source, use_count, last_used, created_at
		FROM questions WHERE %s 
		ORDER BY id DESC LIMIT ? OFFSET ?`, whereClause)

	args2 := append(args, pageSize, offset)
	rows, err := s.db.Query(querySQL, args2...)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	defer rows.Close()

	var questions []Question
	for rows.Next() {
		var q Question
		var optionsJSON, tagsJSON, imagesJSON string
		rows.Scan(&q.ID, &q.Type, &q.Stem, &optionsJSON, &q.Answer, &q.Solution,
			&q.Chapter, &q.Section, &q.Difficulty,
			&tagsJSON, &imagesJSON, &q.Source, &q.UseCount, &q.LastUsed, &q.CreatedAt)
		json.Unmarshal([]byte(optionsJSON), &q.Options)
		json.Unmarshal([]byte(tagsJSON), &q.Tags)
		json.Unmarshal([]byte(imagesJSON), &q.Images)
		questions = append(questions, q)
	}

	if questions == nil {
		questions = []Question{}
	}

	writeJSON(w, map[string]interface{}{
		"questions": questions,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"pages":     (total + pageSize - 1) / pageSize,
	})
}

// 查询单题详情（含solution）
func (s *Service) handleTikuGet(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": "invalid id"})
		return
	}

	var q Question
	var optionsJSON, tagsJSON, imagesJSON string
	err = s.db.QueryRow(`
		SELECT id, type, stem, options, answer, solution, chapter, section, 
		       difficulty, tags, images, source, use_count, last_used, created_at, updated_at
		FROM questions WHERE id = ?`, id).Scan(
		&q.ID, &q.Type, &q.Stem, &optionsJSON, &q.Answer, &q.Solution,
		&q.Chapter, &q.Section, &q.Difficulty,
		&tagsJSON, &imagesJSON, &q.Source, &q.UseCount, &q.LastUsed, &q.CreatedAt, &q.UpdatedAt)

	if err != nil {
		writeJSON(w, map[string]interface{}{"error": "not found"})
		return
	}

	json.Unmarshal([]byte(optionsJSON), &q.Options)
	json.Unmarshal([]byte(tagsJSON), &q.Tags)
	json.Unmarshal([]byte(imagesJSON), &q.Images)

	writeJSON(w, q)
}

// 新建题目
func (s *Service) handleTikuCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var q Question
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeJSON(w, map[string]interface{}{"error": "bad json"})
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	q.CreatedAt = now
	q.UpdatedAt = now

	optionsJSON, _ := json.Marshal(q.Options)
	tagsJSON, _ := json.Marshal(q.Tags)
	imagesJSON, _ := json.Marshal(q.Images)

	result, err := s.db.Exec(`
		INSERT INTO questions (type, stem, options, answer, solution, chapter, section, 
		       difficulty, tags, images, source, use_count, last_used, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', ?, ?)`,
		q.Type, q.Stem, string(optionsJSON), q.Answer, q.Solution,
		q.Chapter, q.Section, q.Difficulty,
		string(tagsJSON), string(imagesJSON), q.Source, q.CreatedAt, q.UpdatedAt)

	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}

	id, _ := result.LastInsertId()
	q.ID = id
	writeJSON(w, q)
}

// 更新题目
func (s *Service) handleTikuUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var q Question
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeJSON(w, map[string]interface{}{"error": "bad json"})
		return
	}

	if q.ID == 0 {
		writeJSON(w, map[string]interface{}{"error": "id required"})
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	q.UpdatedAt = now

	optionsJSON, _ := json.Marshal(q.Options)
	tagsJSON, _ := json.Marshal(q.Tags)
	imagesJSON, _ := json.Marshal(q.Images)

	_, err := s.db.Exec(`
		UPDATE questions SET 
			type=?, stem=?, options=?, answer=?, solution=?, 
			chapter=?, section=?, difficulty=?, tags=?, images=?, 
			source=?, updated_at=?
		WHERE id=?`,
		q.Type, q.Stem, string(optionsJSON), q.Answer, q.Solution,
		q.Chapter, q.Section, q.Difficulty,
		string(tagsJSON), string(imagesJSON), q.Source, now, q.ID)

	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]interface{}{"ok": true})
}

// 删除题目
func (s *Service) handleTikuDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID int64 `json:"id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.ID == 0 {
		writeJSON(w, map[string]interface{}{"error": "id required"})
		return
	}

	s.db.Exec("DELETE FROM questions WHERE id=?", req.ID)
	writeJSON(w, map[string]interface{}{"ok": true})
}

// 统计信息（用于筛选面板显示数量）
func (s *Service) handleTikuStats(w http.ResponseWriter, r *http.Request) {
	stats := TikuStats{
		ByChapter: []OptionItem{},
		ByType:    []OptionItem{},
		ByDiff:    []OptionItem{},
	}

	s.db.QueryRow("SELECT COUNT(*) FROM questions").Scan(&stats.Total)

	// 按章节
	rows, _ := s.db.Query("SELECT chapter, COUNT(*) as cnt FROM questions WHERE chapter != '' GROUP BY chapter ORDER BY cnt DESC")
	for rows.Next() {
		var item OptionItem
		rows.Scan(&item.Value, &item.Count)
		stats.ByChapter = append(stats.ByChapter, item)
	}
	rows.Close()

	// 按题型
	rows, _ = s.db.Query("SELECT type, COUNT(*) as cnt FROM questions WHERE type != '' GROUP BY type ORDER BY cnt DESC")
	for rows.Next() {
		var item OptionItem
		rows.Scan(&item.Value, &item.Count)
		stats.ByType = append(stats.ByType, item)
	}
	rows.Close()

	// 按难度
	rows, _ = s.db.Query("SELECT difficulty, COUNT(*) as cnt FROM questions WHERE difficulty != '' GROUP BY difficulty ORDER BY cnt DESC")
	for rows.Next() {
		var item OptionItem
		rows.Scan(&item.Value, &item.Count)
		stats.ByDiff = append(stats.ByDiff, item)
	}
	rows.Close()

	writeJSON(w, stats)
}

// 增加使用次数
func (s *Service) handleTikuUse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID int64 `json:"id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.ID == 0 {
		writeJSON(w, map[string]interface{}{"error": "id required"})
		return
	}

	now := time.Now().Format("2006-01-02")
	s.db.Exec("UPDATE questions SET use_count = use_count + 1, last_used = ? WHERE id = ?", now, req.ID)
	writeJSON(w, map[string]interface{}{"ok": true})
}

// 导出选题篮为 JSON
func (s *Service) handleTikuExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IDs []int64 `json:"ids"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if len(req.IDs) == 0 {
		writeJSON(w, map[string]interface{}{"error": "ids required"})
		return
	}

	// 按传入顺序查询
	questions := []Question{}
	for _, id := range req.IDs {
		var q Question
		var optionsJSON, tagsJSON, imagesJSON string
		err := s.db.QueryRow(`
			SELECT id, type, stem, options, answer, solution, chapter, section, 
			       difficulty, tags, images, source, use_count, last_used, created_at, updated_at
			FROM questions WHERE id = ?`, id).Scan(
			&q.ID, &q.Type, &q.Stem, &optionsJSON, &q.Answer, &q.Solution,
			&q.Chapter, &q.Section, &q.Difficulty,
			&tagsJSON, &imagesJSON, &q.Source, &q.UseCount, &q.LastUsed, &q.CreatedAt, &q.UpdatedAt)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(optionsJSON), &q.Options)
		json.Unmarshal([]byte(tagsJSON), &q.Tags)
		json.Unmarshal([]byte(imagesJSON), &q.Images)
		questions = append(questions, q)
	}

	writeJSON(w, map[string]interface{}{
		"questions": questions,
		"count":     len(questions),
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}
