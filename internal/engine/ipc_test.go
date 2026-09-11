//go:build windows

package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"caps/internal/model"
)

// probeScript 是一个最小 AHK 探针脚本：创建指定标题的窗口接收 WM_COPYDATA，
// 并把收到的内容写入 outFile，用于验证 IPC 链路。
const probeScript = `#Requires AutoHotkey v2.0
#SingleInstance Force
Persistent
outFile := A_Args[2]
OnCopyData(wParam, lParam, msg, hwnd) {
    cbData := NumGet(lParam, A_PtrSize, "UInt")
    lpData := NumGet(lParam, A_PtrSize * 2, "Ptr")
    payload := StrGet(lpData, cbData, "UTF-8")
    FileAppend payload, outFile
    return true
}
ipcGui := Gui("+ToolWindow -Caption", A_Args[1])
ipcGui.Show("Hide")
OnMessage(0x4A, OnCopyData)
SetTimer(() => ExitApp(), -8000)
`

// TestSendIPC 验证能通过 WM_COPYDATA 把 payload 送达目标窗口。
func TestSendIPC(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "caps_ipc_test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	e := New(dir)
	if err := e.ensureAssets(); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "probe.ahk")
	if err := os.WriteFile(script, []byte("\ufeff"+probeScript), 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "ipc.log")
	_ = os.Remove(logPath)

	title := "CapsLayerIPC_PROBE"
	cmd := exec.Command(e.exePath(), script, title, logPath)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	payload := "S\t200\t1\nM\th\tkey\t{Left}\n"
	deadline := time.Now().Add(5 * time.Second)
	sent := false
	for time.Now().Before(deadline) {
		if err := sendIPC(title, []byte(payload)); err == nil {
			sent = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !sent {
		t.Fatal("could not find IPC window in time")
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(logPath); err == nil && string(b) == payload {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, _ := os.ReadFile(logPath)
	t.Fatalf("payload mismatch: got %q want %q", string(b), payload)
}

// TestEngineStartUpdate 验证引擎能启动并热更新配置。
func TestEngineStartUpdate(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "caps_engine_test")
	_ = os.RemoveAll(dir)
	e := New(dir)
	cfg := model.Default()
	if err := e.Start(cfg); err != nil {
		t.Fatal(err)
	}
	defer e.Stop()

	cfg2 := cfg
	cfg2.Mappings = append([]model.Mapping{}, cfg.Mappings...)
	cfg2.Mappings[0].Key = "Right"

	deadline := time.Now().Add(8 * time.Second)
	var err error
	for time.Now().Before(deadline) {
		if err = e.Update(cfg2); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("live update failed: %v", err)
	}
	if !e.Running() {
		t.Fatal("engine unexpectedly stopped")
	}
}

// currentPid 返回当前引擎进程 PID（无进程时返回 0）。
func currentPid(e *Engine) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cmd == nil || e.cmd.Process == nil {
		return 0
	}
	return e.cmd.Process.Pid
}

// waitRunning 在给定时间内轮询等待引擎进入运行状态。
func waitRunning(e *Engine, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if e.Running() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// TestEngineWatchdogRestart 验证进程被杀死后守护逻辑会自动重启。
func TestEngineWatchdogRestart(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "caps_watchdog_test")
	_ = os.RemoveAll(dir)
	e := New(dir)
	if err := e.Start(model.Default()); err != nil {
		t.Fatal(err)
	}
	defer e.Stop()

	if !waitRunning(e, 5*time.Second) {
		t.Fatal("engine not running after start")
	}
	first := currentPid(e)

	e.mu.Lock()
	proc := e.cmd.Process
	e.mu.Unlock()
	if err := proc.Kill(); err != nil {
		t.Fatalf("failed to kill engine: %v", err)
	}

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		pid := currentPid(e)
		if e.Running() && pid != 0 && pid != first {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("engine was not restarted (first=%d now=%d running=%v)", first, currentPid(e), e.Running())
}

// TestEngineStopDisablesWatchdog 验证调用 Stop 后不会自动重启。
func TestEngineStopDisablesWatchdog(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "caps_watchdog_stop")
	_ = os.RemoveAll(dir)
	e := New(dir)
	if err := e.Start(model.Default()); err != nil {
		t.Fatal(err)
	}
	if !waitRunning(e, 5*time.Second) {
		t.Fatal("engine not running after start")
	}
	e.Stop()
	time.Sleep(2 * time.Second)
	if e.Running() {
		t.Fatal("engine restarted after Stop")
	}
}
