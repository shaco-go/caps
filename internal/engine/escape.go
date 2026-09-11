package engine

import (
	"strings"

	"caps/internal/model"
)

// ahkString 把 s 渲染为 AutoHotkey v2 的双引号字符串字面量。
func ahkString(s string) string {
	r := strings.NewReplacer(
		"`", "``",
		`"`, `""`,
		"\r\n", "`n",
		"\n", "`n",
		"\r", "`r",
		"\t", "`t",
	)
	return `"` + r.Replace(s) + `"`
}

// normalizeKey 把键名转换为 Send() 支持的写法：单个可打印字符原样输出，
// 其余键名（如 Left、F1）用大括号包裹。
func normalizeKey(key string) string {
	if key == "" {
		return ""
	}
	if len([]rune(key)) == 1 {
		switch key {
		case "{":
			return "{{}"
		case "}":
			return "{}}"
		case "+", "^", "!", "#": // AHK 中这些符号是修饰符前缀，需转义
			return "{" + key + "}"
		}
		return key
	}
	return "{" + key + "}"
}

// sendSeq 为 key/combo 映射构建 Send() 序列。
func sendSeq(m model.Mapping) string {
	var b strings.Builder
	if m.Mode == "combo" {
		// AHK 修饰符前缀：^=Ctrl !=Alt +=Shift #=Win
		if m.Ctrl {
			b.WriteString("^")
		}
		if m.Alt {
			b.WriteString("!")
		}
		if m.Shift {
			b.WriteString("+")
		}
		if m.Win {
			b.WriteString("#")
		}
	}
	b.WriteString(normalizeKey(m.Key))
	return b.String()
}
