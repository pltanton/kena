# kena: visibility levels and deck editor

Date: 2026-10-17. Status: approved. Supersedes the shared/private toggle in
`2026-10-17-ui-sharing-design.md`.

## Visibility

`sets.visibility text NOT NULL CHECK (visibility IN ('private','friends','public')) DEFAULT 'private'`.
Migration: `shared = true` → `public`, else `private`; drop `shared`.

| level   | who can view                                                        |
| ------- | ------------------------------------------------------------------- |
| private | owner only (admins via MCP as today)                                |
| friends | owner + any logged-in user; listed in /browse; starrable            |
| public  | anyone incl. anonymous; listed on the anonymous home and in /browse |

- Anonymous home: public decks. Logged-in home: own + starred (still visible).
- /browse (logged in): friends + public decks of other users. Anonymous /browse:
  public decks only.
- Set page for owner: three-way toggle 🔒 Private · 👥 Friends · 🌍 Public
  (`POST /s/{slug}/visibility`, value private|friends|public). Replaces the share toggle.
- Stars on a deck that becomes private: dormant, as today.
- MCP: `visibility` replaces `shared` in create_set (default private), update_set
  and all outputs. Guide: ask the user which level after creating a set.

## Editor (owner only)

`GET/POST /s/{slug}/edit`; `GET/POST /new` for a new deck (logged in).

- Details form: title, topic, level, instructions, source_ref, visibility.
- Source textarea: the whole deck as plain text.
  - Tasks separated by blank lines. First line = sentence; blanks written
    `{answer / alternative | hint}` (hint optional; answers split on `/`, trimmed).
  - Following lines starting with `>` = explanation (joined with a space).
  - Literal `{` `}` `|` are not supported inside sentences (rare in Greek prose).
  - Serialiser and parser round-trip: `parse(format(tasks)) == tasks`.
- Save: parse → same NFC + lint as MCP (`prepare`) → on any error re-render the
  form with errors as `line N: message`, nothing written. On success: tasks are
  matched by position; same text+blanks+explanation → untouched (keeps attempts);
  changed → UpdateTask; extra → AddTasks; missing at the end → DeleteTask.
  One transaction.
- Delete deck: `POST /s/{slug}/delete` after a confirm page
  (`GET /s/{slug}/delete`, "Delete <title> and its N tasks? This removes everyone's
  progress on it."). Redirect to `/`.
- Entry points: "Edit" on the set page (owner), "New deck" on the logged-in home.
- All POSTs behind the existing CrossOrigin protection; non-owners get 404.

## Out of scope

Reordering by drag, per-task forms, version history.
