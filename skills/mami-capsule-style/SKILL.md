---
name: mami-capsule-style
description: Create or modify a custom voice-input capsule ("胶囊样式") for the Mami desktop app as a web page (HTML/CSS/JS, any animation library such as mo.js, GSAP, Lottie, Canvas, WebGL). Use when the user wants a new Mami recording pill/orb/bubble look, wants to restyle or animate the capsule, or mentions Mami 胶囊 / 自定义样式 / web style / voice_style "web:".
---

# Mami 胶囊样式（网页版）

Mami 按住热键说话时，屏幕上会出现一个“胶囊”。胶囊可以换成你写的一张**透明网页**：Mami 把状态推给页面，页面想怎么画都行，按钮通过 `window.mami` 交还给 Mami。

## 一个样式就是一个文件夹

```
%APPDATA%\Mami\styles\<folder>\      ← 设置 → 自定义 →「打开样式文件夹」
  index.html      必需。入口页面
  manifest.json   可选。尺寸、位置、开发模式
  mami-sim.js     建议。浏览器里预览用的模拟器（在 Mami 里自动失效）
  preview.html    建议。预览台：按真实尺寸放进“桌面”、手动切状态、看可点区域
  preview.png     建议。设置「自定义」里展示的样子：录音状态、透明底、裁到只剩胶囊
  ...             你自己的 js/css/图片/字体，全部用相对路径引用
```

文件夹名只能是一层名字（不含 `.` `/` `\` `:`）。放进去以后，「设置 → 自定义」的列表和「语音识别 → 外观 → 浮窗样式」下拉里都会多出这一项；也可以直接在 `config.json` 里写 `"voice_style": "web:<folder>"`。

最快的起步：「设置 → 自定义 → 新建样式」，就是复制一份 `template/`，开发模式已经打开。

## manifest.json

| 字段 | 默认 | 说明 |
|---|---|---|
| `name` | 文件夹名 | 设置里显示的名字 |
| `width`, `height` | 260, 110 | 窗口大小，DIP（CSS 像素）。范围 40–1600 × 40–900。留出动画溢出的空间，不用的地方鼠标会穿透 |
| `position` | `"bottom"` | `bottom`（工作区底部居中）、`top`、`center` |
| `offsetX`, `offsetY` | 0 | 再挪一点，正数往右/往下 |
| `dev` | false | 开发模式：文件夹里任何文件一改，页面自动重载；`manifest.json` 一改，窗口重建；可以开 DevTools。发布前改回 false |

## 页面 API：`window.mami`

Mami 在页面脚本之前注入 `window.mami`（浏览器里由 `mami-sim.js` 模拟，接口相同）。

```js
const off = mami.onState((s) => { /* 每次状态变化都会调 */ });
mami.action("cancel");   // 按钮
mami.state;              // 最近一次的状态
mami.version;            // 1
```

### 状态 `s`

| 字段 | 说明 |
|---|---|
| `kind` | `"recording"` 录音中 · `"recognizing"` 识别/润色中 · `"text"` 一句话（结果或提示） · `"hidden"` 收起 |
| `level` | 录音时最新音量，已归一到 0–1，直接拿来驱动缩放/亮度 |
| `levels` | 最近约 48 个原始音量（0–0.1 左右），旧的在前，画波形用 |
| `live` | 边说边出的字（用户开了实时出字才有） |
| `text` | `kind=text` 时要显示的话：识别结果（没地方贴时）或提示，如“没听清，请再说一次” |
| `hint` | 附加的一句提示，如“润色失败，保留原话” |
| `copy` / `retry` / `undo` | 为真时要显示对应按钮 |
| `tone` | 设置里选的底色：`dark` / `light` / `auto`（auto 用 `prefers-color-scheme`） |

节奏：录音时大约每 50ms 来一次（音量变化）；其它状态只在变化时来。正常流程 `recording → recognizing → hidden`（文字已经贴进输入框）；贴不了、出错或有提示时是 `→ text`，之后再 `hidden`。同一状态可能连续来多次，按“当前状态”渲染，不要按“事件次数”写逻辑。

`hidden` 来了以后约 **320ms** 窗口被藏起，退场动画要在这之内放完。下一次说话时窗口直接重新出现，页面不会重新加载，所以出场动画要在 `hidden → recording` 时自己触发。

### 动作 `mami.action(name)`

| name | 什么时候给 |
|---|---|
| `cancel` | 录音或识别中：放弃这次 |
| `stop` | 录音中：说完了，开始识别（和再按一次热键一样） |
| `close` | `kind=text`：关掉这条 |
| `copy` | `copy` 为真：复制结果，胶囊随后自己收起 |
| `retry` | `retry` 为真：用刚才的录音重新识别 |
| `undo` | `undo` 为真：撤销刚才的改写 |

### 可点区域：`data-mami-hit`

窗口是一个矩形，但只有带 `data-mami-hit` 属性的元素接鼠标，别处的点击穿透到下面的程序。Mami 每帧读取这些元素的位置，看不见的（`display:none`、`visibility:hidden`、透明度 0、`pointer-events:none`）不算。

- 页面里**一个都没标**时，整个窗口都接鼠标。窗口大于胶囊本身时，一定要标。
- 一般给胶囊外壳标就够了（按钮在里面）。只做动画、不需要点的装饰不要标。

## 必须遵守

- `html, body` 背景透明。WebView2 只认全透明或不透明，半透明阴影/光晕画在元素上就行。
- 每个状态都要有出口：`recording`/`recognizing` 能取消，`text` 能关，`copy`/`retry`/`undo` 为真时有按钮。
- 不要抢焦点：不放 `<input>`、不调 `focus()`、不用 `alert`。
- 不要依赖网络：库（mo.js、GSAP、Lottie…）下载下来放进文件夹，用 `<script src>` / `<img src>` 相对路径加载。页面跑在 `file://` 下，别用 `fetch` 读本地文件（可能被拦），数据直接写进 js。
- 省电：动画用 CSS 或 `requestAnimationFrame`，不要开高频 `setInterval`。页面在后台常驻（为了按热键立刻出现），`hidden` 以后停掉自己的循环。
- 文字可能很长（一整段话），要截断（`text-overflow: ellipsis`）或滚动，`title` 放全文。

## 开发和测试

1. **浏览器里**：双击 `index.html`，`mami-sim.js` 会循环播放各个状态。加 `?state=text`（或 `recording` `recognizing` `retry` `hint` `undo`）停在一个状态，`?tone=light` 看浅色，`?hits` 描出可点区域，`?live` 带实时出字。
2. **预览台**：在样式文件夹里 `python -m http.server 8765`，打开 `http://localhost:8765/preview.html`。按 manifest 的尺寸放到“桌面”上，底部按钮切状态，红框是可点区域，点胶囊按钮会显示动作名。
3. **在 Mami 里**：manifest 里 `"dev": true`，在设置里选上这个样式，说一句话。改文件后自动重载。不想开麦也能看：
   `"%LOCALAPPDATA%\Programs\Mami\Mami.exe" -demo-voice -demo-style web:<folder>`（先退出正在运行的 Mami；会用假音量播一遍各状态然后退出）。
4. 日志在托盘菜单“打开日志”：`voice web: <folder> loaded …ms`、`first frame …ms`、加载失败原因都在里面。

## 检查清单（交付前）

- [ ] 浏览器里 recording / recognizing / text / retry / hint / undo / hidden 都看过，深浅色都看过
- [ ] 长句子不撑破、不溢出窗口
- [ ] `data-mami-hit` 只覆盖该点的地方（`?hits` 或预览台看红框）
- [ ] 退场动画 ≤ 300ms；再次出现时出场动画会重播
- [ ] 放了 `preview.png`：`msedge --headless=new --force-device-scale-factor=2 --default-background-color=00000000 --window-size=<宽>,<高> --screenshot=preview.png "file:///…/index.html?state=recording&bare"`，再按不透明像素裁边（窗口窄于 520 时 Edge 会撑宽，开 900 宽再裁）
- [ ] 没有外链资源；`manifest.json` 的 `dev` 改回 false
