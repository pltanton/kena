# Task kinds and question cards

Date: 2026-10-28. Status: accepted. Builds on
`2026-10-28-choice-blanks-design.md`.

## Problem

Every task is a sentence with gaps. Decks should be able to mix card types,
starting with a question and a list of options ("Which tense is «έγραψα»?").

## Model

- `tasks.kind text NOT NULL DEFAULT 'gap' CHECK (kind IN ('gap', 'choice'))`
  (migration 013). In Go `greek.TaskInput.Kind` and `store.Task.Kind` are `""`
  for gap and `"choice"` (`greek.ChoiceTask`); the store maps `""` ↔ `'gap'`.
- A choice task is a question card: `text` is the question without `___`, and
  its single blank holds the right option(s) in `answers` and 1–5 wrong ones in
  `choices`; no hint. Checking, attempts, partial checks, retries, struck
  options (`x1`), `/sync` and the offline engine are the choice-blank machinery
  unchanged; only the card's markup differs.
- New kinds later plug into the same places: lint, `deckfmt`, the `task_body`
  template, `offline.js` and the golden file.

## Lint

Unknown kind; `___` in a question; not exactly one blank; no choices; a hint.
Text lines may no longer start with `[` (the editor's option marker).

## Editor

```
Ποιος χρόνος είναι το «έγραψα»;
[x] Αόριστος
[ ] Ενεστώτας
[ ] Παρατατικός
@rule
> explanation
```

Option lines follow the question; `[x]` (or `[X]`) right, `[ ]` wrong. Errors:
an option without a question, text after options, `{blanks}` in a question,
an empty option. `Format` and `Parse` round-trip; the fuzzer covers questions.

## Page

`task_body` renders a gap task as before (`<p class="sentence">`) and a
question as `<div class="question">`: the question, then full-width options
(radio `c1` inside a label, visually hidden). A pick submits at once; a wrong
pick is struck and disabled and carried as `x1`; after the answer or Give up a
list shows the right option green and a wrong pick struck. Cards, the
worksheet, `/mistakes` and the offline engine share the markup.

## MCP and guide

`TaskIn.kind`, `TaskOut.kind`, `update_task.kind`. `item-rules` rule 13: when a
question card fits (naming a tense or case, a translation, a described
situation), short parallel options, gaps stay the backbone. `format`: the
field, the editor syntax and the lint errors.

## Tests

greek question lint; deckfmt parse, format, errors, fuzz; web card states,
mistakes, worksheet, editor round-trip; offline golden (fresh, wrong, right,
give-up) and node tests; MCP create, get, update and lint errors.
