# Offline practice and the sync indicator

Date: 2026-10-18. Status: accepted. Extends `2026-10-17-focus-mode-design.md`,
`2026-10-17-swipe-nav-design.md`, `2026-10-17-practice-flow-design.md` and
`2026-10-17-i18n-theme-csp-design.md`.

## Problem

kena is used on a phone, often on the metro or a plane. Without a network the
practice page dies on the first Check, and the home-screen app does not start
at all. The learner wants to keep practising offline and see whether what
they answered has reached the server.

## Principles

- Online behaviour does not change: pages are server-rendered, answers never
  reach the page before Give up or a correct check, checking is server-side.
  The offline engine runs only when the network is gone.
- Offline, focus mode looks and behaves the same (card, retry with shake and
  nudge, Give up, Why?, ⓘ sheet, streak, progress, ‹ › and swipes, done
  screen). The client renders the same HTML as the server templates; a golden
  file produced by the Go handlers is replayed by the JS engine in tests, so
  the two can't drift silently.
- A device downloads only decks the viewer opted into: logged in, their own
  and starred decks; anyone, decks saved with "Save for offline".
- Offline answers are only suggestions to the server: it re-checks them,
  never trusts the client's verdict, and records each one exactly once.
- CSP stays as is (`script-src 'self'`, `connect-src 'self'`).

## Service worker

`GET /sw.js` (scope `/`, served by Go, `Cache-Control: no-cache`, the page
CSP). The script is `internal/web/sw.js` with two values filled in at start:
the build id (first 12 hex digits of a SHA-256 over every embedded static file
and template, so any UI change makes a new worker) and the list of precached
assets. `app.js` registers it when `navigator.serviceWorker` exists.

- Install: precache every `/static/` file except licence texts and the
  manifest screenshots, and `/offline`, into `kena-static-<build>`; put `/`
  (the start URL) into `kena-pages`; `skipWaiting`. Activate: delete other
  `kena-static-*` caches, `clients.claim`, re-fetch every saved page in the
  page cache (best effort).
- `/static/*` and navigations: network first. After 3 s without an answer a
  cached copy is served if there is one; without one the request keeps
  waiting for the network (a slow connection is not an offline one), and
  only a network error falls back. Static files go to the network first (the
  HTTP cache still answers within `max-age`), so an old worker never pairs
  new HTML with old scripts.
- Navigations: a successful response refreshes the page cache (`kena-pages`)
  when it already holds that page (`/`, `/s/{slug}`, `/s/{slug}/play`, query
  ignored); `/` is always kept. A cached page is served with
  `data-offline="1"` added to `<html>`; a network error without one gets the
  precached `/offline` page.
- Everything else (htmx fragments, `/sync`, JSON) goes to the network
  untouched.

`/offline` is a localised page: "You're offline", the decks saved on this
device (filled in by the script) and a Try again link.

## Offline deck pack

`GET /s/{slug}/pack.json` (same visibility as the deck: 404 otherwise),
`Cache-Control: private, no-cache`, `ETag` = the deck's offline version;
`If-None-Match` → 304.

```json
{
  "v": 1, "version": "…", "slug": "…", "title": "…", "lang": "ru",
  "loggedIn": true, "key": "<base64, 16 random bytes>",
  "rules": [{"key", "title", "label", "preview", "general", "html"}],
  "tasks": [{"id", "text", "rule", "hints": ["…"], "x": "<base64>"}],
  "state": {"<task id>": {"answers": ["…"], "correct": false, "gaveUp": true}},
  "msgs": {"play.check": "Проверить", …}, "plurals": {"count.in_a_row": {"one", "few", "many", "other"}, …}
}
```

- `html` is `markdown.Render` output — the same sanitised HTML the ⓘ sheet
  shows; the client inserts it as is and renders nothing else as HTML.
- `x` is `{"a": [[accepted answers per blank]], "e": "explanation"}` as JSON,
  XOR-ed with `key` and base64-encoded. **This is not security**: anyone can
  decode it. It only keeps answers from being readable by a glance at the
  pack in devtools or storage, which is all a learner needs to not spoil a
  deck for themselves.
- `state` is the viewer's last attempt per task (`progress_id` logged in,
  device cookie otherwise), so the offline engine starts where the learner is.
- `msgs` / `plurals` are the raw catalog entries the card, bar and done
  templates use, in the request's UI language.
- Version: `<sets.updated_at µs hex>.<viewer's attempt count in the deck>.<max
attempt id>.<UI language>`; every task, rule, title or visibility change
  bumps `updated_at`, every recorded attempt bumps the counts, and a language
  switch changes the strings.

`GET /offline/decks.json?s=<slug>…` lists `{slug, version}` for the logged-in
viewer's own and starred (non-private) decks (`auto`) and for the requested
slugs the viewer can see (`saved`, at most 100). The client compares versions
and re-downloads only changed packs; a saved slug missing from the answer
is no longer visible and is dropped.

## On the device (`static/sync.js`, every page)

IndexedDB `kena` (version 1):

- `packs` (key `slug`): `{slug, version, lang, auto, saved, user, fetched, pack}`.
  `auto` = own or starred, `saved` = the toggle, `user` = fetched while logged in.
- `attempts` (key `id` = client UUID): `{id, slug, task, answers, correct,
gaveUp, at, synced}`.

Downloading a pack also caches `/s/{slug}` and `/s/{slug}/play` in
`kena-pages`; dropping it deletes them.

- Logged in (`<body data-auth>`), online: on page load, when the app comes
  back to the foreground and every 30 s while it is visible, at most once a
  minute (an agent's edit must reach a pack before the network goes, even in
  a long htmx session that never reloads the page), and at once on the overview of an own/starred deck that is not on
  the device yet, `sync.js` reads `/offline/decks.json` with the saved slugs,
  fetches new or changed packs, drops auto packs that left the list.
  Anonymous: only saved decks are checked.
- A pack is also refreshed (conditional GET) when its deck's overview or
  practice page loads online, or when its language differs from the page's.
- Every queued attempt carries the identity it was recorded under
  (`<body data-identity>`: `user:<id>` logged in, `anon:<first 16 hex of
  SHA-256(device id)>` otherwise, so the HttpOnly device cookie never reaches
  the page). Only attempts of the page's current identity are synced or
  counted by the indicator; others stay dormant until that identity is back
  and are dropped after 30 days. Untagged attempts (queued before this rule,
  one deploy that lived minutes) are adopted once by the identity of the first
  page that sees them and then sync normally.
- Logout: pending attempts are synced first (3 s at most), then user packs,
  synced attempts and every cached page are removed, then the form submits.
  Attempts that could not be sent stay queued and dormant under their
  identity.
- Deck overview: a line under Practise / Worksheet. Own and starred decks
  (logged in): "Available offline ✓" (or "Saving for offline…"). Others:
  a "Save for offline" toggle button (`aria-pressed`); saved, it reads
  "Available offline ✓" and pressing it again removes the deck from the
  device. Hidden without service worker or IndexedDB support.
- Deck rows (home, Discover): "Available offline ✓" after the meta line.

## Offline engine (`static/offline.js`, focus mode)

A pure core (no DOM; loads in node for tests) and the browser glue.

- Core: `normalize`, `check`, `classify` (ports of `internal/greek`; Go's
  per-rune lowercasing, not JS `toLowerCase` final-sigma rules; Go's
  `unicode.IsSpace` set), the PCG-DXSM shuffle of `math/rand/v2` (BigInt) so a
  shuffled session keeps its order, the handlers `next`, `card`, `check` and
  the initial `play` state, and renderers of the `card`, `done`, `bar`
  templates producing the same markup.
- Shared vectors: `internal/greek/testdata/vectors.json` (normalise, check,
  classify, levenshtein) and `internal/web/testdata/shuffle.json` are asserted
  by Go tests and by `node --test`.
  `internal/web/testdata/offline_golden.json` is written by a Go test from
  the real handlers (a pack and a sequence of fragment requests with their
  responses); the Go test fails when the handlers' output changes and the
  file wasn't regenerated (`KENA_UPDATE_GOLDEN=1 go test ./internal/web -run
TestOfflineGolden`), and the JS test replays the sequence against the
  engine and compares the markup (whitespace-collapsed).
- When it runs: htmx requests from the focus page (`/s/{slug}/next`,
  `/s/{slug}/play/card`, `POST /t/{id}/check`) are handled locally when the
  deck has a pack and either `navigator.onLine` is false, an earlier request
  failed less than 30 s ago, or this request fails (`htmx:sendError`) or
  times out (`htmx.config.timeout` = 3 s on this page). Local handling cancels
  the htmx request and swaps the rendered fragment through `htmx.swap`, so
  `play.js` (history, swipes, focus, animation) and `info.js` work unchanged.
  The network-error flash is suppressed for handled requests.
- Started from the cache (`<html data-offline>`): the card area stays hidden
  until the engine has rendered the page state for the URL (`at`, `after`,
  filter, order; a shuffle without a seed gets one and `replaceState`), the
  ⋯ sheet's filter and order links are updated; `play.js` records history only
  after that render.
- Local state = `pack.state` overlaid with the device's attempts made after
  the pack was fetched (online checks included), so a pack downloaded
  before a session stays right.
- Every check on the focus page carries `cid` (a client UUID). Online the
  server stores it in `attempts.client_id`; when the request times out, the
  local replay queues the same `cid`, so the attempt is recorded once even if
  the server got the first request.

## Sync

`POST /sync`, JSON `{"identity", "attempts": [{"id", "task", "answers",
"gaveUp", "correct", "at"}]}`; `identity` is required (400) and must equal the
session's (409 for the whole batch, with the current identity in the body);
`at` = ms since epoch; same-origin `fetch` with
credentials, `session.CrossOrigin` applies, `Content-Type: application/json`
required (415), body ≤ 256 KiB (413), at most 200 attempts (400).

Then per attempt, under the viewer's progress (`progress_id` or device cookie):

- id not a UUID or answers ≠ blanks → `invalid`;
- task missing or its deck not visible to the viewer → `skipped`;
- otherwise re-checked with `greek.Check` (the client's `correct` is
  ignored; `gaveUp` counts only when wrong), recorded with `created_at` =
  `at` clamped to [now − 30 days, now] and `client_id` = id; an id already
  recorded → `duplicate` (migration 011: `attempts.client_id uuid UNIQUE`,
  NULL for older rows and other writers).

Answer: `{"results": [{"id", "status", "correct"}]}`. The client marks
`recorded`, `duplicate`, `skipped` and `invalid` as done; network errors and
5xx leave items pending. Sync runs on page load, on the `online` event, after
each offline attempt, every 30 s while items are pending, and on tap.

## Indicator

- Focus bar, between › and ⋯: `✓` (all synced), `⟳ N` (N pending), a
  cloud-with-a-slash icon when offline (with `⟳ N` when something is
  pending). Tap: sync now (toast "Syncing…" / "All answers synced"), or
  offline a toast that answers are kept on this device and sync when back
  online. `aria-label`/`title` carry the localised state ("Offline", "Not
  synced yet: 3", "All answers synced").
- Site header: the same control, only while something is pending, and only
  `⟳ N` (no icon) so the nav still fits at 390 px; below that the nav scrolls
  sideways as it already does under 360 px.
- Deviation from the brief: the bars show a cloud-with-a-slash icon instead of
  the word "offline" — at 375 px the Greek «εκτός σύνδεσης» doesn't fit next
  to the progress bar. The word is in the label/title and the tap toast.
- Back online: the `online` event syncs at once (retrying after 1, 3 and
  10 s); without that event (lie-fi ends quietly) a probe every 30 s while
  something is pending gets there.

All strings are in the en/ru/el catalogs and reach the scripts through
`data-*` attributes.

## Verification

Go: migration 011; pack (visibility, obfuscation round trip, state, ETag/304,
strings); decks index (own/starred/saved, private dropped); `/sync`
(idempotent by id, server re-check overrides `correct`, give-up only when
wrong, invisible task skipped, timestamp clamp, size and count caps,
Content-Type, cross-origin rejected); online `cid`; `/sw.js` (build id,
precache list, headers); `/offline`; i18n completeness; shared vectors;
golden file current.

JS: `node --test internal/web/jstest/*.test.mjs` (vectors, shuffle, plurals,
golden replay, errors, state overlay); also a `nix flake check` check
(`offline-js`), so node is a check input but not a build input of the package.

Browser (headless Chromium over CDP): online visit saves the deck; network
offline (`Network.emulateNetworkConditions`); reload the play page from the
worker; wrong answer (shake + nudge), give up, correct, swipes; indicator
shows pending; back online → sync → ✓; the attempts are in the database
exactly once and a second sync adds nothing; no CSP violations throughout.
Screenshots of the indicator states.
