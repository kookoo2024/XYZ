// Package store 管理唯一的 SQLite 数据库：打开、建表/升级、导入 FireCloud 旧数据。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Open 打开（必要时创建）数据库并执行升级。
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) + "?" + url.Values{
		"_pragma": []string{"journal_mode(WAL)", "busy_timeout(5000)", "foreign_keys(ON)", "synchronous(NORMAL)"},
	}.Encode()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite 同一时刻只允许一个写入者；限制连接数避免 “database is locked”，对单校负载足够。
	db.SetMaxOpenConns(4)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migrations 按顺序执行，序号即 PRAGMA user_version。只能追加，不能修改已发布的条目。
var migrations = []string{
	// v1：FireCloud 现有功能全部入库
	`
	CREATE TABLE IF NOT EXISTS questions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL DEFAULT '',
		stem TEXT NOT NULL DEFAULT '',
		options TEXT NOT NULL DEFAULT '[]',
		answer TEXT NOT NULL DEFAULT '',
		solution TEXT NOT NULL DEFAULT '',
		chapter TEXT NOT NULL DEFAULT '',
		section TEXT NOT NULL DEFAULT '',
		difficulty TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]',
		images TEXT NOT NULL DEFAULT '[]',
		source TEXT NOT NULL DEFAULT '',
		use_count INTEGER NOT NULL DEFAULT 0,
		last_used TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_q_chapter ON questions(chapter);
	CREATE INDEX IF NOT EXISTS idx_q_type ON questions(type);
	CREATE INDEX IF NOT EXISTS idx_q_difficulty ON questions(difficulty);
	CREATE INDEX IF NOT EXISTS idx_q_source ON questions(source);

	CREATE TABLE IF NOT EXISTS papers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT '',
		items TEXT NOT NULL DEFAULT '[]',
		updated INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS file_tags (
		path TEXT PRIMARY KEY,
		tags TEXT NOT NULL DEFAULT '[]'
	);

	CREATE TABLE IF NOT EXISTS markers (
		path TEXT PRIMARY KEY,
		markers TEXT NOT NULL DEFAULT '[]'
	);

	CREATE TABLE IF NOT EXISTS lessons (
		name TEXT PRIMARY KEY,
		data TEXT NOT NULL,
		updated INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS kv (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS legacy_imports (
		name TEXT PRIMARY KEY,
		imported_at TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT ''
	);
	`,
}

// Migrate 把数据库升级到最新版本。
func Migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	for i := v; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// GetKV / SetKV 简单键值存储（教材目录树等）。
func GetKV(db *sql.DB, key string) (string, bool) {
	var v string
	if err := db.QueryRow("SELECT value FROM kv WHERE key=?", key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

func SetKV(db *sql.DB, key, value string) error {
	_, err := db.Exec("INSERT INTO kv(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return err
}

func sqlQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func bgCtx() context.Context { return context.Background() }
