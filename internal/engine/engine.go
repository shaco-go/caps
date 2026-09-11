package engine

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"caps/internal/model"
)

//go:embed assets/AutoHotkey64.exe
var ahkExe []byte // 内嵌的 AutoHotkey v2 解释器

//go:embed assets/template.ahk
var scriptTemplate string // 内嵌的脚本模板，{{PAYLOAD}} 会被替换为实际配置

const (
	exeName    = "AutoHotkey64.exe" // 释放到运行目录的解释器文件名
	scriptName = "runtime.ahk"      // 生成的脚本文件名

	restartDelay = 700 * time.Millisecond // 崩溃后重启前的等待时间
	healthyAfter = 5 * time.Second        // 运行超过该时长视为稳定，重置重启计数
	maxRestarts  = 10                     // 连续崩溃重启次数上限
)

// Engine 管理内嵌的 AutoHotkey 进程、生成的脚本，以及一个在进程意外退出时
// 负责重启的守护逻辑。
type Engine struct {
	mu          sync.Mutex
	dir         string         // 运行文件（exe/脚本）所在目录
	cmd         *exec.Cmd      // 当前 AHK 进程
	running     bool           // 进程是否存活
	desired     bool           // 期望运行状态（用于决定是否重启）
	cfg         model.Config   // 当前生效的配置
	consecutive int            // 连续重启计数
	onState     func(bool)     // 运行状态变化回调
}

// New 创建 Engine，运行文件存放在 dir。
func New(dir string) *Engine {
	return &Engine{dir: dir}
}

// SetOnState 注册运行状态变化时的回调。
func (e *Engine) SetOnState(fn func(bool)) {
	e.mu.Lock()
	e.onState = fn
	e.mu.Unlock()
}

// notify 在锁外调用回调，避免死锁。
func (e *Engine) notify(running bool) {
	e.mu.Lock()
	fn := e.onState
	e.mu.Unlock()
	if fn != nil {
		fn(running)
	}
}

func (e *Engine) exePath() string    { return filepath.Join(e.dir, exeName) }
func (e *Engine) scriptPath() string { return filepath.Join(e.dir, scriptName) }

// ensureAssets 确保运行目录存在，并释放（或更新）内嵌的 AutoHotkey 解释器。
func (e *Engine) ensureAssets() error {
	if err := os.MkdirAll(e.dir, 0o755); err != nil {
		return err
	}
	// 文件缺失或大小不一致时重新写入，实现版本升级后的自动替换。
	if fi, err := os.Stat(e.exePath()); err != nil || fi.Size() != int64(len(ahkExe)) {
		if err := os.WriteFile(e.exePath(), ahkExe, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// boolDigit 将布尔值转成 payload 中的 "1"/"0"。
func boolDigit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// escapePayload 转义 payload 字段中的特殊字符，避免破坏制表符/换行分隔。
func escapePayload(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		"\t", `\t`,
		"\n", `\n`,
		"\r", `\r`,
	).Replace(s)
}

// EncodePayload 把 cfg 渲染成 ApplyPayload 能识别的按行文本格式。
// 首行 S 表示全局设置，后续每行 M 表示一条映射。
func EncodePayload(cfg model.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "S\t%d\t%s\n", cfg.Settings.Threshold, boolDigit(cfg.Settings.ShortPress))
	for _, m := range cfg.Mappings {
		mode := "key"
		var value string
		if m.Mode == "text" {
			mode = "text"
			value = m.Text
		} else {
			value = sendSeq(m)
		}
		fmt.Fprintf(&b, "M\t%s\t%s\t%s\n", escapePayload(m.Source), mode, escapePayload(value))
	}
	return b.String()
}

// Generate 为 cfg 渲染完整的 AutoHotkey 脚本。
func Generate(cfg model.Config) (string, error) {
	s := strings.ReplaceAll(scriptTemplate, "{{PAYLOAD}}", ahkString(EncodePayload(cfg)))
	return s, nil
}

// ipcWindowTitle 生成用来定位引擎进程 IPC 窗口的标题（含当前进程 PID）。
func ipcWindowTitle() string {
	return fmt.Sprintf("CapsLayerIPC_%d", os.Getpid())
}

// writeScript 生成脚本并写入磁盘。
func (e *Engine) writeScript(cfg model.Config) (string, error) {
	src, err := Generate(cfg)
	if err != nil {
		return "", err
	}
	// AutoHotkey v2 推荐使用带 BOM 的 UTF-8。
	if err := os.WriteFile(e.scriptPath(), []byte("\ufeff"+src), 0o644); err != nil {
		return "", err
	}
	return e.scriptPath(), nil
}

// stopLocked 结束当前进程，调用者需持有 e.mu。
func (e *Engine) stopLocked() {
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
	}
	e.cmd = nil
	e.running = false
}

// launchLocked 启动 AutoHotkey 进程。调用者需持有 e.mu。
func (e *Engine) launchLocked() error {
	if err := e.ensureAssets(); err != nil {
		return err
	}
	path, err := e.writeScript(e.cfg)
	if err != nil {
		return err
	}
	e.stopLocked()
	// 第二个参数是脚本路径，第三个参数把宿主 PID 传给脚本用于存活探测。
	cmd := exec.Command(e.exePath(), path, strconv.Itoa(os.Getpid()))
	cmd.Dir = e.dir
	if err := cmd.Start(); err != nil {
		return err
	}
	e.cmd = cmd
	e.running = true
	go e.watch(cmd)
	return nil
}

// Start 启动引擎并标记为“期望运行”。
func (e *Engine) Start(cfg model.Config) error {
	e.mu.Lock()
	e.desired = true
	e.cfg = cfg
	e.consecutive = 0 // 手动启动时重置重启计数
	err := e.launchLocked()
	e.mu.Unlock()
	if err != nil {
		return err
	}
	e.notify(true)
	return nil
}

// watch 等待进程结束，并在仍期望运行时自动重启。
func (e *Engine) watch(cmd *exec.Cmd) {
	start := time.Now()
	_ = cmd.Wait()

	e.mu.Lock()
	if e.cmd != cmd {
		// 已被更新的进程取代，无需处理。
		e.mu.Unlock()
		return
	}
	e.cmd = nil
	e.running = false
	// 稳定运行足够久则清零连续重启计数。
	if time.Since(start) >= healthyAfter {
		e.consecutive = 0
	}
	desired := e.desired
	if !desired {
		e.mu.Unlock()
		e.notify(false)
		return
	}
	e.consecutive++
	over := e.consecutive > maxRestarts
	if over {
		// 反复崩溃，放弃自动重启，避免无限循环。
		e.desired = false
	}
	e.mu.Unlock()

	e.notify(false)
	if over {
		return
	}

	time.Sleep(restartDelay)

	e.mu.Lock()
	if !e.desired || e.running {
		e.mu.Unlock()
		return
	}
	err := e.launchLocked()
	e.mu.Unlock()
	if err != nil {
		e.notify(false)
		return
	}
	e.notify(true)
}

// Update 通过 IPC 把新的设置/映射热更新到正在运行的引擎。
func (e *Engine) Update(cfg model.Config) error {
	e.mu.Lock()
	e.cfg = cfg
	running := e.running
	e.mu.Unlock()
	if !running {
		return fmt.Errorf("engine not running")
	}
	payload := []byte(EncodePayload(cfg))
	title := ipcWindowTitle()
	// 窗口可能尚未就绪，短时间内重试若干次。
	var lastErr error
	for i := 0; i < 15; i++ {
		if lastErr = sendIPC(title, payload); lastErr == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return lastErr
}

// Stop 结束 AutoHotkey 进程并关闭守护重启。
func (e *Engine) Stop() {
	e.mu.Lock()
	e.desired = false
	e.stopLocked()
	e.mu.Unlock()
	e.notify(false)
}

// Running 报告 AutoHotkey 进程是否存活。
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}
