// Package backup 每天自动把数据库快照到备份目录，并清理过期快照。
package backup

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const prefix = "xyz-"

// Now 执行一次快照，返回备份文件路径。VACUUM INTO 在线生成一致的副本，不影响使用。
func Now(db *sql.DB, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	name := prefix + time.Now().Format("20060102-150405") + ".db"
	dst := filepath.Join(dir, name)
	if _, err := db.Exec("VACUUM INTO ?", dst); err != nil {
		return "", fmt.Errorf("备份失败: %w", err)
	}
	return dst, nil
}

// Prune 删除超过 keepDays 天的快照。
func Prune(dir string, keepDays int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// DoneToday 判断今天是否已有快照。
func DoneToday(dir string) bool {
	today := prefix + time.Now().Format("20060102")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), today) {
			return true
		}
	}
	return false
}

// List 返回已有快照（新的在前）。
func List(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".db") {
			out = append(out, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

// Schedule 后台运行：每 10 分钟检查一次，过了设定钟点且今天还没备份就执行。
// 笔记本夜里关机也没关系，第二天开机后会补做。
func Schedule(db *sql.DB, dir string, hour, keepDays int) {
	go func() {
		check := func() {
			if time.Now().Hour() < hour || DoneToday(dir) {
				return
			}
			if p, err := Now(db, dir); err != nil {
				log.Println(err)
			} else {
				log.Println("自动备份完成:", p)
				Prune(dir, keepDays)
			}
		}
		check()
		t := time.NewTicker(10 * time.Minute)
		for range t.C {
			check()
		}
	}()
}
