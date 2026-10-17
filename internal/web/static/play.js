(function () {
  // iOS opens the keyboard only on a focus made during a user gesture, so focus moves to the keeper while a card loads and never to a tapped control.
  var keeper = document.getElementById("kbd-keeper");
  if (!keeper) return;
  var root = document.documentElement;
  var card = document.getElementById("card");
  var main = document.getElementById("focus-main");
  var menu = document.getElementById("focus-menu");
  var vv = window.visualViewport;
  function fit() {
    if (!vv) return;
    root.style.setProperty("--vvh", vv.height + "px");
    root.style.setProperty("--vvt", vv.offsetTop + "px");
  }
  if (vv) {
    vv.addEventListener("resize", fit);
    vv.addEventListener("scroll", fit);
    fit();
  }
  window.addEventListener("scroll", function () {
    if (window.scrollY !== 0) window.scrollTo(0, 0);
  });
  function target() {
    return card.querySelector('input[name="a"]:not([readonly])') || keeper;
  }
  function holdFocus(e) {
    if (e.target.closest("input:not([readonly]), textarea")) return;
    e.preventDefault();
  }
  var info = document.getElementById("info-sheet");
  [main, info].forEach(function (el) {
    el.addEventListener("pointerdown", holdFocus);
    el.addEventListener("mousedown", holdFocus);
  });
  document.addEventListener("info:close", function () { target().focus({preventScroll: true}); });
  document.body.addEventListener("htmx:beforeRequest", function (e) {
    if (card.contains(e.detail.elt)) keeper.focus({preventScroll: true});
  });
  document.body.addEventListener("htmx:afterSettle", function (e) {
    if (e.detail.target !== card) return;
    var el = card.querySelector('input[name="a"]:not([readonly])') || (card.querySelector("button[data-next]") && keeper) || card.querySelector(".done a");
    if (el) el.focus({preventScroll: true});
    main.scrollTop = 0;
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && menu.open) {
      menu.open = false;
      return;
    }
    if (e.key !== "Enter" || e.repeat || e.isComposing) return;
    if (e.target !== keeper && e.target !== document.body && !card.contains(e.target)) return;
    var next = card.querySelector("button[data-next]");
    if (!next) return;
    e.preventDefault();
    next.click();
  });
  menu.addEventListener("toggle", function () {
    if (!menu.open) target().focus({preventScroll: true});
  });
  menu.querySelector("[data-close]").addEventListener("click", function () { menu.open = false; });
  document.addEventListener("click", function (e) {
    if (menu.open && !menu.contains(e.target)) menu.open = false;
  });

  var bar = document.querySelector(".focus-bar");
  var reduce = window.matchMedia("(prefers-reduced-motion: reduce)");
  var q = new URLSearchParams(location.search);
  var key = "kena:nav:" + location.pathname + "?" + (q.get("filter") || "all") + "&" + (q.get("shuffle") === "1" ? q.get("seed") : "");
  var hist = {ids: [], at: -1};
  try {
    var saved = JSON.parse(sessionStorage.getItem(key));
    if (saved && Array.isArray(saved.ids) && saved.at < saved.ids.length) hist = saved;
  } catch (err) {}
  var prevFor = {};
  var pending = null;
  var busy = false;
  function save() {
    try { sessionStorage.setItem(key, JSON.stringify(hist)); } catch (err) {}
  }
  function navs() {
    var pos = document.getElementById("focus-pos");
    return {prev: pos.querySelector('[data-nav="prev"]'), next: pos.querySelector('[data-nav="next"]')};
  }
  function shown() {
    var n = navs().next;
    return n && n.dataset.id || "";
  }
  function serverPrev(id, prev) {
    return (id && prevFor[id]) || (prev && prev.dataset.prev) || "";
  }
  function sync() {
    var b = navs();
    if (!b.prev || !b.next) return;
    var id = shown();
    if (id) {
      if (b.prev.dataset.prev) prevFor[id] = b.prev.dataset.prev;
      if (pending && pending.front) {
        hist.ids.unshift(id);
        hist.at = 0;
      } else if (pending && pending.at != null && hist.ids[pending.at] === id) {
        hist.at = pending.at;
      } else if (hist.ids[hist.at] !== id) {
        hist.ids = hist.ids.slice(0, hist.at + 1);
        hist.ids.push(id);
        if (hist.ids.length > 200) hist.ids.shift();
        hist.at = hist.ids.length - 1;
      }
      save();
    }
    var canBack = id ? hist.at > 0 || !!serverPrev(id, b.prev) : hist.at >= 0 || !!b.prev.dataset.prev;
    if (canBack) b.prev.removeAttribute("aria-disabled");
    else b.prev.setAttribute("aria-disabled", "true");
  }
  function cardURL(base, id) {
    return base + (base.indexOf("?") < 0 ? "?" : "&") + "id=" + encodeURIComponent(id);
  }
  function go(url, dir, at, front) {
    if (busy) return;
    busy = true;
    pending = {dir: dir, at: at, front: front};
    keeper.focus({preventScroll: true});
    htmx.ajax("GET", url, {target: card, swap: "innerHTML show:none"}).catch(function () {
      busy = false;
      pending = null;
    });
  }
  function back() {
    var b = navs();
    if (!b.prev) return;
    var id = shown();
    if (!id) {
      if (hist.at >= 0) go(cardURL(b.prev.dataset.card, hist.ids[hist.at]), -1, hist.at, false);
      else if (b.prev.dataset.prev) go(cardURL(b.prev.dataset.card, b.prev.dataset.prev), -1, null, false);
    } else if (hist.at > 0 && hist.ids[hist.at] === id) {
      go(cardURL(b.prev.dataset.card, hist.ids[hist.at - 1]), -1, hist.at - 1, false);
    } else if (serverPrev(id, b.prev)) {
      go(cardURL(b.prev.dataset.card, serverPrev(id, b.prev)), -1, null, true);
    }
  }
  function forward() {
    var b = navs();
    var id = shown();
    if (!id) return;
    if (hist.ids[hist.at] === id && hist.at < hist.ids.length - 1) {
      go(cardURL(b.prev.dataset.card, hist.ids[hist.at + 1]), 1, hist.at + 1, false);
    } else {
      go(b.next.dataset.next, 1, null, false);
    }
  }
  function sheetOpen() {
    return menu.open || (info && !info.hidden);
  }
  document.body.addEventListener("htmx:afterSettle", function (e) {
    if (e.detail.target !== card) return;
    var p = pending;
    sync();
    pending = null;
    busy = false;
    if (p && p.dir && !reduce.matches && card.animate) {
      card.animate([{transform: "translateX(" + p.dir * 40 + "px)", opacity: 0}, {transform: "none", opacity: 1}], {duration: 180, easing: "ease-out"});
    }
  });
  document.body.addEventListener("htmx:afterRequest", function (e) {
    if (e.detail.failed && pending) {
      busy = false;
      pending = null;
    }
  });
  bar.addEventListener("pointerdown", function (e) {
    if (e.target.closest("[data-nav]")) e.preventDefault();
  });
  bar.addEventListener("mousedown", function (e) {
    if (e.target.closest("[data-nav]")) e.preventDefault();
  });
  bar.addEventListener("click", function (e) {
    var b = e.target.closest("[data-nav]");
    if (!b) return;
    e.preventDefault();
    if (b.dataset.nav === "prev") back();
    else forward();
  });
  document.addEventListener("keydown", function (e) {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey || e.isComposing || sheetOpen()) return;
    if (e.target !== keeper && e.target.closest && e.target.closest("input:not([readonly]), textarea, select, [contenteditable]")) return;
    e.preventDefault();
    if (e.key === "ArrowLeft") back();
    else forward();
  });
  var touch = null;
  main.addEventListener("touchstart", function (e) {
    touch = null;
    if (e.touches.length !== 1 || sheetOpen() || e.target.closest("input, textarea")) return;
    var t = e.touches[0];
    if (t.clientX < 20 || t.clientX > window.innerWidth - 20) return;
    touch = {x: t.clientX, y: t.clientY, at: e.timeStamp};
  }, {passive: true});
  main.addEventListener("touchmove", function (e) {
    if (e.touches.length !== 1) touch = null;
  }, {passive: true});
  main.addEventListener("touchcancel", function () { touch = null; }, {passive: true});
  main.addEventListener("touchend", function (e) {
    var s = touch;
    touch = null;
    if (!s || e.touches.length || sheetOpen()) return;
    var t = e.changedTouches[0];
    var dx = t.clientX - s.x;
    var dy = t.clientY - s.y;
    if (Math.abs(dx) < 60 || Math.abs(dx) <= 2 * Math.abs(dy) || e.timeStamp - s.at >= 600) return;
    if (dx < 0) forward();
    else back();
  }, {passive: true});
  sync();
})();
