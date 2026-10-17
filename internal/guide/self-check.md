---
name: self-check
description: Checklist to run on every batch of 20 tasks before calling validate_tasks (level spec, blind re-solve, derive forms from the paradigm, native reading, explanations side by side, mechanical lint, level measurement).
---

# Self-check (run on every batch of 20)

Do every step on every item. Write down the result per item; fix or replace
failing items, then re-run the steps on the replacements.

## 1. Re-read the spec

Re-read the spec table from `workflow`: target, slots, word pool, hint format,
level. Drift is the most common failure: by item 60 sentences get longer, the
vocabulary climbs and the target blurs.

- [ ] Every blank tests the target, in a slot from the spec.
- [ ] Every hint uses the set's hint format.

## 2. Blind re-solve

For each item, hide the answer and solve it as a learner who sees only the
instruction line, the sentence and the hint.

- [ ] List every form that fits grammatically and fits the hint: other persons,
      numbers, tenses, aspects, variants.
- [ ] Compare this list with `answers`. Every extra form either goes into
      `answers` or is ruled out by changing the sentence (add a subject, a time
      cue, a preposition).
- [ ] If more than two or three forms remain, rewrite the item.
- [ ] Mixed-tense deck: cover the hint and ask for every item: could the other
      tense fit? If yes, add a cue to the sentence or the tag to the hint.

## 3. Derive each answer from the paradigm

Do not trust recall. For each answer write the derivation:
lemma → stem → ending → accent.

- `γράφω`, aorist, 1st pl.: stem γραψ- + -αμε = γράψαμε; three syllables,
  accent on the third from the end, no augment.
- `άνθρωπος`, gen. pl.: ανθρωπ- + -ων; the long ending pulls the accent to the
  second from the end: ανθρώπων.

- [ ] Every answer matches its derivation letter for letter.
- [ ] Any lemma whose form you are not sure of is replaced by a common one.

## 4. Native reading pass

Read each sentence as a Greek speaker would.

- [ ] Natural word order and collocations; no word-for-word English.
- [ ] Articles, agreement (gender, number, case) and prepositions outside the
      blank are correct.
- [ ] The scene makes sense and fits everyday life.
- [ ] No two items in the batch share a frame; the blank and the cue move around.
- [ ] No subject pronoun (εγώ, εσύ, αυτός…) unless it marks a contrast; the
      person comes from a name, another verb, a possessive or the dialogue.

## 5. Explanations side by side

List the batch's explanations one under another, without the sentences.

- [ ] Every task has a `rule` from the deck's rules, or an explanation, or both.
- [ ] No explanation is a copy of a rule, or a rule with only the words or
      numbers swapped: move that text into the rule and drop the explanation.
- [ ] Each explanation, at most ~120 characters in the deck's language, names
      what is specific to its item (an irregular form, an accent shift, the cue).
- [ ] If two explanations are identical or interchangeable, they are a rule:
      add it to the deck's rules and point both items at it.

## 6. Mechanical lint

- [ ] Text is NFC; only monotonic Greek letters; nothing from U+1F00–U+1FFF.
- [ ] No Latin lookalikes inside Greek words (Latin o vs Greek ο, Latin v vs ν,
      Latin A, E, H, K, M, T, X vs Greek capitals).
- [ ] ς at the end of every word, medial sigma (U+03C3) never at the end.
- [ ] Every word of two or more syllables has exactly one accent, except the
      enclitic double accent (ο δάσκαλός μου). Monosyllables have none, except ή,
      πού, πώς.
- [ ] Number of `___` equals the number of blanks; no answer is empty.
- [ ] No sentence duplicates another one in this batch or in the set.
- [ ] Questions end with `;`.

## 7. Measure the level

- [ ] Each sentence has 6–14 words (count the blank as a word).
- [ ] List every lemma outside the A2–B1 core (or the book's level). Replace it
      unless it is the target or the book uses it.
- [ ] Count the slots covered so far against the grid quotas.

## Then

Call `validate_tasks` with the batch. It repeats the mechanical checks (errors
reject the call; warnings flag duplicates, missing accents, long sentences, repeated or missing
explanations, subject pronouns). It cannot check meaning, ambiguity or level:
steps 1–5 and 7 are yours.
