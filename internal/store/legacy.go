package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImportLegacy 把原 FireCloud 散落在数据目录里的隐藏文件导入数据库。
// 每类数据只导入一次（记录在 legacy_imports）；原文件保持不动，随时可回到旧版。
func ImportLegacy(db *sql.DB, dataDir string) {
	steps := []struct {
		name string
		fn   func(*sql.DB, string) (string, error)
	}{
		{"fire_tiku_db", importTikuDB},
		{"fire_papers_json", importPapers},
		{"fire_tags_json", importTags},
		{"fire_markers_json", importMarkers},
		{"fire_lessons", importLessons},
		{"fire_curriculum_json", importCurriculum},
	}
	for _, st := range steps {
		var done int
		db.QueryRow("SELECT COUNT(*) FROM legacy_imports WHERE name=?", st.name).Scan(&done)
		if done > 0 {
			continue
		}
		detail, err := st.fn(db, dataDir)
		if err != nil {
			log.Printf("导入旧数据 %s 失败（下次启动会重试）: %v", st.name, err)
			continue
		}
		if detail == "" {
			continue // 没有旧文件，不记录，以后放进来还能导入
		}
		db.Exec("INSERT INTO legacy_imports(name, imported_at, detail) VALUES(?,?,?)",
			st.name, time.Now().Format("2006-01-02 15:04:05"), detail)
		log.Printf("已导入旧数据 %s: %s", st.name, detail)
	}
}

func readLegacy(dataDir, name string) ([]byte, bool) {
	data, err := os.ReadFile(filepath.Join(dataDir, name))
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return nil, false
	}
	return data, true
}

func importTikuDB(db *sql.DB, dataDir string) (string, error) {
	path := filepath.Join(dataDir, ".fire_tiku.db")
	if _, err := os.Stat(path); err != nil {
		return "", nil
	}
	var n int
	db.QueryRow("SELECT COUNT(*) FROM questions").Scan(&n)
	if n > 0 {
		return "skipped: questions not empty", nil
	}
	conn, err := db.Conn(bgCtx())
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(bgCtx(), "ATTACH DATABASE "+sqlQuote(filepath.ToSlash(path))+" AS old"); err != nil {
		return "", err
	}
	defer conn.ExecContext(bgCtx(), "DETACH DATABASE old")
	cols := "id,type,stem,options,answer,solution,chapter,section,difficulty,tags,images,source,use_count,last_used,created_at,updated_at"
	res, err := conn.ExecContext(bgCtx(), "INSERT INTO main.questions("+cols+") SELECT "+cols+" FROM old.questions")
	if err != nil {
		return "", err
	}
	c, _ := res.RowsAffected()
	return fmt.Sprintf("%d questions", c), nil
}

func importPapers(db *sql.DB, dataDir string) (string, error) {
	data, ok := readLegacy(dataDir, ".fire_papers.json")
	if !ok {
		return "", nil
	}
	var m map[string]struct {
		ID      string          `json:"id"`
		Name    string          `json:"name"`
		Kind    string          `json:"kind"`
		Items   json.RawMessage `json:"items"`
		Updated int64           `json:"updated"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	for id, p := range m {
		if p.ID == "" {
			p.ID = id
		}
		items := string(p.Items)
		if items == "" || items == "null" {
			items = "[]"
		}
		if _, err := tx.Exec("INSERT OR IGNORE INTO papers(id,name,kind,items,updated) VALUES(?,?,?,?,?)",
			p.ID, p.Name, p.Kind, items, p.Updated); err != nil {
			tx.Rollback()
			return "", err
		}
	}
	return fmt.Sprintf("%d papers", len(m)), tx.Commit()
}

func importPathJSON(db *sql.DB, dataDir, file, table, col string) (string, error) {
	data, ok := readLegacy(dataDir, file)
	if !ok {
		return "", nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	for path, v := range m {
		if _, err := tx.Exec("INSERT OR IGNORE INTO "+table+"(path,"+col+") VALUES(?,?)", path, string(v)); err != nil {
			tx.Rollback()
			return "", err
		}
	}
	return fmt.Sprintf("%d entries", len(m)), tx.Commit()
}

func importTags(db *sql.DB, dataDir string) (string, error) {
	return importPathJSON(db, dataDir, ".fire_tags.json", "file_tags", "tags")
}

func importMarkers(db *sql.DB, dataDir string) (string, error) {
	return importPathJSON(db, dataDir, ".fire_markers.json", "markers", "markers")
}

func importLessons(db *sql.DB, dataDir string) (string, error) {
	dir := filepath.Join(dataDir, ".fire_lessons")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var meta struct {
			Updated int64 `json:"updated"`
		}
		json.Unmarshal(data, &meta)
		name := strings.TrimSuffix(e.Name(), ".json")
		if _, err := db.Exec("INSERT OR IGNORE INTO lessons(name,data,updated) VALUES(?,?,?)", name, string(data), meta.Updated); err != nil {
			return "", err
		}
		n++
	}
	if n == 0 {
		return "", nil
	}
	return fmt.Sprintf("%d lessons", n), nil
}

func importCurriculum(db *sql.DB, dataDir string) (string, error) {
	data, ok := readLegacy(dataDir, ".fire_curriculum.json")
	if !ok {
		return "", nil
	}
	if !json.Valid(data) {
		return "", fmt.Errorf("invalid json")
	}
	if _, exists := GetKV(db, KeyCurriculum); exists {
		return "skipped: already set", nil
	}
	return "curriculum tree", SetKV(db, KeyCurriculum, string(data))
}

// KeyCurriculum 教材目录树在 kv 表中的键。
const KeyCurriculum = "tiku.curriculum_tree"
