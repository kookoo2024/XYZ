// Package config 读取/生成 xyz.json 配置文件。换端口、换目录、换域名都改这里，不用重新编译。
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FileName 配置文件名，默认放在 exe 同目录。
const FileName = "xyz.json"

type Config struct {
	// Listen 监听地址，如 ":80"、"127.0.0.1:8080"。走 Cloudflare 隧道时可只监听本机。
	Listen string `json:"listen"`
	// DataDir 数据根目录（课件、视频、图片等），默认 Windows 为 D:\Fire。
	DataDir string `json:"data_dir"`
	// PublicURL 对外访问地址，如 "https://150700.xyz"；留空则按访问者的 Host 自动生成链接。
	PublicURL string `json:"public_url"`
	// SchoolName 学校名称（界面显示用，后续多校时作为默认学校）。
	SchoolName string `json:"school_name"`
	// OpenBrowser 启动后是否自动打开浏览器。
	OpenBrowser bool `json:"open_browser"`
	// Backup 自动备份设置。
	Backup Backup `json:"backup"`

	path string // 配置文件实际路径
}

type Backup struct {
	Enabled  bool   `json:"enabled"`
	Hour     int    `json:"hour"`      // 每天几点之后执行（0-23）
	KeepDays int    `json:"keep_days"` // 保留天数
	Dir      string `json:"dir"`       // 备份目录，留空为 <data_dir>/.xyz/backups
}

// Default 返回默认配置，与原 FireCloud 行为一致（:80、D:\Fire）。
func Default() *Config {
	dataDir := `D:\Fire`
	if runtime.GOOS != "windows" {
		dataDir = "./data"
	}
	return &Config{
		Listen:      ":80",
		DataDir:     dataDir,
		SchoolName:  "",
		OpenBrowser: true,
		Backup:      Backup{Enabled: true, Hour: 2, KeepDays: 30},
	}
}

// Path 返回配置文件路径。
func (c *Config) Path() string { return c.path }

// SystemDir 返回程序自用目录 <data_dir>/.xyz（数据库、备份），以 . 开头不会出现在文件列表里。
func (c *Config) SystemDir() string { return filepath.Join(c.DataDir, ".xyz") }

// DBPath 返回主数据库路径。
func (c *Config) DBPath() string { return filepath.Join(c.SystemDir(), "xyz.db") }

// BackupDir 返回备份目录。
func (c *Config) BackupDir() string {
	if c.Backup.Dir != "" {
		return c.Backup.Dir
	}
	return filepath.Join(c.SystemDir(), "backups")
}

// LocalURL 返回本机访问地址，用于托盘菜单和自动打开浏览器。
func (c *Config) LocalURL() string {
	host, port := splitListen(c.Listen)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	if port == "80" || port == "" {
		return "http://" + host
	}
	return "http://" + host + ":" + port
}

// Port 返回端口号字符串。
func (c *Config) Port() string {
	_, p := splitListen(c.Listen)
	return p
}

func splitListen(l string) (host, port string) {
	i := strings.LastIndex(l, ":")
	if i < 0 {
		return l, ""
	}
	return l[:i], l[i+1:]
}

// DefaultPath 返回 exe 同目录下的 xyz.json。
func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		return FileName
	}
	return filepath.Join(filepath.Dir(exe), FileName)
}

// Load 读取配置；文件不存在时写出一份默认配置，方便老师直接用记事本修改。
func Load(path string) (*Config, error) {
	c := Default()
	c.path = path
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, c.Save()
	}
	if err != nil {
		return nil, err
	}
	// 去掉 UTF-8 BOM（记事本保存时可能带上）
	data = []byte(strings.TrimPrefix(string(data), "\ufeff"))
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	c.normalize()
	return c, nil
}

func (c *Config) normalize() {
	d := Default()
	if strings.TrimSpace(c.Listen) == "" {
		c.Listen = d.Listen
	}
	if strings.TrimSpace(c.DataDir) == "" {
		c.DataDir = d.DataDir
	}
	if abs, err := filepath.Abs(c.DataDir); err == nil {
		c.DataDir = abs
	}
	c.PublicURL = strings.TrimRight(strings.TrimSpace(c.PublicURL), "/")
	if c.Backup.Hour < 0 || c.Backup.Hour > 23 {
		c.Backup.Hour = d.Backup.Hour
	}
	if c.Backup.KeepDays <= 0 {
		c.Backup.KeepDays = d.Backup.KeepDays
	}
}

// Save 写回配置文件。
func (c *Config) Save() error {
	c.normalize()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0644)
}
