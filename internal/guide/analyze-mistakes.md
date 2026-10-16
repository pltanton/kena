---
name: analyze-mistakes
description: How to read get_mistakes_report, find the learner's 2-4 main weak spots, explain them in plain words and offer a targeted practice set of new tasks.
---

# Analyze mistakes

Typical request: "analyze my kena mistakes and make a practice set for my weak
spots". The goal is a short, concrete diagnosis the learner can act on, then new
tasks aimed at exactly those spots.

## 1. Get the report

- Call `get_mistakes_report`. Default window: the last 30 days. Pass `days` (up
  to 365) for a longer view, `set` (id or slug) to focus on one set.
- The report holds only the caller's own attempts. You cannot read another
  learner's mistakes.
- If `sets` is empty, the report carries a `note`. Tell the learner there is
  nothing to analyse yet, suggest practising a set first, and stop.

What each task carries:

| field            | meaning                                                                   |
| ---------------- | ------------------------------------------------------------------------- |
| `text`, `blanks` | the sentence, accepted `answers` and `hint` per blank                     |
| `wrong_count`    | wrong attempts in the window                                              |
| `attempt_count`  | all attempts in the window                                                |
| `fixed`          | the latest attempt (any time) was correct                                 |
| `recent_wrong`   | up to 5 latest wrong attempts: `given` per blank, `kinds` per blank, `at` |

`summary.by_kind` counts the blanks in `recent_wrong` by kind;
`summary.top_unfixed` lists the tasks with the most wrong attempts that are still
not fixed. Start there.

## 2. Separate spelling from grammar

Every blank of a wrong attempt has a kind:

| kind          | what happened                                                | example (given → expected) |
| ------------- | ------------------------------------------------------------ | -------------------------- |
| `accent`      | right letters, accent or dieresis missing or misplaced       | εγραψα → έγραψα            |
| `final_sigma` | medial sigma typed at the end of a word, or ς inside a word  | (letters otherwise right)  |
| `near_miss`   | one or two letters off once accents are ignored              | έγραψε → έγραψα            |
| `other`       | a different form or word                                     | έγραφα → έγραψα            |
| `empty`       | blank left empty: the learner did not know the form          |                            |
| `correct`     | this blank was right; another blank of the attempt was wrong |                            |

- `accent` and `final_sigma` are spelling. Report them once, as one pattern,
  with a practical tip (on the Greek keyboard the accent key comes before the
  vowel; the final sigma has its own key). Build practice around them only if
  they are most of the errors.
- `near_miss` needs a look: a wrong ending (έγραψε for έγραψα) is grammar
  (person, number, case); a swapped or doubled letter is a typo.
- `other` and `empty` are grammar or vocabulary. They matter most.

## 3. Group by grammar point

Compare `given` with `answers` and use the set `topic`, the `hint` (lemma or
tag) and the `explanation`. Look for the same mistake across different tasks:

- Paradigm slot: the same person, number or case goes wrong across lemmas
  (γράψαμε, διαβάσαμε, αγοράσαμε all missed: 1st plural of the aorist).
- Aspect or tense: an imperfective form where a perfective is needed, or the
  reverse (έγραφα → έγραψα, να γράφει → να γράψει after μόλις or a deadline).
- Lexical class: one class of lemmas fails while others pass (irregular
  aorists: πίναμε → ήπιαμε, τρώγαμε → φάγαμε).
- Agreement: article, adjective and noun do not match in gender, number or case.

Weigh patterns by `wrong_count` and by whether they are fixed. A task wrong once
and then fixed is a slip, not a weak spot. A pattern seen in three or more
unfixed tasks is a weak spot.

## 4. Explain it to the learner

- Answer in the language the learner writes to you in.
- Name 2-4 main patterns, most important first. For each one:
  - the pattern in plain words ("the aorist of irregular verbs");
  - how often it happened;
  - 2-3 of the learner's own answers as given → expected;
  - the rule in one or two sentences, with the paradigm if it helps.
- Then mention spelling issues briefly, and what the learner already fixed.
- Keep it short. No tables of every mistake: the learner can see those on the
  site's Mistakes page.

## 5. Offer a practice set

Offer one practice set for the patterns you named; create it when the learner
agrees (or right away if they already asked for it).

- 20-40 new tasks. Build the spec as in `workflow` step 1, with the patterns as
  the target and as slots, and a word pool from the lemmas the learner missed
  plus other lemmas of the same class.
- New sentences only. Never resubmit the learner's original tasks verbatim, and
  do not just move the blank in one of them: reuse lemmas, not sentences.
- Start with a block on the main pattern, then interleave the patterns. Mix in
  nearby forms the learner got right, so the expected form is not predictable.
- Title: `Practice: <pattern>`, e.g. `Practice: aorist of irregular verbs`.
  topic = the grammar label, level = the level of the sets the mistakes came
  from, source_ref = `kena mistakes report, <date>`.
- Same rules as always: `item-rules`, `greek-pitfalls`, `self-check`, then
  `validate_tasks` and `create_set`. Give the learner the set URL.
