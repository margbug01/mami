<div align="center">

<img src="resources/icon.png" width="96" alt="Mami">

# Mami

**Dictate and translate right where you type.**<br>
Tap Right <kbd>Alt</kbd> and talk: what you said lands in the input box, cleaned up. Select text and click the small button to translate it in place.

[![Release](https://img.shields.io/github/v/release/margbug01/mami?style=flat-square&color=2f6fed)](https://github.com/margbug01/mami/releases/latest)
[![License](https://img.shields.io/github/license/margbug01/mami?style=flat-square)](LICENSE)
![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-444?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat-square&logo=go&logoColor=white)

[Website](https://margbug01.github.io/mami/) · [Download](https://github.com/margbug01/mami/releases/latest) · [中文](README_zh.md)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/voice.png">
  <img src="docs/voice-light.png" width="620" alt="The dictation pill while recording, while recognizing, and when there is nowhere to paste">
</picture>

</div>

Mami is a Windows tray app written in Go with [MyGo](https://github.com/egoist/mygo). It does two things: **voice input** and **selection translation**. Speech recognition, polishing and translation all use endpoints and keys you choose, and your audio and text go only to those providers.

## Why

I write a lot every day in Alma, ChatGPT and WeChat, mostly long prompts for AI. Typing is slow. Speaking is faster, but then the text is full of "um", "well" and "no wait, I mean…", and the Chinese often needs to become English afterwards. Mami sits in the tray and works in the box you are already typing in. You talk and get tidy text; you select and get a translation.

## Voice input

- **Tap to talk, tap again to paste.** Right <kbd>Alt</kbd> by default. Any left or right modifier, F1–F24, CapsLock, a shortcut, or the media keys a remote or presenter sends will also work. <kbd>Esc</kbd> cancels. Pressing another key while holding the trigger counts as a shortcut, so normal typing never starts a recording.
- **Text right after you stop.** With StepFun, Mami cuts the recording at pauses and sends the earlier parts while you are still talking. With Qwen 3.1 streaming, audio is pushed as you speak and the result is usually ready about half a second after you stop. Takes that are too short or too quiet are never sent.
- **Five polish levels, matched to the app.** Off, light, standard, structured and prompt. In AI tools like Cursor and ChatGPT the prompt level is used automatically: every requirement is kept and file names, commands and English terms stay as they are. Chat apps like WeChat get the light level. Every level drops fillers and self-corrections ("at three, no, at four" becomes "at four"). It does not answer your questions and does not translate behind your back. Structured turns explicit lists into bullet points. DeepSeek is the default, and any OpenAI-compatible endpoint works. If polish fails, the raw text is pasted.
- **Vocabulary and auto-learning.** Add names and product names, and recognition and polish will spell them your way. When you add a word, Mami guesses a few common mishearings of it. When you fix a word after pasting, Mami notices, and once you have made the same fix twice it applies it automatically. Only fixes that sound alike are learned. If polish swaps a word it shouldn't (a different person with a similar name), the swap is undone.
- **Long-form edit samples (clearable).** For dictations of 10 characters or more, Mami keeps the raw text, the polished text and your final version on this PC. These samples are collected for learning your writing style later; they are only recorded for now.
- **Select, then say how to change it.** Select some text, press the trigger and say "translate to English", "make it formal", "cut it in half" or "drop the last sentence", and it is rewritten in place. If what you say isn't an instruction (a new sentence or a question), it simply replaces the selection. The pill offers ↺ to undo, and nothing changes if the rewrite fails.
- **Retry when recognition fails.** If the network fails, the recording is kept and the ↻ button in the pill sends it again.
- **Nothing gets lost.** When the cursor is not in an input box, the pill shows the text with a copy button. In apps like WeChat whose input box Windows can't see, Mami still pastes and also leaves the text on the clipboard.
- **Eight pill styles.** Gradient bars, jelly orb, Flow, frosted glass, retro pixel, mercury, three islands and LED segments. Flow is a standalone animated orb with five palettes — Ice Sea, Sunset, Aurora, Berry and Jade — and matching result pills and copy buttons. The pill's dark or light tone can differ from the app theme. Soft clicks play on start, stop and cancel. An optional live-text mode scrolls the latest few words through the pill (off by default); Flow stays a standalone orb until the result is ready.
- **No keyboard needed.** `Mami.exe -voice toggle|start|stop|cancel` hands the command to the running Mami, so foot pedals, Stream Deck or AutoHotkey can drive it.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/words.png">
  <img src="docs/words-light.png" width="620" alt="Vocabulary page with the auto-learn switch and a grid of words">
</picture>

## Selection translation

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/card.png">
  <img src="docs/card-light.png" width="380" alt="Translation card with Copy and Replace buttons">
</picture>

- **Select → button → card.** Select text in an input box and a pixel-style 「文A」 button appears at the end of the selection. Click it for a card with **Copy** and **Replace**. <kbd>Enter</kbd> replaces and <kbd>Esc</kbd> closes.
- **<kbd>Alt</kbd>+<kbd>T</kbd> replaces in place**, and <kbd>Alt</kbd>+<kbd>Shift</kbd>+<kbd>T</kbd> shows the card only. With nothing selected, the whole box is selected first.
- **Input boxes only.** UI Automation tells input boxes from page text, so reading a web page never sprinkles buttons around.
- **Prompts per model.** Tencent Hy-MT2 and bilibili Index-Translate get their own prompt formats; other models get a general system prompt. 20 target languages, extra instructions and glossaries, and an optional live-streaming card.
- **Safe in terminals.** In Windows Terminal Mami sends no keystrokes. It reads the selection through UI Automation and the card offers Copy only.

## Shared by both

- **Never steals focus.** Floating windows are non-activating, so your selection stays put, and your clipboard is restored after every paste or translation.
- **Providers in one click.** Pick a provider from the strip of logos and the endpoint and model are filled in. "Get a key" opens the right page. Keys are remembered per endpoint, so switching away and back refills them.
- **History page.** For voice: a half-year heatmap, usage by hour, count, characters, time saved versus typing and average polish time, plus every entry with its raw and polished text. For translation (off by default): source, the exact prompt sent, result, timing and what you did with it, saved as daily JSONL so you can review it with AI later.
- **One-click updates, start with Windows, per-app exclusions, proxy.** Light and dark themes, and the settings window remembers its size and position. The UI font is [MiSans](https://hyperos.mi.com/font).
- **9 interface languages**: Simplified Chinese, Traditional Chinese, English, Japanese, Korean, Spanish, French, German, Russian.

## Install

Download `Mami Setup x.y.z amd64.exe` (installer) or `Mami.exe` (portable) from [Releases](https://github.com/margbug01/mami/releases/latest). Windows 10/11, x64.

On first launch a short hands-on tour lets you try both: press the voice key and read a sentence aloud (the pill follows your voice and the words light up as you speak, then the fillers drop out), and select a sentence to get the 文A button, the card and Replace. The microphone is only used for the volume; the results are scripted, so nothing is sent anywhere. Skip it at any point, or reopen it later from **Tutorial…** in the tray menu. The tour needs no keys; when it ends Mami goes to the tray, and the first time you press the voice key it takes you to the place to add a speech key.

- **Translation**: under Translation → API, pick a provider and click Test connection. Out of the box Mami uses bilibili's **free** Index-Translate endpoint (no key needed, may return 504 at peak hours).
- **Voice**: turn voice input on under Dictation → General, pick a recognition provider and paste its key under API, and add a key on the Polishing page (without one, the raw text is pasted).

## Recommended endpoints

| For | Provider | Base URL | Model | Notes |
|---|---|---|---|---|
| Speech | Alibaba Cloud Model Studio | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `qwen-audio-3.1-asr-flash-streaming` | Streams while you talk; about 0.5 s after you stop. `qwen3-asr-flash` also works |
| Speech | StepFun | `https://api.stepfun.com/v1` | `stepaudio-3-asr-max` | Split at pauses, about 1 s |
| Polish | DeepSeek | `https://api.deepseek.com/v1` | `deepseek-flash` | Thinking switched off, 0.5–1 s. Model Studio `qwen-flash` or any OpenAI-compatible endpoint also works |
| Translate | Tencent TokenHub | `https://tokenhub.tencentmaas.com/v1` | `hy-mt2-pro` | Paid, stable, about 1 s; what I use daily |
| Translate | OpenRouter | `https://openrouter.ai/api/v1` | `tencent/hy-mt2-30b-a3b` | Same model family, pay as you go |
| Translate | bilibili (free) | `https://index-translate.bilibili.com/v1` | `Index-Translate-35B-A3B` | No key; flaky in the evening |

### Tip: translating prompts

Translation models given a multi-line prompt often try to carry it out instead of translating it. Put something like this in **Extra instructions**:

```
This is a prompt for an AI. Make the translation precise and unambiguous, natural and direct, not formal.
Keep the structure: line breaks, numbering, lists, headings, code blocks and placeholders like {name} or <tag>.
Do not translate code, commands, file paths, URLs, model names or product names.
Glossary: 划词 → text selection, 提示词 → prompt
```

## Where your data lives

Everything is in `%AppData%\Mami\`: `config.json` (settings; every API key is encrypted with Windows DPAPI, so only your account on this PC can read it), `learned.json` (vocabulary), `samples.jsonl` (long-form edit samples, clearable in settings), `history\` (daily JSONL) and `logs\mami.log`. Recordings are never written to disk and go only to the recognition endpoint you chose.

## Known limitations

- **WeChat 4.x** (Qt, self-drawn) exposes nothing about its input box. Selection translation can't tell the input box from the chat history there, and voice still pastes but can't learn from your edits.
- Auto-learning reads the input box through UI Automation, so apps that don't expose their text can't be learned from.
- Places without `Ctrl+C` / `Ctrl+V` (games, remote desktop) can't be read or pasted into. For apps running as administrator, Mami must run as administrator too.
- Only tested on Windows 11 at 125% scaling with one monitor. Translations of the UI beyond Chinese and English haven't been reviewed by native speakers yet; corrections welcome.

## Build

Requires Go 1.27.3+ (`GOTOOLCHAIN=auto` downloads it).

```bash
go test ./...
go tool mygo build      # → build/windows-amd64/Mami.exe + NSIS installer
```

Pushing a `v*` tag runs `.github/workflows/release.yml`, which tests, builds, and uploads the installer and portable exe to the release. The tag must match `version` in `mygo.json`.

- `Mami.exe -demo` draws the selection button and a sample card.
- `Mami.exe -demo-voice [-demo-style mercury]` plays the dictation pill's states with fake levels, without opening the microphone.
- The screenshots in `docs/` are rendered offscreen: `MAMI_DOCS=$PWD/docs go test ./internal/app -run TestSnapDocs`.
- Live API tests are skipped by default: `MAMI_ASR_LIVE=1` (StepFun), `MAMI_QWEN_KEY=sk-…` (Qwen).

### Code map

`main.go` at the root is only the entry point. The code is in `internal/app/` and `internal/i18n/` (UI strings: write `T("中文")` in code and add the other languages to the dictionaries; a missing one fails the tests).

| File | What it does |
|---|---|
| `app.go` / `run.go` | Startup, tray, shortcuts, single instance and the `-voice` command, selection button and card |
| `views.go` / `settings.go` | MyGo UI: button, card, settings pages |
| `hook.go` / `keycapture.go` | Low-level mouse and keyboard hooks on their own OS thread, shortcut recording |
| `uia.go` | UI Automation over raw COM: is the focus editable, where is the selection, text around the caret |
| `worker.go` / `clipboard.go` | Message-window thread: clipboard, simulated `Ctrl+C` / `Ctrl+V` |
| `translate.go` / `config.go` / `dpapi.go` | Translation requests, settings, key encryption |
| `voice_windows.go` / `voice_keys.go` / `voice_record_windows.go` | Dictation flow, trigger-key state machine, waveIn recording, the pill window |
| `voice_islands_windows.go` / `voice_sound_windows.go` | Island styles, sounds synthesized in memory |
| `asr.go` / `asr_stream.go` | Recognition: StepFun SSE, OpenAI-compatible chat, Qwen 3.1 WebSocket streaming; splitting at pauses |
| `polish.go` | Polish, and the guard that only lets sound-alike swaps through when a learned fix backs them |
| `learn.go` / `learn_windows.go` / `samples.go` | Vocabulary, auto-learning (checked with pinyin), long-form edit samples |
| `history*.go` | History, stats, heatmap and the History page |
| `guide.go` | First-launch tutorial pages inside the settings window and their demos |

Windows, tray, global shortcuts, single instance and autostart come from MyGo. The hooks, recording, cross-process selection reading and non-activating windows are plain Win32 via `syscall`, no cgo.

## License

[MIT](LICENSE). Provider logos from [LobeHub Icons](https://github.com/lobehub/lobe-icons) (MIT).
