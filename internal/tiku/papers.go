package tiku

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PaperItem 组卷篮中的一道题
type PaperItem struct {
	QID   int64   `json:"qid"`
	Score float64 `json:"score"`
}

// Paper 一份组卷/专题/练习工作台
type Paper struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Kind    string      `json:"kind"` // 试卷 / 专题 / 练习 / 教案
	Items   []PaperItem `json:"items"`
	Updated int64       `json:"updated"`
}

// PaperSummary 列表用摘要
type PaperSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Count   int    `json:"count"`
	Updated int64  `json:"updated"`
}

// 生成简单唯一 ID
func newPaperID() string {
	return fmt.Sprintf("p%d", time.Now().UnixNano())
}

// 每份组卷是数据库里的一行；保存/删除只动这一行，不再整文件重写。
func (s *Service) listPapers() ([]PaperSummary, error) {
	rows, err := s.db.Query("SELECT id, name, kind, items, updated FROM papers ORDER BY updated DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []PaperSummary{}
	for rows.Next() {
		var p PaperSummary
		var items string
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &items, &p.Updated); err != nil {
			return nil, err
		}
		var arr []json.RawMessage
		json.Unmarshal([]byte(items), &arr)
		p.Count = len(arr)
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Service) getPaper(id string) (Paper, bool, error) {
	var p Paper
	var items string
	err := s.db.QueryRow("SELECT id, name, kind, items, updated FROM papers WHERE id=?", id).
		Scan(&p.ID, &p.Name, &p.Kind, &items, &p.Updated)
	if err == sql.ErrNoRows {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	json.Unmarshal([]byte(items), &p.Items)
	return p, true, nil
}

func (s *Service) savePaper(p Paper) error {
	items, err := json.Marshal(p.Items)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO papers(id,name,kind,items,updated) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, kind=excluded.kind, items=excluded.items, updated=excluded.updated`,
		p.ID, p.Name, p.Kind, string(items), p.Updated)
	return err
}

func (s *Service) deletePaper(id string) error {
	_, err := s.db.Exec("DELETE FROM papers WHERE id=?", id)
	return err
}

// ===== API =====

// GET /api/tiku/papers
func (s *Service) handleTikuPapersList(w http.ResponseWriter, r *http.Request) {
	list, err := s.listPapers()
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"papers": list})
}

// GET /api/tiku/papers/get?id=xxx
func (s *Service) handleTikuPapersGet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, map[string]interface{}{"error": "id required"})
		return
	}
	p, ok, err := s.getPaper(id)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, map[string]interface{}{"error": "not found"})
		return
	}
	if p.Items == nil {
		p.Items = []PaperItem{}
	}
	writeJSON(w, p)
}

// POST /api/tiku/papers/save
func (s *Service) handleTikuPapersSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var p Paper
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, map[string]interface{}{"error": "bad json"})
		return
	}
	if p.Name == "" {
		writeJSON(w, map[string]interface{}{"error": "name required"})
		return
	}
	if p.ID == "" {
		p.ID = newPaperID()
	}
	p.Updated = time.Now().Unix()
	if p.Items == nil {
		p.Items = []PaperItem{}
	}

	if err := s.savePaper(p); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, p)
}

// POST /api/tiku/papers/delete
func (s *Service) handleTikuPapersDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, map[string]interface{}{"error": "id required"})
		return
	}
	if err := s.deletePaper(req.ID); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}
