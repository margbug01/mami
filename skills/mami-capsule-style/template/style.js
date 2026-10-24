// Mami 胶囊样式模板：一颗药丸，录音时一排波柱，识别时三个点，有话要说时显示文字和按钮。
// Mami 每帧调一次 draw(g, s)：s 是当前状态，g 是画笔。接口见 SKILL.md。

function draw(g, s) {
  var fg = s.dark ? "#f2f2f5" : "#1d1d22"
  var bg = s.dark ? "#18181ef0" : "#fffffff5"
  var chip = s.dark ? "#ffffff1f" : "#0000000f"
  var chipHover = s.dark ? "#ffffff3a" : "#00000024"
  var accent = s.dark ? "#7fb2ff" : "#3a6df0"

  // 退场：沉下去、淡掉（s.leave 从 0 走到 1，约 0.3 秒）
  g.alpha(1 - s.leave)
  var H = 40, B = 28
  var y = s.h - H - 12 + s.leave * 10

  // 右边要放哪些按钮
  var right = []
  if (s.kind === "recording") right.push(["stop", "check"])
  if (s.kind === "text" && s.copy) right.push(["copy", "copy"])
  if (s.kind === "text" && s.retry) right.push(["retry", "retry"])
  if (s.kind === "text" && s.undo) right.push(["undo", "undo"])
  var left = s.kind === "text" ? ["close", "x"] : ["cancel", "x"]

  var message = s.hint ? (s.text ? s.text + " · " + s.hint : s.hint) : s.text
  // 有话要说：结果，或者识别太慢时的「响应较慢，可取消」（s.slow 时在 s.hint 里）
  var talking = s.kind === "text" || (s.kind === "recognizing" && s.slow)
  var middle = 80
  if (talking) middle = Math.min(g.measure(message, 13) + 16, s.w - 40 - (1 + right.length) * (B + 6))
  var W = 12 + B + middle + right.length * (B + 6) + 6
  var x = (s.w - W) / 2

  g.shadow(x, y, W, H, H / 2, 16, "#00000040", 0, 4, 0)
  g.rect(x, y, W, H, bg, H / 2)

  function button(bx, b) {
    var by = y + (H - B) / 2
    g.circle(bx + B / 2, by + B / 2, B / 2, s.hover === b[0] ? chipHover : chip)
    g.icon(b[1], bx + B / 2, by + B / 2, 13, fg)
    g.button(bx, by, B, B, b[0])
  }
  button(x + 6, left)
  for (var i = 0; i < right.length; i++) button(x + W - 6 - (right.length - i) * (B + 6) + 6, right[i])

  var mx = x + 6 + B + 8, cy = y + H / 2
  if (s.kind === "recording") {
    var n = 12, step = (middle - 16) / n
    for (var j = 0; j < n; j++) {
      var v = s.levels[s.levels.length - n + j] || 0
      var bh = 4 + v * 20
      g.rect(mx + j * step, cy - bh / 2, 3, bh, accent, 1.5)
    }
  } else if (talking) {
    g.text(message, mx, cy, 13, fg, "left", middle - 8)
  } else if (s.kind === "recognizing") {
    for (var k = 0; k < 3; k++) {
      var a = 0.3 + 0.7 * Math.max(0, Math.sin(s.t * 6 - k * 0.8))
      g.circle(mx + middle / 2 - 20 + k * 12, cy, 3, g.mix(bg, fg, a))
    }
  }
}
