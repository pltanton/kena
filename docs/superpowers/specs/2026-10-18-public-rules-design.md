# Public rules

Date: 2026-10-18. Status: accepted. Extends `2026-10-17-rich-rules-design.md`:
a shared library of rules any deck can use, organised by category.

## Problem

Rules live inside one deck (`sets.rules`). The same grammar point (the πόσος
paradigm, article agreement) is copied into every deck that practises it, and
the copies drift. Authors need one place to write a rule and many decks to use
it.

## Model

- A public rule's id is `category/name`; both parts match the deck key format
  `^[a-z0-9-]{1,32}$`. Deck keys cannot contain `/`, so the two never collide.
- Title and body follow the deck rule limits (title ≤ 80, Markdown body ≤ 4000)
  and go through `greek.PrepareRules`.
- Neither categories nor rule names can be renamed.

Migration `012_public_rules.sql`:

```sql
rule_categories(id, name text UNIQUE, created_by → users, created_at)
public_rules(id, category_id → rule_categories ON DELETE RESTRICT,
             name, title, body, owner_id → users ON DELETE RESTRICT,
             created_at, updated_at, UNIQUE (category_id, name))
tasks ADD public_rule_id → public_rules ON DELETE RESTRICT,
      CHECK (rule IS NULL OR public_rule_id IS NULL)
set_public_rules(set_id → sets ON DELETE CASCADE,
                 rule_id → public_rules ON DELETE RESTRICT,
                 position, PRIMARY KEY (set_id, rule_id))
```

A deck uses a public rule two ways: a task points at it (`public_rule_id`), or
the deck attaches it (`set_public_rules`), which makes it general — shown on
every card's ⓘ. At most 10 attached public rules per deck.

## Permissions

Enforced in the store, so web and MCP agree.

| Action                              | Who                                                                                |
| ----------------------------------- | ---------------------------------------------------------------------------------- |
| Read categories and public rules    | everyone, logged out included                                                      |
| Create a category                   | admins                                                                             |
| Delete a category                   | admins, only when it has no rules                                                  |
| Create a public rule                | any logged-in user, in an existing category                                        |
| Edit title / body                   | the rule's creator or an admin                                                     |
| Delete a rule                       | creator or admin; refused while used (`used by N tasks in M decks`, no deck names) |
| Attach / detach, point a task at it | the deck owner (existing owner-only paths)                                         |
| Reassign a rule's creator           | admins (admin user page)                                                           |

Deleting a user who created public rules is refused until the rules are
reassigned.

## Store

- Loading a set fills `Set.Public []greek.Rule`: the attached rules in
  `position` order with `General = true`, then the rules its tasks point at
  that are not attached, with `General = false`. Keys are `category/name`.
- `Task.Rule` reads `category/name` for a task with `public_rule_id`.
- `Set.AllRules()` = `Rules` followed by `Public`; every reader (`ruleFor`,
  `ruleViews`, offline pack, worksheet, overview) uses it.
- `Set.PublicStamp`: `max(updated_at)` of the set's public rules, computed in
  the set query.
- `SaveDeck`, `AddTasks`, `UpdateTask`: a `Rule` containing `/` is resolved to
  `public_rule_id` (with `rule` NULL); an unknown id is an error.
- `MistakesReport` resolves `public_rule_id` too.
- Attaching or detaching bumps `sets.updated_at`; editing a public rule does
  not (the deck editor's 409 must not fire for other people's edits).
- The store still does not lint.

## Lint

`greek.PrepareDeck(tasks, existing, rules, public)` takes the set of existing
public ids; a task's `rule` passes when it is a deck key or a public id.
`deckfmt` already reads `@` followed by any text, so `@ellenika/posos` parses
and round-trips; the fuzzer seeds include one.

## Web

All POSTs behind `session.CrossOrigin`; every string in all three catalogs.

- `/rules`: one `<details>` per category; each rule shows its id, title,
  preview and creator. Logged in: a "New rule" form (category select, name,
  title, body; Preview / Save). Admins: a "New category" form. Linked from the
  header.
- `/rules/{category}/{name}`: the rendered rule and "used by N decks". Creator
  and admins: an edit form (Preview / Save / Delete). Saves carry `updated_at`;
  a stale save gives 409. Unknown ids give 404.
- `/s/{slug}/rules`: a "Public rules" section listing attached rules (Remove)
  and an Attach select grouped by category (`<optgroup>`). Removing only
  detaches; tasks pointing at the rule keep it.
- Deck editor: `@category/name` works on tasks; the format help says so.
- ⓘ sheet, overview, worksheet and `/mistakes` show public rules exactly like
  deck rules.

## MCP

New tools (existing names unchanged):

- `list_public_rules{category?}`: categories with their rules (id, title,
  preview, creator).
- `get_public_rule{id}`: the full rule and the number of decks using it.
- `create_rule_category{name}`: admins only.
- `set_public_rule{id, title?, body}`: upsert; a new rule needs an existing
  category, an existing one its creator or an admin; omitted title keeps it.
- `delete_public_rule{id}`: refused while used.
- `attach_public_rule{set, id}`, `detach_public_rule{set, id}`: deck owner
  (admins: any deck, like `set_rule`).

Existing tools: task `rule` accepts `category/name` in `create_set`,
`update_set`, `add_tasks`, `update_task`, `validate_tasks`; an unknown id is an
error suggesting `list_public_rules`. `get_set` adds `public_rules` (full
rules, each with `attached`). `prepare` passes the public ids to
`PrepareDeck`.

## Guide

- `item-rules.md`: a "Public rules" section — check `list_public_rules` before
  writing a deck rule; reuse one that covers the point; create a public rule
  only for grammar beyond this deck; deck-specific notes stay deck rules.
- `workflow.md`: call `list_public_rules` before defining deck rules.
- `format.md`: `@ellenika/posos` in the editor format.
- Server instructions: one line about public rules.
- The guide still passes the task lint.

## Offline

- The pack's `rules` include `Set.Public`, keyed `category/name`; `offline.js`
  looks rules up by key and needs no change.
- `offlineVersion` adds `PublicStamp`, so `/offline/decks.json` reports a deck
  stale after one of its public rules changes.
- Templates do not change; the golden file is regenerated with a fixture that
  has a task on a public rule and an attached one; node tests run.

## Tests

- store: resolve and reject ids, every permission guard, delete refused while
  used, user delete refused, `PublicStamp` moves on edit, attach limit.
- greek: `PrepareDeck` with public ids.
- deckfmt: round-trip and fuzz seed with `@category/name`.
- mcpserver: each new tool and its permission errors; `get_set.public_rules`.
- web: both pages (who sees which form, CrossOrigin, 409, 404), attach and
  detach, i18n keys.
- offline: golden, node tests, version change after an edit.
