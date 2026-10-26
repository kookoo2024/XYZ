// Package server 组装各模块，创建 HTTP 服务。
package server

import (
	"database/sql"
	"net"
	"net/http"
	"strings"
	"time"

	"xyz/internal/config"
	"xyz/internal/files"
	"xyz/internal/httpx"
	"xyz/internal/lesson"
	"xyz/internal/tiku"
	"xyz/internal/web"
)

// Version 由编译参数注入：-ldflags "-X xyz/internal/server.Version=2.0.0"
var Version = "dev"

// New 创建 http.Server。新模块（班级、轮播、PK……）都在这里注册一行即可。
func New(cfg *config.Config, db *sql.DB) *http.Server {
	mux := http.NewServeMux()
	web.Register(mux)
	tiku.New(db, cfg.DataDir).Register(mux)
	lesson.New(db).Register(mux)
	files.New(db, cfg.DataDir, cfg.PublicURL, web.Index, func() map[string]string {
		ip := LocalIP()
		return map[string]string{
			"status":  "running",
			"address": ip + ":" + cfg.Port(),
			"ip":      ip,
			"rootDir": cfg.DataDir,
			"version": Version,
		}
	}).Register(mux) // 含 "/" 兜底，放最后

	return &http.Server{
		Addr:              cfg.Listen,
		Handler:           httpx.Recover(httpx.Logger(mux)),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		// 不设 ReadTimeout/WriteTimeout：大文件上传和视频播放可能很久
	}
}

// LocalIP 优先返回 192.168.* 局域网地址，方便教室大屏直连。
func LocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	var first string
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		ip := ipnet.IP.String()
		if strings.HasPrefix(ip, "192.168.") {
			return ip
		}
		if first == "" {
			first = ip
		}
	}
	if first != "" {
		return first
	}
	return "127.0.0.1"
}
