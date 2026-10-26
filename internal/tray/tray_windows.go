//go:build windows

// Package tray Windows 托盘图标、菜单与开机自启。
package tray

import (
	"os"
	"os/exec"

	"github.com/getlantern/systray"
)

// Options 托盘需要的信息与回调。
type Options struct {
	LocalURL string
	LANAddr  string
	DataDir  string
	OnReady  func() // 托盘就绪后调用（启动服务）
	OnExit   func()
}

// Run 显示托盘并阻塞，直到用户点「退出」。
func Run(o Options) {
	systray.Run(func() { onReady(o) }, func() {
		if o.OnExit != nil {
			o.OnExit()
		}
		os.Exit(0)
	})
}

func onReady(o Options) {
	systray.SetIcon(Icon())
	systray.SetTitle("XYZ")
	systray.SetTooltip("XYZ 教学服务平台 · 运行中")

	mOpen := systray.AddMenuItem("🌐 打开浏览器", "打开管理页面")
	mTiku := systray.AddMenuItem("📖 题库", "打开题库系统")
	mDir := systray.AddMenuItem("📁 打开 "+o.DataDir, "打开文件目录")
	systray.AddSeparator()
	mAuto := systray.AddMenuItemCheckbox("🚀 开机启动", "设置程序开机自动运行", isAutoStartEnabled())
	systray.AddSeparator()
	mInfo := systray.AddMenuItem("📡 "+o.LANAddr, "局域网访问地址")
	mInfo.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("❌ 退出", "关闭服务并退出")

	if o.OnReady != nil {
		go o.OnReady()
	}
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				OpenBrowser(o.LocalURL)
			case <-mTiku.ClickedCh:
				OpenBrowser(o.LocalURL + "/tiku")
			case <-mDir.ClickedCh:
				exec.Command("explorer", o.DataDir).Start()
			case <-mAuto.ClickedCh:
				if mAuto.Checked() {
					if disableAutoStart() {
						mAuto.Uncheck()
					}
				} else if enableAutoStart() {
					mAuto.Check()
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
const appName = "XYZ"

func isAutoStartEnabled() bool {
	return exec.Command("reg", "query", runKey, "/v", appName).Run() == nil
}

func enableAutoStart() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return exec.Command("reg", "add", runKey, "/v", appName, "/t", "REG_SZ", "/d", `"`+exe+`"`, "/f").Run() == nil
}

func disableAutoStart() bool {
	return exec.Command("reg", "delete", runKey, "/v", appName, "/f").Run() == nil
}

// OpenBrowser 用默认浏览器打开网址。
func OpenBrowser(url string) {
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
