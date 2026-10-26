#!/usr/bin/env sh
# Linux 服务器版（无托盘），也可交叉编译 Windows 版：GOOS=windows ./build.sh
set -e
VERSION=${VERSION:-2.3.0}
go test ./...
EXT=""; [ "${GOOS:-}" = "windows" ] && EXT=".exe"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X xyz/internal/server.Version=$VERSION" -o "xyz$EXT" ./cmd/xyz
echo "built xyz$EXT"
