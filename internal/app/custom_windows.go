//go:build windows

package app

import (
	"fmt"
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

// customOpenSkill 「给 AI 的 skill」里展开的那一个（界面线程）
var customOpenSkill string

// deskColors 设置页预览用的假桌面底色和说明字颜色（和外观页的预览一致）
func deskColors(dark bool) (desk, label ui.Color) {
	if dark {
		return ui.RGBA(22, 24, 29, 1), ui.RGBA(232, 234, 240, 0.55)
	}
	return ui.RGBA(221, 226, 234, 1), ui.RGBA(30, 32, 38, 0.55)
}

// pageCustom 侧边栏「自定义」：上面是自己的胶囊样式（实时画在假桌面上），下面是给 AI 的 skill，一行一个、默认折叠
func (a *App) pageCustom(c *ui.Context, d *Config) {
	t := c.Theme()
	pageTitle(c, T("自定义"))
	settingGroup(c, T("胶囊样式"), func() {
		a.styleGallery(c, d)
		ui.Row(c).Padding(0, 0, 12, 0).Children(func() {
			if smallLink(c, T("打开样式文件夹")).Clicked() {
				a.openCapsuleStyles()
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

// styleGallery 跟外观页的预览一样：一块假桌面上直接画每个样式录音时的样子（脚本实时跑），名字写在下面。
// 点一下选它；选中的名字加粗、前面一个小点；脚本出错的名字标红，悬停看原因
func (a *App) styleGallery(c *ui.Context, d *Config) {
	t := c.Theme()
	desk, label := deskColors(t.Dark)
	dark := toneDark(d.VoiceTone, t.Dark)
	now := time.Now()
	ui.Row(c).FillWidth().Wrap().Padding(18, 16).Margin(12, 0, 8, 0).Gap(28).Justify(ui.Center).AlignItems(ui.End).Background(desk).Radius(t.Radius).Children(func() {
		for _, e := range a.capsuleStyles() {
			value := capsulePrefix + e.folder
			picked := d.VoiceStyle == value
			item := ui.ButtonBase(c).Key(e.folder).Shrink(0).Cursor(ui.CursorPointer).Label(e.man.Name)
			hover := item.Hovered()
			item.Children(func() {
				ui.Column(c).Gap(6).AlignItems(ui.Center).Children(func() {
					err := a.capsulePreview(c, e.folder, fakeCapsuleScene("recording", now, dark), 260, 64)
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						if picked {
							ui.Box(c).Size(5, 5).Radius(3).Background(t.Text)
						}
						name := ui.Text(c, e.man.Name).FontSize(11).SingleLine()
						switch {
						case err != nil:
							name.TextColor(t.Danger).Tooltip(T("脚本出错：") + err.Error())
						case picked:
							name.TextColor(t.Text).FontWeight(600)
						case hover:
							name.TextColor(t.Text)
						default:
							name.TextColor(label)
						}
					})
				})
			})
			if item.Clicked() && !picked {
				d.VoiceStyle = value
				a.set.status, a.set.statusErr = T("记得点保存"), false
			}
		}
		add := ui.ButtonBase(c).Shrink(0).Cursor(ui.CursorPointer).Label(T("新建样式"))
		addHover := add.Hovered()
		add.Children(func() {
			ui.Column(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ring := ui.Box(c).Size(44, 44).Radius(22).Border(1, label).BorderStyle(ui.BorderDashed).Center()
				ring.Children(func() { ui.Text(c, "+").FontSize(18).TextColor(label) })
				color := label
				if addHover {
					color = t.Text
				}
				ui.Text(c, T("新建样式")).FontSize(11).TextColor(color).SingleLine()
			})
		})
		if add.Clicked() {
			a.newCapsuleStyle()
		}
	})
}

// capsuleLookPreview 外观页选了自定义样式时：和内置样式一样，在假桌面上把三种状态并排画出来
func (a *App) capsuleLookPreview(c *ui.Context, d *Config, folder string) {
	t := c.Theme()
	desk, label := deskColors(t.Dark)
	dark := toneDark(d.VoiceTone, t.Dark)
	now := time.Now()
	var failed error
	captions := []string{T("录音中"), T("识别中…"), T("结果待复制")}
	ui.Row(c).FillWidth().Wrap().Padding(16, 12).Margin(4, 0, 12, 0).Gap(20).Justify(ui.Center).AlignItems(ui.End).Background(desk).Radius(t.Radius).Children(func() {
		for i, kind := range []string{"recording", "recognizing", "text"} {
			ui.Column(c).Shrink(0).Label(captions[i]).Gap(4).AlignItems(ui.Center).Children(func() {
				if err := a.capsulePreview(c, folder, fakeCapsuleScene(kind, now, dark), 180, 80); err != nil {
					failed = err
				}
				ui.Text(c, captions[i]).FontSize(11).TextColor(label)
			})
		}
	})
	ui.Row(c).Gap(12).Padding(0, 0, 8, 0).AlignItems(ui.Center).Children(func() {
		if failed != nil {
			ui.Text(c, T("脚本出错：")+failed.Error()).FontSize(12).TextColor(t.Danger).Grow(1).Shrink(1).MaxLines(2)
		} else {
			ui.Text(c, T("在「自定义」里管理和新建样式")).FontSize(12).TextColor(t.TextMuted).Grow(1).Shrink(1)
		}
		if smallLink(c, T("去自定义")).Clicked() {
			a.set.page = pageCustom
		}
	})
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

// newCapsuleStyle 从胶囊样式 skill 的模板新建一个样式文件夹（改了文件预览和胶囊自己刷新），打开它
func (a *App) newCapsuleStyle() {
	dir := a.capsuleStylesDir()
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
	a.capWin.listAt = time.Time{}
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
