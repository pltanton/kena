(function (global) {
  "use strict";
  var DORMANT_FOR = 30 * 24 * 60 * 60 * 1000;

  function queue(items, identity, now) {
    var out = {send: [], drop: [], adopt: []};
    items.forEach(function (a) {
      if (a.synced) return;
      if (!a.identity && identity) {
        a.identity = identity;
        out.adopt.push(a);
      }
      if (a.identity && a.identity === identity) out.send.push(a);
      else if (now - a.at > DORMANT_FOR) out.drop.push(a);
    });
    return out;
  }

  if (typeof module === "object" && module.exports) module.exports = {queue: queue};
  if (typeof window === "undefined" || !("indexedDB" in window) || !window.fetch || !window.Promise) return;

  var body = document.body;
  var strings = body.dataset;
  var authed = body.hasAttribute("data-auth");
  var identity = body.dataset.identity || "";
  var lang = document.documentElement.lang;
  var offlinePage = document.documentElement.dataset.offline;
  var OFFLINE_FOR = 30 * 1000;
  var PAGES = "kena-pages";
  var REFRESH_EVERY = 60 * 1000;
  var offlineUntil = offlinePage ? Date.now() + OFFLINE_FOR : 0;
  var dbp = null;

  function db() {
    if (!dbp) {
      dbp = new Promise(function (resolve, reject) {
        var req = indexedDB.open("kena", 1);
        req.onupgradeneeded = function () {
          req.result.createObjectStore("packs", {keyPath: "slug"});
          req.result.createObjectStore("attempts", {keyPath: "id"});
        };
        req.onsuccess = function () { resolve(req.result); };
        req.onerror = function () { reject(req.error); };
      });
    }
    return dbp;
  }

  function tx(store, mode, fn) {
    return db().then(function (d) {
      return new Promise(function (resolve, reject) {
        var t = d.transaction(store, mode);
        var out = fn(t.objectStore(store));
        t.oncomplete = function () { resolve(out && "result" in out ? out.result : undefined); };
        t.onerror = function () { reject(t.error); };
        t.onabort = function () { reject(t.error); };
      });
    });
  }

  function all(store) {
    return tx(store, "readonly", function (s) { return s.getAll(); });
  }

  function get(store, key) {
    return tx(store, "readonly", function (s) { return s.get(key); });
  }

  function put(store, value) {
    return tx(store, "readwrite", function (s) { s.put(value); });
  }

  function del(store, key) {
    return tx(store, "readwrite", function (s) { s.delete(key); });
  }

  function uuid() {
    if (crypto.randomUUID) return crypto.randomUUID();
    var b = crypto.getRandomValues(new Uint8Array(16));
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;
    var h = Array.from(b, function (x) { return (x + 0x100).toString(16).slice(1); }).join("");
    return h.slice(0, 8) + "-" + h.slice(8, 12) + "-" + h.slice(12, 16) + "-" + h.slice(16, 20) + "-" + h.slice(20);
  }

  function isOffline() {
    return !navigator.onLine || Date.now() < offlineUntil;
  }

  function markOffline() {
    offlineUntil = Date.now() + OFFLINE_FOR;
    paint();
  }

  function timed(url, opts, ms) {
    var ctl = window.AbortController ? new AbortController() : null;
    var timer = ctl && setTimeout(function () { ctl.abort(); }, ms || 8000);
    opts = Object.assign({credentials: "same-origin"}, opts || {});
    if (ctl) opts.signal = ctl.signal;
    return fetch(url, opts).finally(function () { clearTimeout(timer); });
  }

  function pageURLs(slug) {
    var base = "/s/" + encodeURIComponent(slug);
    return [base, base + "/play"];
  }

  function cachePages(slug) {
    if (!window.caches) return Promise.resolve();
    return caches.open(PAGES).then(function (c) { return c.addAll(pageURLs(slug)); }).catch(function () {});
  }

  function uncachePages(slug) {
    if (!window.caches) return Promise.resolve();
    return caches.open(PAGES).then(function (c) {
      return Promise.all(pageURLs(slug).map(function (u) { return c.delete(u); }));
    }).catch(function () {});
  }

  function merged(rec, flags) {
    return {auto: "auto" in flags ? !!flags.auto : !!(rec && rec.auto), saved: !!flags.saved || !!(rec && rec.saved)};
  }

  function fetchPack(slug, flags) {
    return get("packs", slug).then(function (rec) {
      var headers = {};
      if (rec) headers["If-None-Match"] = '"' + rec.version + '"';
      var fetched = Date.now();
      return timed("/s/" + encodeURIComponent(slug) + "/pack.json", {headers: headers}).then(function (r) {
        if (r.status === 404) return drop(slug).then(function () { return null; });
        var f = merged(rec, flags);
        if (r.status === 304 && rec) {
          rec.auto = f.auto;
          rec.saved = f.saved;
          return put("packs", rec).then(function () { return rec; });
        }
        if (!r.ok) throw new Error("pack " + r.status);
        return r.json().then(function (pack) {
          var next = {
            slug: slug, version: pack.version, lang: pack.lang, title: pack.title, fetched: fetched, pack: pack, user: authed,
            auto: f.auto, saved: f.saved,
          };
          return put("packs", next).then(function () { return prune(slug, fetched); }).then(function () {
            return cachePages(slug);
          }).then(function () { return next; });
        });
      });
    });
  }

  function prune(slug, before) {
    return all("attempts").then(function (list) {
      return Promise.all(list.filter(function (a) { return a.slug === slug && a.synced && a.at < before; }).map(function (a) {
        return del("attempts", a.id);
      }));
    });
  }

  function drop(slug) {
    return del("packs", slug).then(function () { return uncachePages(slug); });
  }

  function refresh(force) {
    if (isOffline()) return Promise.resolve();
    var stamp = "";
    var now = (authed ? "u" : "a") + Date.now();
    try { stamp = localStorage.getItem("kena:offline-refresh") || ""; } catch (err) {}
    if (!force && stamp.charAt(0) === now.charAt(0) && Date.now() - +stamp.slice(1) < REFRESH_EVERY) return Promise.resolve();
    return all("packs").then(function (packs) {
      var saved = packs.filter(function (p) { return p.saved; }).map(function (p) { return p.slug; });
      if (!authed && !packs.length) return null;
      try { localStorage.setItem("kena:offline-refresh", now); } catch (err) {}
      var q = saved.map(function (s) { return "s=" + encodeURIComponent(s); }).join("&");
      return timed("/offline/decks.json" + (q ? "?" + q : "")).then(function (r) {
        if (!r.ok) throw new Error("decks " + r.status);
        return r.json();
      }).then(function (list) {
        var have = {};
        packs.forEach(function (p) { have[p.slug] = p; });
        var auto = {}, visible = {};
        list.auto.forEach(function (d) { auto[d.slug] = d.version; visible[d.slug] = d.version; });
        list.saved.forEach(function (d) { visible[d.slug] = d.version; });
        var jobs = [];
        Object.keys(visible).forEach(function (slug) {
          var rec = have[slug];
          var isAuto = slug in auto;
          if (!rec || rec.version !== visible[slug] || rec.lang !== lang || rec.auto !== isAuto) {
            jobs.push(function () { return fetchPack(slug, {auto: isAuto, saved: rec && rec.saved}); });
          }
        });
        packs.forEach(function (p) {
          if (!(p.slug in visible)) jobs.push(function () { return drop(p.slug); });
        });
        return jobs.reduce(function (chain, job) { return chain.then(job).catch(function () {}); }, Promise.resolve());
      });
    }).then(paintAll, function () {});
  }

  function load(slug) {
    return Promise.all([get("packs", slug), all("attempts")]).then(function (r) {
      if (!r[0]) return null;
      r[0].attempts = r[1].filter(function (a) { return a.slug === slug; });
      return r[0];
    });
  }

  function record(a, synced) {
    a.at = Date.now();
    a.identity = identity;
    a.synced = !!synced;
    return put("attempts", a).then(function () {
      paint();
      if (!synced) syncSoon();
    });
  }

  var syncing = null;

  function pending() {
    return all("attempts").then(function (list) {
      var q = queue(list, identity, Date.now());
      return Promise.all(q.drop.map(function (a) { return del("attempts", a.id); }).concat(q.adopt.map(function (a) { return put("attempts", a); }))).then(function () { return q.send; });
    });
  }

  function syncNow() {
    if (syncing) return syncing;
    syncing = pending().then(function (list) {
      if (!list.length) return true;
      if (!navigator.onLine || !identity) return false;
      paint("syncing");
      var batch = list.slice(0, 100);
      return timed("/sync", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify({identity: identity, attempts: batch.map(function (a) {
          return {id: a.id, task: a.task, answers: a.answers, gaveUp: a.gaveUp, correct: a.correct, at: a.at};
        })}),
      }).then(function (r) {
        if (r.status === 409) return {results: []};
        if (!r.ok) throw new Error("sync " + r.status);
        return r.json();
      }).then(function (res) {
        if (!res.results.length) return false;
        var done = {};
        res.results.forEach(function (x) { done[x.id] = true; });
        return Promise.all(batch.filter(function (a) { return done[a.id]; }).map(function (a) {
          a.synced = true;
          return put("attempts", a);
        })).then(function () {
          offlineUntil = 0;
          return list.length > batch.length ? null : true;
        });
      }, function (err) {
        if (err && err.name === "AbortError" || err instanceof TypeError) markOffline();
        return false;
      });
    }).then(function (ok) {
      syncing = null;
      if (ok === null) return syncNow();
      paint();
      return ok;
    }, function () {
      syncing = null;
      paint();
      return false;
    });
    return syncing;
  }

  var soon = null;
  function syncSoon() {
    clearTimeout(soon);
    soon = setTimeout(function () { if (!isOffline()) syncNow(); }, 300);
  }


  var inds = [document.getElementById("sync-ind"), document.getElementById("site-sync")].filter(Boolean);

  function label(n, offline) {
    var parts = [];
    if (offline) parts.push(strings.msgOffline);
    if (n) parts.push((strings.msgPending || "%d").replace("%d", n));
    else if (!offline) parts.push(strings.msgSynced);
    return parts.join(" · ");
  }

  function paint(state) {
    if (!inds.length) return Promise.resolve();
    return pending().then(function (list) {
      var n = list.length;
      var offline = isOffline();
      inds.forEach(function (b) {
        var site = b.id === "site-sync";
        b.hidden = site && !n;
        b.dataset.state = state || (offline ? "offline" : n ? "pending" : "synced");
        b.querySelector(".sync-text").textContent = n ? "⟳ " + n : offline ? "" : "✓";
        var l = label(n, offline);
        b.setAttribute("aria-label", l);
        b.title = l;
      });
    }).catch(function () {});
  }

  inds.forEach(function (b) {
    b.addEventListener("pointerdown", function (e) { e.preventDefault(); });
    b.addEventListener("click", function () {
      var say = window.kenaSay || function () {};
      if (isOffline()) {
        say(strings.msgOfflineHint);
        return;
      }
      say(strings.msgSyncing);
      syncNow().then(function (ok) { say(ok ? strings.msgSynced : strings.msgSyncFailed); });
    });
  });


  function markRows(packs) {
    var have = {};
    packs.forEach(function (p) { have[p.slug] = p; });
    document.querySelectorAll("a.set-row").forEach(function (a) {
      var m = a.getAttribute("href").match(/^\/s\/([^/?#]+)$/);
      var mark = a.querySelector(".offline-mark");
      var slug = m && decodeURIComponent(m[1]);
      if (slug && have[slug] && !mark) {
        mark = document.createElement("span");
        mark.className = "offline-mark";
        mark.textContent = strings.msgOfflineReady;
        var meter = a.querySelector(".meter");
        a.insertBefore(mark, meter);
      } else if ((!slug || !have[slug]) && mark) {
        mark.remove();
      }
    });
  }

  function listOffline(packs) {
    var ul = document.getElementById("offline-decks");
    if (!ul) return;
    ul.replaceChildren();
    packs.slice().sort(function (a, b) { return (a.title || a.slug).localeCompare(b.title || b.slug); }).forEach(function (p) {
      var li = document.createElement("li");
      var a = document.createElement("a");
      a.className = "set-row";
      a.href = "/s/" + encodeURIComponent(p.slug) + "/play";
      var title = document.createElement("span");
      title.className = "title";
      title.textContent = p.title || p.slug;
      a.appendChild(title);
      li.appendChild(a);
      ul.appendChild(li);
    });
    ul.hidden = !packs.length;
    var h = document.getElementById("offline-decks-h");
    if (h) h.hidden = !packs.length;
  }

  var line = document.getElementById("offline-line");

  function paintLine(rec, busy) {
    if (!line) return;
    var auto = authed && line.hasAttribute("data-auto");
    var stateEl = line.querySelector(".offline-state");
    var toggle = line.querySelector(".offline-toggle");
    line.hidden = false;
    if (busy) {
      stateEl.textContent = line.dataset.labelSaving;
      toggle.hidden = true;
      return;
    }
    stateEl.textContent = rec ? strings.msgOfflineReady : "";
    toggle.hidden = auto || (offlinePage && !rec);
    toggle.textContent = rec ? line.dataset.labelRemove : line.dataset.labelSave;
    toggle.setAttribute("aria-pressed", String(!!rec));
  }

  function paintAll() {
    return all("packs").then(function (packs) {
      markRows(packs);
      listOffline(packs);
      if (line) paintLine(packs.filter(function (p) { return p.slug === line.dataset.slug; })[0]);
      return paint();
    }).catch(function () {});
  }

  if (line && "serviceWorker" in navigator) {
    var slug = line.dataset.slug;
    line.querySelector(".offline-toggle").addEventListener("click", function () {
      get("packs", slug).then(function (rec) {
        if (rec) return drop(slug);
        paintLine(null, true);
        return fetchPack(slug, {saved: true}).then(null, function () {
          if (window.kenaSay) window.kenaSay(line.dataset.labelFailed);
        });
      }).then(paintAll);
    });
    get("packs", slug).then(function (rec) {
      paintLine(rec);
      if (isOffline()) return;
      if (rec) return fetchPack(slug, {}).then(paintAll);
      if (authed && line.hasAttribute("data-auto")) {
        paintLine(null, true);
        return fetchPack(slug, {auto: true}).then(paintAll, paintAll);
      }
    }).catch(function () {});
  }

  var focus = document.getElementById("focus-app");
  if (focus && !offlinePage && navigator.onLine) {
    get("packs", focus.dataset.slug).then(function (rec) {
      if (rec) return fetchPack(rec.slug, {});
    }).catch(function () {});
  }


  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (form.getAttribute("action") !== "/logout" || form.dataset.offlineCleared) return;
    e.preventDefault();
    var guard = new Promise(function (resolve) { setTimeout(resolve, 3000); });
    Promise.race([syncNow(), guard]).then(function () {
      return all("packs");
    }).then(function (packs) {
      return Promise.all(packs.filter(function (p) { return p.user; }).map(function (p) { return drop(p.slug); }));
    }).then(function () {
      return all("attempts");
    }).then(function (list) {
      return Promise.all(list.filter(function (a) { return a.synced; }).map(function (a) { return del("attempts", a.id); }));
    }).then(function () {
      return window.caches ? caches.delete(PAGES) : null;
    }).catch(function () {}).then(function () {
      form.dataset.offlineCleared = "1";
      form.submit();
    });
  });


  function refreshOfflinePage() {
    if (!window.caches || isOffline()) return;
    var key = "kena:offline-lang";
    try {
      if (localStorage.getItem(key) === lang) return;
    } catch (err) {}
    caches.keys().then(function (keys) {
      var name = keys.filter(function (k) { return k.indexOf("kena-static-") === 0; })[0];
      if (!name) return;
      return caches.open(name).then(function (c) { return c.add("/offline"); }).then(function () {
        try { localStorage.setItem(key, lang); } catch (err) {}
      });
    }).catch(function () {});
  }

  window.kenaSync = {uuid: uuid, isOffline: isOffline, markOffline: markOffline, load: load, record: record, syncNow: syncNow};

  function syncRetrying(delays) {
    return syncNow().then(function (ok) {
      if (ok === false && delays.length) {
        return new Promise(function (resolve) { setTimeout(resolve, delays[0]); }).then(function () { return syncRetrying(delays.slice(1)); });
      }
      return ok;
    });
  }

  window.addEventListener("online", function () {
    offlineUntil = 0;
    paint();
    syncRetrying([1000, 3000, 10000]).then(function () { return refresh(true); });
  });
  window.addEventListener("offline", function () { paint(); });
  setInterval(function () {
    if (document.visibilityState !== "visible") return;
    pending().then(function (list) {
      if (list.length && navigator.onLine) return syncNow().then(function () { return refresh(false); });
      paint();
      return offlinePage ? null : refresh(false);
    }).catch(function () {});
  }, 30000);
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState !== "visible" || offlinePage) return;
    syncNow().then(function () { return refresh(false); });
  });

  paintAll();
  if (!offlinePage) {
    syncNow().then(function () { return refresh(false); });
    if (navigator.serviceWorker) navigator.serviceWorker.ready.then(refreshOfflinePage);
  }
})(typeof window !== "undefined" ? window : globalThis);
