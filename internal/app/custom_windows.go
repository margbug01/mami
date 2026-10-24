//go:build windows

package app

import (
	"fmt"
	"image/png"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"mami/skills"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const pageCustom = "custom"

var iconCustom = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22a1 1 0 0 1 0-20 10 9 0 0 1 10 9 5 5 0 0 1-5 5h-2.25a1.75 1.75 0 0 0-1.4 2.8l.3.4a1.75 1.75 0 0 1-1.4 2.8z"/><circle cx="13.5" cy="6.5" r="1" fill="currentColor"/><circle cx="17.5" cy="10.5" r="1" fill="currentColor"/><circle cx="6.5" cy="12.5" r="1" fill="currentColor"/><circle cx="8.5" cy="7.5" r="1" fill="currentColor"/></svg>`))

var iconChevronRight = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>`))

// 只在界面线程用
var (
	customOpenSkill string                    // 「给 AI 的 skill」里展开的那一个
	styleThumbs     = map[string]styleThumb{} // 样式缩略图，按文件路径缓存
)

type styleThumb struct {
	mod time.Time
	bmp *ui.Bitmap
}

const styleCardW, styleThumbH = 160, 76

// pageCustom 侧边栏「自定义」：上面是自己的网页胶囊样式（卡片墙），下面是给 AI 的 skill，一行一个、默认折叠
func (a *App) pageCustom(c *ui.Context, d *Config) {
	t := c.Theme()
	pageTitle(c, T("自定义"))
	settingGroup(c, T("胶囊样式"), func() {
		ui.Row(c).Wrap().Gap(12).Padding(14, 0).Children(func() {
			for _, e := range a.webStyles() {
				a.styleCard(c, d, e)
			}
			add := ui.ButtonBase(c).Width(styleCardW).Height(styleThumbH+46).Radius(10).Border(1, t.Border).BorderStyle(ui.BorderDashed).
				Center().Gap(4).Cursor(ui.CursorPointer).Label(T("新建样式"))
			if add.Hovered() {
				add.Background(t.SurfaceHover)
			}
			add.Children(func() {
				ui.Text(c, "+").FontSize(22).TextColor(t.TextMuted)
				ui.Text(c, T("新建样式")).FontSize(12).TextColor(t.TextMuted)
			})
			if add.Clicked() {
				a.newWebStyle()
			}
		})
		ui.Row(c).Padding(0, 0, 12, 0).Children(func() {
			if smallLink(c, T("打开样式文件夹")).Clicked() {
				a.openWebStyles()
			}
		})
	})
	settingGroup(c, T("给 AI 的 skill"), func() {
		ui.Text(c, T("复制给 AI，或者导出文件夹装进 Claude Code、Cursor 这类工具。")).FontSize(12).TextColor(t.TextMuted).Padding(12, 0, 4, 0)
		for _, sk := range embeddedSkills() {
			a.skillRow(c, sk)
		}
		ui.Box(c).Height(6)
	})
}

// styleCard 一张样式卡：缩略图（文件夹里的 preview.png，没有就画个药丸），名字、尺寸；点一下选它，悬停露出「打开」
func (a *App) styleCard(c *ui.Context, d *Config, e webStyleEntry) {
	t := c.Theme()
	value, dir := "web:"+e.folder, filepath.Join(a.webVoiceStylesDir(), e.folder)
	picked := d.VoiceStyle == value
	card := ui.Column(c).Key(e.folder).Width(styleCardW).Radius(10).Clip().Border(1, t.Border).Background(t.Surface)
	if picked {
		card.Border(2, t.Accent)
	}
	hover := card.Hovered()
	card.Children(func() {
		thumb := ui.ButtonBase(c).FillWidth().Height(styleThumbH).Cursor(ui.CursorPointer).Label(e.man.Name)
		bmp := styleThumbFor(filepath.Join(dir, "preview.png"))
		thumb.Draw(func(p *ui.Painter, r ui.Rect) {
			desk := ui.RGBA(221, 226, 234, 1)
			if t.Dark {
				desk = ui.RGBA(22, 24, 29, 1)
			}
			p.Fill(r, desk, 0)
			if bmp != nil {
				p.Image(bmp, r, ui.Cover)
				return
			}
			w, h := float32(84), float32(26)
			p.Fill(ui.Rect{X: r.X + (r.W-w)/2, Y: r.Y + (r.H-h)/2, W: w, H: h}, ui.RGBA(30, 30, 36, 1), h/2)
		})
		if picked {
			thumb.Children(func() {
				ui.Text(c, T("正在用")).FontSize(11).TextColor(t.AccentText).Background(t.Accent).Radius(6).Padding(2, 8).Margin(8, 0, 0, 8)
			})
		}
		if thumb.Clicked() && !picked {
			d.VoiceStyle = value
			a.set.status, a.set.statusErr = T("记得点保存"), false
		}
		ui.Row(c).FillWidth().Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
				ui.Text(c, e.man.Name).FontSize(13).FontWeight(600).SingleLine()
				sub := fmt.Sprintf("%d×%d", e.man.Width, e.man.Height)
				if e.man.Dev {
					sub += " · " + T("开发模式")
				}
				ui.Text(c, sub).FontSize(11).TextColor(t.TextMuted).SingleLine()
			})
			if hover && smallLink(c, T("打开")).Clicked() {
				mygo.Shell.OpenPath(dir)
			}
		})
	})
}

func styleThumbFor(file string) *ui.Bitmap {
	info, err := os.Stat(file)
	if err != nil {
		return nil
	}
	if th, ok := styleThumbs[file]; ok && th.mod.Equal(info.ModTime()) {
		return th.bmp
	}
	th := styleThumb{mod: info.ModTime()}
	if f, err := os.Open(file); err == nil {
		if img, err := png.Decode(f); err == nil {
			th.bmp = ui.NewBitmap(img)
		}
		f.Close()
	}
	styleThumbs[file] = th
	return th.bmp
}

// embeddedSkill 打进程序的一个 skill（skills/<dir>/SKILL.md）
type embeddedSkill struct{ dir, title, text string }

// embeddedSkills skills 包里每个带 SKILL.md 的文件夹；以后加 skill 只要加文件夹，这页不用改
func embeddedSkills() []embeddedSkill {
	var out []embeddedSkill
	entries, _ := fs.ReadDir(skills.FS, ".")
	for _, e := range entries {
		b, err := fs.ReadFile(skills.FS, path.Join(e.Name(), "SKILL.md"))
		if !e.IsDir() || err != nil {
			continue
		}
		sk := embeddedSkill{dir: e.Name(), title: e.Name(), text: string(b)}
		for _, l := range strings.Split(sk.text, "\n") {
			if h, ok := strings.CutPrefix(strings.TrimSpace(l), "# "); ok {
				sk.title = h
				break
			}
		}
		out = append(out, sk)
	}
	return out
}

// skillRow 一个 skill 一行：标题 + 文件夹名，右边复制、导出；点开才露原文（给人瞄一眼，真要用就复制给 AI）
func (a *App) skillRow(c *ui.Context, sk embeddedSkill) {
	t := c.Theme()
	open := customOpenSkill == sk.dir
	ui.Box(c).FillWidth().Height(1).Background(paneBorder(t))
	ui.Row(c).Key(sk.dir).FillWidth().Padding(10, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		head := ui.ButtonBase(c).Grow(1).Shrink(1).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(sk.title)
		head.Children(func() {
			icon := iconChevronRight
			if open {
				icon = iconChevron
			}
			ui.Icon(c, icon).FontSize(14).TextColor(t.TextMuted)
			ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
				ui.Text(c, sk.title).FontSize(13).SingleLine()
				ui.Text(c, sk.dir).FontSize(11).TextColor(t.TextMuted).SingleLine()
			})
		})
		if head.Clicked() {
			if open {
				customOpenSkill = ""
			} else {
				customOpenSkill = sk.dir
			}
		}
		if textButton(c, T("复制")).Clicked() {
			text := sk.text
			theWorker.do(func(owner uintptr) { setClipboardText(owner, toCRLF(text), false) })
			a.set.status, a.set.statusErr = T("已复制，粘贴给 AI 就行"), false
		}
		if smallLink(c, T("导出")).Clicked() {
			a.exportSkill(sk.dir)
		}
	})
	if open {
		codeBG := ui.RGBA(20, 30, 45, 0.05)
		if t.Dark {
			codeBG = ui.RGBA(255, 255, 255, 0.05)
		}
		ui.Scroll(c).FillWidth().Height(320).Margin(0, 0, 12, 0).Radius(8).Background(codeBG).Children(func() {
			ui.Text(c, sk.text).Font("Cascadia Mono, Consolas, monospace").FontSize(12).Selectable().FillWidth().Padding(12, 14)
		})
	}
}

// copySkillDir 把内嵌的 skills/<skill>/<sub> 拷到 dst
func copySkillDir(skill, sub, dst string) error {
	root := path.Join(skill, sub)
	return fs.WalkDir(skills.FS, root, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(p, root), "/")))
		if de.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := fs.ReadFile(skills.FS, p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

// newWebStyle 从胶囊样式 skill 的模板新建一个样式文件夹（开发模式开着，改了文件胶囊自己刷新），打开它
func (a *App) newWebStyle() {
	dir := a.webVoiceStylesDir()
	name := "my-style"
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("my-style-%d", i)
	}
	target := filepath.Join(dir, name)
	if err := copySkillDir(skills.CapsuleStyle, "template", target); err != nil {
		a.set.status, a.set.statusErr = T("新建失败：")+err.Error(), true
		return
	}
	a.voiceWeb.listAt = time.Time{}
	a.set.status, a.set.statusErr = fmt.Sprintf(T("已建好 %s，复制胶囊样式的 skill 给 AI 改它"), name), false
	mygo.Shell.OpenPath(target)
}

// exportSkill 把一个 skill 文件夹导出到配置目录旁边，打开给用户拖进 AI 工具
func (a *App) exportSkill(skill string) {
	target := filepath.Join(filepath.Dir(a.cfgPath), "skills", skill)
	if err := copySkillDir(skill, "", target); err != nil {
		a.set.status, a.set.statusErr = T("导出失败：")+err.Error(), true
		return
	}
	a.set.status, a.set.statusErr = fmt.Sprintf(T("已导出到 %s"), target), false
	mygo.Shell.OpenPath(target)
}
