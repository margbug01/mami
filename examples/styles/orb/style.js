// 光球：药丸中间一颗渐变光球跟着音量缩放，声音冲高时炸开一圈小粒子；出现时散开一圈光环。

var parts = []   // 粒子：x y vx vy 生命 颜色
var lastBurst = 0
var shownAt = -1
var colors = ["#8fd8ff", "#4d7cff", "#8b4dff", "#ff7ad9"]

function draw(g, s) {
  var cx = s.w / 2, cy = s.h - 30
  if (s.kind === "hidden") shownAt = -1
  else if (shownAt < 0) shownAt = s.t
  g.alpha(1 - s.leave)

  // 出场光环
  var ring = s.t - shownAt
  if (shownAt >= 0 && ring < 0.5) g.ring(cx, cy, 14 + ring * 70, 6 * (1 - ring * 2) + 0.5, g.rgba(143, 216, 255, 1 - ring * 2))

  var H = 40, B = 28, text = s.kind === "text" || (s.kind === "recognizing" && s.slow) // 识别太慢：换成一行「响应较慢，可取消」
  var msg = s.hint ? (s.text ? s.text + " · " + s.hint : s.hint) : s.text
  var W = text ? Math.min(g.measure(msg, 13) + 24 + 2 * (B + 10), s.w - 8) : 132
  var x0 = cx - W / 2, y0 = cy - H / 2 + s.leave * 12
  g.shadow(x0, y0, W, H, H / 2, 20, "#00000045", 0, 6, 0)
  g.rect(x0, y0, W, H, s.dark ? "#16161ceb" : "#fffffff5", H / 2)
  var fg = s.dark ? "#f2f2f5" : "#1d1d22"

  function btn(bx, action, icon) {
    var by = y0 + (H - B) / 2
    g.circle(bx + B / 2, by + B / 2, B / 2, s.hover === action ? "#8f8f9a55" : "#8f8f9a2e")
    g.icon(icon, bx + B / 2, by + B / 2, 13, fg)
    g.button(bx, by, B, B, action)
  }
  btn(x0 + 6, s.kind === "text" ? "close" : "cancel", "x")
  var right = s.kind === "recording" ? ["stop", "check"] : s.copy ? ["copy", "copy"] : s.retry ? ["retry", "retry"] : s.undo ? ["undo", "undo"] : null
  if (right && s.kind !== "recognizing") btn(x0 + W - 6 - B, right[0], right[1])

  if (text) {
    g.text(msg, cx, cy + s.leave * 12, 13, fg, "center", W - 2 * (B + 14))
    return
  }

  // 光球：录音时跟着音量，识别时转圈呼吸
  var r = s.kind === "recording" ? 9 + s.level * 7 : 8 + Math.sin(s.t * 5) * 1.5
  var angle = s.kind === "recognizing" ? (s.t * 360) % 360 : 135
  g.circle(cx, cy + s.leave * 12, r + 6, g.rgba(90, 168, 255, 0.18 + s.level * 0.2))
  g.circle(cx, cy + s.leave * 12, r, g.gradient("#bfe9ff", "#6b4dff", angle))
  g.circle(cx - r * 0.3, cy - r * 0.35 + s.leave * 12, r * 0.32, "#ffffff8c")

  // 声音冲高：炸开一圈
  if (s.kind === "recording" && s.level > 0.72 && s.t - lastBurst > 0.22) {
    lastBurst = s.t
    for (var i = 0; i < 7; i++) {
      var a = (i / 7) * Math.PI * 2 + Math.random() * 0.4
      parts.push({ x: cx, y: cy, vx: Math.cos(a) * 70, vy: Math.sin(a) * 45, life: 1, c: colors[i % colors.length] })
    }
  }
  var next = []
  for (var j = 0; j < parts.length; j++) {
    var p = parts[j]
    p.life -= 1 / 36
    if (p.life <= 0) continue
    var k = 1 - p.life
    g.alpha((1 - s.leave) * p.life)
    g.circle(p.x + p.vx * k * 0.6, p.y + p.vy * k * 0.6, 3 * p.life + 0.5, p.c)
    next.push(p)
  }
  parts = next
  g.alpha(1)
}
