package main

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"caps/internal/autostart"
	"caps/internal/engine"
	"caps/internal/model"
	"caps/internal/store"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// State 是推送给前端 UI 的引擎/开关状态。
type State struct {
	Enabled   bool `json:"enabled"`   // 主开关是否开启（配置层面）
	Running   bool `json:"running"`   // AutoHotkey 引擎进程是否存活
	Autostart bool `json:"autostart"` // 是否已设置开机自启
}

// App 是绑定到 Wails 的应用主结构体，其导出方法可被前端调用。
type App struct {
	ctx       context.Context
	mu        sync.Mutex     // 保护下方字段的并发访问
	eng       *engine.Engine // 改键引擎（内置 AutoHotkey 进程）
	cfg       model.Config   // 当前配置的内存副本
	quitting  bool           // 是否正在退出（用于区分“隐藏”与“退出”）
	restoring atomic.Bool    // 是否正在恢复窗口（避免误当作最小化而隐藏）
}

// NewApp 创建一个新的 App 实例。
func NewApp() *App {
	a := &App{eng: engine.New(store.Dir())}
	a.eng.SetOnState(a.onEngineState)
	return a
}

// onEngineState 在引擎进程启动或停止时被回调。
func (a *App) onEngineState(bool) {
	a.emitState()
	a.updateTray()
}

// GetState 返回当前启用/运行/自启状态，供 UI 查询。
func (a *App) GetState() State {
	a.mu.Lock()
	enabled := a.cfg.Settings.Enabled
	a.mu.Unlock()
	return State{Enabled: enabled, Running: a.eng.Running(), Autostart: autostart.Enabled()}
}

// emitState 把当前状态通过事件推送给前端。
func (a *App) emitState() {
	a.mu.Lock()
	ctx := a.ctx
	enabled := a.cfg.Settings.Enabled
	a.mu.Unlock()
	if ctx == nil {
		return
	}
	runtime.EventsEmit(ctx, "engine:state", State{
		Enabled:   enabled,
		Running:   a.eng.Running(),
		Autostart: autostart.Enabled(),
	})
}

// startup 在应用启动时加载配置，并在启用状态下拉起引擎。
func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
	firstRun := !store.Exists() // 是否首次运行（尚无配置文件）
	cfg, err := store.Load()
	if err != nil {
		cfg = model.Default()
	}
	a.cfg = cfg
	_ = store.Save(cfg)
	if cfg.Settings.Enabled {
		_ = a.eng.Start(cfg)
	}
	go a.runTray()     // 托盘图标消息循环
	go a.watchWindow() // 监听窗口最小化事件
	a.emitState()
	if firstRun {
		// 首次启动时把窗口显示出来，方便用户完成配置。
		time.AfterFunc(300*time.Millisecond, a.showWindow)
	}
}

// showWindow 恢复窗口并将其置顶显示到前台。
func (a *App) showWindow() {
	// 标记“正在恢复”，避免 watchWindow 把这次显示误判为最小化而立即隐藏。
	a.restoring.Store(true)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	// 先置顶再取消置顶，用于把窗口强拉到前台。
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
	time.AfterFunc(700*time.Millisecond, func() { a.restoring.Store(false) })
}

// watchWindow 在用户最小化窗口时将其隐藏到托盘。
func (a *App) watchWindow() {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	prevMin := false
	for range ticker.C {
		if a.isQuitting() {
			return
		}
		min := runtime.WindowIsMinimised(a.ctx)
		// 仅在“从非最小化变为最小化”的瞬间隐藏，避免重复触发。
		if min && !prevMin && !a.restoring.Load() {
			runtime.WindowHide(a.ctx)
		}
		prevMin = min
	}
}

// shutdown 在应用退出时停止引擎。
func (a *App) shutdown(ctx context.Context) {
	a.eng.Stop()
}

// GetConfig 返回当前完整配置。
func (a *App) GetConfig() model.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// SaveConfig 持久化配置，并在可能时热更新到正在运行的引擎。
func (a *App) SaveConfig(cfg model.Config) error {
	cfg.Normalize() // 清洗非法/重复的映射
	if err := store.Save(cfg); err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	err := a.apply(cfg)
	a.emitState()
	return err
}

// apply 让引擎与 cfg 保持一致：能热更新则走 IPC，否则整体重启。
func (a *App) apply(cfg model.Config) error {
	if !cfg.Settings.Enabled {
		a.eng.Stop()
		return nil
	}
	if a.eng.Running() {
		if err := a.eng.Update(cfg); err == nil {
			return nil
		}
	}
	return a.eng.Start(cfg)
}

// SetEnabled 启停引擎主开关，但不会改动映射。
func (a *App) SetEnabled(enabled bool) error {
	a.mu.Lock()
	a.cfg.Settings.Enabled = enabled
	cfg := a.cfg
	a.mu.Unlock()
	if err := store.Save(cfg); err != nil {
		return err
	}
	var err error
	if enabled {
		err = a.eng.Start(cfg)
	} else {
		a.eng.Stop()
	}
	a.emitState()
	a.updateTray()
	return err
}

// EngineRunning 返回 AutoHotkey 引擎进程是否在运行。
func (a *App) EngineRunning() bool {
	return a.eng.Running()
}

// PreviewScript 返回生成的 AutoHotkey 脚本内容（用于调试）。
func (a *App) PreviewScript() (string, error) {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	return engine.Generate(cfg)
}

// GetAutostart 返回是否已设置随 Windows 启动。
func (a *App) GetAutostart() bool {
	return autostart.Enabled()
}

// SetAutostart 开启或关闭随用户登录自启。
func (a *App) SetAutostart(on bool) error {
	var err error
	if !on {
		err = autostart.Disable()
	} else {
		var exe string
		exe, err = os.Executable() // 以当前可执行文件路径注册
		if err == nil {
			err = autostart.Enable(exe)
		}
	}
	a.emitState()
	a.updateTray()
	return err
}

// --- 系统托盘使用的辅助方法 ---

// ToggleEnabled 翻转主开关并返回新状态。
func (a *App) ToggleEnabled() bool {
	a.mu.Lock()
	enabled := !a.cfg.Settings.Enabled
	a.mu.Unlock()
	_ = a.SetEnabled(enabled)
	return enabled
}

// EngineEnabled 返回配置层面的主开关状态。
func (a *App) EngineEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Settings.Enabled
}

// isQuitting 返回是否正在退出。
func (a *App) isQuitting() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.quitting
}

// setQuitting 设置退出标志。
func (a *App) setQuitting(v bool) {
	a.mu.Lock()
	a.quitting = v
	a.mu.Unlock()
}
