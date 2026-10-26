package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// remote 模拟一台局域网/外网电脑（带 X-Forwarded-For，不算本机），自带 cookie。
type remote struct {
	t  *testing.T
	ts *httptest.Server
	c  *http.Client
}

type xffTransport struct{}

func (xffTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("X-Forwarded-For", "192.168.1.50")
	return http.DefaultTransport.RoundTrip(r)
}

func newRemote(t *testing.T, ts *httptest.Server) *remote {
	jar, _ := cookiejar.New(nil)
	return &remote{t: t, ts: ts, c: &http.Client{Jar: jar, Transport: xffTransport{},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (r *remote) do(method, path, body string) (int, string, http.Header) {
	req, _ := http.NewRequest(method, r.ts.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := r.c.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header
}

func (r *remote) must(method, path, body string, want int) string {
	r.t.Helper()
	code, b, _ := r.do(method, path, body)
	if code != want {
		r.t.Fatalf("%s %s -> %d %s (want %d)", method, path, code, b, want)
	}
	return b
}

func idOf(t *testing.T, body string) string {
	var x struct{ ID int64 }
	json.Unmarshal([]byte(body), &x)
	if x.ID == 0 {
		t.Fatalf("no id in %s", body)
	}
	return jsonNum(x.ID)
}

func jsonNum(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestAuthFlow(t *testing.T) {
	ts, _, _ := setup(t)
	anon := newRemote(t, ts)

	// 未登录：接口 401，页面跳登录，登录页和静态资源可访问
	anon.must("GET", "/api/tiku/list", "", 401)
	code, _, h := anon.do("GET", "/tiku", "")
	if code != 302 || !strings.HasPrefix(h.Get("Location"), "/login?next=") {
		t.Fatalf("expected redirect, got %d %s", code, h.Get("Location"))
	}
	anon.must("GET", "/login", "", 200)
	anon.must("GET", "/assets/xyz.css", "", 200)
	if !strings.Contains(anon.must("GET", "/api/v1/auth/status", "", 200), `"setup_needed":true`) {
		t.Fatal("should need setup")
	}

	// 初始化只能在本机
	anon.must("POST", "/api/v1/auth/setup", `{"username":"chen","name":"陈老师","password":"123456"}`, 403)
	code, body := post(t, ts, "/api/v1/auth/setup", `{"username":"chen","name":"陈老师","password":"123456","school":"黄石三中"}`)
	if code != 200 {
		t.Fatalf("local setup: %d %s", code, body)
	}
	if c, _ := post(t, ts, "/api/v1/auth/setup", `{"username":"x","password":"123456"}`); c != 409 {
		t.Fatal("second setup must fail")
	}

	// 登录
	anon.must("POST", "/api/v1/auth/login", `{"username":"chen","password":"wrong"}`, 401)
	admin := newRemote(t, ts)
	admin.must("POST", "/api/v1/auth/login", `{"username":"CHEN","password":"123456"}`, 200) // 用户名不区分大小写
	if !strings.Contains(admin.must("GET", "/api/v1/auth/me", "", 200), `"role":"admin"`) {
		t.Fatal("me should be admin")
	}
	admin.must("GET", "/api/tiku/list", "", 200)
	admin.must("GET", "/tiku", "", 200)
	if !strings.Contains(admin.must("GET", "/api/v1/school", "", 200), "黄石三中") {
		t.Fatal("school name not saved")
	}

	// 建教师、班级
	tid := idOf(t, admin.must("POST", "/api/v1/users", `{"username":"wang","name":"王老师","password":"654321"}`, 200))
	admin.must("POST", "/api/v1/users", `{"username":"wang","name":"重复","password":"654321"}`, 400)
	cid := idOf(t, admin.must("POST", "/api/v1/classes", `{"name":"211班","grade":"高二","enroll_year":2025,"head_teacher_id":`+tid+`}`, 200))
	cid2 := idOf(t, admin.must("POST", "/api/v1/classes", `{"name":"215班","grade":"高二"}`, 200))
	subs := admin.must("GET", "/api/v1/subjects", "", 200)
	if !strings.Contains(subs, "数学") {
		t.Fatal("default subjects missing")
	}
	var sl []struct {
		ID   int64
		Name string
	}
	json.Unmarshal([]byte(subs), &sl)
	math := ""
	for _, s := range sl {
		if s.Name == "数学" {
			math = jsonNum(s.ID)
		}
	}
	admin.must("PUT", "/api/v1/teaching", `{"class_id":`+cid2+`,"subject_id":`+math+`,"teacher_id":`+tid+`}`, 200)

	// 教师登录：本班可导入名单，别的班不行；不能管账号
	teacher := newRemote(t, ts)
	teacher.must("POST", "/api/v1/auth/login", `{"username":"wang","password":"654321"}`, 200)
	roster := "学号\t姓名\t性别\n2025001\t张三\t男\n2025002\t李四\t女\n王五\n"
	r := teacher.must("POST", "/api/v1/classes/"+cid+"/students/import", `{"text":`+jsonStr(roster)+`}`, 200)
	if !strings.Contains(r, `"added":3`) {
		t.Fatalf("import: %s", r)
	}
	r = teacher.must("POST", "/api/v1/classes/"+cid+"/students/import", `{"text":"2025001 张三丰 男"}`, 200)
	if !strings.Contains(r, `"updated":1`) {
		t.Fatalf("re-import should update: %s", r)
	}
	st := teacher.must("GET", "/api/v1/classes/"+cid+"/students", "", 200)
	if !strings.Contains(st, "张三丰") || strings.Count(st, `"id"`) != 3 {
		t.Fatalf("students: %s", st)
	}
	teacher.must("POST", "/api/v1/classes/"+cid2+"/students/import", `{"text":"赵六"}`, 403)
	teacher.must("POST", "/api/v1/users", `{"username":"x","password":"123456"}`, 403)
	teacher.must("POST", "/api/v1/classes", `{"name":"x"}`, 403)
	my := teacher.must("GET", "/api/v1/me/classes", "", 200)
	if !strings.Contains(my, `"is_head":true`) || !strings.Contains(my, "数学") {
		t.Fatalf("my classes: %s", my)
	}
	admin.must("DELETE", "/api/v1/classes/"+cid, "", 400) // 有学生不能删

	// 停用教师：立即下线
	admin.must("PUT", "/api/v1/users/"+tid, `{"disabled":true}`, 200)
	teacher.must("GET", "/api/v1/auth/me", "", 401)
	teacher.must("POST", "/api/v1/auth/login", `{"username":"wang","password":"654321"}`, 403)

	// 不能停用最后一个管理员
	var me struct{ ID int64 }
	json.Unmarshal([]byte(admin.must("GET", "/api/v1/auth/me", "", 200)), &me)
	admin.must("PUT", "/api/v1/users/"+jsonNum(me.ID), `{"disabled":true}`, 400)

	// 改密码后旧密码失效
	admin.must("POST", "/api/v1/auth/password", `{"old":"123456","new":"abcdef"}`, 200)
	newRemote(t, ts).must("POST", "/api/v1/auth/login", `{"username":"chen","password":"123456"}`, 401)
	admin.must("POST", "/api/v1/auth/logout", "", 200)
	admin.must("GET", "/api/tiku/list", "", 401)
}

func TestShareLinkWithoutLogin(t *testing.T) {
	ts, _, _ := setup(t)
	_, body := get(t, ts, "/api/share?path=视频/a.mp4") // 本机生成分享
	var sh struct{ URL string }
	json.Unmarshal([]byte(body), &sh)
	u, err := url.Parse(sh.URL)
	if err != nil || u.Query().Get("s") == "" {
		t.Fatalf("share url: %s", sh.URL)
	}
	anon := newRemote(t, ts)
	if got := anon.must("GET", u.RequestURI(), "", 200); got != "fake" {
		t.Fatalf("shared file: %q", got)
	}
	// 签名不能挪到别的文件上
	code, _, _ := anon.do("GET", "/files/课件/index.html?s="+u.Query().Get("s"), "")
	if code == 200 {
		t.Fatal("signature reused on another file")
	}
	code, _, _ = anon.do("GET", "/files/视频/a.mp4?s=deadbeef", "")
	if code == 200 {
		t.Fatal("bad signature accepted")
	}
}

func TestLoginRateLimit(t *testing.T) {
	ts, _, _ := setup(t)
	post(t, ts, "/api/v1/auth/setup", `{"username":"chen","password":"123456"}`)
	r := newRemote(t, ts)
	for i := 0; i < 8; i++ {
		r.must("POST", "/api/v1/auth/login", `{"username":"chen","password":"bad"}`, 401)
	}
	r.must("POST", "/api/v1/auth/login", `{"username":"chen","password":"123456"}`, 429)
}

func TestCloudflareIsNotLocal(t *testing.T) {
	ts, _, _ := setup(t)
	req, _ := http.NewRequest("GET", ts.URL+"/api/tiku/list", nil)
	req.Header.Set("Cf-Connecting-Ip", "1.2.3.4") // cloudflared 从本机转发进来
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("tunnel traffic must not be trusted as localhost, got %d", resp.StatusCode)
	}
}

func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }
