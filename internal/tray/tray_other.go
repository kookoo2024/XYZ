//go:build !windows

package tray

import (
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
)

type Options struct {
	LocalURL string
	LANAddr  string
	DataDir  string
	OnReady  func()
	OnExit   func()
}

// Run 在 Linux/macOS 上无托盘，直接运行服务，Ctrl+C 或 systemd stop 时退出。
func Run(o Options) {
	if o.OnReady != nil {
		go o.OnReady()
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	if o.OnExit != nil {
		o.OnExit()
	}
}

func OpenBrowser(url string) {
	if runtime.GOOS == "darwin" {
		exec.Command("open", url).Start()
		return
	}
	exec.Command("xdg-open", url).Start()
}
