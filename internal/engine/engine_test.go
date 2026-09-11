package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"caps/internal/model"
)

// sample 构造一份测试用配置，覆盖 key/combo/text 三种模式。
func sample() model.Config {
	return model.Config{
		Settings: model.Settings{Enabled: true, Threshold: 250, ShortPress: true},
		Mappings: []model.Mapping{
			{Source: "h", Mode: "key", Key: "Left"},
			{Source: "j", Mode: "combo", Key: "c", Ctrl: true, Shift: true},
			{Source: "t", Mode: "text", Text: `he said "hi"` + "\n" + "line2"},
		},
	}
}

// TestEncodePayload 验证配置被编码为预期的按行 payload。
func TestEncodePayload(t *testing.T) {
	got := EncodePayload(sample())
	want := "S\t250\t1\n" +
		"M\th\tkey\t{Left}\n" +
		"M\tj\tkey\t^+c\n" +
		"M\tt\ttext\the said \"hi\"\\nline2\n"
	if got != want {
		t.Errorf("EncodePayload mismatch\n got: %q\nwant: %q", got, want)
	}
}

// TestGenerate 验证生成的脚本包含关键函数与转义后的映射内容。
func TestGenerate(t *testing.T) {
	s, err := Generate(sample())
	if err != nil {
		t.Fatal(err)
	}
	tab, nl := "`t", "`n"
	for _, want := range []string{
		"ApplyPayload(",
		"RunAction(",
		"OnCopyData(",
		`"CapsLayerIPC_" A_Args[1]`,
		"S" + tab + "250" + tab + "1" + nl,
		"M" + tab + "h" + tab + "key" + tab + "{Left}",
		"M" + tab + "j" + tab + "key" + tab + "^+c",
		`he said ""hi""\nline2`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("generated script missing %q", want)
		}
	}
}

// TestGenerateEmpty 验证空映射配置不会生成任何 M 行。
func TestGenerateEmpty(t *testing.T) {
	s, err := Generate(model.Config{Settings: model.Settings{Threshold: 200}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "ApplyPayload(") {
		t.Errorf("expected ApplyPayload call")
	}
	if strings.Contains(s, "\tM\t") {
		t.Errorf("unexpected mapping line")
	}
}

// TestWriteAndDump 验证资源释放与脚本写盘流程。
func TestWriteAndDump(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "caps_gen")
	e := New(dir)
	if err := e.ensureAssets(); err != nil {
		t.Fatal(err)
	}
	path, err := e.writeScript(sample())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}
