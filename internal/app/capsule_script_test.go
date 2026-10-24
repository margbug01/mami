package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func writeStyle(t *testing.T, js, manifest string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "s")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "style.js"), []byte(js), 0o644)
	if manifest != "" {
		os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644)
	}
	return dir
}

func TestCapsuleScriptFrame(t *testing.T) {
	dir := writeStyle(t, `
var n = 0
function draw(g, s) {
  n++
  g.rect(0, 0, s.w, s.h, "#1d1d22", s.h / 2)
  g.circle(s.w / 2, s.h / 2, 10 + s.level * 8, g.gradient("#8fd8ff", "#8b4dff", 120))
  g.poly([0, 0, 10, 0, 5, 8], "rgba(255,0,0,0.5)")
  g.icon("x", 20, 20, 12, "#fff")
  if (s.kind === "recording") g.button(4, 4, 28, 28, "cancel")
  g.text(s.text || "hi", s.w / 2, s.h / 2, 12, "#fff", "center", 40)
}`, `{"name":"Test","width":200,"height":50}`)
	s := loadCapsuleStyle(dir)
	if s.err != nil {
		t.Fatal(s.err)
	}
	if s.man.Name != "Test" || s.man.Width != 200 || s.man.Height != 50 || s.man.Position != "bottom" {
		t.Fatalf("manifest %+v", s.man)
	}
	f, err := s.frame(capsuleScene{kind: "recording", level: 0.5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.ops) != 6 || f.ops[0].kind != 'r' || f.ops[0].w != 200 || f.ops[1].grad == nil || f.ops[2].color.A != 128 {
		t.Fatalf("ops %+v", f.ops)
	}
	if len(f.buttons) != 1 || f.buttons[0].action != "cancel" {
		t.Fatalf("buttons %+v", f.buttons)
	}
	if f, _ = s.frame(capsuleScene{kind: "text", text: strings.Repeat("长", 30)}, nil); len(f.buttons) != 0 || !strings.HasSuffix(f.ops[len(f.ops)-1].text, "…") {
		t.Fatalf("text frame %+v", f)
	}
}

func TestCapsuleScriptGuards(t *testing.T) {
	for name, tc := range map[string]struct {
		js       string
		timedOut bool // 超时：浮窗下次说话会重新加载再试；其余要改了文件
	}{
		"loop":    {"function draw(g, s) { for (;;) {} }", true},
		"syntax":  {"function draw(g, s) {", false},
		"no draw": {"var x = 1", false},
		"throw":   {"function draw(g, s) { throw new Error('boom') }", false},
		"console": {"function draw(g, s) { console.log('hi', s.kind); nope() }", false},
	} {
		t.Run(name, func(t *testing.T) {
			s := loadCapsuleStyle(writeStyle(t, tc.js, ""))
			start := time.Now()
			_, err := s.frame(capsuleScene{kind: "recording"}, nil)
			if err == nil {
				t.Fatal("want an error")
			}
			if d := time.Since(start); d > time.Second {
				t.Fatalf("took %v", d)
			}
			if s.timedOut != tc.timedOut {
				t.Fatalf("timedOut = %v (%v)", s.timedOut, err)
			}
			if name == "console" && !strings.Contains(err.Error(), "nope") {
				t.Fatalf("console.log should work, the error should be the later call: %v", err)
			}
			if _, err2 := s.frame(capsuleScene{}, nil); err2 == nil {
				t.Fatal("a failed style should stay failed until the file changes")
			}
		})
	}
	if s := loadCapsuleStyle(filepath.Join(t.TempDir(), "missing")); s.err == nil {
		t.Fatal("missing folder should fail")
	}
	// manifest.json 坏了：报错，但大小照样有默认值，设置页能画出错的样子
	s := loadCapsuleStyle(writeStyle(t, "function draw(g, s) {}", `{"name": "x", "width": "wide"}`))
	if s.err == nil || !strings.Contains(s.err.Error(), "manifest.json") || s.man.Width != 260 || s.man.Height != 110 {
		t.Fatalf("bad manifest: err %v, man %+v", s.err, s.man)
	}
}

// 每帧都吃掉一整帧时间的脚本：连着 capsuleSlowFrames 帧以后按超时算
func TestCapsuleScriptSlowFrames(t *testing.T) {
	s := loadCapsuleStyle(writeStyle(t, "function draw(g, s) { var t = Date.now(); while (Date.now() - t < 18) {} }", ""))
	for i := 1; i <= capsuleSlowFrames; i++ {
		_, err := s.frame(capsuleScene{kind: "recording"}, nil)
		if (err != nil) != (i == capsuleSlowFrames) {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	if !s.timedOut {
		t.Fatal("a slow style should count as timed out")
	}
}

// 几千字的句子也只量十来次（量字打断不了），截出来的是放得下的最长前缀
func TestCapsuleEllipsizeLongText(t *testing.T) {
	calls := 0
	width := func(text string, size float32) (float32, float32) {
		return float32(len([]rune(text))) * size * 0.6, size
	}
	s := &capsuleStyle{measure: func(text string, size float32) (float32, float32) { calls++; return width(text, size) }}
	long := []rune(strings.Repeat("长", 3000))
	got := s.ellipsize(string(long), 13, 300)
	if calls > 20 {
		t.Fatalf("%d measurements", calls)
	}
	n := len([]rune(got)) - 1
	if !strings.HasSuffix(got, "…") || string([]rune(got)[:n]) != string(long[:n]) {
		t.Fatalf("got %q", got)
	}
	if w, _ := width(got, 13); w > 300 {
		t.Fatalf("%q is %v wide", got, w)
	}
	if w, _ := width(string(long[:n+1])+"…", 13); w <= 300 {
		t.Fatalf("%q is not the longest that fits", got)
	}
	if got := s.ellipsize("短", 13, 300); got != "短" {
		t.Fatalf("short text changed: %q", got)
	}
}

func TestParseCapColor(t *testing.T) {
	for in, want := range map[string]ui.Color{
		"#fff":               ui.RGB(255, 255, 255),
		"#11223380":          {R: 0x11, G: 0x22, B: 0x33, A: 0x80},
		"rgb(1, 2, 3)":       ui.RGB(1, 2, 3),
		"rgba(10,20,30,0.5)": ui.RGBA(10, 20, 30, 0.5),
		"red":                {},
		"#zz":                {},
	} {
		if got, _ := parseCapColor(in); got != want {
			t.Errorf("%q = %+v, want %+v", in, got, want)
		}
	}
}

// 仓库里的模板和示例都要能跑每种状态
func TestCapsuleShippedStyles(t *testing.T) {
	dirs, _ := filepath.Glob("../../examples/styles/*")
	dirs = append(dirs, "../../skills/mami-capsule-style/template")
	if len(dirs) < 3 {
		t.Fatalf("found %v", dirs)
	}
	for _, dir := range dirs {
		s := loadCapsuleStyle(dir)
		for _, kind := range []string{"recording", "recognizing", "text", "hidden"} {
			for i := 0; i < 40; i++ {
				sc := fakeCapsuleScene(kind, time.Now().Add(time.Duration(i)*50*time.Millisecond), i%2 == 0)
				sc.level, sc.copy, sc.retry, sc.hint = float64(i%10)/9, i%3 == 0, i%3 == 1, map[bool]string{true: "润色失败"}[i%4 == 0]
				if kind == "hidden" {
					sc.leave = float64(i) / 39
				}
				if kind == "recognizing" && i%2 == 1 {
					sc.slow, sc.hint = true, "响应较慢，可取消"
				}
				if kind == "recording" && i == 0 {
					sc.levels, sc.level = nil, 0 // 刚开麦，还没有音量
				}
				f, err := s.frame(sc, nil)
				if err != nil {
					t.Fatalf("%s %s: %v", dir, kind, err)
				}
				if len(f.ops) == 0 {
					t.Fatalf("%s %s: drew nothing", dir, kind)
				}
				if kind == "recording" && len(f.buttons) != 2 {
					t.Fatalf("%s recording: buttons %+v", dir, f.buttons)
				}
			}
		}
	}
}
