<div align="center">

<img src="resources/icon.png" width="96" alt="Huaci">

# Huaci · 划词翻译

**在输入框里选中文字，点一下小按钮出译文；或者按 `Alt+T` 原地翻译替换。**

Windows 托盘小工具，Go + [MyGo](https://github.com/egoist/mygo) 写的，接任何 OpenAI 兼容接口。

[English](README.md) · [下载](https://github.com/margbug01/huaci/releases/latest)

<img src="docs/card.png" width="420" alt="翻译卡片">

</div>

## 为什么做这个

平时用中文写提示词、发出去要英文。复制 → 打开翻译 → 粘贴 → 复制 → 贴回来，来回几次就烦了。Huaci 待在托盘里，在你打字的地方直接翻：Alma、ChatGPT、网页输入框、微信、记事本都行。

## 功能

- **划词 → 按钮 → 卡片**：选区末尾弹出一个像素点阵「文A」小按钮，点它出卡片，可以**复制**或**替换**。
- **`Alt+T` 原地替换**：没选中就先全选输入框，翻完直接贴回去。
- **只在输入框里弹**：用 UI Automation 区分可编辑框和网页正文，看网页时不会到处弹按钮；双击空输入框也不弹。
- **点按钮后三种模式**：先弹「翻译中…」卡片 / 出结果再弹 / 直接替换。后两种按钮会原地变成转圈，出错一定弹卡片说明原因，不会悄悄失败。
- **不抢焦点**：浮窗带 `WS_EX_NOACTIVATE`，点按钮原来的选区还在；剪贴板用完会还原。
- **终端安全**：Windows Terminal 里一个键都不发（误发 `Ctrl+C` 会中断正在跑的程序），改用 UIA 直接读选区，卡片只给复制。
- **按模型适配提示词**：腾讯 **Hy-MT2**、B 站 **Index-Translate** 这类专用翻译模型用它们官方的提示词格式；其他模型（DeepSeek、GPT、Qwen…）用通用 system 提示词。
- 20 种目标语言、额外要求/术语表、代理、按程序排除、开机自启、深浅色主题。

<p align="center"><img src="docs/settings.png" width="380" alt="设置"></p>

## 安装

去 [Releases](https://github.com/margbug01/huaci/releases/latest) 下载 `Huaci Setup x.y.z amd64.exe`（安装版）或 `Huaci.exe`（绿色版）。支持 Windows 10/11 x64。

第一次打开会弹设置窗口。默认填的是 B 站**免费**的 Index-Translate-35B 接口，不用 key 就能先试；不过免费接口晚上高峰容易 504，长期用建议换下面的付费接口。

## 推荐接口

| 平台 | 接口地址 | 模型 | 说明 |
|---|---|---|---|
| 腾讯 TokenHub | `https://tokenhub.tencentmaas.com/v1` | `hy-mt2-pro` | 付费、稳定、约 1 秒，我日常用这个。还有 `hy-mt2-plus`、`hy-mt2-lite` |
| OpenRouter | `https://openrouter.ai/api/v1` | `tencent/hy-mt2-30b-a3b` | 同一系列，按量付费 |
| B 站（免费） | `https://index-translate.bilibili.com/v1` | `Index-Translate-35B-A3B` | 免 key，高峰期不稳 |
| 其他 OpenAI 兼容 | … | … | DeepSeek、Qwen、本地 vLLM / Ollama |

### 翻译提示词的小技巧

翻译模型遇到多行提示词，有时会直接**照着执行**而不是翻译。在设置的「额外要求」里填上这几行就好：

```
这是写给 AI 的提示词，译文要指令明确、没有歧义，语气自然直接，不要书面腔
保留原文的结构：换行、编号、列表、标题、代码块、变量占位符（如 {name}、<tag>）原样保留
代码、命令、文件路径、URL、模型名、产品名不翻译
术语对照：划词→text selection、提示词→prompt
```

## 用法

| 操作 | 效果 |
|---|---|
| 在输入框里选中文字 | 选区旁边弹出按钮 |
| 点按钮 | 卡片 / 出结果再弹 / 直接替换（设置里选） |
| `Alt+T` | 翻译并原地替换 |
| `Alt+Shift+T` | 翻译，只弹卡片 |
| 托盘图标右键 | 设置、开关划词按钮、打开日志、退出 |

方向自动判断：含汉字的翻成「中文翻成」那个语言（默认英语），其他翻成「其他翻成」那个语言（默认简体中文）。

配置在 `%AppData%\Huaci\config.json`，日志在 `%AppData%\Huaci\logs\huaci.log`。API Key 只存在这个文件里，只发给你填的接口。

## 已知限制

- **微信 4.x**（Qt 自绘）不向系统暴露任何输入框信息，分不清输入框和聊天记录。「认不出输入框的程序也弹」开着时，在聊天记录里划词也会弹；介意的话关掉它，微信里用 `Alt+T`。
- 不支持 `Ctrl+C` 的地方（游戏、远程桌面）取不到字；管理员权限运行的程序，Huaci 也得用管理员权限运行。
- 终端里不能原地替换：粘贴不会删掉选中的字。
- 只在 Windows 11、125% 缩放、单显示器上测过。

## 构建

需要 Go 1.27.1+（`GOTOOLCHAIN=auto` 会自动下载）。

```bash
go test ./...
go tool mygo build      # → build/windows-amd64/Huaci.exe + NSIS 安装包
```

`Huaci.exe -demo` 会画出按钮和一张示例卡片，用来看界面。

### 代码结构

| 文件 | 作用 |
|---|---|
| `main.go` / `app.go` | 启动、托盘、快捷键、按钮和卡片定位、翻译流程 |
| `views.go` / `settings.go` | MyGo 界面：按钮（按物理像素对齐的点阵）、卡片、设置 |
| `hook.go` | 低级鼠标 + 键盘钩子，单独一条 OS 线程 |
| `uia.go` | 原生 COM 调 UI Automation：焦点是不是输入框、选区在哪、终端选区文字 |
| `worker.go` / `clipboard.go` | 消息窗口线程：持有剪贴板、模拟 `Ctrl+C` / `Ctrl+V` |
| `translate.go` / `config.go` | 接口请求、按模型拼提示词、配置读写 |

窗口、托盘、全局快捷键、单实例、开机自启交给 MyGo；全局钩子、跨进程读选区、不抢焦点的浮窗是纯 Win32（`syscall`），不用 cgo。

## 许可

[MIT](LICENSE)
