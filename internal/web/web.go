// Package web 内嵌前端页面（打包进 exe）。
package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFS embed.FS

// 注入到旧页面的用户栏脚本（显示当前账号、管理后台入口、退出）
var userbarTag = []byte(`<script src="/assets/userbar.js" defer></script>`)

func page(name string, withUserbar bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFS.ReadFile("static/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if withUserbar {
			if i := bytes.LastIndex(data, []byte("</body>")); i >= 0 {
				data = append(append(append([]byte{}, data[:i]...), userbarTag...), data[i:]...)
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
	}
}

// Index 文件管理界面（作为 "/" 的兜底页面）。
var Index = page("index.html", true)

// Register 注册各功能页面与公共静态资源。
func Register(mux *http.ServeMux) {
	mux.HandleFunc("/lesson", page("lesson.html", true))
	mux.HandleFunc("/tiku", page("tiku.html", true))
	mux.HandleFunc("/reader", page("reader.html", true))
	mux.HandleFunc("/login", page("login.html", false))
	mux.HandleFunc("/setup", page("login.html", false))
	mux.HandleFunc("/admin", page("admin.html", false))
	mux.HandleFunc("/screen", page("screen.html", false))
	mux.HandleFunc("/pk", page("pk.html", false))
	assets, _ := fs.Sub(staticFS, "static/assets")
	fsrv := http.StripPrefix("/assets/", http.FileServer(http.FS(assets)))
	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fsrv.ServeHTTP(w, r)
	})
}
