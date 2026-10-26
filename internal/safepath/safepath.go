// Package safepath 负责把前端传来的相对路径安全地映射到数据目录内。
package safepath

import (
	"path/filepath"
	"strings"
)

// CleanRel 规范化相对路径：统一为 / 分隔，去掉 . / .. / 空段与首尾斜杠。
func CleanRel(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	parts := strings.Split(p, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		clean = append(clean, part)
	}
	return strings.Join(clean, "/")
}

// Join 把相对路径拼到 root 下，返回绝对路径；若结果跑出 root（含 D:\Fire2 这类前缀绕过），ok=false。
func Join(root, rel string) (abs string, ok bool) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	abs = filepath.Join(rootAbs, filepath.FromSlash(CleanRel(rel)))
	return abs, Within(rootAbs, abs)
}

// Within 判断 abs 是否位于 root 之内（或就是 root）。Windows 下不区分大小写。
func Within(root, abs string) bool {
	r, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	if r == "." {
		return true
	}
	if r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(r) {
		return false
	}
	return true
}
