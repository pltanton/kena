# AGENTS.md — kena

Greek gap-fill trainer: htmx web UI + MCP authoring server + minimal OAuth
server, one Go binary, Postgres. User overview in README.md. Design:
`docs/superpowers/specs/2026-10-16-kena-design.md`, accounts:
`docs/superpowers/specs/2026-10-16-accounts-design.md`, mistakes report:
`docs/superpowers/specs/2026-10-17-mistakes-report-design.md`, UI and sharing:
`docs/superpowers/specs/2026-10-17-ui-sharing-design.md`, stars:
`docs/superpowers/specs/2026-10-17-stars-design.md`, visibility and editor:
`docs/superpowers/specs/2026-10-17-visibility-editor-design.md`, practice flow, QR login and theme:
`docs/superpowers/specs/2026-10-17-practice-flow-design.md`, deck rules:
`docs/superpowers/specs/2026-10-17-deck-rules-design.md`, rich rules and the ⓘ sheet:
`docs/superpowers/specs/2026-10-17-rich-rules-design.md`, swipe navigation:
`docs/superpowers/specs/2026-10-17-swipe-nav-design.md`, admin:
`docs/superpowers/specs/2026-10-17-admin-design.md`, UI languages, theme button and CSP:
`docs/superpowers/specs/2026-10-17-i18n-theme-csp-design.md`, offline practice and sync:
`docs/superpowers/specs/2026-10-18-offline-design.md`, partial check:
`docs/superpowers/specs/2026-10-18-partial-check-design.md`, public rules:
`docs/superpowers/specs/2026-10-18-public-rules-design.md`, choice blanks:
`docs/superpowers/specs/2026-10-28-choice-blanks-design.md`, task kinds and question cards:
`docs/superpowers/specs/2026-10-28-question-cards-design.md`, single sign-on:
`docs/superpowers/specs/2026-10-28-sso-design.md`, success sound:
`docs/superpowers/specs/2026-10-28-success-sound-design.md`; client/OAuth research:
`docs/research.md`.

## Commands

```
go test ./...        # needs initdb + postgres on PATH (throwaway cluster) or KENA_TEST_DATABASE_URL
go vet ./...
node --test internal/web/jstest/*.test.mjs   # offline engine vs Go (nodejs is in the dev shell)
KENA_UPDATE_GOLDEN=1 go test ./internal/web -run 'TestOfflineGolden|TestShuffleVectors'   # after changing card/done/bar templates or practice handlers
nix build            # after go.mod changes: update vendorHash in nix/package.nix
nix flake check
```

## Layout

- `cmd/kena`: serve, migrate, user create/passwd, token create, version.
- `internal/app`: routing; `/mcp` behind `auth.RequireBearerToken`.
- `internal/db`: pool, embedded SQL migrations; `dbtest` starts a throwaway cluster per test binary.
- `internal/store`: all SQL. Does not lint.
- `internal/greek`: NFC, answer normalisation, strict check, error kinds (`Classify`), task lint, `PrepareDeck` (NFC + lint + duplicate, repeated-explanation and batch style warnings).
- `internal/deckfmt`: the editor's plain-text deck format; `Format` and `Parse` round-trip, parse errors carry line numbers.
- `internal/markdown`: goldmark rendering for rule bodies (no raw HTML, safe link schemes, tables wrapped for scrolling), `Preview`, `Issues`.
- `internal/guide`: authoring guide markdown served by `get_authoring_guide`, `skill://kena/<topic>` and server instructions.
- `internal/mcpserver`: tools; every write goes through `prepare` (`greek.PrepareDeck`, with the set's existing tasks) before touching the store.
- `internal/oauth`: discovery metadata, DCR, CIMD (SSRF-guarded fetch), authorize page (login + Allow via the web session), token endpoint, bearer verifier.
- `internal/passphrase`: EFF short wordlist passphrases, argon2id hashing.
- `internal/i18n`: UI message catalogs (`en.go`, `ru.go`, `el.go`), plurals, `kena_lang` / Accept-Language selection, template funcs `t`, `tf`, `tn`, `th`.
- `internal/csp`: the Content-Security-Policy every HTML page sends.
- `internal/session`: `kena_session` cookie, login, cross-origin protection; shared by `web` and `oauth` (oauth must not import web).
- `internal/web`: templates, htmx, static scripts (`app.js`: flash, toast, theme and language switches, copy buttons, `data-confirm`, service worker registration; `play.js`: focus mode; `sync.js`: IndexedDB packs and attempt queue, `/sync`, the sync indicator, Save for offline; `offline.js`: the offline engine, a port of `greek` checking and of the focus-mode handlers and templates; `picker.js`: the app-styled list that replaces a choice select's native popup), `sw.js` (served at `/sw.js` with the build id and precache list), offline pack `/s/{slug}/pack.json`, `/offline/decks.json`, `POST /sync`, `/offline`, `kena_device` cookie, deck overview (`/s/{slug}`: which-tasks chooser, Practise and Worksheet), focus-mode practice (`/s/{slug}/play`; ‹ › and swipes, `/s/{slug}/play/card` renders a card in its current state), worksheet (`/s/{slug}/list`), login, account and mistakes pages, deck editor (`/new`, `/s/{slug}/edit`, `/s/{slug}/delete`; saves through `greek.PrepareDeck` and `store.SaveDeck`), rules page (`/s/{slug}/rules`; `store.ReplaceRules`; attach public rules), public rules (`/rules`, `/rules/{category}/{name}`), ⓘ rule sheet (`static/info.js`). Logged in, attempts go under `users.progress_id` instead of the device id.

## Rules

- Answers never reach the practice page before Give up or a correct check: a wrong check re-renders the form with the typed text and right blanks locked; a check with an empty blank (not Give up) is partial: it marks only the filled blanks and records nothing; checking is server-side. `/mistakes` shows expected answers, but only for tasks already attempted. A revisited card (`/play/card`, `?at=`) shows the answer only when the viewer's last attempt is correct or a real give-up (`attempts.gave_up`); a wrong check comes back as the retry form.
- SSO (`KENA_OIDC_*`, `internal/web/sso.go`): code flow with PKCE, state and nonce in the `kena_sso` cookie, ID token verified with go-oidc. It only links to existing accounts (`preferred_username` → username once, then `users.oidc_subject`), never creates or promotes users; passphrase login stays.
- QR login codes (`login_codes`) are hashed, one-time, 5 minutes, one unused code per user; the GET page never consumes a code, only the POST does. kena's logs redact `/login/qr/<secret>` (`session.RedactPath`); a reverse proxy's own access log still records full paths, so keep that log private or redact there too.
- `get_mistakes_report` reads only the caller's own `progress_id`; no tool reads another user's attempts. The report (MCP and `/mistakes`) leaves out other users' private sets.
- State-changing web and authorize POSTs go through `session.CrossOrigin` (net/http CrossOriginProtection, KENA_PUBLIC_URL trusted).
- MCP tool errors the model should see are returned as Go errors from handlers (the SDK then omits structuredContent).
- `sets.visibility`: `private` = owner only (web 404 for everyone else, admins included; MCP not found except for admins), `friends` = any logged-in user, `public` = everyone. `store.CanView` is the single rule; listings use the same SQL conditions.
- Stars only add non-private sets to a user's home list; a starred set that turns private disappears and the star stays dormant.
- `/admin` answers 404 to everyone but logged-in admins. The store enforces the user guards (no changing your own admin flag, no deleting yourself, never zero admins) for web and MCP alike.
- Editor, visibility and delete endpoints are owner-only (404 otherwise, admins included) and behind `session.CrossOrigin`. Editor saves are all-or-nothing: any parse or lint error writes nothing, and a save over a deck changed since the editor opened (`sets.updated_at`, bumped by every task change) is refused with 409.
- Lint rejects whatever the editor's text format cannot hold (`{`, `}`, `|`, `/` in answers, empty lines or lines starting with `>` in text, line breaks outside text and explanation, control characters), so every lint-clean task round-trips through `deckfmt`. Text may span lines (dialogues); `deckfmt` reads consecutive non-`>` lines as one task.
- Choice blanks (`Blank.Choices`, wrong options): the shown order comes only from `choiceOptions` on the server (page and offline pack alike); the form sends `cN` for a choice blank and `a` for typed ones, struck wrong picks travel back as `xN`; checking stays text-based through `greek.Check`. Choice selects must stay exempt from focus mode's pointerdown `preventDefault` (`play.js`), or they never open. Focus mode moves focus to the keyboard keeper only while the keyboard is already up (`typing` in `play.js`), so picking never opens it.
- Task kinds (`tasks.kind`: `gap`, `choice`; `""` in Go means gap): a choice task is a question card — text without `___`, one blank with answers + choices — rendered by the `task_body` template (and `taskBody` in `offline.js`). A new kind goes into lint, `deckfmt`, `task_body`, `offline.js` and the golden file together.
- Rule bodies reach HTML only through `markdown.Render`; never `template.HTML` a body yourself.
- Public rules (`public_rules`, id `category/name`): only admins create or delete categories; any logged-in user creates a rule; only its creator or an admin edits or deletes it, and deleting is refused while a task (`tasks.public_rule_id`) or deck (`set_public_rules`) uses it. Readers see them through `Set.AllRules()` and `Task.Rule` = `category/name`; editing one does not bump `sets.updated_at` but moves `Set.PublicStamp`, which is part of the offline version. A user who created public rules cannot be deleted until an admin transfers them.
- Every UI string goes through `internal/i18n` and exists in all three catalogs (tests check keys used in templates and Go). Deck content, MCP texts, the guide, logs and lint/parse diagnostics stay untranslated. `th` messages are trusted HTML from the catalog; their arguments are escaped.
- No inline `<script>` or `on…=` handlers: scripts live in `static/*.js` and read localised strings from `data-*` attributes (CSP `script-src 'self'`). The authorize page's `form-action` names the validated redirect origin, because browsers apply `form-action` to the redirect after Allow/Cancel.
- Keep MCP tool names stable; agents and the guide reference them.
- Offline: online practice stays server-rendered; `offline.js` only answers htmx requests when the network is gone. It must render exactly what `card.html`, `done.html` and the bar/sentence/notes templates render: change them together, regenerate `testdata/offline_golden.json` and run the node tests. Answer checking lives twice (`greek` and `offline.js`); `greek/testdata/vectors.json` is the shared contract. The pack's answers are XOR-obfuscated, not secret. `/sync` re-checks every attempt, dedupes by `attempts.client_id` and refuses a batch whose `identity` isn't the session's; never trust the client's `correct`, never sync a queued attempt under another identity. Packs download only for the viewer's own and starred decks (logged in) and decks saved with Save for offline.
- The guide must pass the task lint itself (tests enforce no polytonic, no final σ).
