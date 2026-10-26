package server

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xyz/internal/backup"
	"xyz/internal/config"
	"xyz/internal/store"
)

// 模拟一台用过 FireCloud 的电脑：旧题库数据库 + 各种隐藏 JSON。
func makeLegacy(t *testing.T, dir string) {
	old, err := sql.Open("sqlite", filepath.Join(dir, ".fire_tiku.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = old.Exec(`CREATE TABLE questions (id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL DEFAULT '', stem TEXT NOT NULL DEFAULT '', options TEXT NOT NULL DEFAULT '[]', answer TEXT NOT NULL DEFAULT '', solution TEXT NOT NULL DEFAULT '', chapter TEXT NOT NULL DEFAULT '', section TEXT NOT NULL DEFAULT '', difficulty TEXT NOT NULL DEFAULT '', tags TEXT NOT NULL DEFAULT '[]', images TEXT NOT NULL DEFAULT '[]', source TEXT NOT NULL DEFAULT '', use_count INTEGER NOT NULL DEFAULT 0, last_used TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '');
	INSERT INTO questions(id,type,stem,chapter,difficulty) VALUES (7,'单选题','已知集合 $A=\{1,2\}$','第一章 集合与常用逻辑用语','基础');`)
	if err != nil {
		t.Fatal(err)
	}
	old.Close()
	w := func(name, s string) { os.WriteFile(filepath.Join(dir, name), []byte(s), 0644) }
	w(".fire_papers.json", `{"p1":{"id":"p1","name":"月考","kind":"试卷","items":[{"qid":7,"score":5}],"updated":100}}`)
	w(".fire_tags.json", `{"视频/a.mp4":["重点"]}`)
	w(".fire_markers.json", `{"视频/a.mp4":[{"time":12.5,"label":"例题"}]}`)
	w(".fire_curriculum.json", `[{"id":"x","name":"自定义教材","type":"book"}]`)
	os.MkdirAll(filepath.Join(dir, ".fire_lessons"), 0755)
	w(".fire_lessons/第一课.json", `{"name":"第一课","slides":[],"updated":5}`)
	os.MkdirAll(filepath.Join(dir, "视频"), 0755)
	w("视频/a.mp4", "fake")
	os.MkdirAll(filepath.Join(dir, "课件"), 0755)
	w("课件/index.html", "<h1>H5课件</h1>")
}

func setup(t *testing.T) (*httptest.Server, *config.Config, *sql.DB) {
	dir := t.TempDir()
	makeLegacy(t, dir)
	cfg, err := config.Load(filepath.Join(dir, "xyz.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = dir
	db, err := store.Open(cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	store.ImportLegacy(db, dir)
	store.ImportLegacy(db, dir) // 第二次不能重复导入
	store.EnsureDefaults(db, "")
	ts := httptest.NewServer(New(cfg, db).Handler)
	t.Cleanup(func() { ts.Close(); db.Close() })
	return ts, cfg, db
}

func get(t *testing.T, ts *httptest.Server, path string) (int, string) {
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func post(t *testing.T, ts *httptest.Server, path, body string) (int, string) {
	resp, err := http.Post(ts.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestLegacyDataVisibleThroughOldAPIs(t *testing.T) {
	ts, _, _ := setup(t)
	checks := map[string]string{
		"/api/tiku/list":                 "已知集合",
		"/api/tiku/get?id=7":             "单选题",
		"/api/tiku/papers":               `"count":1`,
		"/api/tiku/papers/get?id=p1":     `"qid":7`,
		"/api/tiku/tree":                 "自定义教材",
		"/api/tags/getAll":               "已标注",
		"/api/markers/get?path=视频/a.mp4": "例题",
		"/api/lesson/list":               "第一课",
		"/api/lesson/get?name=第一课":       `"slides"`,
		"/api/tree":                      "a.mp4",
		"/课件/":                           "H5课件",
		"/tiku":                          "<html",
	}
	for p, want := range checks {
		code, body := get(t, ts, p)
		if code != 200 || !strings.Contains(body, want) {
			t.Errorf("%s -> %d %q, want contains %q", p, code, trunc(body), want)
		}
	}
	// 题目数量不能因为导入两次而翻倍
	_, body := get(t, ts, "/api/tiku/stats")
	if !strings.Contains(body, `"total":1`) {
		t.Errorf("stats: %s", body)
	}
}

func TestWritesGoToDatabase(t *testing.T) {
	ts, _, _ := setup(t)
	_, body := post(t, ts, "/api/tiku/papers/save", `{"name":"周测","kind":"练习","items":[]}`)
	var p struct{ ID string }
	json.Unmarshal([]byte(body), &p)
	if p.ID == "" {
		t.Fatalf("save paper: %s", body)
	}
	_, body = get(t, ts, "/api/tiku/papers")
	if !strings.Contains(body, "周测") || !strings.Contains(body, "月考") {
		t.Fatalf("papers list: %s", body)
	}
	post(t, ts, "/api/tiku/papers/delete", `{"id":"`+p.ID+`"}`)
	_, body = get(t, ts, "/api/tiku/papers")
	if strings.Contains(body, "周测") {
		t.Fatal("paper not deleted")
	}
	post(t, ts, "/api/lesson/save", `{"name":"第二课","slides":[{"name":"s1","template":"t","slots":{}}]}`)
	_, body = get(t, ts, "/api/lesson/get?name=第二课")
	if !strings.Contains(body, "s1") {
		t.Fatalf("lesson: %s", body)
	}
	post(t, ts, "/api/tags/save", `{"视频/a.mp4":[]}`)
	_, body = get(t, ts, "/api/tags/getAll")
	if strings.Contains(body, "重点") {
		t.Fatalf("tag not removed: %s", body)
	}
	post(t, ts, "/api/markers/save?path=视频/a.mp4", `{"markers":[]}`)
	_, body = get(t, ts, "/api/markers/get?path=视频/a.mp4")
	if !strings.Contains(body, `"markers":[]`) {
		t.Fatalf("markers: %s", body)
	}
}

func TestUploadAndSafety(t *testing.T) {
	ts, cfg, _ := setup(t)
	code, _ := post(t, ts, "/api/upload?path=资料/新文件.txt", "hello")
	if code != 200 {
		t.Fatal("upload failed")
	}
	if b, _ := os.ReadFile(filepath.Join(cfg.DataDir, "资料", "新文件.txt")); string(b) != "hello" {
		t.Fatal("uploaded content wrong")
	}
	_, body := get(t, ts, "/files/资料/新文件.txt")
	if body != "hello" {
		t.Fatal("file serve wrong")
	}
	// 系统目录（数据库、备份）不能通过网页读写
	for _, p := range []string{"/files/.xyz/xyz.db", "/api/md?path=.xyz/xyz.db"} {
		if code, _ := get(t, ts, p); code != 403 {
			t.Errorf("%s should be 403, got %d", p, code)
		}
	}
	if code, _ := post(t, ts, "/api/delete?path=.xyz/xyz.db", ""); code != 403 {
		t.Error("deleting db must be forbidden")
	}
	// ../ 被清洗掉，留在数据目录内
	for _, p := range []string{"/files/../../etc/passwd", "/files/..%2f..%2fetc%2fpasswd", "/api/md?path=../../etc/passwd"} {
		if _, body := get(t, ts, p); strings.Contains(body, "root:") {
			t.Errorf("traversal succeeded: %s", p)
		}
	}
	_, body = get(t, ts, "/api/list")
	if strings.Contains(body, ".xyz") || strings.Contains(body, "fire_") {
		t.Errorf("hidden files listed: %s", body)
	}
}

func TestBackup(t *testing.T) {
	_, cfg, db := setup(t)
	p, err := backup.Now(db, cfg.BackupDir())
	if err != nil {
		t.Fatal(err)
	}
	bk, _ := sql.Open("sqlite", p)
	defer bk.Close()
	var n int
	bk.QueryRow("SELECT COUNT(*) FROM questions").Scan(&n)
	if n != 1 || !backup.DoneToday(cfg.BackupDir()) {
		t.Fatalf("backup has %d questions", n)
	}
}

func trunc(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
