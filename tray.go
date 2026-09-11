package main

import (
	"log"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync/atomic"

	"fyne.io/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"caps/internal/autostart"
	"caps/internal/icon"
	"caps/internal/store"
)

var (
	trayReady     atomic.Bool      // 托盘是否已就绪（就绪前不要操作图标/菜单）
	menuToggle    *systray.MenuItem // “改键”复选菜单项
	menuAutostart *systray.MenuItem // “开机启动”复选菜单项
)

// runTray 承载通知区域图标。该 goroutine 必须固定在同一个 OS 线程上：
// Windows 只把窗口消息投递给创建该窗口的线程，而 systray 库会在调用线程上
// 创建自己的窗口。
func (a *App) runTray() {
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()
	systray.Run(a.onTrayReady, func() { trayReady.Store(false) })
}

// redirectSystrayLog 把标准日志重定向到文件，便于在无界面构建中排查托盘错误。
func redirectSystrayLog() {
	if err := os.MkdirAll(store.Dir(), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(store.Dir(), "systray.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(f)
}

// onTrayReady 在托盘初始化完成后构建菜单并进入消息循环。
func (a *App) onTrayReady() {
	redirectSystrayLog()

	mOpen := systray.AddMenuItem("打开面板", "显示设置窗口")
	systray.AddSeparator()
	menuAutostart = systray.AddMenuItemCheckbox("开机启动", "开机自动运行 CapsLayer", autostart.Enabled())
	menuToggle = systray.AddMenuItemCheckbox("改键", "启用 / 暂停改键引擎", a.EngineEnabled())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出 CapsLayer")

	trayReady.Store(true)
	a.updateTray()

	// 左键单击托盘图标即可切换引擎开关。各处理函数在自己的 goroutine 中运行，
	// 以免文件/进程操作阻塞托盘消息循环。
	systray.SetOnTapped(func() { go a.ToggleEnabled() })

	for {
		select {
		case <-mOpen.ClickedCh:
			go a.showWindow()
		case <-menuAutostart.ClickedCh:
			go a.SetAutostart(!autostart.Enabled())
		case <-menuToggle.ClickedCh:
			go a.ToggleEnabled()
		case <-mQuit.ClickedCh:
			go func() {
				a.setQuitting(true) // 放行 OnBeforeClose，允许真正退出
				systray.Quit()
				runtime.Quit(a.ctx)
			}()
			return
		}
	}
}

// updateTray 让托盘图标、提示文字与复选框状态与应用状态保持同步。
func (a *App) updateTray() {
	if !trayReady.Load() {
		return
	}
	running := a.eng.Running()
	systray.SetIcon(icon.TrayIcon(running))
	if running {
		systray.SetTooltip("CapsLayer · 运行中")
	} else {
		systray.SetTooltip("CapsLayer · 已暂停")
	}
	if menuToggle != nil {
		if a.EngineEnabled() {
			menuToggle.Check()
		} else {
			menuToggle.Uncheck()
		}
	}
	if menuAutostart != nil {
		if autostart.Enabled() {
			menuAutostart.Check()
		} else {
			menuAutostart.Uncheck()
		}
	}
}
