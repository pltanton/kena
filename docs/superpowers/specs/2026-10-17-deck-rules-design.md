# Deck rules

Date: 2026-10-17. Status: accepted. Replaces the templated-explanation
heuristic and the "(xx) abbreviation" warning from the multi-line batch.

## Problem

Agents put the general grammar of a deck ("πόσος agrees with the noun") into
every task's explanation, swapping a word or a number each time. Guessing that
from text (stripping digits, quotes, answers) is brittle. An explicit place
for the general rule makes the intent clear and the data reusable.

## Model

- `sets.rules jsonb NOT NULL DEFAULT '[]'`: `[{"key": "...", "text": "..."}]`.
  Key `^[a-z0-9-]{1,32}$`, unique in the deck; text 1–400 characters, one
  line, no control characters; at most 30 rules.
- `tasks.rule text NULL`: the key of one deck rule, or NULL.
- `greek.Rule{Key, Text}`; `greek.TaskInput.Rule`; `store.Set.Rules`,
  `store.Task.Rule`; `SetInput.Rules`, `SetPatch.Rules`, `TaskPatch.Rule`.
  `CreateSet`, `UpdateSet`, `AddTasks`, `UpdateTask` and `SaveDeck` carry them.

## Lint

- `greek.PrepareRules` normalises (NFC, trimmed) and checks rules: key format,
  duplicates, empty or over-long text, line breaks or control characters, more
  than 30 rules. Errors.
- `greek.PrepareDeck(tasks, existing, rules)`: everything `Prepare` does, plus
  an error when a task's `rule` is not a key of the deck's rules.
- Text lines must not start with `>`, `@` or `=` (the editor's markers).
- The exact-duplicate explanation warning stays (case, spacing and final
  punctuation ignored) and now says: "explanation repeated N times — put it in
  a deck rule and reference it with `rule`". The templated-shape heuristic and
  the "(xx) abbreviation" warning are removed. The hint-pattern warning stays.

## MCP

- `create_set` and `update_set` take `rules` (update replaces the whole list;
  a key still used by a task cannot be removed — the error lists the tasks).
- Task input gains `rule`; `add_tasks` / `update_task` / `validate_tasks` check
  it against the deck's rules (`validate_tasks` takes optional `rules`).
- `get_set` returns `rules` and each task's `rule`.
- Descriptions: general grammar that applies to many items → a deck rule;
  `explanation` only for what is specific to the item; both optional.

## Editor format

```
= agree: πόσος παίρνει το γένος, τον αριθμό και την πτώση του ουσιαστικού.
= polys: πολύς/λίγος/αρκετός συμφωνούν με το ουσιαστικό: πολλοί, πολλές, πολλά.

— Πόσους {φίλους | φίλος} κάλεσες;
— Κάλεσα {πολλούς | πολύς}.
@polys
> Item-specific note, if any.
```

Rule lines (`= key: text`) come before the first task; blank lines between
them are fine; a rule line after a task is an error. A task refers to a rule
with one `@key` line after its sentence, before or after its `>` lines.
`Format`/`Parse` round-trip; the fuzzer covers rules too.

## Display

After Give up and in Why? (correct answers), the learner sees the item's
explanation, then the rule text in muted type under a small "Rule" label.
Why? appears when either exists. `/mistakes` shows both under each item in a
collapsed Why?.

## Guide

`item-rules.md` and `format.md` get a deck-rules section (JSON and editor
examples); `workflow.md`: define 2–6 deck rules from the grammar point before
writing items; `self-check.md`: no explanation is a copy of a rule.

## Prod data

A reviewable SQL file (not applied by the agent) rewrites decks 2 and 3: line
breaks before «Β:» in deck 3's dialogues, Greek deck rules replacing templated
explanations, a rule on every task, explanations kept only when item-specific.
One transaction, only sets 2 and 3, task ids and positions unchanged, aborts
if the task counts differ from the export.
