package autostart

import (
	"golang.org/x/sys/windows/registry"
)

const (
	// 当前用户下的“运行”注册表项（无需管理员权限）。
	runKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName = "CapsLayer"
)

// Enable 将指定可执行文件注册为登录时自动运行。
func Enable(exePath string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	// 用引号包裹路径，避免路径中含空格时被错误解析。
	return k.SetStringValue(valueName, `"`+exePath+`"`)
}

// Disable 删除登录自启注册表项。
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return nil // 项不存在，视为已关闭
	}
	defer k.Close()
	_ = k.DeleteValue(valueName)
	return nil
}

// Enabled 报告登录自启项是否存在。
func Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(valueName)
	return err == nil
}
