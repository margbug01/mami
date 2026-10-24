---
name: mami-capsule-style
description: Create or modify a custom voice-input capsule ("胶囊样式") for the Mami desktop app. A style is a folder with style.js (a draw(g, s) function Mami calls every frame, drawing with Mami's native painter, not a browser) and an optional manifest.json. Use when the user wants a new Mami recording pill/orb/bubble look, wants to restyle or animate the capsule, or mentions Mami 胶囊 / 自定义样式 / custom style / voice_style "custom:".
---

# Mami 自定义胶囊样式

按住热键说话时，屏幕上会出现一个“胶囊”。自定义样式就是一段 JavaScript：Mami 每帧调一次 `draw(g, s)`，`s` 是当前状态，`g` 是画笔。Mami 用自己的原生画笔照着画，不开浏览器、不占额外内存，设置页里的预览也是同一段脚本实时画出来的。

## 一个样式就是一个文件夹

```
%APPDATA%\Mami\styles\<folder>\      ← 设置 → 自定义 →「打开样式文件夹」
  style.js        必需。定义全局函数 draw(g, s)
  manifest.json   可选。名字、窗口大小、位置
  *.png           可选。g.image 用的图片
```

文件夹名只能是一层名字（不含 `.` `/` `\` `:`）。放进去以后，「设置 → 自定义」和「语音识别 → 外观 → 浮窗样式」里都会出现它；`config.json` 里对应 `"voice_style": "custom:<folder>"`。改了 style.js、manifest.json 或 png，预览和胶囊半秒内自动重载。最快的起步：「设置 → 自定义 → 新建样式」，复制的就是 `template/`。

## manifest.json

```json
{ "name": "极光", "width": 560, "height": 170, "position": "bottom", "offsetX": 0, "offsetY": 0 }
```

`width`/`height` 是窗口大小（DIP，默认 260×110，范围 40–1600 × 40–900），留出动画溢出的地方，按钮以外都点击穿透。`position`：`bottom`（工作区底部居中，默认）、`top`、`center`；`offsetX/Y` 再挪一点。

## 状态 s

| 字段 | 说明 |
|---|---|
| `kind` | `recording` 录音中 · `recognizing` 识别/润色中 · `text` 有句话要显示 · `hidden` 正在退场 |
| `level` | 最新音量 0–1；`levels` 最近 48 个音量 0–1（100ms 一个，约 4.8 秒），旧的在前。刚开始录音时不满 48 个，可能是空数组 |
| `live` | 边说边出的字（用户开了实时出字才有） |
| `text` / `hint` | `kind=text` 时要显示的话 / 附加提示（如“润色失败，保留原话”）；可能只有 hint 没有 text |
| `slow` | `kind=recognizing` 时识别超过 8 秒还没结果，这时 `hint` 是“响应较慢，可取消”（已按用户语言翻译），要显示出来 |
| `copy` `retry` `undo` | 为真时要给对应按钮 |
| `dark` | 用户选的浮窗底色是深色 |
| `w` `h` | 窗口大小（同 manifest） |
| `t` | 秒，一直往前走，做动画用；`since` 进入当前 kind 多少秒 |
| `leave` | `kind=hidden` 时退场进度 0→1（约 0.3 秒），到 1 窗口藏起来 |
| `hover` | 鼠标停在哪个按钮上（它的 action），没有是 `""` |

流程通常是 `recording → recognizing → hidden`（字已经贴进输入框），贴不了、出错或有提示时是 `→ text → hidden`。同一状态会连续很多帧，按“当前状态”画就行。脚本里的全局变量会保留到下一帧，可以拿来做平滑、粒子；设置页的每种预览各跑一份脚本，和屏幕上的胶囊互不影响。

## 画笔 g

坐标是 DIP，窗口左上角 (0,0)。颜色写 `"#rgb"` `"#rrggbb"` `"#rrggbbaa"` `"rgb(r,g,b)"` `"rgba(r,g,b,a)"`，认不出的写法（比如 `"white"`）当透明；标着“填色”的地方也可以给 `g.gradient(...)`。

```js
g.rect(x, y, w, h, 填色, radius)            // 圆角矩形
g.strokeRect(x, y, w, h, color, radius, width)
g.circle(cx, cy, r, 填色)
g.ring(cx, cy, r, width, color)             // 圆环
g.poly([x0,y0, x1,y1, ...], 填色)            // 封闭多边形；曲线就多取点
g.polyline([x0,y0, ...], width, color)      // 折线
g.line(x0, y0, x1, y1, width, color)
g.shadow(x, y, w, h, radius, blur, color, dx, dy, spread)   // 盒子阴影，先画阴影再画盒子
g.text(str, x, y, size, color, align, maxWidth)   // 一行字（换行当空格），size 默认 13；y 是这行的竖直中线；align left|center|right；超宽加 …；返回画出的宽
g.measure(str, size)                        // 文字宽，size 默认 13
g.image("file.png", x, y, w, h)             // 文件夹里的 png
g.icon(name, cx, cy, size, color)           // 内置图标：x check copy retry undo
g.button(x, y, w, h, action)                // 一块能点的地方，见下
g.gradient(from, to, angle)                 // 线性渐变，angle 度数（90 从左到右，180 从上到下）
g.alpha(a)                                  // 之后画的东西整体透明度 0–1，每帧开头是 1
g.rgba(r, g, b, a)  g.mix(colorA, colorB, t) // 颜色小工具，返回颜色字符串
```

## 按钮：g.button(x, y, w, h, action)

只有按钮区域接鼠标，别处的点击穿透到下面的程序。按钮本身不画任何东西，样子自己画，悬停效果看 `s.hover`。action：

| action | 什么时候给 |
|---|---|
| `cancel` | 录音或识别中：放弃 |
| `stop` | 录音中：说完了（和再按一次热键一样） |
| `close` | `kind=text`：关掉 |
| `copy` / `retry` / `undo` | 对应的 `s.copy` / `s.retry` / `s.undo` 为真时 |

## 必须遵守

- 每个状态都有出口：`recording` 给 `cancel` 和 `stop`，`recognizing` 给 `cancel`，`text` 给 `close`，`copy/retry/undo` 为真时给对应按钮。
- `kind=hidden` 时用 `s.leave` 做退场（淡出/下沉），别画突兀的新东西。
- 一帧要快：几百个图形没问题。一帧超过 60ms 会被打断，连着 30 帧都超过 16ms 也算超时：这次说话先退回内置胶囊，下次说话再试。语法错、throw、少了 draw 这类错误要改了文件才再试。
- 别写死循环、大数组、复杂正则、超长字符串拼接：正则和字符串、数组的内置函数是原生代码，跑到一半打断不了。
- 文字可能很长（几百字），给 `maxWidth`。
- 没有 DOM、网络、文件、`setTimeout`；动画全靠 `s.t` / `s.since` 和自己的全局变量。`console.log` 写进 Mami 的日志（每次加载最多 100 行）。

## 测试

- 存文件后看「设置 → 自定义」：预览就是这段脚本实时画的录音状态；「语音识别 → 外观」选上它，能看到录音 / 识别 / 结果三种状态。预览不画的分支（重试、撤销、提示、识别太慢、刚开始录音、退场）也会各跑一帧检查。脚本或 manifest.json 出错时名字标红，悬停看原因。
- 真实效果：选上后按热键说一句。
- 日志（托盘菜单“打开日志”）里有 `capsule: loaded <folder>`、出错原因和 `console.log` 的输出。

## 交付前检查

- [ ] recording / recognizing（含 slow）/ text（含 copy、retry、undo、hint）/ hidden 都画得对，深浅色都看过
- [ ] 每个状态都有出口按钮，悬停有反馈
- [ ] 长句子不出窗口
- [ ] 退场在 0.3 秒内画完
