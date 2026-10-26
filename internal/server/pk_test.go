package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPK(t *testing.T) {
	ts, _, db := setup(t)
	_, body := post(t, ts, "/api/v1/classes", `{"name":"215班"}`)
	cid := idOf(t, body)
	post(t, ts, "/api/v1/classes/"+cid+"/students/import", `{"text":"1 甲\n2 乙"}`)
	_, body = get(t, ts, "/api/v1/screen/call?class_id="+cid)
	var st struct{ Students []struct{ ID int64 } }
	json.Unmarshal([]byte(body), &st)
	a, b := st.Students[0].ID, st.Students[1].ID

	// 旧题（学科空 = 数学）+ 一道物理题 + 一道解答题（不能进 PK）
	db.Exec(`INSERT INTO questions(type,stem,options,answer,chapter,section,tags) VALUES
		('单选','1+1=?','["A. 1","B. 2","C. 3","D. 4","E. 5"]','B','第二章 函数','2.1','["加法"]'),
		('单选','g≈?','["9.8","10.8"]','A','第一章 力','','[]'),
		('解答题','证明','[]','','第二章 函数','','[]')`)
	db.Exec(`UPDATE questions SET subject='物理' WHERE stem='g≈?'`)

	_, body = get(t, ts, "/api/v1/pk/scope?subject=数学")
	if !strings.Contains(body, `"name":"物理","count":1`) || !strings.Contains(body, "第二章 函数") || strings.Contains(body, "第一章 力") || !strings.Contains(body, "加法") {
		t.Fatalf("scope: %s", body)
	}
	_, body = post(t, ts, "/api/v1/pk/questions", `{"subject":"数学","count":5,"options":3,"class_id":`+cid+`}`)
	var qs []struct {
		ID      int64
		Options []string
		Letters []string
		Answer  int
	}
	json.Unmarshal([]byte(body), &qs)
	// 原题选项 5 个，被裁到 3 个，正确项「2」一定保留且字母前缀去掉
	if len(qs) < 1 || len(qs[0].Options) != 3 || qs[0].Options[qs[0].Answer] != "2" || qs[0].Letters[qs[0].Answer] != "B" {
		t.Fatalf("questions: %s", body)
	}
	for _, q := range qs {
		if q.ID == 0 || len(q.Options) < 2 {
			t.Fatalf("bad q %s", body)
		}
	}
	if len(qs) != 1 {
		t.Fatalf("only 1 math choice question, got %d: %s", len(qs), body)
	}
	qid := qs[0].ID

	save := fmt.Sprintf(`{"class_id":%s,"subject":"数学","mode":"same","total":1,
		"players":[{"id":%d,"name":"甲","score":140},{"id":%d,"name":"乙","score":0}],
		"answers":[{"student_id":%d,"question_id":%d,"choice":"B","correct":true,"ms":3000},
		           {"student_id":%d,"question_id":%d,"choice":"A","correct":false,"ms":5000}]}`, cid, a, b, a, qid, b, qid)
	if c, body := post(t, ts, "/api/v1/pk/matches", save); c != 200 {
		t.Fatalf("save: %d %s", c, body)
	}
	// 外班学生不能写进来
	if c, _ := post(t, ts, "/api/v1/pk/matches", strings.Replace(save, fmt.Sprintf(`"id":%d,"name":"乙"`, b), `"id":99999,"name":"乙"`, 1)); c != 400 {
		t.Fatalf("foreign student accepted: %d", c)
	}
	_, body = get(t, ts, "/api/v1/pk/stats?class_id="+cid+"&subject=数学")
	var s struct {
		Ranks []struct {
			Name          string
			Matches, Wins int
			Acc           float64
		}
		Recent []struct{ Winner string }
		Wrong  []struct {
			Wrong int
			Names string
		}
	}
	json.Unmarshal([]byte(body), &s)
	if len(s.Ranks) != 2 || s.Ranks[0].Name != "甲" || s.Ranks[0].Wins != 1 || s.Ranks[0].Acc != 1 || s.Ranks[1].Acc != 0 {
		t.Fatalf("ranks: %s", body)
	}
	if len(s.Recent) != 1 || s.Recent[0].Winner != "甲" || len(s.Wrong) != 1 || s.Wrong[0].Names != "乙" {
		t.Fatalf("recent/wrong: %s", body)
	}
	// 个人错题
	_, body = get(t, ts, fmt.Sprintf("/api/v1/pk/stats?class_id=%s&student_id=%d", cid, a))
	if !strings.Contains(body, `"wrong":[]`) {
		t.Fatalf("甲 should have no wrong: %s", body)
	}
	if c, _ := get(t, ts, "/pk"); c != 200 {
		t.Fatal("pk page")
	}
}
