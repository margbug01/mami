<div align="center">

<img src="resources/icon.png" width="96" alt="Mami">

# Mami

**Select text in any input box, click a tiny button, get the translation.**<br>
Or press <kbd>Alt</kbd>+<kbd>T</kbd> and it is translated and replaced right where you type.

[![Release](https://img.shields.io/github/v/release/margbug01/mami?style=flat-square&color=2f6fed)](https://github.com/margbug01/mami/releases/latest)
[![License](https://img.shields.io/github/license/margbug01/mami?style=flat-square)](LICENSE)
![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-444?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat-square&logo=go&logoColor=white)

[Website](https://margbug01.github.io/mami/) · [Download](https://github.com/margbug01/mami/releases/latest) · [中文说明](README_zh.md)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/card.png">
  <img src="docs/card-light.png" width="380" alt="Translation card with Copy and Replace buttons">
</picture>

</div>

A lightweight Windows tray app written in Go with [MyGo](https://github.com/egoist/mygo). It talks to any OpenAI-compatible API, so you bring your own model and key.

## Why

I write a lot of prompts in Chinese and send them in English. Copy, open a translator, paste, copy, paste back — it gets old fast. Mami lives in the tray and does it where you are already typing: Alma, ChatGPT, a browser textarea, WeChat, Notepad.

## Features

- **Select → button → card.** A small pixel-art 「文A」 button appears at the end of your selection. Click it for a card with **Copy** and **Replace**.
- **`Alt+T` replaces in place.** Nothing selected? It selects the whole input box first, translates, and pastes the result back.
- **Only in input boxes.** UI Automation tells an editable field from page text, so reading a web page doesn't scatter buttons around. Double-clicking an empty box doesn't trigger it either.
- **Three click modes.** Show a loading card, show the card only when the result arrives, or replace directly. In the quiet modes the button turns into a spinner; errors always show a card.
- **Never steals focus.** Floating windows are non-activating, so your selection stays put. Your clipboard is saved and restored afterwards.
- **Safe in terminals.** In Windows Terminal it sends *no* keystrokes (a stray `Ctrl+C` would kill your running program) and reads the selection through UI Automation. The card shows Copy only.
- **Model-aware prompts.** Dedicated formats for translation models such as Tencent **Hy-MT2** and bilibili **Index-Translate**, and a generic system prompt for everything else (DeepSeek, GPT, Qwen…).
- **20 target languages**, extra instructions and glossary, proxy, per-app exclusions, start at login.
- **Light and dark themes**, following Windows or set by hand.
- **9 interface languages**: 简体中文, 繁體中文, English, 日本語, 한국어, Español, Français, Deutsch, Русский.

## Screenshots

<table>
  <tr>
    <td align="center">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="docs/settings.png">
        <img src="docs/settings-light.png" alt="Settings window, translation page">
      </picture>
      <br><sub>Settings follow your theme. The extra-instructions box grows with its content, or drag the handle to size it yourself.</sub>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/settings-ja.png" alt="Settings window in Japanese">
      <br><sub>The same window in Japanese.</sub>
    </td>
  </tr>
</table>

## Install

Download `Mami Setup x.y.z amd64.exe` (installer) or `Mami.exe` (portable) from [Releases](https://github.com/margbug01/mami/releases/latest). Windows 10/11, x64.

On first launch the settings window opens. Out of the box it points at bilibili's **free** Index-Translate-35B endpoint (no key needed) so you can try it immediately. That free endpoint gets overloaded at peak hours and may return 504.

## Recommended backends

| Provider | Base URL | Model | Notes |
|---|---|---|---|
| Tencent TokenHub | `https://tokenhub.tencentmaas.com/v1` | `hy-mt2-pro` | Paid, stable, about 1 s. What I use daily. Also `hy-mt2-plus`, `hy-mt2-lite` |
| OpenRouter | `https://openrouter.ai/api/v1` | `tencent/hy-mt2-30b-a3b` | Same model family, pay as you go |
| bilibili (free) | `https://index-translate.bilibili.com/v1` | `Index-Translate-35B-A3B` | No key; flaky at night |
| Anything OpenAI-compatible | … | … | DeepSeek, Qwen, local vLLM / Ollama |

### Tip: translating prompts

Translation models will happily *execute* a multi-line prompt instead of translating it. Put something like this in **Extra instructions** (额外要求):

```
这是写给 AI 的提示词，译文要指令明确、没有歧义，语气自然直接，不要书面腔
保留原文的结构：换行、编号、列表、标题、代码块、变量占位符（如 {name}、<tag>）原样保留
代码、命令、文件路径、URL、模型名、产品名不翻译
术语对照：划词→text selection、提示词→prompt
```

## Usage

| Action | What happens |
|---|---|
| Select text in an input box | The button appears next to the selection |
| Click the button | Card, quiet card, or direct replace (configurable) |
| `Alt+T` | Translate and replace in place |
| `Alt+Shift+T` | Translate and show the card only |
| Tray icon → right click | Settings, toggle the button, open log, quit |

Direction is automatic. Text containing Chinese goes to your **Chinese to** language (English by default); everything else goes to **Others to** (Simplified Chinese by default).

Settings has five pages: **API**, **Translation**, **Shortcuts**, **Selection button**, and **General** (interface language, theme, start at login). Language and theme changes preview live and apply when you press Save.

Config lives in `%AppData%\Mami\config.json`, logs in `%AppData%\Mami\logs\mami.log`. Your API key stays in that file and is only sent to the endpoint you configured.

## Known limitations

- **WeChat 4.x** (Qt) exposes nothing to Windows accessibility, so Mami can't tell its input box from the chat history. With 「认不出输入框的程序也弹」 (*Also show in apps that hide their input boxes*) on, selecting a chat message also shows the button. Turn it off and use `Alt+T` if that bothers you.
- Apps that don't support `Ctrl+C` (games, remote desktop) can't be read. Elevated apps need Mami running elevated too.
- In terminals there is no in-place replace, because pasting doesn't remove the selected text.
- Only tested on Windows 11 at 125% scaling, single monitor.
- The interface translations for Korean, Spanish, French, German and Russian are machine-written and not yet reviewed by native speakers. Corrections are welcome.

## Build

Requires Go 1.27.1+ (`GOTOOLCHAIN=auto` downloads it).

```bash
go test ./...
go tool mygo build      # → build/windows-amd64/Mami.exe + NSIS installer
```

`Mami.exe -demo` draws the button and a sample card for a quick UI check. The screenshots in `docs/` are rendered off-screen: `MAMI_DOCS=docs go test -run TestSnapDocs`.

### How it's put together

| File | Role |
|---|---|
| `main.go` / `app.go` | Startup, tray, hotkeys, button and card placement, translate flow |
| `views.go` / `settings.go` | MyGo views: button (pixel-snapped dot matrix), card, settings |
| `i18n.go` / `i18n_*.go` | Interface translations. Write `T("中文")` in code and add the other languages to the dictionaries; a test fails if one is missing |
| `hook.go` | Low-level mouse and keyboard hooks on a dedicated OS thread |
| `uia.go` | UI Automation via raw COM vtables: is the focus editable, where is the selection, terminal selection text |
| `worker.go` / `clipboard.go` | Message-only window that owns the clipboard and sends `Ctrl+C` / `Ctrl+V` |
| `translate.go` / `config.go` | API requests, per-model prompt formats, config |

MyGo handles the window, tray, global shortcuts, single instance and auto-start. The global hook, cross-process selection, and non-activating floating windows are plain Win32 via `syscall` — no cgo.

## License

[MIT](LICENSE)
