#Requires AutoHotkey v2.0
#SingleInstance Force
#NoTrayIcon
Persistent
SendMode "Input"
SetWorkingDir A_ScriptDir

; ---- 超级全局状态（super-global）----
global LayerActive := false      ; Caps 层是否处于激活（按住）状态
global CapsDownAt := 0           ; Caps 按下的时间戳，用于判断短按/长按
global CapsUsed := false         ; 本次按住期间是否触发过改键
global LongPressMs := 200        ; 长按判定阈值（毫秒），由宿主下发
global ShortPressEnabled := true ; 短按是否触发原生 Caps Lock
global ActiveHotkeys := []       ; 当前已注册的热键列表，便于重新加载

; 上下文函数：只有当 Caps 被按住时，层内热键才生效。
LayerCheck(ThisHotkey) {
    global LayerActive
    return LayerActive
}

; Caps 按下：立即进入层（无延迟），并防止自动重复触发。
LayerDown() {
    global LayerActive, CapsDownAt, CapsUsed
    if LayerActive
        return
    LayerActive := true
    CapsUsed := false
    CapsDownAt := A_TickCount
}

; Caps 松开：退出层；若为短按且未使用过改键，则触发原生 Caps Lock 切换。
LayerUp() {
    global LayerActive, CapsDownAt, CapsUsed, LongPressMs, ShortPressEnabled
    LayerActive := false
    if (ShortPressEnabled && (A_TickCount - CapsDownAt < LongPressMs) && !CapsUsed) {
        if GetKeyState("CapsLock", "T")
            SetCapsLockState "AlwaysOff"
        else
            SetCapsLockState "AlwaysOn"
    }
}

; 通用动作分发器，绑定到每个层内热键。
RunAction(mode, value, *) {
    global CapsUsed
    CapsUsed := true
    if (mode = "text")
        SendText value
    else
        Send "{Blind}" value
}

; 解码宿主下发的 payload：以换行分隔行、以制表符分隔字段，反斜杠转义。
Unescape(s) {
    out := ""
    i := 1
    n := StrLen(s)
    while (i <= n) {
        c := SubStr(s, i, 1)
        if (c = "\") {
            i += 1
            d := SubStr(s, i, 1)
            if (d = "n")
                out .= "`n"
            else if (d = "r")
                out .= "`r"
            else if (d = "t")
                out .= "`t"
            else
                out .= d
        } else
            out .= c
        i += 1
    }
    return out
}

; 解析并应用 payload：先注销旧热键，再按行重建设置与层内热键。
ApplyPayload(payload) {
    global ActiveHotkeys, LongPressMs, ShortPressEnabled
    HotIf LayerCheck
    for hk in ActiveHotkeys {
        try Hotkey hk, "Off" ; 关闭失败（已不存在）时忽略
    }
    ActiveHotkeys := []
    for line in StrSplit(payload, "`n") {
        if (line = "")
            continue
        parts := StrSplit(line, "`t")
        if (parts.Length < 2)
            continue
        if (parts[1] = "S") {
            ; S 行：全局设置（阈值、短按开关）
            LongPressMs := Integer(parts[2])
            ShortPressEnabled := (parts[3] = "1")
        } else if (parts[1] = "M" && parts.Length >= 4) {
            ; M 行：源按键 + 模式 + 动作值
            source := Unescape(parts[2])
            mode := parts[3]
            value := Unescape(parts[4])
            try {
                Hotkey "*" source, RunAction.Bind(mode, value)
                ActiveHotkeys.Push("*" source)
            }
        }
    }
    HotIf
}

; 接收宿主通过 WM_COPYDATA 发来的 payload 并应用。
OnCopyData(wParam, lParam, msg, hwnd) {
    cbData := NumGet(lParam, A_PtrSize, "UInt")
    lpData := NumGet(lParam, A_PtrSize * 2, "Ptr")
    if (cbData = 0)
        return true
    payload := StrGet(lpData, cbData, "UTF-8")
    ApplyPayload(payload)
    return true
}

; ---- 宿主在生成脚本时内联的初始配置 ----
ApplyPayload({{PAYLOAD}})

; ---- 宿主 IPC 窗口 + 父进程守护 ----
if (A_Args.Length >= 1) {
    ; 创建一个隐藏窗口，标题含宿主 PID，供 Go 端 FindWindowW 定位。
    ipcGui := Gui("+ToolWindow -Caption", "CapsLayerIPC_" A_Args[1])
    ipcGui.Show("Hide")
    OnMessage(0x4A, OnCopyData)
    SetTimer WatchParent, 3000 ; 每 3 秒检查宿主是否还存活
}

; 宿主退出后自动结束本脚本，避免残留进程。
WatchParent() {
    if !ProcessExist(A_Args[1])
        ExitApp()
}

; 始终捕获 CapsLock 的按下/抬起（* 表示忽略修饰符状态）。
*CapsLock:: LayerDown()
*CapsLock up:: LayerUp()
