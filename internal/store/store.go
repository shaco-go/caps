package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"caps/internal/model"
)

// Dir 返回应用数据目录（%AppData%\CapsLayer）。
func Dir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "CapsLayer")
	}
	return filepath.Join(os.Getenv("APPDATA"), "CapsLayer")
}

// path 返回配置文件完整路径。
func path() string { return filepath.Join(Dir(), "config.json") }

// Exists 判断配置文件是否已存在（用于识别首次运行）。
func Exists() bool {
	_, err := os.Stat(path())
	return err == nil
}

// Load 读取配置，失败时回退到默认配置。
func Load() (model.Config, error) {
	b, err := os.ReadFile(path())
	if err != nil {
		if os.IsNotExist(err) {
			return model.Default(), nil
		}
		return model.Config{}, err
	}
	var c model.Config
	if err := json.Unmarshal(b, &c); err != nil {
		// 配置损坏时静默回退到默认值，避免应用无法启动。
		return model.Default(), nil
	}
	c.Normalize()
	return c, nil
}

// Save 将配置写入磁盘。
func Save(c model.Config) error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), b, 0o644)
}
