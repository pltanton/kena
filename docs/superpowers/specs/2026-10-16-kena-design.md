# kena — Greek grammar gap-fill trainer

Date: 2026-10-16. Status: approved for MVP.

## Goal

A web app to practise Modern Greek grammar with gap-fill tasks (a sentence with
one or more blanks, the learner types the missing form). The app never generates
content. Tasks are authored by external agents (Claude app, ChatGPT, Claude Code,
Hermes) through an MCP server: "here is a photo of a book exercise, make 100 more
like it and upload them".

## Architecture

One Go binary (`kena`), Postgres for storage.

- `/` and `/s/<slug>`: server-rendered HTML (`html/template` + htmx). Templates,
  CSS and `htmx.min.js` are embedded with `go:embed`. No Node toolchain.
- `/mcp`: MCP server, Streamable HTTP, official `github.com/modelcontextprotocol/go-sdk`.
- OAuth 2.1 authorization server for MCP clients that only speak OAuth
  (claude.ai connectors, ChatGPT apps).
- CLI subcommands for bootstrap: `kena serve`, `kena migrate`,
  `kena token create --user NAME [--admin] [--label LABEL]`.

Config via env: `KENA_DATABASE_URL`, `KENA_LISTEN` (default `127.0.0.1:8080`),
`KENA_PUBLIC_URL` (e.g. `https://kena.example.org`, used for OAuth metadata and
the MCP resource URL).

Packaging: Nix flake with `buildGoModule` package and a NixOS module
(`services.kena`: enable, publicUrl, listen, database via local
`services.postgresql` with peer auth by default). Reverse proxy is left to the host.

## Data model (Postgres)

Migrations are embedded SQL files applied in order at `serve`/`migrate` start,
tracked in `schema_migrations`.

- `users(id, username unique, is_admin, created_at)`
- `tokens(id, user_id, label, hash unique, created_at, last_used_at, revoked_at)` —
  `hash` is sha256 of the token; tokens are `kena_` + 32 random bytes base32.
- `oauth_clients(client_id pk, redirect_uris text[], client_name, created_at)` —
  DCR-registered clients. CIMD clients (`client_id` is an https URL) are fetched
  and cached here too.
- `oauth_codes(code_hash pk, client_id, user_id, redirect_uri, code_challenge, resource, scope, expires_at, used_at)`
- `oauth_grants(id, user_id, client_id, access_hash unique, access_expires_at, refresh_hash unique, refresh_expires_at, revoked_at)`
  — refresh rotation: each refresh revokes the row and inserts a new one.
- `sets(id, owner_id, slug unique, title, topic, level, instructions, source_ref, created_at, updated_at)`
- `tasks(id, set_id, position, text, blanks jsonb, explanation, created_at, updated_at)`
  — `text` contains one `___` per blank; `blanks` is
  `[{"answers": ["έγραψα"], "hint": "γράφω"}]`. Order of `___` = order of `blanks`.
- `attempts(id, device_id uuid, task_id, answers jsonb, correct, created_at)`

Deleting a set deletes its tasks and attempts (cascade).

## Authentication

Every MCP request needs `Authorization: Bearer <t>`. The verifier accepts either
a personal token (`tokens` table) or an OAuth access token (`oauth_grants`).
Both resolve to a user.

OAuth (for claude.ai and ChatGPT), per `docs/research.md`:

- `GET /.well-known/oauth-protected-resource` and
  `/.well-known/oauth-protected-resource/mcp` (RFC 9728): `resource` equals
  `<public>/mcp` exactly; one authorization server = `<public>`.
- Unauthenticated `/mcp` → `401` with
  `WWW-Authenticate: Bearer resource_metadata="<public>/.well-known/oauth-protected-resource/mcp"`.
- `GET /.well-known/oauth-authorization-server` (RFC 8414) with
  `code_challenge_methods_supported: ["S256"]`,
  `token_endpoint_auth_methods_supported: ["none"]`,
  `client_id_metadata_document_supported: true`,
  `authorization_response_iss_parameter_supported: true`,
  `grant_types_supported: ["authorization_code","refresh_token"]`.
- `POST /oauth/register` (RFC 7591 DCR, public clients only).
- CIMD: if `client_id` is an https URL, fetch it (5 s timeout, 64 KB cap), check
  that its `client_id` equals the URL and that `redirect_uri` is listed.
- `GET /oauth/authorize`: validates client, redirect_uri, PKCE S256; shows a form
  "Paste your kena token". `POST` checks the token, issues a code (10 min), and
  redirects with `code`, `state`, `iss`.
- Redirect URI rules: exact match against registered URIs, except loopback
  (`http://localhost:*`, `http://127.0.0.1:*`) where the port is ignored.
- `POST /oauth/token` (form-urlencoded): `authorization_code` (PKCE verify,
  single use, `resource` echoed) and `refresh_token` (rotate; unknown/expired/
  revoked → `invalid_grant`). Access token TTL 1 h, refresh 90 days.

Admin bootstrap: `kena token create --user anton --admin` creates the user if
missing and prints a token once.

## MCP server

Server `instructions` hold the 10 most important authoring rules (first 512
chars are the key ones) and tell the agent to call `get_authoring_guide` first.
Tool descriptions repeat the hard rules relevant to that tool.

Tools (all results are JSON text content; errors are tool errors with a readable
message naming the task index):

| tool                                                                        | who         | what                                               |
| --------------------------------------------------------------------------- | ----------- | -------------------------------------------------- |
| `get_authoring_guide(topic?)`                                               | any         | guide text; no topic = index + workflow            |
| `list_sets(owner?)`                                                         | any         | id, slug, title, topic, level, owner, task count   |
| `get_set(set)`                                                              | any         | set meta + all tasks (by id or slug)               |
| `create_set(title, topic, level, instructions, source_ref?, slug?, tasks?)` | any         | creates set owned by caller; returns id, slug, url |
| `add_tasks(set, tasks[])`                                                   | owner/admin | appends; max 50 per call                           |
| `update_set(set, fields…)`                                                  | owner/admin | partial update of meta                             |
| `update_task(task_id, text?, blanks?, explanation?)`                        | owner/admin |                                                    |
| `delete_task(task_id)` / `delete_set(set)`                                  | owner/admin |                                                    |
| `validate_tasks(tasks[])`                                                   | any         | dry-run lint, returns errors and warnings          |
| `admin_create_user(username, is_admin?)`                                    | admin       |                                                    |
| `admin_issue_token(username, label?)`                                       | admin       | returns token once                                 |
| `admin_revoke_token(token_id)`                                              | admin       |                                                    |
| `admin_list_users()`                                                        | admin       | users with token ids/labels/last use               |

The same guide topics are also exposed as MCP resources `skill://kena/<topic>`
(SKILL-style markdown) for clients that read resources.

### Task lint (on every write)

Errors (reject the whole call):

- number of `___` ≠ number of blanks; blank with no answers; empty text.
- Latin letters or digits mixed into a Greek word (lookalikes like `o`, `v`, `A`).
- Polytonic / oxia code points (U+1F00–U+1FFF).
- `σ` at end of word, `ς` inside a word (in text and answers).
- Answer containing `___`.

Warnings (returned, write proceeds):

- Duplicate sentence within the set.
- Polysyllabic answer word without a tonos.
- Text longer than 20 words.

All text is NFC-normalised before storing.

## Authoring guide

Markdown files in `internal/guide/*.md`, embedded. Topics:

- `workflow`: photo → extract a spec (grammar target, paradigm slots, word pool,
  hint format, level) → discard the book's sentences → write new ones in
  batches of 20 → self-check → `validate_tasks` → `create_set`/`add_tasks`.
  ~30 blocked items first, then ~70 interleaved.
- `item-rules`: one target per blank; context forces the form; hint = lemma or
  feature tag, never the answer; 6–14 words; A2–B1 vocabulary; meaningful
  sentences; variety grid; no giveaways.
- `greek-pitfalls`: accent shift, enclitic double accent, final ς, monotonic only,
  aspect after θα/να (force with cues), clitic placement, τη(ν)/δε(ν)/μη(ν),
  variant forms (list as extra answers), lemmas to avoid.
- `self-check`: blind re-solve, derive forms from paradigm, native-reading pass,
  level measurement, mechanical lint.
- `format`: JSON schema and examples for tasks and sets.

## Web UI

Mobile-first, no login. A `kena_device` cookie (random UUID, 1 year, HttpOnly,
SameSite=Lax) identifies the device for progress.

- `/`: sets grouped by owner; each shows task count and the device's
  solved/total and mistakes count.
- `/s/<slug>?mode=cards|list&filter=all|unsolved|mistakes&shuffle=1`. Mode is
  remembered in a `kena_mode` cookie.
  - "solved" = last attempt correct; "mistakes" = last attempt wrong.
- Cards mode: one task per screen, progress bar, input autofocused,
  `lang="el"`, `autocapitalize="off"`, `autocomplete="off"`. Submit →
  `POST /t/<id>/check` (htmx) → fragment:
  - correct: green state, next card loads after 600 ms (`hx-trigger="load delay:600ms"`).
  - wrong: shows given vs. expected per blank, explanation if any, "Next" button.
  - Next card = next task in order (or shuffled order, seeded per page load and
    carried in the request) within the active filter; at the end, a summary with
    "again" and "mistakes only" links.
- List mode: all tasks of the filter on one page, each a small form; check
  swaps just that task.

Checking happens on the server; answers are never sent to the browser before an
attempt. Normalisation: NFC, trim, collapse internal whitespace, lowercase. Then
exact match against any accepted answer (accents and final ς matter).

## Errors

- HTTP handlers: 404 page for unknown set/task, 400 for bad form input, 500 logs
  and shows a plain message.
- MCP: tool errors carry user-readable text; ownership violations say whose set it is.
- OAuth: RFC 6749 error JSON from the token endpoint; invalid redirect URIs are
  never redirected to (error page instead).

## Testing

- Tests run against a throwaway Postgres started from the local `initdb`/`postgres`
  binaries in `TestMain` (skipped with a message if they are absent), or
  `KENA_TEST_DATABASE_URL` if set.
- Unit: answer checker, lint rules, token hashing, redirect URI matching, PKCE.
- Store: CRUD, ownership, cascade, progress queries.
- OAuth end to end with `httptest`: discovery → DCR → authorize (token form) →
  code → token → MCP call → refresh rotation → old refresh rejected.
- MCP: tools through the SDK client over the HTTP handler, including ownership
  and admin checks.
- Web: handler tests asserting key fragments (correct/wrong, filter, cookie).

## Out of scope (MVP)

Web login, task types other than gap-fill, spaced repetition, image upload,
editing in the web UI, rate limiting, i18n of the UI (UI is English).
