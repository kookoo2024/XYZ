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
	"os/exec"
	"path/filepath"
	"time"

	"xyz/internal/backup"
	"xyz/internal/config"
	"xyz/internal/server"
	"xyz/internal/store"
	"xyz/internal/tray"
	"xyz/internal/update"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "配置文件路径")
	doBackup := flag.Bool("backup", false, "立即备份一次数据库后退出")
	noBrowser := flag.Bool("no-browser", false, "启动时不打开浏览器（在线更新重启时使用）")
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
	if err := store.EnsureDefaults(db, cfg.SchoolName); err != nil {
		fatal("初始化学校信息失败: " + err.Error())
	}

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

	update.Cleanup()
	var srv *http.Server
	up := update.New(update.Options{Repo: cfg.Update.Repo, Mirrors: cfg.Update.Mirrors, Current: server.Version},
		func(exe string) {
			// 在线更新/回退后重启：先关服务释放端口和数据库，再启动新程序
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			srv.Shutdown(ctx)
			cancel()
			db.Close()
			cmd := exec.Command(exe, "-config", cfg.Path(), "-no-browser")
			cmd.Dir = filepath.Dir(exe)
			if err := cmd.Start(); err != nil {
				log.Println("重启失败，请手动打开程序:", err)
			}
			os.Exit(0)
		})
	if cfg.Update.AutoCheck && server.Version != "dev" {
		up.StartAutoCheck()
	}
	srv = server.New(cfg, db, up)
	tray.Run(tray.Options{
		LocalURL: cfg.LocalURL(),
		LANAddr:  server.LocalIP() + ":" + cfg.Port(),
		DataDir:  cfg.DataDir,
		OnReady: func() {
			if cfg.OpenBrowser && !*noBrowser {
				go func() {
					time.Sleep(500 * time.Millisecond)
					tray.OpenBrowser(cfg.LocalURL())
				}()
			}
			log.Printf("服务地址 %s（局域网 %s:%s）", cfg.LocalURL(), server.LocalIP(), cfg.Port())
			// 端口可能还被刚退出的旧进程占着（在线更新重启时），重试 15 秒
			for i := 0; ; i++ {
				err := srv.ListenAndServe()
				if err == nil || err == http.ErrServerClosed {
					break
				}
				if i >= 15 {
					log.Println("服务启动失败（端口被占用？可在 xyz.json 改 listen）:", err)
					break
				}
				time.Sleep(time.Second)
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
