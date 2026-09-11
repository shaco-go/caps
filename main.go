package main

import (
	"context"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// 将打包后的前端静态资源（frontend/dist）嵌入二进制，运行时无需外部文件。
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 应用核心对象，其导出方法会通过 Wails 绑定给前端调用。
	app := NewApp()

	err := wails.Run(&options.App{
		Title:             "CapsLayer",
		Width:             1000,
		Height:            700,
		MinWidth:          860,
		MinHeight:         560,
		StartHidden:       true, // 启动时隐藏窗口，仅显示托盘图标
		HideWindowOnClose: true, // 点关闭按钮时隐藏而非退出
		// 单实例锁：重复启动时唤醒已有实例的窗口，而不是再开一个进程。
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "CapsLayer-SingleInstance",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				app.showWindow()
			},
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 20, B: 26, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose: func(ctx context.Context) bool {
			// 返回 true 会阻止关闭；只有用户明确选择“退出”时才允许真正退出。
			return !app.isQuitting()
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
