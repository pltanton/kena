# kena

Gap-fill trainer for Modern Greek grammar. A sentence has blanks (`___`), you
type the missing forms, the server checks them strictly (accents count).

kena never generates exercises. An agent does: you show Claude or ChatGPT a photo
of a textbook exercise, it writes new tasks on the same grammar point and uploads
them through kena's MCP server. The MCP server also hands the agent an authoring
guide (`get_authoring_guide`) so the tasks are unambiguous and level-appropriate.

- Web: `/` lists your decks, `/s/<slug>` is a deck's overview: progress, a
  "which tasks" chooser (Unsolved N · Mistakes N · All N, Shuffle; Unsolved
  by default once you have started) and two buttons: Practise (one task at a
  time in a full-screen focus mode, `/s/<slug>/play`: a thin bar with back,
  progress and a ⋯ menu; Enter for the next task, a streak counter, resumes at
  the first unsolved task) and Worksheet (all chosen tasks on one page,
  `/s/<slug>/list`). A wrong answer is not revealed:
  you try again (with a nudge for a wrong accent, final ς or a letter or two
  off) or press Give up to see it; only first-try answers grow the streak, and
  every wrong try still counts in Mistakes. "First unsolved" jumps back,
  "Reset progress" (bottom of the deck page) clears your answers on that deck.
  Practising needs no login: progress is tied to an anonymous device cookie.
  Logging in (username + passphrase) keeps progress in the account, so every
  device sees the same; what you solved before logging in moves to the account.
- Account page (`/account`): log in on your phone by scanning a one-time QR
  code (5 minutes), theme (Auto, Light, Dark; also in the page footer), change
  passphrase, personal tokens, connected apps.
- MCP: `/mcp` (Streamable HTTP). OAuth (Claude app, ChatGPT: you log in and
  press Allow on a kena page) or personal bearer tokens (Claude Code, Hermes).
  You can only edit your own sets; admins can edit everything and manage users.
- Visibility: every deck is Private (only its author), Friends (anyone
  logged in) or Public (everyone, also without login). A new deck is
  private. The author switches the level in the editor (Who can see it), or
  by asking the agent (`update_set` with `visibility`); the agent asks after
  creating a set. Decks that were shared before the levels existed are public.
- Discover and star: logged in, My decks (the home page) shows only your own
  decks (with a private / friends badge) and the ones you starred. Header →
  Discover lists other authors' friends and public decks; ☆ adds one to My
  decks, ★ removes it (also on the set page). A starred deck that its author
  makes private disappears from your list. Anonymous visitors see public decks
  on the home page and in Discover.
- Editing: the author edits a whole deck as text (set page → Edit deck) and
  creates one from scratch with New deck on the home page. One task per
  paragraph, blanks in braces, `>` lines for the explanation:

  ```
  = present: Present tense: stem + ending of the person (γράφ-εις, γράφ-ει).

  Εσύ {γράφεις | γράφω} πολύ.
  @present
  > Explanation, only what is specific to this task.

  Θα {πάω / θα πάω} αύριο στη θάλασσα.
  ```

  `{a / b}` accepts both answers, `| hint` is shown next to the blank. A task
  may span several lines (one dialogue turn per line). `= key: text` lines at
  the top are deck rules, the grammar many tasks share; `@key` under a task
  points it at one, and the learner sees the rule with the explanation. Saving
  runs the same checks as the MCP tools: any error is listed as `line N: …`
  and nothing is saved; warnings show after the save. Unchanged tasks keep
  everyone's progress even when you insert or reorder tasks. If the deck
  changed since you opened the editor (for example an agent added tasks),
  the save is refused and your text stays in the form to re-apply. Delete deck (bottom
  of the editor) asks for confirmation first.

## Run locally

Needs Go and a Postgres.

```
export KENA_DATABASE_URL="postgres://localhost/kena"
export KENA_PUBLIC_URL="http://127.0.0.1:8080"
go run ./cmd/kena user create anton --admin   # prints a passphrase
go run ./cmd/kena serve
```

| env                 | default          |                                                        |
| ------------------- | ---------------- | ------------------------------------------------------ |
| `KENA_DATABASE_URL` | required         | pgx connection string                                  |
| `KENA_PUBLIC_URL`   | required (serve) | external base URL; OAuth metadata and set links use it |
| `KENA_LISTEN`       | `127.0.0.1:8080` |                                                        |

Migrations run on `serve`, `migrate`, `user` and `token create`.

## NixOS

```nix
# flake inputs
kena.url = "github:pltanton/kena";

# configuration
imports = [inputs.kena.nixosModules.default];
services.kena = {
  enable = true;
  publicUrl = "https://kena.example.org";
};
services.caddy.virtualHosts."kena.example.org".extraConfig = ''
  reverse_proxy 127.0.0.1:8080
'';
```

The module creates a local Postgres database and a `kena` system user, and
puts a `kena-admin` wrapper on the PATH that runs the CLI as that user:

```
sudo kena-admin user create anton --admin   # prints anton's passphrase
sudo kena-admin user create maria           # prints maria's passphrase
sudo kena-admin user passwd maria           # new passphrase; logs maria out everywhere
```

## Accounts and connecting agents

1. An admin creates the account on the Admin page (Account → Admin, `/admin`),
   with `sudo kena-admin user create maria`, or by asking an admin agent to
   call `admin_create_user`. Each shows a generated passphrase like
   `olive-tiger-harbor-seven` once.
2. The admin sends maria the username and passphrase.
3. Maria logs in on the site (header → Log in). She can change the passphrase
   on her Account page.

Then, to let an agent write sets for her:

- **Claude app (claude.ai):** Settings → Connectors → Add custom connector →
  URL `https://kena.example.org/mcp`. On the kena page that opens, log in (if
  not already) and press Allow.
- **ChatGPT:** Settings → Apps → developer mode → create app with the same URL,
  authentication OAuth. Log in / Allow on the kena page.
- **Claude Code:** Account → New token, then
  `claude mcp add --transport http kena https://kena.example.org/mcp --header "Authorization: Bearer kena_…"`
  (the Account page shows this command with the token filled in).
- **Hermes / other agents:** Account → New token; Streamable HTTP endpoint
  `https://kena.example.org/mcp`, header `Authorization: Bearer kena_…`.

Admins can still issue tokens for someone with `admin_issue_token` or
`sudo kena-admin token create --user maria --label laptop`.

Example prompt with a photo attached:

> This is exercise 3 from unit 5 of my B1 book. Make 100 more tasks on the
> same grammar point and upload them to kena.

The agent reads the guide, writes new sentences in batches, validates them and
returns the set URL.

## Mistakes and analysis

Logged in, `/mistakes` (header → Mistakes) lists the tasks you got wrong in the
last 30 days: your answer → the expected one, with a badge for accent, final ς
or near-miss slips, and a ✓ once you solved the task later. Practising
anonymously does not count until you log in.

A connected agent can read the same data with `get_mistakes_report` (only your
own) and turn it into practice:

> Analyze my kena mistakes and make a practice set for my weak spots.

It groups the mistakes by grammar point, separates spelling slips from wrong
forms, explains the main patterns and offers a "Practice: …" set of new tasks.

## Revoking access

- Users revoke their own tokens and disconnect apps on the Account page.
  Changing the passphrase logs out other devices and disconnects all apps.
- Forgotten passphrase: Admin → maria → Reset passphrase, `sudo kena-admin
  user passwd maria` or `admin_reset_password`. This also logs maria out
  everywhere and disconnects her apps; her personal tokens keep working.
- Admin → a user also makes or removes admins, logs the user out everywhere
  (tokens stay) and deletes the user with their decks and progress (type the
  name to confirm). Nobody can change their own admin flag or delete
  themselves, and the last admin stays. MCP: `admin_set_admin`,
  `admin_logout_user`, `admin_delete_user` (with `confirm_username`).
- Admins revoke a token with `admin_revoke_token` (ids from `admin_list_users`).

## Development

See `AGENTS.md`. Design: `docs/superpowers/specs/2026-10-16-kena-design.md`,
accounts: `docs/superpowers/specs/2026-10-16-accounts-design.md`, mistakes:
`docs/superpowers/specs/2026-10-17-mistakes-report-design.md`, UI and sharing:
`docs/superpowers/specs/2026-10-17-ui-sharing-design.md`, stars:
`docs/superpowers/specs/2026-10-17-stars-design.md`, visibility and editor:
`docs/superpowers/specs/2026-10-17-visibility-editor-design.md`.
