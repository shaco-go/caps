# CapsLayer

CapsLayer 是一个 Windows 平台的**改键工具**：把 `Caps Lock` 当作“层”键使用——
**按住** `Caps Lock` 时进入改键层（触发你配置的映射），**短按**时仍保留原生的大小写切换功能。

界面基于 [Wails](https://wails.io/)（Go + 原生 WebView），改键引擎内嵌
[AutoHotkey v2](https://www.autohotkey.com/)，无需单独安装，解压即用。

## 功能特性

- **Caps 层改键**：按住 `Caps Lock` + 任意键触发映射，短按仍是大小写切换。
- **三种动作类型**：
  - `单键`：映射到某个按键（如 `h` → `←`）。
  - `组合键`：映射到带 `Ctrl/Alt/Shift/Win` 的组合（如 `j` → `Ctrl+C`）。
  - `文本`：直接输出一段文本。
- **可视化按键录制**：点击“录制”后按实际按键即可，无需手写键名。
- **长按阈值可调**：自定义长按/短按的判定时间，默认 200ms。
- **热更新**：保存后通过 `WM_COPYDATA` 把新配置即时下发给引擎，无需重启。
- **引擎守护**：AutoHotkey 进程意外退出时自动重启（带次数上限）。
- **托盘常驻**：关闭窗口隐藏到托盘，左键单击托盘图标快速启停改键。
- **开机自启**：一键写入注册表 `HKCU\...\Run`，无需管理员权限。
- **单实例运行**：重复启动会唤醒已有窗口，而不是再开一个进程。

## 工作原理

```
Wails 前端 (JS)  ──绑定方法──▶  App (Go)  ──tray/窗口──▶  系统托盘
                                   │
                                   │ 生成脚本 + 启动
                                   ▼
                     AutoHotkey64.exe (内嵌) ── 实际执行改键
                                   ▲
                                   │ WM_COPYDATA (IPC 热更新)
                                   │
                              Engine (Go)
```

- Go 端把配置编码为按行文本 payload，渲染进内嵌的 `template.ahk`，写入
  `%AppData%\CapsLayer\runtime.ahk` 并启动内嵌的 `AutoHotkey64.exe`。
- 保存配置时优先通过 `WM_COPYDATA` 热更新；失败则整体重启引擎。
- AHK 脚本按 `A_Args[1]`（宿主 PID）创建隐藏的 IPC 窗口，并定时检查宿主是否存活，
  宿主退出后自动结束，避免残留进程。

## 目录结构

```
.
├── main.go                 # Wails 启动入口与窗口选项
├── app.go                  # 应用主结构体与前后端绑定的方法
├── tray.go                 # 系统托盘菜单与图标
├── internal/
│   ├── autostart/          # 开机自启（注册表读写）
│   ├── engine/             # 改键引擎：脚本生成、进程守护、IPC
│   │   ├── assets/         # 内嵌的 AutoHotkey 解释器与脚本模板
│   │   ├── engine.go       # 引擎生命周期与配置编码
│   │   ├── escape.go       # AHK 字符串/按键转义
│   │   └── ipc_windows.go  # Windows 下的 WM_COPYDATA 实现
│   ├── icon/               # 内嵌托盘图标
│   ├── model/              # 配置数据模型与默认值
│   └── store/              # 配置持久化
├── frontend/               # 前端（原生 JS + Vite）
│   ├── index.html
│   └── src/
│       ├── main.js         # 界面渲染、事件绑定、录制逻辑
│       ├── keymap.js       # 浏览器按键 → AHK 键名映射
│       └── style.css       # 深色主题样式
└── build/                  # 打包资源（图标、安装器、平台配置）
```

## 开发环境

- Windows 10/11
- [Go](https://go.dev/) 1.25+
- [Node.js](https://nodejs.org/)（构建前端）
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2：
  `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- 首次运行前端需安装依赖：`cd frontend && npm install`

## 开发与构建

```bash
# 实时开发（前端热重载，附带浏览器调试服务）
wails dev

# 构建生产版本，产物位于 build/bin/
wails build

# 仅运行 Go 单元测试
go test ./...
```

> 提示：涉及 AutoHotkey 进程与 IPC 的测试需要 Windows 环境，且会短暂启动真实进程。

## 使用说明

1. 启动 `caps.exe`，首次运行会自动弹出设置窗口。
2. 在面板中添加映射：点击“录制”设置**源按键**，选择“方式”，再录制/填写**目标**。
3. 点击“保存映射”即时生效：
   - 顶部的**启用开关**与**开机自启**立即生效，无需保存；
   - **长按阈值与映射**需要点击“保存映射”。
4. 关闭窗口后程序隐藏到托盘：
   - **左键单击**托盘图标：快速启用/暂停改键；
   - 右键菜单：打开面板 / 开机启动 / 改键开关 / 退出。
5. 正常使用时：**按住** `Caps Lock` 进入改键层，**短按** `Caps Lock` 切换大小写。

## 数据与文件位置

| 路径 | 说明 |
| --- | --- |
| `%AppData%\CapsLayer\config.json` | 应用配置（映射与设置） |
| `%AppData%\CapsLayer\AutoHotkey64.exe` | 运行时释放的解释器 |
| `%AppData%\CapsLayer\runtime.ahk` | 生成的改键脚本 |
| `%AppData%\CapsLayer\systray.log` | 托盘错误日志 |

## 图标

图标为静态资源，需直接替换以下文件：

| 文件 | 用途 |
| --- | --- |
| `internal/icon/tray-on.ico`、`tray-off.ico` | 托盘图标（运行中 / 暂停） |
| `build/windows/icon.ico` | exe / 窗口 / 任务栏 / 安装器图标 |
| `build/appicon.png` | `icon.ico` 缺失时的回退，及 macOS 图标来源 |

## 许可证

本项目内嵌了 AutoHotkey v2，其授权条款见
[`internal/engine/assets/LICENSE-AutoHotkey.txt`](internal/engine/assets/LICENSE-AutoHotkey.txt)。
