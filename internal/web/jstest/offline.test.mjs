import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {createRequire} from "node:module";

const require = createRequire(import.meta.url);
const core = require("../static/offline.js");
const read = (path) => JSON.parse(readFileSync(new URL(path, import.meta.url), "utf8"));

test("shared greek vectors", () => {
  const v = read("../../greek/testdata/vectors.json");
  for (const c of v.normalize) assert.equal(core.normalize(c.in), c.out, JSON.stringify(c.in));
  for (const c of v.check) {
    const got = core.check(c.blanks, c.given);
    assert.deepEqual(got, {results: c.results, all: c.all}, JSON.stringify(c));
  }
  for (const c of v.classify) assert.equal(core.classify(c.given, c.accepted), c.kind, JSON.stringify(c));
  for (const c of v.levenshtein) assert.equal(core.levenshtein(c.a, c.b), c.d, JSON.stringify(c));
});

test("shuffle matches math/rand/v2", () => {
  for (const c of read("../testdata/shuffle.json")) {
    const ids = Array.from({length: c.n}, (_, i) => i + 1);
    assert.deepEqual(core.shuffle(ids, c.seed), c.perm, `seed ${c.seed} n ${c.n}`);
  }
});

test("plural forms match internal/i18n", () => {
  const cases = [["en", 0, "other"], ["en", 1, "one"], ["el", 1, "one"], ["el", 5, "other"], ["ru", 0, "many"], ["ru", 1, "one"],
    ["ru", 2, "few"], ["ru", 4, "few"], ["ru", 5, "many"], ["ru", 11, "many"], ["ru", 14, "many"], ["ru", 21, "one"], ["ru", 22, "few"],
    ["ru", 111, "many"], ["ru", 1004, "few"]];
  for (const [lang, n, want] of cases) assert.equal(core.pluralForm(lang, n), want, `${lang} ${n}`);
});

const squash = (s = "") => s.replace(/\s+/g, " ").trim();

test("engine renders what the server rendered (golden)", () => {
  const g = read("../testdata/offline_golden.json");
  const deck = core.deckOf(g.pack);
  assert.ok(deck.tasks.every((t) => t.blanks.length > 0 && t.blanks.every((b) => b.answers.length > 0)));
  const eng = core.engine(deck, core.stateOf(g.pack, [], 0));
  g.steps.forEach((s, i) => {
    const what = `step ${i}: ${s.method} ${s.url}`;
    const url = new URL(s.url, "http://kena.test");
    if (s.method === "POST") {
      const form = new URLSearchParams();
      for (const [k, vs] of Object.entries(s.form)) for (const v of vs) form.append(k, v);
      const id = Number(url.pathname.split("/")[2]);
      assert.equal(squash(eng.check(id, form).html), squash(s.html), what);
    } else if (s.method === "GET") {
      const html = url.pathname.endsWith("/next") ? eng.next(url.searchParams) : eng.card(url.searchParams);
      assert.equal(squash(html), squash(s.html), what);
    } else {
      const r = eng.play(url.searchParams, () => 7);
      assert.equal(squash(r.card), squash(s.card), what + " card");
      assert.equal(squash(r.bar), squash(s.bar), what + " bar");
      const menu = r.menu.slice();
      if (url.searchParams.get("shuffle") !== "1") menu[1] = menu[1].replace(/seed=\d+/g, "seed=*");
      assert.deepEqual(menu, s.menu, what + " menu");
    }
  });
});

test("play without a seed asks for a redirect with one", () => {
  const g = read("../testdata/offline_golden.json");
  const eng = core.engine(core.deckOf(g.pack), {});
  const r = eng.play(new URLSearchParams("shuffle=1&filter=unsolved"), () => 99);
  assert.equal(r.redirect, `/s/${g.pack.slug}/play?filter=unsolved&seed=99&shuffle=1`);
});

test("bad parameters throw like the server's 400 and 404", () => {
  const g = read("../testdata/offline_golden.json");
  const eng = core.engine(core.deckOf(g.pack), {});
  assert.throws(() => eng.next(new URLSearchParams("after=x")), {status: 400});
  assert.throws(() => eng.next(new URLSearchParams("after=1&filter=nope")), {status: 400});
  assert.throws(() => eng.card(new URLSearchParams("id=999999")), {status: 404});
  assert.throws(() => eng.check(999999, new URLSearchParams("a=x")), {status: 404});
  const id = g.pack.tasks[0].id;
  assert.throws(() => eng.check(id, new URLSearchParams("a=x&a=y")), {status: 400});
});

test("local attempts newer than the pack override its state", () => {
  const pack = {state: {"1": {answers: ["a"], correct: false, gaveUp: false}, "2": {answers: ["b"], correct: true, gaveUp: false}}};
  const st = core.stateOf(pack, [
    {task: 1, answers: ["old"], correct: true, gaveUp: false, at: 5},
    {task: 1, answers: ["x"], correct: false, gaveUp: true, at: 30},
    {task: 1, answers: ["y"], correct: true, gaveUp: false, at: 20},
    {task: 3, answers: ["z"], correct: true, gaveUp: false, at: 11},
  ], 10);
  assert.deepEqual(st, {
    "1": {answers: ["x"], correct: false, gaveUp: true},
    "2": {answers: ["b"], correct: true, gaveUp: false},
    "3": {answers: ["z"], correct: true, gaveUp: false},
  });
});

test("a partial check queues no attempt and leaves the state alone", () => {
  const g = read("../testdata/offline_golden.json");
  const state = {};
  const eng = core.engine(core.deckOf(g.pack), state);
  const two = g.pack.tasks.find((t) => t.hints.length === 2).id;
  const right = eng.check(two, new URLSearchParams("mode=cards&a=πίνεις&streak=2"));
  assert.equal(right.attempt, null);
  assert.match(right.html, /name="streak" value="2"/);
  assert.doesNotMatch(right.html, /name="tries"|aria-invalid|struck/);
  const wrong = eng.check(two, new URLSearchParams("mode=cards&a=&c2=Έπινα&streak=2"));
  assert.equal(wrong.attempt, null);
  assert.match(wrong.html, /name="tries" value="1"/);
  assert.match(wrong.html, /class="choice struck"><input type="radio" name="c2" value="Έπινα" lang="el" disabled>/);
  assert.match(wrong.html, /<input type="hidden" name="x2" value="Έπινα">/);
  assert.doesNotMatch(wrong.html, /name="streak"|πίνεις/);
  assert.deepEqual(state, {});
  assert.throws(() => eng.check(two, new URLSearchParams("mode=cards&a=πίνεις&a=Πίνω")));
  assert.equal(eng.check(two, new URLSearchParams("mode=cards&a=πίνεις&c2=Πίνω&tries=1")).attempt.correct, true);
  assert.equal(state[two].correct, true);
});
