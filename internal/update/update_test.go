package update

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.3.0", "2.2.0", true}, {"v2.10.0", "2.9.9", true}, {"2.2.0", "2.2.0", false},
		{"2.1.9", "2.2.0", false}, {"2.2.1", "dev", true}, {"3.0.0-beta", "2.9.0", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q,%q)=%v", c.a, c.b, got)
		}
	}
}

func TestSHAFromBody(t *testing.T) {
	m := reSHA.FindStringSubmatch("XYZ 2.3.0\n\nSHA256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n")
	if m == nil || m[1][:4] != "0123" {
		t.Fatal("sha not found")
	}
}

func TestCheckAndDownload(t *testing.T) {
	bin := bytes.Repeat([]byte("Z"), 2<<20)
	sum := sha256.Sum256(bin)
	good := hex.EncodeToString(sum[:])
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/k/XYZ/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v9.1.0","body":"修复\n\nSHA256: %s","assets":[{"name":"XYZ.exe","size":%d,"browser_download_url":"%s/dl/XYZ.exe"}]}`, good, len(bin), srv.URL)
		case "/dl/XYZ.exe":
			w.Write(bin)
		case "/bad/" + srv.URL[7:] + "/dl/XYZ.exe": // 镜像返回被篡改的文件
			w.Write(bytes.Repeat([]byte("X"), 2<<20))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL
	s := New(Options{Repo: "k/XYZ", Current: "2.2.0"}, nil)
	rel, err := s.Check()
	if err != nil || rel.Version != "9.1.0" || rel.SHA256 != good || !s.snapshot().HasUpdate {
		t.Fatalf("check: %+v %v", rel, err)
	}
	dst := filepath.Join(t.TempDir(), "x.new")
	// 第一个镜像篡改文件，应校验失败并自动换下一个
	s.opt.Mirrors = []string{srv.URL + "/bad/", ""}
	if err := s.download(rel, dst); err != nil {
		t.Fatalf("download: %v", err)
	}
	b, _ := os.ReadFile(dst)
	if !bytes.Equal(b, bin) {
		t.Fatal("wrong file")
	}
	s.opt.Mirrors = []string{srv.URL + "/bad/"}
	if err := s.download(rel, dst); err == nil {
		t.Fatal("tampered file accepted")
	}
}
