// Package web 内嵌前端页面（打包进 exe）。
package web

import (
	"embed"
	"net/http"
)

//go:embed static/*
var staticFS embed.FS

func page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFS.ReadFile("static/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
	}
}

// Index 文件管理界面（作为 "/" 的兜底页面）。
var Index = page("index.html")

// Register 注册各功能页面。
func Register(mux *http.ServeMux) {
	mux.HandleFunc("/lesson", page("lesson.html"))
	mux.HandleFunc("/tiku", page("tiku.html"))
	mux.HandleFunc("/reader", page("reader.html"))
}
