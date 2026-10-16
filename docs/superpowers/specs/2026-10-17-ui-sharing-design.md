# kena UI polish and set sharing

Date: 2026-10-17. Status: approved. Extends the kena, accounts and mistakes specs.

## 1. Cards: manual advance and streak

- No auto-advance after a correct answer: the result card (green, "✓") always
  has a Next button.
- Streak: consecutive correct answers in this cards session. Carried as
  `streak=<n>` in the card's hidden fields, the Next URL and the no-JS redirect;
  never in mode/filter/shuffle links. Correct → n+1, wrong → 0. Shown on the
  result as "🔥 N in a row" when N ≥ 2.
- Enter anywhere advances while a result is shown (one `keydown` handler on the
  document, ignoring key repeat). After a check the focus stays on `#kbd-keeper`
  so the iOS keyboard stays up; Enter in it triggers Next.

## 2. Persistent results

- `store.LastAttempts(device, set)` → per task the latest attempt's answers and
  `correct`.
- List mode renders each tried task in its latest state: correct → green, typed
  answers filled in; wrong → given → expected, explanation, "Try again".
  Untried tasks: empty inputs. Answers are still never sent for untried tasks.
- Cards mode without `after` starts at the first task in the current ordering
  (filter, shuffle) that is not last-correct; if all are solved, the first task.

## 3. Answer font

Playpen Sans (OFL), self-hosted from `static/fonts/` as variable woff2, greek
and latin subsets only, OFL text alongside, `font-display: swap`. Used only for
typed inputs and rendered given/expected answers (results, list, /mistakes).
`.woff2` is served as `font/woff2`.

## 4. Sharing and author filter

- Migration 004: `sets.shared boolean NOT NULL DEFAULT false`; existing sets
  become shared in the same migration.
- Web visibility: owner always; everyone else only shared sets. Private set
  page, next, check and item for anyone else → 404 (admins included on the web).
- Home: shared sets + the logged-in user's own (own private ones get a
  "private" badge). Author chips: All · Mine (logged in) · each visible author;
  `?author=me` or `?author=<username>`; an unknown author shows an empty list.
  Superseded by the stars spec (`2026-10-17-stars-design.md`): chips removed.
- Set page, owner only: Shared / Private toggle, `POST /s/{slug}/share`
  (`shared=1|0`, CrossOrigin-protected, 303 back to the set). Non-owner → 404.
- MCP: `list_sets` returns shared sets + the caller's own (admins: all sets);
  `get_set`, `validate_tasks{set}`, `get_mistakes_report{set}` and every edit
  tool treat another user's private set as not found, except for admins.
  `create_set` takes `shared` (default false), `update_set` takes `shared`;
  set outputs include `shared`.
- Guide (`workflow`, `format`): sets are private by default; after creating one,
  ask the user whether to share it and use `update_set{shared}`.
- The mistakes report keeps the learner's own attempts on sets that later
  became private (they already saw those tasks).
