// Package lesson 备课系统：备课方案的保存、列表、读取（数据库存储）。
package lesson

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"xyz/internal/httpx"
)

// Slide / LessonPlan 与原 FireCloud 前端格式一致。
type Slide struct {
	Name     string                 `json:"name"`
	Template string                 `json:"template"`
	Slots    map[string]interface{} `json:"slots"`
}

type LessonPlan struct {
	Name    string  `json:"name"`
	Slides  []Slide `json:"slides"`
	Updated int64   `json:"updated"`
}

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/lesson/save", s.handleSave)
	mux.HandleFunc("/api/lesson/list", s.handleList)
	mux.HandleFunc("/api/lesson/get", s.handleGet)
}

func (s *Service) handleSave(w http.ResponseWriter, r *http.Request) {
	var plan LessonPlan
	if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
		http.Error(w, "Bad JSON", 400)
		return
	}
	plan.Name = strings.TrimSpace(plan.Name)
	if plan.Name == "" {
		http.Error(w, "Name required", 400)
		return
	}
	plan.Updated = time.Now().Unix()
	data, _ := json.Marshal(plan)
	if _, err := s.db.Exec(`INSERT INTO lessons(name,data,updated) VALUES(?,?,?)
		ON CONFLICT(name) DO UPDATE SET data=excluded.data, updated=excluded.updated`,
		plan.Name, string(data), plan.Updated); err != nil {
		http.Error(w, "保存失败", 500)
		return
	}
	w.Write([]byte("OK"))
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	list := []string{}
	rows, err := s.db.Query("SELECT name FROM lessons ORDER BY name")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var n string
			if rows.Scan(&n) == nil {
				list = append(list, n)
			}
		}
	}
	httpx.JSON(w, list)
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Name required", 400)
		return
	}
	var data string
	if err := s.db.QueryRow("SELECT data FROM lessons WHERE name=?", name).Scan(&data); err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(data))
}
