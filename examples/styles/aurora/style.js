// 极光：一整条极光跟着声音起伏，两头收细；中间一颗半透明的胶囊放按钮和字。只有按钮接鼠标，极光那片点击穿透。

var level = 0 // 平滑过的音量，极光不跟着每一帧抖

var ribbons = [
  { c: [120, 220, 255], f: 0.011, s: 0.9, p: 0, y: 0.66 },
  { c: [150, 110, 255], f: 0.008, s: -0.6, p: 2, y: 0.72 },
  { c: [255, 120, 210], f: 0.014, s: 1.3, p: 4, y: 0.78 },
  { c: [110, 255, 200], f: 0.006, s: 0.4, p: 1, y: 0.62 },
]

function draw(g, s) {
  var target = s.kind === "recording" ? s.level : 0
  level += (target - level) * 0.18
  var calm = s.kind === "text" ? 0.3 : s.kind === "recognizing" ? 0.55 : 1
  g.alpha(1 - s.leave)

  for (var i = 0; i < ribbons.length; i++) {
    var r = ribbons[i]
    var amp = (8 + level * 40) * calm
    var base = s.h * r.y
    var top = [], bottom = []
    for (var x = 0; x <= s.w; x += 8) {
      var edge = Math.sin((x / s.w) * Math.PI) // 两头收细，像一束光
      var wave = amp * Math.sin(x * r.f + s.t * r.s + r.p) + amp * 0.5 * Math.sin(x * r.f * 2.3 - s.t * r.s * 1.7)
      var thick = edge * edge * (14 + level * 18)
      top.push(x, base - edge * wave - thick)
      bottom.unshift(x, base - edge * wave + thick * 0.4)
    }
    var a = 0.22 + level * 0.3
    if (s.kind === "recognizing") a = 0.18 + 0.12 * Math.sin(s.t * 3 + i)
    var col = g.rgba(r.c[0], r.c[1], r.c[2], a)
    g.poly(top.concat(bottom), g.gradient(col, g.rgba(r.c[0], r.c[1], r.c[2], 0), 180))
  }

  // 中间的胶囊
  var H = 40, B = 28
  var msg = s.kind === "text" ? (s.hint ? s.text + " · " + s.hint : s.text) : s.kind === "recognizing" ? "识别中…" : s.live || ""
  var inner = msg ? Math.min(g.measure(msg, 13) + 20, 300) : 90
  var right = s.kind === "recording" ? ["stop", "check"] : s.copy ? ["copy", "copy"] : s.retry ? ["retry", "retry"] : s.undo ? ["undo", "undo"] : null
  var W = 6 + B + inner + (right ? B + 6 : 6)
  var x0 = (s.w - W) / 2, y0 = s.h - H - 14 + s.leave * 12
  var fg = s.dark ? "#f4f7ff" : "#1b1d2b"
  g.shadow(x0, y0, W, H, H / 2, 24, "#14003c55", 0, 8, 0)
  g.rect(x0, y0, W, H, s.dark ? "#121628a8" : "#ffffffb0", H / 2)
  g.strokeRect(x0, y0, W, H, s.dark ? "#ffffff2e" : "#ffffffcc", H / 2, 1)

  function btn(bx, action, icon) {
    var by = y0 + (H - B) / 2
    g.circle(bx + B / 2, by + B / 2, B / 2, s.hover === action ? "#ffffff48" : "#ffffff22")
    g.icon(icon, bx + B / 2, by + B / 2, 13, fg)
    g.button(bx, by, B, B, action)
  }
  btn(x0 + 6, s.kind === "text" ? "close" : "cancel", "x")
  if (right) btn(x0 + W - 6 - B, right[0], right[1])
  if (msg) g.text(msg, x0 + 6 + B + inner / 2, y0 + H / 2, 13, fg, "center", inner - 12)
}

