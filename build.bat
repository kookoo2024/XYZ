@echo off
chcp 65001 >nul
SET CGO_ENABLED=0
SET GOOS=windows
SET GOARCH=amd64
SET VERSION=2.0.0

echo [XYZ] 正在编译...
go test ./... || goto :fail
go build -trimpath -ldflags "-s -w -H=windowsgui -X xyz/internal/server.Version=%VERSION%" -o XYZ.exe ./cmd/xyz || goto :fail
echo [XYZ] 编译成功: XYZ.exe
pause
exit /b 0

:fail
echo [XYZ] 编译失败
pause
exit /b 1
