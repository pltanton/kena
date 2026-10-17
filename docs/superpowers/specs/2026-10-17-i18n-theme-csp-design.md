# UI languages, theme button, strict CSP

Date: 2026-10-17. Status: accepted. Extends `2026-10-17-practice-flow-design.md`
(theme) and `2026-10-17-focus-mode-design.md` (the ⋯ menu).

## 1. Languages

The whole site UI is available in English (`en`), Russian (`ru`) and Greek
(`el`): every template (web and the OAuth authorize page), page titles,
aria-labels and titles, flash messages, form errors and notices, nudges,
mistake kinds, the editor and rules cheat-sheets, the Connect page, admin
pages, dates (month names) and the prompts we suggest to paste into an agent.

Not translated: deck content (titles, tasks, rules, instructions), MCP tool
texts, the authoring guide, server logs, and the diagnostics of the deck
parser and the task/markdown lint (`deckfmt`, `greek`, `markdown.Issues`):
those strings are shared with MCP, so the editor and rules page show them in
English inside localised framing ("Nothing was saved…").

### Catalog

- `internal/i18n`: one Go map per language (`en.go`, `ru.go`, `el.go`), keys are
  dotted message ids (`play.check`, `nudge.accent`). Plain messages are
  `fmt` formats; plurals live in a second map with CLDR forms:
  en and el `one`/`other`, ru `one`/`few`/`many` (integers only).
- Template functions, bound per language (each language has its own parsed
  template set): `t "key"`, `tf "key" args…` (Sprintf), `tn "key" n args…`
  (plural; `n` is the first format argument), `th "key" args…` (the catalog
  string is trusted HTML, every argument is HTML-escaped; only for messages
  that carry a link), `lang` (the code for `<html lang>`).
- Go code translates with `i18n.T(lang, key, args…)` / `i18n.N(lang, key, n, args…)`;
  views carry message keys for fixed labels (filters, visibility, theme).
- A missing key renders as `!key`; tests assert that every catalog has the same
  keys and the required plural forms, that every key used in templates and Go
  code exists, and that rendered pages contain no `!` markers.
- Terms: deck = «набор» / «σετ» (indeclinable, keeps Greek forms simple),
  task = «задание» / «άσκηση», rule = «правило» / «κανόνας», practise =
  «тренировка» / «εξάσκηση», worksheet = «лист заданий» / «φύλλο ασκήσεων»,
  passphrase = «кодовая фраза» / «φράση πρόσβασης». Russian addresses the user
  as «вы», Greek uses the polite plural imperative («Ελέγξτε»), modern
  monotonic spelling.

### Selection

1. Cookie `kena_lang` = `en` | `ru` | `el` (1 year, HttpOnly, Lax).
2. Otherwise `Accept-Language`: the highest-q tag whose primary subtag is one
   of en/ru/el (`q=0` excluded; ties keep header order).
3. Otherwise `en`.

`<html lang>` follows the UI language; Greek sentences, inputs, instructions
and the wordmark keep `lang="el"` on their own elements. HTML pages send
`Vary: Accept-Language`.

### Switch

- `POST /lang` (`session.CrossOrigin`) with `lang` and a safe `next`: sets the
  cookie, 303 to `next`; with `HX-Request` 204. Unknown language → 400.
- Logged in: Account page, section "Language" (three buttons).
- Anonymous: a language menu in the header (globe button showing the current
  code, a `<details>` with three submit buttons), next to the theme button.
- Focus mode: a "Language" row in the ⋯ sheet.
- Without JS each button is a plain form submit (redirect back). With JS the
  form posts in the background and the page reloads.

## 2. Theme button

- The footer and its theme switch are gone.
- One icon button in the header, right side, before the account icon (or the
  language menu for anonymous visitors). It shows the current mode: half-filled
  circle (Auto), sun (Light), moon (Dark); inline SVG, `currentColor`, 44px tap
  target; `aria-label`/`title` "Theme: Auto" etc., localised.
- Click cycles Auto → Light → Dark → Auto. With JS: `data-theme` and the
  theme-color metas change at once, the button updates, a small transient
  toast (`#toast`, `role="status"`, ~1.6 s) names the new mode, and
  `POST /theme` runs in the background (`HX-Request` → 204). Without JS the
  button is a form posting the next mode with `next` = current page.
- The Account page keeps "Appearance" (Auto · Light · Dark) and the focus ⋯
  sheet gets a "Theme" row of the same three buttons.
- The header stays on one line at 375px in all three languages: nav labels
  are short («Мои наборы», «Каталог», «Ошибки»; «Τα σετ μου», «Κατάλογος»,
  «Λάθη»), the wordmark hides below 400px, nav padding tightens.

## 3. Content-Security-Policy

All inline scripts move to static files: `static/app.js` (flash, toast, theme
button and switches, language forms, copy buttons, `data-confirm` submit
guard), `static/play.js` (focus mode), existing `reveal.js` and `info.js`.
Localised strings the scripts need come from `data-*` attributes. No inline
event handlers exist.

Every HTML page (web `page`/`pageAs`, error pages, the OAuth authorize page)
sends:

```
default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline';
img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none';
base-uri 'none'; form-action <targets>; frame-ancestors 'none'
```

plus `X-Frame-Options: DENY` (and `nosniff` for everything from `app`).
`style-src 'unsafe-inline'` stays for `style=""` widths on progress bars and
htmx's indicator style.

`form-action`: browsers apply it to the redirects that follow a form
submission. Web pages only post to themselves and redirect to safe local
`next`s, so they send `form-action 'self'`. The authorize page posts to
`/oauth/authorize`, which answers Allow and Cancel with a 302 to the client's
`redirect_uri`, an arbitrary registered https URL or a loopback http URL with
any port. So each authorize response sends `form-action 'self' <origin of
the validated redirect_uri>` (`https://claude.ai`, `http://127.0.0.1:53123`);
an IPv6 loopback literal can't be written as a CSP host, so that case allows
the `http:` scheme. Fatal-error pages (no validated redirect) send `'self'`.

A headless Chrome run over every page (home, deck, play with swipes and the ⓘ
sheet, worksheet, editor, rules, account, mistakes, connect, admin, login,
authorize through Allow to a loopback client) must report no
`securitypolicyviolation` events.
