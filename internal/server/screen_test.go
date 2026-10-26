package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScreenSlidesAndCall(t *testing.T) {
	ts, cfg, _ := setup(t)
	_, body := post(t, ts, "/api/v1/classes", `{"name":"211班"}`)
	cid := idOf(t, body)
	_, body = post(t, ts, "/api/v1/classes", `{"name":"215班"}`)
	cid2 := idOf(t, body)

	// 轮播：根目录 + 211班 子文件夹，自然排序，非图片忽略
	root := filepath.Join(cfg.DataDir, "轮播")
	os.MkdirAll(filepath.Join(root, "211班"), 0o755)
	for _, f := range []string{"10.jpg", "2.png", "说明.txt", "1.mp4"} {
		os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644)
	}
	os.WriteFile(filepath.Join(root, "211班", "a.jpg"), []byte("x"), 0o644)
	_, body = get(t, ts, "/api/v1/screen/slides?class_id="+cid2)
	var sl struct {
		Slides []struct{ Name, URL, Type string }
	}
	json.Unmarshal([]byte(body), &sl)
	names := []string{}
	for _, s := range sl.Slides {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "1.mp4,2.png,10.jpg" || sl.Slides[0].Type != "video" {
		t.Fatalf("root slides: %s", body)
	}
	if c, b := get(t, ts, sl.Slides[1].URL); c != 200 || b != "x" {
		t.Fatalf("slide url not served: %d", c)
	}
	_, body = get(t, ts, "/api/v1/screen/slides?class_id="+cid)
	if !strings.Contains(body, "a.jpg") || strings.Contains(body, "10.jpg") {
		t.Fatalf("class folder should win: %s", body)
	}

	// 点名：5 人，1 人缺席；一轮内不重复
	post(t, ts, "/api/v1/classes/"+cid+"/students/import", `{"text":"1 甲\n2 乙\n3 丙\n4 丁\n5 戊"}`)
	_, body = get(t, ts, "/api/v1/screen/call?class_id="+cid)
	var st struct {
		Students []struct {
			ID   int64
			Name string
		}
	}
	json.Unmarshal([]byte(body), &st)
	absent := st.Students[4].ID
	seen := map[int64]bool{}
	for i := 0; i < 4; i++ {
		_, body = post(t, ts, "/api/v1/screen/call", `{"class_id":`+cid+`,"count":1,"exclude":[`+jsonNum(absent)+`]}`)
		var r struct {
			Picked   []struct{ ID int64 }
			NewRound bool `json:"new_round"`
		}
		json.Unmarshal([]byte(body), &r)
		if len(r.Picked) != 1 || r.NewRound {
			t.Fatalf("pick %d: %s", i, body)
		}
		id := r.Picked[0].ID
		if id == absent || seen[id] {
			t.Fatalf("repeat or absent picked: %s", body)
		}
		seen[id] = true
	}
	// 4 个到场的都点过了，再抽 2 人应开启新一轮
	_, body = post(t, ts, "/api/v1/screen/call", `{"class_id":`+cid+`,"count":2,"exclude":[`+jsonNum(absent)+`]}`)
	if !strings.Contains(body, `"new_round":true`) || strings.Count(body, `"id"`) != 2 {
		t.Fatalf("new round: %s", body)
	}
	if c, _ := post(t, ts, "/api/v1/screen/call/reset", `{"class_id":`+cid+`}`); c != 200 {
		t.Fatal("reset failed")
	}
	if c, _ := post(t, ts, "/api/v1/screen/call", `{"class_id":`+cid2+`,"count":1}`); c != 400 {
		t.Fatal("empty class should 400")
	}
	if c, _ := get(t, ts, "/screen"); c != 200 {
		t.Fatal("screen page")
	}
}
