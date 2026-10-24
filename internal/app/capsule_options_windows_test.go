package app

import (
	"os"
	"path/filepath"
	"testing"
)

// 下拉按名字认选项：重名的要能分开选，manifest 坏了的照样列出来，配置里选着、文件夹没了的也有一项
func TestCapsuleOptions(t *testing.T) {
	dir := t.TempDir()
	for folder, manifest := range map[string]string{
		"my-style":   `{"name":"My Style"}`,
		"my-style-2": `{"name":"My Style"}`,
		"copycat":    `{"name":"Bars"}`,
		"broken":     `{"name":`,
	} {
		p := filepath.Join(dir, "styles", folder)
		os.MkdirAll(p, 0o755)
		os.WriteFile(filepath.Join(p, "style.js"), []byte("function draw(g, s) {}"), 0o644)
		os.WriteFile(filepath.Join(p, "manifest.json"), []byte(manifest), 0o644)
	}
	os.MkdirAll(filepath.Join(dir, "styles", "no-script"), 0o755) // 没有 style.js：不是样式
	a := &App{cfgPath: filepath.Join(dir, "config.json")}
	opts := a.capsuleOptions([]option{{"bars", "Bars"}}, "custom:gone")
	labels := map[string]string{}
	for _, o := range opts {
		if prev, ok := labels[o.label]; ok {
			t.Fatalf("label %q is used by %s and %s", o.label, prev, o.value)
		}
		labels[o.label] = o.value
	}
	want := map[string]string{
		"Bars": "bars", "Bars · copycat": "custom:copycat", "My Style": "custom:my-style", "My Style · my-style-2": "custom:my-style-2",
		"broken": "custom:broken", "gone": "custom:gone",
	}
	if len(labels) != len(want) {
		t.Fatalf("options %+v", opts)
	}
	for label, value := range want {
		if labels[label] != value {
			t.Fatalf("%q = %q, want %q (options %+v)", label, labels[label], value, opts)
		}
	}
}
