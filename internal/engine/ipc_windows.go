//go:build windows

package engine

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procFindWindowW  = user32.NewProc("FindWindowW")
	procSendMessageW = user32.NewProc("SendMessageW")
)

// WM_COPYDATA 消息号，用于跨进程传递数据块。
const wmCopyData = 0x004A

// copyDataStruct 对应原生结构体 COPYDATASTRUCT。
// 字段顺序与对齐方式在 386 与 amd64 下均符合 Windows ABI。
type copyDataStruct struct {
	dwData uintptr
	cbData uint32
	lpData uintptr
}

// sendIPC 将 payload 投递到标题为 windowTitle 的 AutoHotkey 窗口。
func sendIPC(windowTitle string, payload []byte) error {
	titlePtr, err := syscall.UTF16PtrFromString(windowTitle)
	if err != nil {
		return err
	}
	// 通过窗口标题找到目标窗口句柄。
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return fmt.Errorf("engine window %q not found", windowTitle)
	}
	if len(payload) == 0 {
		payload = []byte{0}
	}
	cds := copyDataStruct{
		dwData: 1,
		cbData: uint32(len(payload)),
		lpData: uintptr(unsafe.Pointer(&payload[0])),
	}
	procSendMessageW.Call(hwnd, wmCopyData, 0, uintptr(unsafe.Pointer(&cds)))
	return nil
}
