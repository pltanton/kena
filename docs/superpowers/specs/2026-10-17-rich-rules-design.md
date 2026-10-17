# Rich deck rules and the ⓘ info sheet

Date: 2026-10-17. Status: accepted. Extends `2026-10-17-deck-rules-design.md`:
rules become short Markdown documents (tables for paradigms), can be deck-wide,
and are reachable from every card through an ⓘ button.

## Problem

One-line rules can't hold a paradigm ("πόσος, πόση, πόσο × cases"), and the
learner sees a rule only after a mistake. Theory that helps before answering
has no place on the card, and the deck's general grammar has no place at all.

## Model

- `greek.Rule{Key, Title, Body, General}`; JSON `{"key", "title", "body", "general"}`
  in `sets.rules`.
  - key: `^[a-z0-9-]{1,32}$`, unique in the deck;
  - title: optional, one line, at most 80 characters;
  - body: Markdown, 1–4000 characters, line breaks allowed, no control characters
    other than line breaks and tabs;
  - general: deck-wide theory, shown on every card's ⓘ and first on the overview;
  - at most 30 rules.
- `tasks.rule` stays one key; it may name a general rule too (then it is shown
  once, as the card's rule).
- Migration 009 rewrites every `{key, text}` element to
  `{key, title: "", body: text, general: false}`; elements that already have a
  `body` are kept, so it is safe on any data.
- A rule's label is its title, or the first words of its body's text when the
  title is empty.

## Markdown

`internal/markdown`: goldmark (CommonMark) with GFM tables and strikethrough.

- Raw HTML is never rendered (goldmark without `WithUnsafe` writes
  `<!-- raw HTML omitted -->`); dangerous link schemes (`javascript:`,
  `vbscript:`, `file:`, `data:`) are dropped by goldmark's renderer — tests pin
  both.
- Images become their alt text. Links get `rel="noopener nofollow"` and
  `target="_blank"`. Headings shift down by two levels (`#` → `<h3>`) so a rule
  never outranks the page.
- Tables are wrapped in `<div class="md-table">` for horizontal scrolling on
  narrow screens.
- `Render(src) template.HTML` memoises by source text (bounded map, cleared
  when full); rendering is cheap, the cache only saves repeated work per card.
- `Preview(src, n)`: plain text of the first paragraph, cut at a word boundary.
- `greek.PrepareRules` warns (does not reject) when a body contains raw HTML or
  an image, since those are not shown.

## Store

- `ReplaceRules(ctx, setID, version, rules, renames)`: one transaction, set row
  locked; refuses with `ErrConflict` when `version` is non-zero and
  `sets.updated_at` differs; applies `renames` (old key → new key) to tasks;
  refuses with `*RulesInUseError` (key → task ids and positions) when a task
  still points at a key that is gone. Bumps `updated_at`.
- The store still does not lint: callers run `greek.PrepareRules` on the full
  new list first.
- `MistakesReport` returns the rule's title and body.

## Editing

- `/s/{slug}/rules` (owner only, 404 otherwise; POSTs behind `session.CrossOrigin`):
  one form per rule (key, title, deck-wide checkbox, body textarea, "used by N
  tasks"), an empty form to add one, and a Markdown cheat-sheet with a table
  example. Buttons: Save, Preview (re-renders the page with the body rendered
  under the form, saves nothing), Delete (refused while tasks use the rule; the
  error lists them by number and sentence). Changing a key renames it on the
  tasks too. Saves carry the deck version: a deck changed since the page
  opened gives 409, like the editor.
- Linked from the overview (owner: "Rules") and from the deck editor.
- Deck editor: rules are no longer part of the text. `@key` stays. Old
  `= key: text` lines still parse: a new key becomes a body-only rule, a line
  identical to an existing rule is ignored, a different body under an existing
  key is an error pointing to the Rules page. `deckfmt.FormatDeck` writes tasks
  only.

## MCP

- `create_set` / `update_set` `rules`: `{key, title?, body, general?}`; the old
  `{key, text}` is still read (text → body).
- `set_rule{set, key, title?, body, general?}`: upsert; omitted title/general
  keep the existing values. `delete_rule{set, key}`: refused while tasks use
  it; the error lists task ids.
- `get_set` returns `rules` as `{key, title, body, general}`.
- Descriptions: general = theory for the whole deck (a paradigm table, the
  agreement rule); per-card rules = the point a group of items practises;
  tables encouraged for paradigms.

## Display

- Focus mode: an ⓘ button in the card's top-right corner when the card has a
  rule or the deck has general rules. It opens a bottom sheet (the ⋯ sheet's
  look) with the card's rule (title + body), then the general rules (open when
  there is one, collapsed `<details>` when several). The sheet scrolls inside
  itself; Esc, Close and a tap outside close it. Like the card's other
  controls, `pointerdown` on the button and inside the sheet is prevented, so
  the input keeps focus and the keyboard stays open; closing refocuses the
  input. The card's rule is inlined in the card fragment as a `<template>`; the
  general rules are rendered once into the page's sheet. Visible before
  answering: rules are theory, not answers.
- After Give up and in Why?: the item's explanation, then the rule's label, a
  short preview and "Show rule", which opens the same sheet.
- Overview: a "Rules" section, general rules first, each a `<details>` with the
  rendered body.
- Worksheet: the same ⓘ per task with a rule (or every task, when the deck has
  general rules) and the same sheet.
- `/mistakes`: Why? shows the rule's label and rendered body.
- ⓘ and "Show rule" are `hidden` until `info.js` runs; without JavaScript the
  overview's Rules section remains.
- Rendered Markdown (`.md`) uses the stone design: compact tables with a
  sunken header row, Greek-friendly 0.95rem, horizontal scroll in `.md-table`.

## Guide

`item-rules.md`, `format.md`, `workflow.md`: rules as short Markdown with a
paradigm table where helpful (a full πόσος table by gender, number and case),
deck-wide theory marked `general`, `set_rule` / `delete_rule`.

## Prod data

Migration 009 converts the existing rules on start. A separate reviewable SQL
(not applied by the agent) gives set 3's six rules Greek titles and adds one
general rule "Πόσος, πολύς, λίγος, αρκετός" with the paradigm tables; one
transaction, only set 3, guarded on the slug and the exact rule keys.
