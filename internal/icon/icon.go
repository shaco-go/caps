// Package icon 提供内嵌的 CapsLayer 托盘图标。
package icon

import _ "embed"

//go:embed tray-on.ico
var trayOn []byte // 引擎运行中的图标

//go:embed tray-off.ico
var trayOff []byte // 引擎暂停时的图标

// TrayIcon 根据运行状态返回对应的托盘图标数据。
func TrayIcon(running bool) []byte {
	if running {
		return trayOn
	}
	return trayOff
}
