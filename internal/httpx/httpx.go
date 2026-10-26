// Package httpx 放各模块共用的 HTTP 小工具。
package httpx

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// JSON 输出 JSON 响应。
func JSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

// Logger 记录每个请求的方法、路径、状态与耗时（静态资源除外以免刷屏）。
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

// Recover 防止单个请求 panic 拖垮整个服务。
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic %s %s: %v", r.Method, r.URL.Path, err)
				http.Error(w, "服务器内部错误", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush 透传，保证视频等流式响应正常。
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Error 输出 {"error": msg}，并设置状态码。
func Error(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Decode 解析 JSON 请求体（限制 1MB）；失败时已写好 400 响应，返回 false。
func Decode(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		Error(w, http.StatusBadRequest, "请求格式错误")
		return false
	}
	return true
}

// OK 输出 {"ok": true}。
func OK(w http.ResponseWriter) { JSON(w, map[string]bool{"ok": true}) }
