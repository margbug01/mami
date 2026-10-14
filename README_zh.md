<div align="center">

<img src="resources/icon.png" width="96" alt="Mami">

# Mami · 划词翻译

**在任何输入框里选中文字，点一下小按钮就出译文。**<br>
或者按 <kbd>Alt</kbd>+<kbd>T</kbd>，直接在原地翻译并替换。

[![Release](https://img.shields.io/github/v/release/margbug01/mami?style=flat-square&color=2f6fed)](https://github.com/margbug01/mami/releases/latest)
[![License](https://img.shields.io/github/license/margbug01/mami?style=flat-square)](LICENSE)
![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-444?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat-square&logo=go&logoColor=white)

[主页](https://margbug01.github.io/mami/) · [下载](https://github.com/margbug01/mami/releases/latest) · [English](README.md)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/card.png">
  <img src="docs/card-light.png" width="380" alt="译文卡片，带复制和替换按钮">
</picture>

</div>

用 Go 和 [MyGo](https://github.com/egoist/mygo) 写的 Windows 托盘小工具。接任何 OpenAI 兼容的接口，模型和 Key 都用你自己的。

## 为什么做

我经常用中文写提示词、发出去是英文。复制、打开翻译器、粘贴、再复制、再粘贴回去，一次两次还行，天天这样很烦。Mami 常驻托盘，在你正打字的地方直接翻：Alma、ChatGPT、浏览器输入框、微信、记事本都行。

## 功能

- **选中 → 按钮 → 卡片。** 选中文字后，选区末尾冒出一个像素风的「文A」小按钮，点它弹出译文卡片，有**复制**和**替换**。
- **`Alt+T` 原地替换。** 没选中文字时，先全选整个输入框，翻译完再贴回去。
- **只在输入框里弹。** 用 UI Automation 区分输入框和网页正文，看网页时不会到处冒按钮；双击空输入框也不会触发。
- **三种点击后的显示方式。** 先弹「翻译中」卡片、出结果再弹卡片、或者直接替换。后两种模式下按钮会变成转圈，出错时一定会弹卡片。
- **不抢焦点。** 浮窗是不激活窗口，选区不会丢；翻译完会还原你的剪贴板。
- **终端里也安全。** 在 Windows Terminal 里一个键都不发（多发一个 `Ctrl+C` 就会打断你正在跑的程序），改用 UI Automation 读选区，卡片只给「复制」。
- **按模型拼提示词。** 腾讯 **Hy-MT2**、B 站 **Index-Translate** 这类翻译模型有专用格式，其他模型（DeepSeek、GPT、Qwen…）用通用系统提示词。
- **20 种目标语言**、额外要求/术语表、代理、按程序排除、开机自启。
- **浅色和深色主题**，跟随 Windows 或手动选。
- **9 种界面语言**：简体中文、繁體中文、English、日本語、한국어、Español、Français、Deutsch、Русский。

## 截图

<table>
  <tr>
    <td align="center">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="docs/settings.png">
        <img src="docs/settings-light.png" alt="设置窗口，翻译页">
      </picture>
      <br><sub>设置窗口跟随主题。「额外要求」输入框会随内容自动长高，也可以拖右下角的把手自己调。</sub>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/settings-ja.png" alt="日语界面的设置窗口">
      <br><sub>同一个窗口的日语版。</sub>
    </td>
  </tr>
</table>

## 安装

到 [Releases](https://github.com/margbug01/mami/releases/latest) 下载 `Mami Setup x.y.z amd64.exe`（安装包）或 `Mami.exe`（绿色版）。Windows 10/11，x64。

第一次启动会打开设置窗口。默认指向 B 站**免费**的 Index-Translate-35B 接口（不用 Key），装好就能试。这个免费接口高峰期会过载，可能返回 504。

## 推荐的接口

| 服务商 | Base URL | 模型 | 说明 |
|---|---|---|---|
| 腾讯 TokenHub | `https://tokenhub.tencentmaas.com/v1` | `hy-mt2-pro` | 收费、稳定、约 1 秒，我自己天天用。还有 `hy-mt2-plus`、`hy-mt2-lite` |
| OpenRouter | `https://openrouter.ai/api/v1` | `tencent/hy-mt2-30b-a3b` | 同一系列模型，按量付费 |
| B 站（免费） | `https://index-translate.bilibili.com/v1` | `Index-Translate-35B-A3B` | 不用 Key，晚上不太稳 |
| 其他 OpenAI 兼容接口 | … | … | DeepSeek、Qwen、本地 vLLM / Ollama |

### 小技巧：翻译提示词

翻译模型碰到多行的提示词，常常会直接去「执行」而不是翻译。在**额外要求**里写上类似这样的话：

```
这是写给 AI 的提示词，译文要指令明确、没有歧义，语气自然直接，不要书面腔
保留原文的结构：换行、编号、列表、标题、代码块、变量占位符（如 {name}、<tag>）原样保留
代码、命令、文件路径、URL、模型名、产品名不翻译
术语对照：划词→text selection、提示词→prompt
```

## 用法

| 操作 | 效果 |
|---|---|
| 在输入框里选中文字 | 选区旁边出现按钮 |
| 点按钮 | 弹卡片 / 静默卡片 / 直接替换（可设置） |
| `Alt+T` | 翻译并原地替换 |
| `Alt+Shift+T` | 翻译，只弹卡片 |
| 托盘图标右键 | 设置、开关划词按钮、打开日志、退出 |

方向自动判断：含汉字的翻成「中文翻成」那个语言（默认英语），其他翻成「其他翻成」那个语言（默认简体中文）。

设置分五页：**接口**、**翻译**、**快捷键**、**划词按钮**，以及**通用**（界面语言、主题、开机自启）。语言和主题改了立刻预览，点保存才生效。

配置在 `%AppData%\Mami\config.json`，日志在 `%AppData%\Mami\logs\mami.log`。API Key 只存在这个文件里，只发给你填的接口。

## 已知限制

- **微信 4.x**（Qt 自绘）不向系统暴露任何输入框信息，分不清输入框和聊天记录。「认不出输入框的程序也弹」开着时，在聊天记录里划词也会弹；介意的话关掉它，微信里用 `Alt+T`。
- 不支持 `Ctrl+C` 的地方（游戏、远程桌面）取不到字；管理员权限运行的程序，Mami 也得用管理员权限运行。
- 终端里不能原地替换：粘贴不会删掉选中的字。
- 只在 Windows 11、125% 缩放、单显示器上测过。
- 韩语、西班牙语、法语、德语、俄语的界面翻译是机器写的，还没有母语者审过，欢迎指正。

## 构建

需要 Go 1.27.1+（`GOTOOLCHAIN=auto` 会自动下载）。

```bash
go test ./...
go tool mygo build      # → build/windows-amd64/Mami.exe + NSIS 安装包
```

`Mami.exe -demo` 会画出按钮和一张示例卡片，用来看界面。`docs/` 里的截图是离屏渲染出来的：`MAMI_DOCS=docs go test -run TestSnapDocs`。

### 代码结构

| 文件 | 作用 |
|---|---|
| `main.go` / `app.go` | 启动、托盘、快捷键、按钮和卡片定位、翻译流程 |
| `views.go` / `settings.go` | MyGo 界面：按钮（按物理像素对齐的点阵）、卡片、设置 |
| `i18n.go` / `i18n_*.go` | 界面翻译。代码里写 `T("中文")`，再把其他语言补进词典；漏了测试会失败 |
| `hook.go` | 低级鼠标 + 键盘钩子，单独一条 OS 线程 |
| `uia.go` | 原生 COM 调 UI Automation：焦点是不是输入框、选区在哪、终端选区文字 |
| `worker.go` / `clipboard.go` | 消息窗口线程：持有剪贴板、模拟 `Ctrl+C` / `Ctrl+V` |
| `translate.go` / `config.go` | 接口请求、按模型拼提示词、配置读写 |

窗口、托盘、全局快捷键、单实例、开机自启交给 MyGo；全局钩子、跨进程读选区、不抢焦点的浮窗是纯 Win32（`syscall`），不用 cgo。

## 许可

[MIT](LICENSE)
