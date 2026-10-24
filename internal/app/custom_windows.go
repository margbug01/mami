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

// pageCustom 侧边栏「自定义」：自己的网页胶囊样式，和给 AI 看的做样式说明（skills/mami-capsule-style）
func (a *App) pageCustom(c *ui.Context, d *Config) {
	t := c.Theme()
	pageTitle(c, T("自定义"))
	settingGroup(c, T("我的胶囊样式"), func() {
		styles := a.webStyles()
		if len(styles) == 0 {
			ui.Text(c, T("还没有。点「新建样式」从模板开始，或者把别人做好的样式文件夹放进样式文件夹。")).FontSize(12).TextColor(t.TextMuted).Padding(12, 0)
		}
		for _, e := range styles {
			value, folder := "web:"+e.folder, e.folder
			desc := fmt.Sprintf("%s · %d×%d", e.folder, e.man.Width, e.man.Height)
			if e.man.Dev {
				desc += " · " + T("开发模式")
			}
			settingRowW(c, e.man.Name, desc, selectW, func() {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Justify(ui.End).FillWidth().Children(func() {
					if smallLink(c, T("打开")).Clicked() {
						mygo.Shell.OpenPath(filepath.Join(a.webVoiceStylesDir(), folder))
					}
					if d.VoiceStyle == value {
						ui.Text(c, T("正在用")).FontSize(12).TextColor(t.Accent).SingleLine()
					} else if textButton(c, T("用这个")).Clicked() {
						d.VoiceStyle = value
						a.set.status, a.set.statusErr = T("记得点保存"), false
					}
				})
			})
		}
		ui.Row(c).Gap(16).Padding(8, 0, 12, 0).AlignItems(ui.Center).Children(func() {
			if textButton(c, T("新建样式")).Clicked() {
				a.newWebStyle()
			}
			if smallLink(c, T("打开样式文件夹")).Clicked() {
				a.openWebStyles()
			}
		})
	})
	settingGroup(c, T("做样式的说明"), func() {
		ui.Text(c, T("胶囊就是一张透明网页，想怎么画都行。把这份说明交给 Claude、Cursor 之类的 AI，说清想要什么样子，它就能照着写；也可以导出整个 skill 文件夹装进 AI 工具。")).
			FontSize(12).TextColor(t.TextMuted).Padding(12, 0, 4, 0)
		ui.Row(c).Gap(16).Padding(4, 0, 12, 0).AlignItems(ui.Center).Children(func() {
			if textButton(c, T("复制说明")).Clicked() {
				text := skillText()
				theWorker.do(func(owner uintptr) { setClipboardText(owner, toCRLF(text), false) })
				a.set.status, a.set.statusErr = T("已复制，粘贴给 AI 就行"), false
			}
			if smallLink(c, T("导出 skill 文件夹")).Clicked() {
				a.exportSkill()
			}
		})
		ui.Box(c).FillWidth().Height(1).Background(paneBorder(t))
		ui.Column(c).Padding(12, 0, 16, 0).Gap(6).Children(func() { markdownView(c, skillText()) })
	})
}

func skillText() string {
	b, _ := fs.ReadFile(skills.FS, path.Join(skills.CapsuleStyle, "SKILL.md"))
	return string(b)
}

// copySkillDir 把内嵌的 skill 子目录 sub 拷到 dst
func copySkillDir(sub, dst string) error {
	root := path.Join(skills.CapsuleStyle, sub)
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

// newWebStyle 从模板新建一个样式文件夹（开发模式开着，改了文件胶囊自己刷新），打开它
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
	if err := copySkillDir("template", target); err != nil {
		a.set.status, a.set.statusErr = T("新建失败：")+err.Error(), true
		return
	}
	a.voiceWeb.listAt = time.Time{}
	a.set.status, a.set.statusErr = fmt.Sprintf(T("已建好 %s，把「做样式的说明」交给 AI 改它"), name), false
	mygo.Shell.OpenPath(target)
}

// exportSkill 把整个 skill 文件夹导出到配置目录旁边，打开给用户拖进 AI 工具
func (a *App) exportSkill() {
	target := filepath.Join(filepath.Dir(a.cfgPath), "skills", skills.CapsuleStyle)
	if err := copySkillDir("", target); err != nil {
		a.set.status, a.set.statusErr = T("导出失败：")+err.Error(), true
		return
	}
	a.set.status, a.set.statusErr = fmt.Sprintf(T("已导出到 %s"), target), false
	mygo.Shell.OpenPath(target)
}

// markdownView 只认 SKILL.md 里用到的几样：标题、列表、表格、代码块、段落；行内的 ** 和 ` 去掉。文字都能选中复制
func markdownView(c *ui.Context, md string) {
	t := c.Theme()
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[0] == "---" { // frontmatter 不给人看
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				lines = lines[i+1:]
				break
			}
		}
	}
	plain := func(s string) string { return strings.NewReplacer("**", "", "`", "").Replace(s) }
	codeBG := ui.RGBA(20, 30, 45, 0.05)
	if t.Dark {
		codeBG = ui.RGBA(255, 255, 255, 0.05)
	}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "```"):
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(lines[i], "```"); i++ {
				code = append(code, lines[i])
			}
			ui.Text(c, strings.Join(code, "\n")).Font("Cascadia Mono, Consolas, monospace").FontSize(12).Selectable().
				FillWidth().Padding(10, 12).Radius(8).Background(codeBG)
		case strings.HasPrefix(l, "|"):
			var rows [][]string
			for ; i < len(lines) && strings.HasPrefix(lines[i], "|"); i++ {
				cells := strings.Split(strings.Trim(strings.TrimSpace(lines[i]), "|"), "|")
				if strings.Trim(strings.Join(cells, ""), "-: ") == "" {
					continue // |---|---| 分隔行
				}
				rows = append(rows, cells)
			}
			i--
			ui.Column(c).FillWidth().Radius(8).Border(1, paneBorder(t)).Clip().Children(func() {
				for r, cells := range rows {
					row := ui.Row(c).FillWidth().Gap(12).Padding(7, 10)
					if r == 0 {
						row.Background(codeBG)
					}
					row.Children(func() {
						for k, cell := range cells {
							txt := ui.Text(c, plain(strings.TrimSpace(cell))).FontSize(12).Selectable().Basis(0).Shrink(1)
							if k == len(cells)-1 {
								txt.Grow(3)
							} else {
								txt.Grow(1)
							}
							if r == 0 {
								txt.FontWeight(600)
							}
						}
					})
				}
			})
		case strings.HasPrefix(l, "### "):
			ui.Text(c, plain(l[4:])).FontSize(13).FontWeight(600).Padding(8, 0, 0, 0)
		case strings.HasPrefix(l, "## "):
			ui.Text(c, plain(l[3:])).FontSize(15).FontWeight(600).Padding(12, 0, 2, 0)
		case strings.HasPrefix(l, "# "):
			ui.Text(c, plain(l[2:])).FontSize(18).FontWeight(600).Padding(0, 0, 4, 0)
		case strings.HasPrefix(l, "- "):
			item := strings.TrimPrefix(l[2:], "[ ] ")
			mark := "•"
			if item != l[2:] {
				mark = "☐"
			}
			ui.Row(c).Gap(8).Padding(0, 0, 0, 4).Children(func() {
				ui.Text(c, mark).FontSize(13).TextColor(t.TextMuted)
				ui.Text(c, plain(item)).FontSize(13).Selectable().Grow(1).Shrink(1)
			})
		case strings.TrimSpace(l) == "":
		default:
			ui.Text(c, plain(l)).FontSize(13).Selectable().FillWidth()
		}
	}
}
