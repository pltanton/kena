(function () {
  var Ctx = window.AudioContext || window.webkitAudioContext;
  var body = document.body;
  var ctx = null;
  var master = null;
  var doneOnce = false;

  function choice() { return body.dataset.sound || "chime"; }

  function context() {
    if (!Ctx || choice() === "off") return null;
    if (!ctx) {
      try {
        ctx = new Ctx();
      } catch (err) {
        Ctx = null;
        return null;
      }
      master = ctx.createGain();
      master.gain.value = 0.8;
      master.connect(ctx.destination);
    }
    if (ctx.state === "suspended") ctx.resume().catch(function () {});
    return ctx;
  }

  function tone(c, o) {
    var t = c.currentTime + 0.01 + (o.at || 0);
    var osc = c.createOscillator();
    var g = c.createGain();
    osc.type = o.type;
    osc.frequency.setValueAtTime(o.f, t);
    if (o.to) osc.frequency.exponentialRampToValueAtTime(o.to, t + (o.glide || o.d));
    g.gain.setValueAtTime(0.0001, t);
    g.gain.exponentialRampToValueAtTime(o.v, t + (o.attack || 0.006));
    g.gain.exponentialRampToValueAtTime(0.0001, t + o.d);
    osc.connect(g);
    g.connect(master);
    osc.start(t);
    osc.stop(t + o.d + 0.05);
  }

  var voices = {
    chime: function (c, f, at, d) {
      tone(c, {type: "sine", f: f, at: at, d: d * 0.9, v: 0.22});
      tone(c, {type: "sine", f: f * 2, at: at, d: d * 0.45, v: 0.05});
    },
    coin: function (c, f, at, d) {
      tone(c, {type: "square", f: f, at: at, d: d * 0.5, v: 0.07, attack: 0.002});
    },
    pop: function (c, f, at, d) {
      tone(c, {type: "sine", f: f * 1.5, to: f, glide: 0.02, at: at, d: d * 0.25, v: 0.35, attack: 0.002});
      tone(c, {type: "triangle", f: f * 3, at: at, d: 0.04, v: 0.04, attack: 0.001});
    },
  };

  var tunes = {
    correct: {
      chime: [[880, 0, 0.8], [1318.5, 0.1, 1]],
      coin: [[987.8, 0, 0.16], [1318.5, 0.08, 0.9]],
      pop: [[784, 0, 0.6], [1046.5, 0.09, 0.7]],
    },
    done: [[523.25, 0, 0.8], [659.25, 0.11, 0.8], [784, 0.22, 0.8], [1046.5, 0.33, 1.6]],
  };

  function play(name, sound) {
    sound = sound || choice();
    var voice = voices[sound];
    var c = voice && context();
    if (!c) return;
    var notes = name === "done" ? tunes.done : tunes.correct[sound];
    notes.forEach(function (n) { voice(c, n[0], n[1], n[2]); });
  }

  ["pointerdown", "touchend", "keydown"].forEach(function (type) {
    document.addEventListener(type, function () { context(); }, true);
  });

  body.addEventListener("htmx:afterSettle", function (e) {
    var cfg = e.detail && e.detail.requestConfig;
    var el = e.target;
    if (!cfg || el.id !== "card") return;
    if (el.querySelector(".card.done.complete")) {
      if (!doneOnce) play("done");
      doneOnce = true;
      return;
    }
    var path = new URL(cfg.path, location.href).pathname;
    if (cfg.verb === "post" && /^\/t\/\d+\/check$/.test(path) && el.querySelector(".card.result.ok .verdict .tick")) play("correct");
  });

  document.addEventListener("click", function (e) {
    var b = e.target.closest(".sound-switch button[name=sound]");
    if (!b) return;
    e.preventDefault();
    body.dataset.sound = b.value;
    document.querySelectorAll(".sound-switch button[name=sound]").forEach(function (o) {
      o.setAttribute("aria-pressed", String(o.value === b.value));
    });
    play("correct", b.value);
    fetch("/sound", {method: "POST", headers: {"HX-Request": "true"}, body: new URLSearchParams({sound: b.value})}).then(function (r) {
      if (!r.ok) throw new Error(r.status);
    }).catch(function () {
      if (window.kenaSay) window.kenaSay(body.dataset.msgNetwork);
    });
  });
})();
