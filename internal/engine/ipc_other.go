//go:build !windows

package engine

import "errors"

// sendIPC 在非 Windows 平台不可用，仅用于满足跨平台编译。
func sendIPC(windowTitle string, payload []byte) error {
	return errors.New("IPC is only supported on Windows")
}
