(function (global) {
  "use strict";

  // Go's unicode.IsSpace, not JS \s: U+0085 is a space there, U+FEFF is not.
  var SPACE = "\\t\\n\\v\\f\\r \\u0085\\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000";
  var spaceRun = new RegExp("[" + SPACE + "]+");
  var spaceEdges = new RegExp("^[" + SPACE + "]+|[" + SPACE + "]+$", "g");
  var MARKER = "___";

  function fields(s) {
    return s.split(spaceRun).filter(function (f) { return f !== ""; });
  }

  function trimSpace(s) {
    return s.replace(spaceEdges, "");
  }

  // Per code point like Go's strings.ToLower: JS would turn a final Σ into ς.
  function lower(s) {
    var out = "";
    for (var ch of s) {
      var l = ch.toLowerCase();
      out += Array.from(l).length === 1 ? l : ch === "İ" ? "i" : ch;
    }
    return out;
  }

  function normalize(s) {
    return lower(fields(s.normalize("NFC")).join(" "));
  }

  function check(blanks, given) {
    var all = true;
    var results = blanks.map(function (answers, i) {
      var g = i < given.length ? given[i] : "";
      var n = normalize(g);
      var ok = answers.some(function (a) { return normalize(a) === n; });
      all = all && ok;
      return {given: g, correct: ok, expected: answers.length ? answers[0] : ""};
    });
    return {results: results, all: all};
  }

  function stripAccents(s) {
    return s.normalize("NFD").replace(/\p{Mn}/gu, "");
  }

  function foldSigma(s) {
    return s.split("ς").join("σ");
  }

  function levenshtein(a, b) {
    var ra = Array.from(a), rb = Array.from(b);
    var prev = [], cur = [];
    for (var j = 0; j <= rb.length; j++) prev[j] = j;
    for (var i = 1; i <= ra.length; i++) {
      cur[0] = i;
      for (j = 1; j <= rb.length; j++) {
        cur[j] = Math.min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + (ra[i - 1] === rb[j - 1] ? 0 : 1));
      }
      var t = prev;
      prev = cur;
      cur = t;
    }
    return prev[rb.length];
  }

  var kindRank = {correct: 0, accent: 1, final_sigma: 2, near_miss: 3, other: 4};

  function classifyOne(g, a) {
    if (g === a) return "correct";
    if (stripAccents(g) === stripAccents(a)) return "accent";
    if (foldSigma(g) === foldSigma(a)) return "final_sigma";
    if (levenshtein(stripAccents(g), stripAccents(a)) <= 2) return "near_miss";
    return "other";
  }

  function classify(given, accepted) {
    var g = normalize(given);
    if (g === "") return "empty";
    var best = "other";
    accepted.forEach(function (a) {
      var k = classifyOne(g, normalize(a));
      if (kindRank[k] < kindRank[best]) best = k;
    });
    return best;
  }

  // math/rand/v2: PCG-DXSM seeded (seed, 0), Lemire's bounded uint64n, Fisher-Yates from the end.
  var M64 = (1n << 64n) - 1n;
  var M128 = (1n << 128n) - 1n;
  var MUL = (2549297995355413924n << 64n) | 4865540595714422341n;
  var INC = (6364136223846793005n << 64n) | 1442695040888963407n;

  function pcg(seed) {
    var state = BigInt(seed) << 64n;
    return function () {
      state = (state * MUL + INC) & M128;
      var hi = state >> 64n, lo = state & M64;
      hi ^= hi >> 32n;
      hi = (hi * 0xda942042e4dd58b5n) & M64;
      hi ^= hi >> 48n;
      return (hi * (lo | 1n)) & M64;
    };
  }

  function uint64n(next, n) {
    if ((n & (n - 1n)) === 0n) return next() & (n - 1n);
    var p = next() * n;
    var lo = p & M64;
    if (lo < n) {
      var thresh = ((1n << 64n) - n) % n;
      while (lo < thresh) {
        p = next() * n;
        lo = p & M64;
      }
    }
    return p >> 64n;
  }

  function shuffle(list, seed) {
    var out = list.slice();
    var next = pcg(seed);
    for (var i = out.length - 1; i > 0; i--) {
      var j = Number(uint64n(next, BigInt(i + 1)));
      var t = out[i];
      out[i] = out[j];
      out[j] = t;
    }
    return out;
  }

  function decode(key, x) {
    var k = atob(key), b = atob(x);
    var bytes = new Uint8Array(b.length);
    for (var i = 0; i < b.length; i++) bytes[i] = b.charCodeAt(i) ^ k.charCodeAt(i % k.length);
    return JSON.parse(new TextDecoder().decode(bytes));
  }

  function deckOf(pack) {
    return {
      slug: pack.slug,
      loggedIn: pack.loggedIn,
      lang: pack.lang,
      msgs: pack.msgs,
      plurals: pack.plurals,
      rules: pack.rules,
      tasks: pack.tasks.map(function (t) {
        var s = decode(pack.key, t.x);
        return {
          id: t.id,
          text: t.text,
          rule: t.rule || "",
          explanation: s.e,
          blanks: s.a.map(function (answers, i) { return {answers: answers || [], hint: t.hints[i] || "", options: (t.options && t.options[i]) || null}; }),
        };
      }),
    };
  }

  function stateOf(pack, attempts, since) {
    var state = {};
    Object.keys(pack.state).forEach(function (id) { state[id] = pack.state[id]; });
    attempts.filter(function (a) { return a.at > since; }).sort(function (a, b) { return a.at - b.at; }).forEach(function (a) {
      state[a.task] = {answers: a.answers, correct: a.correct, gaveUp: a.gaveUp};
    });
    return state;
  }


  var escapes = {"&": "&amp;", "'": "&#39;", "<": "&lt;", ">": "&gt;", '"': "&#34;", "+": "&#43;", "\0": "�"};

  function esc(s) {
    return String(s).replace(/[&'<>"+\0]/g, function (c) { return escapes[c]; });
  }

  function sprintf(f, args) {
    var i = 0;
    return f.replace(/%[dsv%]/g, function (m) {
      return m === "%%" ? "%" : String(args[i++]);
    });
  }

  function pluralForm(lang, n) {
    n = Math.abs(n);
    if (lang === "ru") {
      if (n % 10 === 1 && n % 100 !== 11) return "one";
      if (n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 12 || n % 100 > 14)) return "few";
      return "many";
    }
    return n === 1 ? "one" : "other";
  }

  function strings(deck) {
    function msg(key) {
      return Object.prototype.hasOwnProperty.call(deck.msgs, key) && deck.msgs[key] !== "" ? deck.msgs[key] : null;
    }
    return {
      t: function (key) {
        var s = msg(key);
        return s === null ? "!" + key : s;
      },
      tf: function (key) {
        var s = msg(key);
        return s === null ? "!" + key : sprintf(s, Array.prototype.slice.call(arguments, 1));
      },
      tn: function (key, n) {
        var p = deck.plurals[key];
        if (!p) return "!" + key;
        var f = p[pluralForm(deck.lang, n)] || "";
        return f.indexOf("%") < 0 ? f : sprintf(f, [n]);
      },
    };
  }

  function escapeBy(s, keep) {
    return encodeURIComponent(s).replace(/[!'()*]/g, function (c) {
      return "%" + c.charCodeAt(0).toString(16).toUpperCase();
    }).replace(keep, function (m) { return decodeURIComponent(m); });
  }

  function queryEscape(s) {
    return escapeBy(s, /$^/g).replace(/%20/g, "+");
  }

  function pathEscape(s) {
    return escapeBy(s, /%(24|26|2B|3A|3D|40)/g);
  }

  function encode(values) {
    return Object.keys(values).sort().map(function (k) {
      return values[k].map(function (v) { return queryEscape(k) + "=" + queryEscape(v); }).join("&");
    }).join("&");
  }


  function invalid(what, v) {
    var err = new Error(what + " " + JSON.stringify(v) + ": invalid");
    err.status = 400;
    return err;
  }

  function notFound(what) {
    var err = new Error(what + ": not found");
    err.status = 404;
    return err;
  }

  function atoi(s) {
    return /^[+-]?\d+$/.test(s) ? parseInt(s, 10) : NaN;
  }

  var MAX_UINT64 = (1n << 64n) - 1n;

  function parseParams(get, defaultMode) {
    var p = {mode: get("mode") || defaultMode, filter: get("filter") || "all", shuffle: false, seed: "0", i: 0, n: 0, streak: 0, tries: 0};
    if (p.mode !== "cards" && p.mode !== "list") throw invalid("mode", p.mode);
    if (p.filter !== "all" && p.filter !== "unsolved" && p.filter !== "mistakes") throw invalid("filter", p.filter);
    ["i", "n", "streak", "tries"].forEach(function (name) {
      var s = get(name);
      if (!s) return;
      var n = atoi(s);
      if (isNaN(n) || n < 0) throw invalid(name, s);
      p[name] = n;
    });
    var sh = get("shuffle");
    if (sh === "" || sh === "0") return p;
    if (sh !== "1") throw invalid("shuffle", sh);
    p.shuffle = true;
    var seed = get("seed");
    if (seed === "") {
      var err = new Error("shuffle without seed");
      err.missingSeed = true;
      throw err;
    }
    if (!/^\d+$/.test(seed) || BigInt(seed) > MAX_UINT64) throw invalid("seed", seed);
    p.seed = BigInt(seed).toString();
    return p;
  }

  function copy(p) {
    return Object.assign({}, p);
  }

  function query(p) {
    var v = {};
    if (p.mode) v.mode = [p.mode];
    if (p.filter !== "all") v.filter = [p.filter];
    if (p.shuffle) {
      v.shuffle = ["1"];
      v.seed = [p.seed];
    }
    return v;
  }

  function deckPath(slug) {
    return "/s/" + pathEscape(slug);
  }

  function deckURL(slug, path, p) {
    var v = query(p);
    delete v.mode;
    for (var i = 3; i < arguments.length; i += 2) {
      if (arguments[i + 1] !== "0") v[arguments[i]] = [arguments[i + 1]];
    }
    var u = deckPath(slug) + path;
    var q = encode(v);
    return q ? u + "?" + q : u;
  }

  function nextURL(slug, after, p) {
    return deckURL(slug, "/next", p, "after", String(after), "n", String(p.n), "streak", String(p.streak));
  }

  function ordered(tasks, p) {
    return p.shuffle ? shuffle(tasks, p.seed) : tasks.slice();
  }

  function has(o, k) {
    return Object.prototype.hasOwnProperty.call(o, k);
  }

  function lastMap(state) {
    var last = {};
    Object.keys(state).forEach(function (id) { last[id] = state[id].correct; });
    return last;
  }

  function filtered(tasks, filter, last) {
    return tasks.filter(function (t) {
      var tried = has(last, t.id);
      var correct = tried && last[t.id];
      return filter === "all" || (filter === "unsolved" && !correct) || (filter === "mistakes" && tried && !correct);
    });
  }

  function indexOf(tasks, id) {
    for (var i = 0; i < tasks.length; i++) if (tasks[i].id === id) return i;
    return -1;
  }

  function prevInOrder(order, filter, last, id) {
    var pos = indexOf(order, id);
    if (pos < 0) pos = order.length;
    var before = filtered(order.slice(0, pos), filter, last);
    return before.length ? before[before.length - 1].id : 0;
  }

  function positionIn(order, filter, last, id, n) {
    var i = n - filtered(order.slice(indexOf(order, id) + 1), filter, last).length;
    return Math.max(1, Math.min(i, n));
  }

  function firstUnsolved(tasks, last) {
    for (var i = 0; i < tasks.length; i++) if (!last[tasks[i].id]) return tasks[i].id;
    return 0;
  }

  function progressOf(tasks, last) {
    var pr = {total: tasks.length, solved: 0, mistakes: 0};
    tasks.forEach(function (t) {
      if (last[t.id]) pr.solved++;
      else if (has(last, t.id)) pr.mistakes++;
    });
    return pr;
  }

  function newTaskView(t, results) {
    var parts = t.text.split(MARKER);
    var v = {id: t.id, autofocus: false, segments: []};
    parts.forEach(function (part, i) {
      if (part !== "") v.segments.push({text: part});
      if (i === parts.length - 1) return;
      var s = {blank: true, index: i + 1, hint: i < t.blanks.length ? t.blanks[i].hint : "", options: null};
      if (!results && i < t.blanks.length) s.options = markOptions(t.blanks[i].options, "", [], false);
      if (results && i < results.length) {
        s.checked = true;
        s.correct = results[i].correct;
        s.given = trimSpace(results[i].given);
        s.expected = results[i].expected;
      }
      v.segments.push(s);
    });
    return v;
  }

  function blankEmpty(s) {
    return normalize(s) === "";
  }

  function markOptions(opts, picked, struck, wrong) {
    if (!opts) return null;
    return opts.map(function (o) {
      var c = {value: o, picked: false, struck: struck.indexOf(o) >= 0};
      if (o === picked) {
        if (wrong) c.struck = true;
        else c.picked = true;
      }
      return c;
    });
  }

  function retryTaskView(t, results, struck) {
    var v = newTaskView(t, null);
    v.segments.forEach(function (s) {
      if (!s.blank || s.index - 1 >= results.length) return;
      var res = results[s.index - 1];
      var empty = blankEmpty(res.given);
      if (s.options) {
        var picked = empty ? "" : trimSpace(res.given);
        s.options = markOptions(t.blanks[s.index - 1].options, picked, (struck && struck[s.index - 1]) || [], !res.correct);
        if (!empty) {
          s.value = picked;
          s.locked = res.correct;
          s.wrong = !res.correct;
        }
        return;
      }
      if (empty) return;
      s.value = trimSpace(res.given);
      s.locked = res.correct;
      s.wrong = !res.correct;
    });
    return v;
  }

  var nudges = {accent: "nudge.accent", final_sigma: "nudge.final_sigma", near_miss: "nudge.near_miss"};
  var nudgeRank = {accent: 1, final_sigma: 2, near_miss: 3};

  function nudge(t, results) {
    var worst = "";
    for (var i = 0; i < results.length; i++) {
      if (results[i].correct || i >= t.blanks.length || blankEmpty(results[i].given) || t.blanks[i].options) continue;
      var k = classify(results[i].given, t.blanks[i].answers);
      if (!nudges[k]) return "";
      if (nudgeRank[k] > (nudgeRank[worst] || 0)) worst = k;
    }
    return nudges[worst] || "";
  }

  function ruleFor(deck, key) {
    if (!key) return null;
    for (var i = 0; i < deck.rules.length; i++) if (deck.rules[i].key === key) return deck.rules[i];
    return null;
  }

  function infoFor(deck, t) {
    var rule = ruleFor(deck, t.rule);
    if (!rule && !deck.rules.some(function (r) { return r.general; })) return null;
    return {rule: rule};
  }

  function newNav(slug, id, prev, p) {
    var streak = String(p.streak);
    var nv = {id: id, prevID: prev, cardURL: deckURL(slug, "/play/card", p, "n", String(p.n), "streak", streak), prevURL: "", skipURL: "", nextURL: ""};
    if (prev !== 0) nv.prevURL = deckURL(slug, "/play", p, "at", String(prev), "streak", streak);
    if (id !== 0) {
      nv.skipURL = deckURL(slug, "/play", p, "after", String(id), "streak", streak);
      nv.nextURL = nextURL(slug, id, p);
    }
    return nv;
  }

  function newCardView(deck, t, p, results, reveal, correct, struck) {
    var v = {info: infoFor(deck, t), params: copy(p), inPage: false, retry: false, soFar: false, nudge: "", checked: false, correct: false, streak: 0, notes: {explanation: "", rule: null}, nextURL: "", bar: {index: 0, total: 0, nav: null}};
    if (!results) {
      v.task = newTaskView(t, null);
    } else if (!reveal) {
      v.task = retryTaskView(t, results, struck);
      v.retry = true;
      v.nudge = nudge(t, results);
      v.soFar = !results.some(function (r) { return !r.correct && !blankEmpty(r.given); });
    } else {
      v.task = newTaskView(t, results);
      v.checked = true;
      v.correct = correct;
      v.notes = {explanation: t.explanation, rule: ruleFor(deck, t.rule)};
      v.nextURL = nextURL(deck.slug, t.id, p);
    }
    return v;
  }

  function answersOf(t) {
    return t.blanks.map(function (b) { return b.answers; });
  }

  function lastView(deck, t, p, la, reveal) {
    if (!la) return newCardView(deck, t, p, null, false, false);
    var results = check(answersOf(t), la.answers).results;
    if (!reveal) {
      p = copy(p);
      p.tries = 1;
    }
    return newCardView(deck, t, p, results, reveal, la.correct);
  }

  function inPage(v) {
    v.inPage = true;
    v.task.autofocus = !v.retry && !v.checked;
    return v;
  }

  function newDone(deck, empty, pr) {
    var path = deckPath(deck.slug);
    return {loggedIn: deck.loggedIn, empty: empty, progress: pr, againURL: path + "/play", mistakesURL: path + "/play?filter=mistakes", deckURL: path, inPage: false, bar: null};
  }

  function freshCard(deck, last, order, t, i, n, p) {
    p = copy(p);
    p.i = i;
    p.n = n;
    var v = newCardView(deck, t, p, null, false, false);
    v.bar = {index: i, total: n, nav: newNav(deck.slug, t.id, prevInOrder(order, p.filter, last, t.id), p)};
    return v;
  }

  function doneBar(slug, todo, p, n) {
    p = copy(p);
    p.n = n;
    var prev = todo.length ? todo[todo.length - 1].id : 0;
    return {index: n, total: n, nav: newNav(slug, 0, prev, p)};
  }

  function playState(deck, state, last, order, p, id, n) {
    var pos = indexOf(order, id);
    if (pos < 0) throw notFound("task " + id);
    p = copy(p);
    p.i = positionIn(order, p.filter, last, id, n);
    p.n = n;
    var la = has(state, id) ? state[id] : null;
    var v = lastView(deck, order[pos], p, la, !!la && (la.correct || la.gaveUp));
    v.bar = {index: p.i, total: n, nav: newNav(deck.slug, id, prevInOrder(order, p.filter, last, id), p)};
    return v;
  }

  function parseID(what, s) {
    var n = atoi(s || "");
    if (isNaN(n)) throw invalid(what, s);
    return n;
  }

  function getter(values) {
    return function (k) { return values.get(k) || ""; };
  }


  var INFO_ICON = '<svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><circle cx="12" cy="12" r="8.75"/><path d="M12 11v5.25"/><circle cx="12" cy="7.9" r="0.6" fill="currentColor"/></svg>';
  var PREV_ICON = '<svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M14.5 6l-6 6 6 6"/></svg>';
  var NEXT_ICON = '<svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M9.5 6l6 6-6 6"/></svg>';
  var MOTIF = '<svg class="motif" viewBox="0 0 20 20" width="20" height="20" aria-hidden="true" focusable="false"><rect x="0" y="0" width="6" height="6" rx="0.6"/><rect x="7" y="0" width="6" height="6" rx="0.6" class="slate"/><rect x="14" y="0" width="6" height="6" rx="0.6"/><rect x="0" y="7" width="6" height="6" rx="0.6" class="slate"/><rect x="7" y="7" width="6" height="6" rx="0.6"/><rect x="14.5" y="7.5" width="5" height="5" class="hole"/><rect x="0" y="14" width="6" height="6" rx="0.6"/><rect x="7" y="14" width="6" height="6" rx="0.6" class="slate"/><rect x="14" y="14" width="6" height="6" rx="0.6"/></svg>';

  function renderer(deck) {
    var s = strings(deck);
    var t = s.t, tf = s.tf, tn = s.tn;

    function sentence(task) {
      return task.segments.map(function (g) {
        if (!g.blank) return esc(g.text);
        if (g.checked) {
          if (g.correct) return '<span class="filled ok">' + esc(g.given) + "</span>";
          return '<span class="fix"><s class="given bad">' + esc(g.given || "—") + '</s> → <span class="expected ok">' + esc(g.expected) + "</span></span>";
        }
        if (g.options) {
          var c = '<span class="gap"><select name="c' + g.index + '" lang="el" aria-label="' + esc(tf("task.blank", g.index)) + '"';
          if (g.locked) c += ' disabled class="locked"';
          else if (g.wrong) c += ' class="wrong" aria-invalid="true"';
          if (task.autofocus && g.index === 1) c += " autofocus";
          c += '><option value=""></option>';
          g.options.forEach(function (o) {
            c += '<option value="' + esc(o.value) + '"' + (o.picked ? " selected" : "") + (o.struck ? " disabled" : "") + ">" + esc(o.value) + "</option>";
          });
          c += "</select>";
          if (g.locked) c += '<input type="hidden" name="c' + g.index + '" value="' + esc(g.value) + '">';
          g.options.forEach(function (o) {
            if (o.struck) c += '<input type="hidden" name="x' + g.index + '" value="' + esc(o.value) + '">';
          });
          if (g.hint) c += ' <span class="hint">(' + esc(g.hint) + ")</span>";
          return c + "</span>";
        }
        var h = '<span class="gap"><input name="a" lang="el" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" enterkeyhint="go" aria-label="' + esc(tf("task.blank", g.index)) + '"';
        if (g.hint) h += ' placeholder="' + esc(g.hint) + '"';
        if (g.value) h += ' value="' + esc(g.value) + '"';
        if (g.locked) h += ' readonly tabindex="-1" class="locked"';
        else if (g.wrong) h += ' class="wrong" aria-invalid="true"';
        if (task.autofocus && g.index === 1) h += " autofocus";
        h += ">";
        if (g.hint) h += ' <span class="hint">(' + esc(g.hint) + ")</span>";
        return h + "</span>";
      }).join("");
    }

    function hidden(p) {
      var h = '<input type="hidden" name="mode" value="' + esc(p.mode) + '">\n<input type="hidden" name="filter" value="' + esc(p.filter) + '">';
      if (p.shuffle) h += '\n<input type="hidden" name="shuffle" value="1">\n<input type="hidden" name="seed" value="' + esc(p.seed) + '">';
      if (p.n) h += '\n<input type="hidden" name="i" value="' + p.i + '">\n<input type="hidden" name="n" value="' + p.n + '">';
      if (p.streak) h += '\n<input type="hidden" name="streak" value="' + p.streak + '">';
      if (p.tries) h += '\n<input type="hidden" name="tries" value="' + p.tries + '">';
      return h;
    }

    function checkActions(v) {
      var h = "";
      if (v.nudge) h = '<p class="nudge" role="status">' + esc(t(v.nudge)) + "</p>";
      else if (v.soFar) h = '<p class="visually-hidden" role="status">' + esc(t("play.right_so_far")) + "</p>";
      else if (v.retry) h = '<p class="visually-hidden" role="status">' + esc(t("play.not_quite")) + "</p>";
      return h + '\n<div class="check-actions"><button type="submit">' + esc(t("play.check")) + '</button><button type="submit" name="giveup" value="1" class="quiet">' + esc(t("play.give_up")) + "</button></div>";
    }

    function notesBody(n) {
      var h = "";
      if (n.explanation) h += '<p class="explanation">' + esc(n.explanation) + "</p>";
      if (n.rule) {
        h += '<p class="rule"><span class="rule-label">' + esc(t("rule.label")) + '</span> <span class="rule-name">' + esc(n.rule.label) + "</span>";
        if (n.rule.preview) h += " — " + esc(n.rule.preview);
        h += ' <button type="button" class="link-button" data-open-info hidden>' + esc(t("rule.show")) + "</button></p>";
      }
      return h;
    }

    function why(n) {
      if (!n.explanation && !n.rule) return "";
      return '<details class="why"><summary>' + esc(t("play.why")) + "</summary>" + notesBody(n) + "</details>";
    }

    // rule.html is markdown.Render output, sanitised on the server like every other rule on the site.
    function ruleFull(r) {
      return '<section class="rule-doc">' + (r.title ? '<h2 class="rule-title">' + esc(r.title) + "</h2>" : "") + '<div class="md">' + r.html + "</div></section>";
    }

    function infoButton(info) {
      if (!info) return "";
      var h = '<button type="button" class="info-btn" data-info aria-label="' + esc(t("rule.label")) + '" title="' + esc(t("rule.label")) + '" aria-haspopup="dialog" aria-controls="info-sheet" hidden>' + INFO_ICON + "</button>";
      if (info.rule) h += '<template class="info-rule" data-key="' + esc(info.rule.key) + '">' + ruleFull(info.rule) + "</template>";
      return h;
    }

    function bar(b) {
      var h = "";
      var nav = b.nav;
      if (nav) {
        h += '<a class="nav-btn" data-nav="prev"' + (nav.prevURL ? ' href="' + esc(nav.prevURL) + '"' : ' aria-disabled="true"') + (nav.prevID ? ' data-prev="' + nav.prevID + '"' : "") +
          ' data-card="' + esc(nav.cardURL) + '" aria-label="' + esc(t("play.prev")) + '" title="' + esc(t("play.prev")) + '">' + PREV_ICON + "</a>";
      }
      if (b.total) {
        h += '<div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax="' + b.total + '" aria-valuenow="' + b.index + '" aria-label="' + esc(tf("play.card_of", b.index, b.total)) +
          '"><div class="bar" style="width: ' + Math.floor((b.index * 100) / b.total) + '%"></div></div>\n<span class="count">' + b.index + "/" + b.total + "</span>";
      }
      if (nav) {
        h += '<a class="nav-btn" data-nav="next"' + (nav.skipURL ? ' href="' + esc(nav.skipURL) + '" data-id="' + nav.id + '" data-next="' + esc(nav.nextURL) + '"' : ' aria-disabled="true"') +
          ' aria-label="' + esc(t("play.skip_long")) + '" title="' + esc(t("play.skip")) + '">' + NEXT_ICON + "</a>";
      }
      return h;
    }

    function barOOB(b) {
      return '<div id="focus-pos" class="focus-pos" hx-swap-oob="true">' + bar(b) + "</div>";
    }

    function card(v) {
      var h = (v.inPage ? "" : barOOB(v.bar)) + "\n";
      if (v.checked) {
        h += '<div class="card result ' + (v.correct ? "ok" : "bad") + '">\n' + infoButton(v.info);
        if (v.correct) {
          h += '\n<p class="verdict"><span class="tick" aria-hidden="true">✓</span> ' + esc(t("play.correct"));
          if (v.streak >= 2) h += '<span class="streak"> · ' + esc(tn("count.in_a_row", v.streak)) + "</span>";
          h += "</p>";
        }
        h += '\n<p class="sentence" lang="el">' + sentence(v.task) + "</p>\n" + (v.correct ? why(v.notes) : notesBody(v.notes));
        return h + '\n<button type="button" data-next hx-get="' + esc(v.nextURL) + '" hx-target="#card" hx-swap="innerHTML show:none">' + esc(t("play.next")) + "</button>\n</div>";
      }
      var action = "/t/" + v.task.id + "/check";
      return h + '<form class="card" action="' + action + '" method="post" hx-post="' + action + '" hx-target="#card" hx-swap="innerHTML show:none">\n' +
        infoButton(v.info) + "\n" + hidden(v.params) + '\n<p class="sentence" lang="el">' + sentence(v.task) + "</p>\n" + checkActions(v) + "\n</form>";
    }

    function done(d) {
      var pr = d.progress;
      var all = pr.total > 0 && pr.solved === pr.total;
      var h = !d.inPage && d.bar ? barOOB(d.bar) : "";
      h += '<div class="card done' + (all || !d.empty ? " complete" : "") + '">\n' + MOTIF + "\n<h2>" + esc(all ? t("done.all_solved") : d.empty ? t("done.empty") : t("done.done")) + "</h2>\n";
      if (!all) {
        h += '<p class="muted">' + esc(tf("progress.solved", pr.solved, pr.total));
        if (pr.mistakes) h += ' · <span class="bad">' + esc(tn("count.mistakes", pr.mistakes)) + "</span>";
        h += "</p>";
      }
      h += '\n<div class="actions">\n<a class="button" href="' + esc(d.againURL) + '">' + esc(t("done.again")) + "</a>";
      if (pr.mistakes) h += '\n<a class="button secondary" href="' + esc(d.mistakesURL) + '">' + esc(t("done.mistakes_only")) + "</a>";
      h += '\n</div>\n<p class="links"><a href="' + esc(d.deckURL) + '">' + esc(t("play.back_to_deck")) + "</a>";
      if (d.loggedIn) h += ' · <a href="/mistakes">' + esc(t("done.review_mistakes")) + "</a>";
      return h + "</p>\n</div>";
    }

    function links(list) {
      return list.map(function (l) {
        return '<a href="' + esc(l.url) + '"' + (l.current ? ' aria-current="true"' : "") + ">" + esc(t(l.label)) + "</a>";
      }).join("");
    }

    function sheetLinks(d) {
      return (d.firstURL ? '<a href="' + esc(d.firstURL) + '">' + esc(t("play.first_unsolved")) + "</a>" : "") +
        '<a href="' + esc(d.listURL) + '">' + esc(t("play.to_worksheet")) + "</a>" +
        (d.resetURL ? '<a href="' + esc(d.resetURL) + '">' + esc(t("reset.title")) + "</a>" : "") +
        '<a href="' + esc(d.deckURL) + '">' + esc(t("play.back_to_deck")) + "</a>";
    }

    return {card: card, done: done, bar: bar, links: links, sheetLinks: sheetLinks};
  }

  function engine(deck, state) {
    var render = renderer(deck);
    var tasks = deck.tasks;

    function next(params) {
      var p = parseParams(getter(params), "cards");
      p.mode = "cards";
      var after = parseID("after", params.get("after"));
      var last = lastMap(state);
      var order = ordered(tasks, p);
      var rest = filtered(order.slice(indexOf(order, after) + 1), p.filter, last);
      var todo = filtered(order, p.filter, last);
      if (!rest.length) {
        var d = newDone(deck, false, progressOf(tasks, last));
        d.bar = doneBar(deck.slug, todo, p, p.n || todo.length);
        return render.done(d);
      }
      var i = p.n - rest.length + 1, n = p.n;
      if (p.n === 0 || i < 1) {
        i = indexOf(todo, rest[0].id) + 1;
        n = todo.length;
      }
      return render.card(freshCard(deck, last, order, rest[0], i, n, p));
    }

    function card(params) {
      var p = parseParams(getter(params), "cards");
      p.mode = "cards";
      p.i = 0;
      p.tries = 0;
      var id = parseID("id", params.get("id"));
      var last = lastMap(state);
      var order = ordered(tasks, p);
      return render.card(playState(deck, state, last, order, p, id, p.n || filtered(order, p.filter, last).length));
    }

    function checkTask(id, form) {
      var task = tasks[indexOf(tasks, id)];
      if (!task) throw notFound("task " + id);
      var p = parseParams(getter(form), "cards");
      if (p.mode !== "cards") throw invalid("mode", p.mode);
      var typed = form.getAll("a").map(String);
      var k = 0;
      var struck = {};
      var answers = task.blanks.map(function (b, i) {
        if (b.options) {
          struck[i] = form.getAll("x" + (i + 1)).map(String);
          return String(form.get("c" + (i + 1)) || "");
        }
        return k < typed.length ? typed[k++] : null;
      });
      if (k !== typed.length || answers.indexOf(null) >= 0) throw invalid("answers", typed.length);
      var res = check(answersOf(task), answers);
      if (form.get("giveup") !== "1" && answers.some(blankEmpty)) {
        if (res.results.some(function (r) { return !r.correct && !blankEmpty(r.given); })) {
          p.streak = 0;
          p.tries++;
        }
        var pv = newCardView(deck, task, p, res.results, false, false, struck);
        pv.bar = {index: p.i, total: p.n, nav: newNav(deck.slug, id, 0, p)};
        return {html: render.card(pv), attempt: null};
      }
      var giveUp = !res.all && form.get("giveup") === "1";
      if (!res.all) p.streak = 0;
      else if (p.tries === 0) p.streak++;
      state[id] = {answers: answers, correct: res.all, gaveUp: giveUp};
      var reveal = res.all || giveUp;
      if (reveal) p.tries = 0;
      else p.tries++;
      var v = newCardView(deck, task, p, res.results, reveal, res.all, struck);
      v.streak = p.streak;
      v.bar = {index: p.i, total: p.n, nav: newNav(deck.slug, id, 0, p)};
      return {html: render.card(v), attempt: {task: id, answers: answers, correct: res.all, gaveUp: giveUp}};
    }

    function play(params, seed) {
      var p;
      try {
        p = parseParams(getter(params), "cards");
      } catch (err) {
        if (!err.missingSeed) throw err;
        var q = {};
        params.forEach(function (v, k) { (q[k] = q[k] || []).push(v); });
        q.seed = [String(seed())];
        return {redirect: deckPath(deck.slug) + "/play?" + encode(q)};
      }
      p.mode = "cards";
      p.i = 0;
      p.n = 0;
      p.tries = 0;
      var last = lastMap(state);
      var order = ordered(tasks, p);
      var todo = filtered(order, p.filter, last);
      var pr = progressOf(tasks, last);
      var slug = deck.slug;
      var data = {deckURL: deckPath(slug), firstURL: "", resetURL: "", listURL: deckURL(slug, "/list", p), card: null, done: null, bar: {index: 0, total: 0, nav: null}};
      var filters = ["all", "unsolved", "mistakes"].map(function (f) {
        var lp = copy(p);
        lp.filter = f;
        return {label: "filter." + f, url: deckURL(slug, "/play", lp), current: f === p.filter};
      });
      var inOrder = copy(p), shuffled = copy(p);
      inOrder.shuffle = false;
      inOrder.seed = "0";
      shuffled.shuffle = true;
      if (!p.shuffle) shuffled.seed = String(seed());
      var orderLinks = [{label: "order.in_order", url: deckURL(slug, "/play", inOrder), current: !p.shuffle}, {label: "order.shuffled", url: deckURL(slug, "/play", shuffled), current: p.shuffle}];
      var first = firstUnsolved(todo, last);
      if (first) data.firstURL = deckURL(slug, "/play", p);
      if (pr.solved + pr.mistakes > 0) data.resetURL = deckPath(slug) + "/reset";

      var rest = first ? todo.slice(indexOf(todo, first)) : todo;
      if (params.get("at")) {
        data.card = inPage(playState(deck, state, last, order, p, parseID("at", params.get("at")), todo.length));
      } else if (!todo.length) {
        data.done = newDone(deck, true, pr);
      } else if (params.get("after")) {
        rest = filtered(order.slice(indexOf(order, parseID("after", params.get("after"))) + 1), p.filter, last);
      }
      if (!data.card && !data.done) {
        if (!rest.length) {
          data.done = newDone(deck, false, pr);
          data.bar = doneBar(slug, todo, p, todo.length);
        } else {
          data.card = inPage(freshCard(deck, last, order, rest[0], indexOf(todo, rest[0].id) + 1, todo.length, p));
        }
      }
      if (data.card) data.bar = data.card.bar;
      else data.done.inPage = true;
      return {
        card: data.card ? render.card(data.card) : render.done(data.done),
        bar: render.bar(data.bar),
        menu: [render.links(filters), render.links(orderLinks), render.sheetLinks(data)],
      };
    }

    return {next: next, card: card, check: checkTask, play: play};
  }

  var core = {
    normalize: normalize, check: check, classify: classify, levenshtein: levenshtein, shuffle: shuffle,
    decode: decode, deckOf: deckOf, stateOf: stateOf, engine: engine, pluralForm: pluralForm,
  };
  if (typeof module === "object" && module.exports) module.exports = core;
  if (typeof document === "undefined") return;


  var app = document.getElementById("focus-app");
  var cardEl = document.getElementById("card");
  var sync = global.kenaSync;
  if (!app || !cardEl || !sync || !global.htmx) return;
  var root = document.documentElement;
  var slug = app.dataset.slug;
  var eng = null;

  var routes = [
    {re: /^\/s\/([^/]+)\/next$/, verb: "get", run: function (m, url) { return {html: eng.next(url.searchParams)}; }},
    {re: /^\/s\/([^/]+)\/play\/card$/, verb: "get", run: function (m, url) { return {html: eng.card(url.searchParams)}; }},
    {re: /^\/t\/(\d+)\/check$/, verb: "post", run: function (m, url, form) { return eng.check(parseInt(m[1], 10), form); }},
  ];

  function route(cfg) {
    if (!eng || !cfg || cfg.target !== cardEl) return null;
    var url = new URL(cfg.path, location.href);
    if (url.origin !== location.origin) return null;
    for (var i = 0; i < routes.length; i++) {
      var r = routes[i];
      var m = url.pathname.match(r.re);
      if (m && r.verb === cfg.verb && (r.verb === "post" || decodeURIComponent(m[1]) === slug)) return {route: r, m: m, url: url};
    }
    return null;
  }

  function swap(html, cfg) {
    htmx.swap(cardEl, html, {swapStyle: "innerHTML", swapDelay: 0, settleDelay: 0}, {eventInfo: {elt: cfg ? cfg.elt : cardEl, target: cardEl, requestConfig: cfg}});
  }

  function runLocal(found, cfg) {
    var res;
    try {
      res = found.route.run(found.m, found.url, cfg.formData);
    } catch (err) {
      return false;
    }
    if (res.attempt) sync.record(attemptOf(res.attempt, cfg), false);
    swap(res.html, cfg);
    return true;
  }

  function attemptOf(a, cfg) {
    a.id = cfg.formData.get("cid") || sync.uuid();
    a.slug = slug;
    return a;
  }

  document.body.addEventListener("htmx:configRequest", function (e) {
    if (e.detail.target !== cardEl) return;
    if (e.detail.verb === "post") e.detail.parameters.cid = sync.uuid();
    if (eng) e.detail.timeout = 3000;
  });

  document.body.addEventListener("htmx:beforeRequest", function (e) {
    if (!sync.isOffline()) return;
    var found = route(e.detail.requestConfig);
    if (found && runLocal(found, e.detail.requestConfig)) e.preventDefault();
  });

  function failed(e) {
    var found = route(e.detail.requestConfig);
    if (!found) return;
    sync.markOffline();
    if (runLocal(found, e.detail.requestConfig)) e.preventDefault();
  }
  document.addEventListener("htmx:sendError", failed, true);
  document.addEventListener("htmx:timeout", failed, true);

  document.body.addEventListener("htmx:afterRequest", function (e) {
    var cfg = e.detail.requestConfig;
    if (!eng || !e.detail.successful || !cfg || cfg.verb !== "post" || cfg.target !== cardEl) return;
    var m = new URL(cfg.path, location.href).pathname.match(/^\/t\/(\d+)\/check$/);
    if (!m) return;
    try {
      var a = eng.check(parseInt(m[1], 10), cfg.formData).attempt;
      if (a) sync.record(attemptOf(a, cfg), true);
    } catch (err) {}
  });

  function seed() {
    return crypto.getRandomValues(new Uint32Array(1))[0];
  }

  function startOffline() {
    var res = eng.play(new URLSearchParams(location.search), seed);
    if (res.redirect) {
      history.replaceState(null, "", res.redirect);
      res = eng.play(new URLSearchParams(location.search), seed);
    }
    var segs = document.querySelectorAll("#focus-menu .seg");
    if (segs.length === 2) {
      segs[0].innerHTML = res.menu[0];
      segs[1].innerHTML = res.menu[1];
    }
    var links = document.querySelector("#focus-menu .sheet-links");
    if (links) links.innerHTML = res.menu[2];
    document.getElementById("focus-pos").innerHTML = res.bar;
    swap(res.card, null);
  }

  sync.load(slug).then(function (rec) {
    if (rec) eng = engine(deckOf(rec.pack), stateOf(rec.pack, rec.attempts, rec.fetched));
  }).catch(function () {}).then(function () {
    if (root.dataset.offline !== "1") return;
    try {
      if (eng) startOffline();
      root.dataset.offline = eng ? "ready" : "nopack";
    } catch (err) {
      root.dataset.offline = "failed";
    }
  });
})(typeof window !== "undefined" ? window : globalThis);
