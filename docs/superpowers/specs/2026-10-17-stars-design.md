# kena star decks

Date: 2026-10-17. Status: approved. Extends the UI and sharing spec; replaces
its author chips.

## Goal

A logged-in learner keeps a short personal list instead of scrolling through
every shared deck: their own decks plus the shared decks they starred.

## Data

- Migration 005: `stars(user_id → users ON DELETE CASCADE, set_id → sets ON
DELETE CASCADE, created_at, PRIMARY KEY (user_id, set_id))`.
- `store.Star(user, set)`: set missing or not visible to the user →
  `ErrNotFound`; own set → `ErrInvalid` (own decks are always listed);
  starring twice is a no-op. `store.Unstar` is idempotent.
- `store.MyDecks(user)`: own sets + starred sets the user can still view
  (shared), ordered by title, each with `Starred`. A starred set that became
  private disappears; its star row stays and comes back if it is shared again.
- `store.BrowseDecks(viewer)`: shared sets of other users (anonymous: all
  shared sets), ordered by title, each with `Starred` for the viewer.
- `store.IsStarred(user, set)` for the set page.

## Web

- Header: `Browse` link for everyone.
- `/` logged in: "My decks" — own + starred visible decks; own private ones get
  the "private" badge, starred ones a ★; progress as before. Empty state:
  "No decks yet — Browse shared decks" linking `/browse`.
- `/` anonymous: shared decks grouped by author as before, plus "Log in to keep
  your own list of decks".
- Author chips and `?author=` are removed.
- `/browse`: shared decks of other users with author and progress. Logged in,
  each row has a ☆/★ toggle; anonymous visitors get a login hint instead.
- `POST /s/{slug}/star` with `star=1|0`: login required (anonymous → login
  redirect), CrossOrigin-protected, set not visible → 404, own set → 400, bad
  value → 400. 303 to the form's `next` (local path only), else the same-host
  Referer, else the set page. Plain form, no htmx.
- Set page: a ☆ Star / ★ Starred toggle for logged-in non-owners.

## Unchanged

MCP (`list_sets` and friends) ignores stars.
