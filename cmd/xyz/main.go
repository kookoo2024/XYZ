// XYZ 教学服务平台（FireCloud 2.0）入口。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"xyz/internal/backup"
	"xyz/internal/config"
	"xyz/internal/server"
	"xyz/internal/store"
	"xyz/internal/tray"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "配置文件路径")
	doBackup := flag.Bool("backup", false, "立即备份一次数据库后退出")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fatal("读取配置失败 " + *cfgPath + ": " + err.Error())
	}
	if err := os.MkdirAll(cfg.SystemDir(), 0755); err != nil {
		fatal("无法创建数据目录 " + cfg.DataDir + ": " + err.Error())
	}
	setupLog(cfg)
	log.Printf("XYZ %s 启动，配置 %s，数据目录 %s", server.Version, cfg.Path(), cfg.DataDir)

	db, err := store.Open(cfg.DBPath())
	if err != nil {
		fatal("打开数据库失败: " + err.Error())
	}
	store.ImportLegacy(db, cfg.DataDir)

	if *doBackup {
		p, err := backup.Now(db, cfg.BackupDir())
		if err != nil {
			fatal(err.Error())
		}
		fmt.Println("备份完成:", p)
		return
	}
	if cfg.Backup.Enabled {
		backup.Schedule(db, cfg.BackupDir(), cfg.Backup.Hour, cfg.Backup.KeepDays)
	}

	srv := server.New(cfg, db)
	tray.Run(tray.Options{
		LocalURL: cfg.LocalURL(),
		LANAddr:  server.LocalIP() + ":" + cfg.Port(),
		DataDir:  cfg.DataDir,
		OnReady: func() {
			if cfg.OpenBrowser {
				go func() {
					time.Sleep(500 * time.Millisecond)
					tray.OpenBrowser(cfg.LocalURL())
				}()
			}
			log.Printf("服务地址 %s（局域网 %s:%s）", cfg.LocalURL(), server.LocalIP(), cfg.Port())
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Println("服务启动失败（端口被占用？可在 xyz.json 改 listen）:", err)
			}
		},
		OnExit: func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			srv.Shutdown(ctx)
			db.Close()
		},
	})
}

// setupLog 日志同时写到 <data_dir>/.xyz/xyz.log（Windows 无窗口模式下看不到控制台）。
func setupLog(cfg *config.Config) {
	f, err := os.OpenFile(filepath.Join(cfg.SystemDir(), "xyz.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
}

func fatal(msg string) {
	log.Println(msg)
	os.Exit(1)
}
