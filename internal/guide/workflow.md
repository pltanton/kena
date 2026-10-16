---
name: workflow
description: End-to-end recipe for turning a textbook exercise (photo, scan or description) into a kena set of new Greek gap-fill tasks. Read this first.
---

# Workflow: from a textbook exercise to a kena set

Typical request: "here is a photo of an exercise from my Greek book (B1), make 100
more like it and upload them". Follow these steps in order. Do not skip the
self-check: learners are marked wrong for a single accent, so every bad key costs
them a correct answer.

## 0. Decide N and scope

- N = what the user asked for. If they say "many", "a lot" or give no number, N = 100.
- No photo, only a topic ("drill the aorist")? Build the spec in step 1 from your
  own knowledge of the level and continue the same way.
- Ask the user one question only if the grammar target itself is unclear.
  Otherwise proceed and state your assumptions in the final report.

## 1. Extract the spec from the photo

Read the exercise and fill this table. It is the only thing you keep from the book.

| field            | what to write                                                           | example (aorist exercise)                                                 |
| ---------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| target           | the one grammar point every blank tests                                 | simple past (αόριστος) of active verbs                                    |
| paradigm slots   | which forms appear: persons, numbers, cases, genders, polarity          | all 6 persons; affirmative, negative, questions                           |
| lexical classes  | sub-patterns the target splits into                                     | γράφω → έγραψα; διαβάζω → διάβασα; irregular πηγαίνω → πήγα, τρώω → έφαγα |
| word pool        | lemmas the book uses plus same-level lemmas of the same classes (25–40) | γράφω, δουλεύω, ανοίγω, αγοράζω, ετοιμάζω, βλέπω, παίρνω, λέω, έρχομαι    |
| hint format      | what goes next to the blank (lemma or tag), one format for the set      | lemma: `γράφω`                                                            |
| instruction line | one Greek sentence telling the learner what to do                       | Συμπληρώστε με τον αόριστο του ρήματος.                                   |
| level            | CEFR level of the book, or the user's stated level                      | B1                                                                        |
| source ref       | book title, unit, exercise number                                       | `<Book title>, unit 5, ex. 3`                                             |

Write the table out in your reasoning before writing any sentence.

## 2. Discard the book's sentences

- Never upload, copy, translate back or lightly edit the book's sentences, names or
  scenes. They are copyrighted, and the learner has already seen them.
- Keep only the spec. Every sentence you upload is new.
- `source_ref` stores the reference ("Book, unit, exercise"), never book text.

## 3. Plan a variety grid for N items

Order matters for learning: first a blocked block, then an interleaved block.

- Items 1–30 (about 30%): **blocked**, one lexical class at a time, so the learner
  sees the pattern.
- Items 31–N (about 70%): **interleaved**, all classes mixed; irregular forms
  return every 10–15 items.

Example grid for N = 100, aorist:

| items  | mode        | content                                                              |
| ------ | ----------- | -------------------------------------------------------------------- |
| 1–10   | blocked     | γράφω → έγραψα, δουλεύω → δούλεψα, ανοίγω → άνοιξα, κλείνω → έκλεισα |
| 11–20  | blocked     | -ζω verbs: διαβάζω → διάβασα, αγοράζω → αγόρασα, ετοιμάζω → ετοίμασα |
| 21–30  | blocked     | irregular: πηγαίνω → πήγα, βλέπω → είδα, τρώω → έφαγα, παίρνω → πήρα |
| 31–100 | interleaved | all of the above mixed, plus λέω → είπα, έρχομαι → ήρθα, πίνω → ήπια |

Quotas across the whole set (adapt to the target):

- every paradigm slot from the spec appears; no person/number/case above ~25%;
- negatives ~20%, questions ~15%, the rest affirmative statements;
- both genders and singular/plural subjects;
- no lemma more than ~5 times in 100 (irregular core verbs may recur, spaced out);
- the blank's position moves: start, middle, end of the sentence.

Before each batch, write its 20 grid rows: item number, lemma, slot
(e.g. 3rd pl.), polarity/type, context cue, blank position.

## 4. Write in batches of 20

Write each batch from its grid rows, following `item-rules` and
`greek-pitfalls`. Never write all N in one go: quality drops and templates repeat.

## 5. Self-check each batch

Run the `self-check` checklist on the batch. Fix or replace every failing item
before moving on.

## 6. Validate

Call `validate_tasks` with the batch. Fix every error. Fix warnings too unless you
can say why the item is right (for example a monosyllabic answer has no tonos).

## 7. Upload

- First batch: `create_set` with the set metadata and `tasks`. Keep the returned
  id and slug. The set starts private (`visibility` private): only the user sees it.
- Every later batch: `add_tasks(set, tasks)`.
- At most 50 tasks per `create_set` or `add_tasks` call. Two checked batches of 20
  may go in one call; more may not.
- A rejected call saves nothing. The error names the task index: fix that task
  and resend the whole call.
- Upload in grid order (blocked first), because the set plays in upload order.

## 8. Report

Tell the user: the set URL from the `create_set` result, the title, the number of
tasks, what the set covers (classes, slots), and any assumptions you made.

Then ask the user who should see the set: only them (`private`), every
logged-in user of the site (`friends`), or everyone, also without logging in
(`public`). Call `update_set(set, visibility=...)` with the answer; they can
change it later on the set page too.

## Set metadata conventions

| field          | convention                                                  | example                                   |
| -------------- | ----------------------------------------------------------- | ----------------------------------------- |
| `title`        | Greek / English                                             | `Αόριστος ρημάτων / Simple past of verbs` |
| `topic`        | short English grammar label                                 | `aorist`                                  |
| `level`        | CEFR: A1, A2, B1, B2, C1, C2                                | `B1`                                      |
| `instructions` | one Greek line, imperative, ends with a full stop           | `Συμπληρώστε με τον αόριστο του ρήματος.` |
| `source_ref`   | optional; "Book, unit, exercise", never book text           | `<Book title>, unit 5, ex. 3`             |
| `slug`         | optional; omit it and the server derives one from the title |                                           |
| `visibility`   | optional; `private` until the user picks another level      | `friends`                                 |

More instruction lines:

- Συμπληρώστε με τον σωστό τύπο του ρήματος.
- Συμπληρώστε με τη γενική του ουσιαστικού.
- Συμπληρώστε με το σωστό άρθρο.

## Fixing or extending an existing set

1. `list_sets` to find it, `get_set(set)` to read its metadata and all tasks.
2. Wrong key or sentence: `update_task(task_id, ...)` with the corrected fields.
   Unfixable item: `delete_task(task_id)`.
3. More items: plan the grid against what already exists (fill missing slots,
   avoid duplicate sentences), then `validate_tasks` and `add_tasks`.
4. Metadata: `update_set(set, ...)`.

You can change only sets you own (admins can change any). Other users' private
sets are invisible to you.
