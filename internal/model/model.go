package model

// Mapping 描述一条映射：在按住 Caps 层时按下 Source 键，触发对应动作。
type Mapping struct {
	Source string `json:"source"` // 源按键的 AHK 键名，例如 "h"
	Mode   string `json:"mode"`   // 动作类型："key"（单键）| "combo"（组合键）| "text"（文本）
	Key    string `json:"key"`    // key/combo 模式的目标主键，例如 "Left"、"c"
	Ctrl   bool   `json:"ctrl"`   // 以下四个为组合键修饰符
	Alt    bool   `json:"alt"`
	Shift  bool   `json:"shift"`
	Win    bool   `json:"win"`
	Text   string `json:"text"` // text 模式要输出的文本
}

// Settings 保存引擎的全局选项。
type Settings struct {
	Enabled    bool `json:"enabled"`    // 主开关
	Threshold  int  `json:"threshold"`  // 长按判定阈值（毫秒）
	ShortPress bool `json:"shortPress"` // 短按是否触发原生 Caps Lock 大小写切换
}

// Config 是持久化到磁盘的应用配置。
type Config struct {
	Settings Settings  `json:"settings"`
	Mappings []Mapping `json:"mappings"`
}

// Default 返回带若干示例映射的初始配置。
func Default() Config {
	return Config{
		Settings: Settings{
			Enabled:    true,
			Threshold:  200,
			ShortPress: true,
		},
		Mappings: []Mapping{
			{Source: "h", Mode: "key", Key: "Left"},
			{Source: "j", Mode: "key", Key: "Down"},
			{Source: "k", Mode: "key", Key: "Up"},
			{Source: "l", Mode: "key", Key: "Right"},
			{Source: "u", Mode: "key", Key: "Home"},
			{Source: "i", Mode: "key", Key: "End"},
		},
	}
}

// Normalize 补齐合理默认值并丢弃非法/重复的映射条目。
func (c *Config) Normalize() {
	if c.Settings.Threshold <= 0 {
		c.Settings.Threshold = 200
	}
	seen := map[string]bool{} // 用于检测重复的源按键
	out := c.Mappings[:0]     // 复用底层数组做原地过滤
	for _, m := range c.Mappings {
		// 跳过空源键、CapsLock 本身以及重复源键。
		if m.Source == "" || m.Source == "CapsLock" || seen[m.Source] {
			continue
		}
		switch m.Mode {
		case "key", "combo":
			if m.Key == "" {
				continue
			}
		case "text":
			if m.Text == "" {
				continue
			}
		default:
			continue // 未知模式直接丢弃
		}
		seen[m.Source] = true
		out = append(out, m)
	}
	c.Mappings = out
}
