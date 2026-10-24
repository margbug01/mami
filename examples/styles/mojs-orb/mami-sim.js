/* Mami 胶囊样式的模拟器：页面不在 Mami 里时，假装出 window.mami。
 * 在 Mami 里 window.mami 已经由 Mami 注入，这个文件什么都不做，留着也没关系。
 * - 直接用浏览器打开 index.html：自动循环播放 录音 → 识别中 → 出结果 → 收起。
 *   网址后面加 ?state=recording|recognizing|text|retry|hint|undo 停在某个状态，?tone=light 换浅色，?hits 描出可点区域。
 * - 放在 preview.html 的框里：由 preview.html 的控制台驱动。
 */
(function () {
  if (window.mami) return;
  var handlers = [], last = null, ctl = null;
  try { if (window.parent !== window && window.parent.__mamiPreview) ctl = window.parent.__mamiPreview; } catch (e) {}
  function emit(s) {
    last = s;
    handlers.slice().forEach(function (h) { try { h(s); } catch (e) { console.error(e); } });
  }
  window.mami = {
    version: 1,
    platform: "preview",
    get state() { return last; },
    onState: function (fn) {
      handlers.push(fn);
      if (last) fn(last);
      return function () { handlers = handlers.filter(function (h) { return h !== fn; }); };
    },
    action: function (name) {
      console.log("[mami] action:", name);
      if (ctl) ctl.onAction(name, window);
      else if (name === "cancel" || name === "close" || name === "copy") emit({ kind: "hidden", level: 0, tone: tone });
      else if (name === "stop") emit({ kind: "recognizing", level: 0, tone: tone });
    }
  };
  if (ctl) { ctl.attach(emit, window); return; }

  var q = new URLSearchParams(location.search), tone = q.get("tone") || "dark", pin = q.get("state");
  var bg = document.createElement("style");
  bg.textContent = "html{background:radial-gradient(circle at 30% 20%,#3b4a7a,#141826 70%) fixed !important;min-height:100%}";
  document.documentElement.appendChild(bg);
  var sample = "帮我把这个接口改成用 Go 写，然后四点开会。";
  function fake(kind, t) {
    var raw = [];
    for (var i = 0; i < 48; i++) {
      var x = t - (47 - i) * 0.05;
      raw.push(x > 1.6 && x < 2.2 ? 0.002 : 0.06 * Math.abs(Math.sin(x * 3) + 0.6 * Math.sin(x * 7.3)));
    }
    var s = { kind: kind, level: 0, tone: tone };
    if (kind === "recording") {
      s.levels = raw;
      s.level = Math.min(1, Math.pow(raw[raw.length - 1] / 0.08, 0.6));
      if (q.has("live")) s.live = sample.slice(0, Math.max(0, Math.floor((t - 0.8) * 8)));
    }
    if (kind === "text") { s.text = sample; s.copy = true; }
    if (kind === "retry") { s.kind = "text"; s.text = "识别未完成"; s.retry = true; }
    if (kind === "hint") { s.kind = "text"; s.text = "润色失败，保留原话"; }
    if (kind === "undo") { s.kind = "text"; s.text = "已按你说的改写"; s.undo = true; }
    return s;
  }
  var t0 = performance.now();
  setInterval(function () {
    var t = (performance.now() - t0) / 1000;
    if (pin) return emit(fake(pin, t));
    var c = t % 9;
    emit(fake(c < 4.5 ? "recording" : c < 5.7 ? "recognizing" : c < 8.4 ? "text" : "hidden", c));
  }, 50);

  if (q.has("hits")) {
    var box = document.createElement("div");
    box.style.cssText = "position:fixed;inset:0;pointer-events:none;z-index:2147483647";
    document.documentElement.appendChild(box);
    (function draw() {
      box.innerHTML = "";
      document.querySelectorAll("[data-mami-hit]").forEach(function (el) {
        var b = el.getBoundingClientRect(), d = document.createElement("div");
        d.style.cssText = "position:fixed;outline:1px dashed #ff3b6b;left:" + b.left + "px;top:" + b.top + "px;width:" + b.width + "px;height:" + b.height + "px";
        box.appendChild(d);
      });
      requestAnimationFrame(draw);
    })();
  }
})();
