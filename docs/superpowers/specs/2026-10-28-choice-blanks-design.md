# Choice blanks

Date: 2026-10-28. Status: accepted.

## Problem

Every blank is typed. Some points (aorist vs imperfect, article cases, easily
confused forms) are better drilled by picking one of a few forms, and a deck
gains variety. Picking must work the same online and offline.

## Model

- `greek.Blank` gains `Choices []string` (JSON `choices`, omitted when empty):
  wrong options. A blank with choices shows `answers[0]` plus its choices;
  other accepted answers still count when checking but are not shown.
- Stored in `tasks.blanks` (jsonb); no migration.
- Checking does not change: the picked option's text goes through
  `greek.Check`, so `vectors.json` and `offline.js` checking stay as they are.

## Order

`choiceOptions(taskID, blank index, blank)` returns the shown options in a
deterministic order seeded by the task id and blank index, so the answer is not
always first and the order never changes between visits. Only the server
computes it: the page and the offline pack both use it.

## Editor format

`{πήγα ~ πηγαίνω ~ θα πάω | πάω}`: accepted answers (separated by `/`) before
the first `~`, wrong options after it, then the hint. `Format` and `Parse`
round-trip; the fuzzer seeds include choices.

## Lint (errors)

- 1–5 choices on a blank that has them;
- a choice equal to an accepted answer (as `greek.Check` compares) or to another
  choice;
- choices follow the answer character rules; `~` is forbidden in answers, hints
  and choices.

## Page

- A choice blank renders a `<select name="cN">` (`N` = blank index, 1-based)
  styled as a typed gap: underline, answer font, a chevron; an empty first
  option, then the options. With JavaScript, `picker.js` replaces the native
  popup (Android draws it as a system dialog) with an app-styled list under the
  gap: tap or Enter/Space/arrows open it, a pick sets the select's value and
  fires `change`, Escape or a tap outside closes it, wrong options are struck.
  Without JavaScript the native select works. (A first version used inline pill
  buttons; they broke the sentence's line and did not read as a gap.)
- Focus mode keeps the keyboard up only while it already is (the focus was in a
  text field or the keeper): picking an option or a question answer never opens
  it, typed cards keep it between cards as before.
- The check handler builds the answers per blank: `cN` for choice blanks, the
  next `a` for typed ones; the count must still match.
- Nothing picked on a choice blank = empty: a partial check, nothing recorded.
- Wrong pick: the select is marked wrong and the picked option is disabled;
  disabled options carry over between tries as hidden `xN`. A correct pick in
  a task that is not yet correct is locked: the select is disabled and a
  hidden `cN` carries the value.
- After a correct answer or Give up the sentence renders as for typed blanks.
- A task whose only blank is a choice blank submits on pick (`app.js`, for
  cards and the worksheet); without JavaScript the Check button works.

## Offline

- Each pack task carries `options`: per blank, the ordered options or `null`.
  Which one is right stays inside the XOR-obfuscated secret.
- `offline.js` renders the same markup and reads the form the same way;
  `offline_golden.json` covers a choice task and a mixed task.

## MCP and guide

- `BlankIn.choices`; `get_set` returns them; every write lints them.
- `item-rules`: when to use choices (confusable forms, low levels, variety;
  typing stays the default) and how to write distractors (same lemma and part
  of speech, plausible, clearly wrong in context, not typos, not accent-only
  unless accents are the deck's point). `format`: the JSON field and the editor
  syntax.

## Tests

greek lint; deckfmt round-trip and fuzz seed; web order, retry, lock, partial,
auto-submit markup; offline golden and node tests, pack without answers; MCP
round-trip and lint errors.
