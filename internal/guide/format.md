---
name: format
description: JSON format of tasks and of create_set / add_tasks arguments, hint and explanation conventions, three full Greek examples, and the lint errors and warnings the server applies.
---

# Format

## Task

```json
{
  "text": "Sentence with ___ for each blank.",
  "blanks": [
    {
      "answers": ["accepted form", "another accepted form"],
      "hint": "lemma or tag"
    }
  ],
  "explanation": "Optional: why this form."
}
```

- `text`: the sentence. Each blank is exactly three underscores `___`, with
  spaces or punctuation around it as in normal text.
- `blanks`: one entry per `___`, in the order they appear.
- `answers`: every accepted form, exact spelling and accents, lowercase. Only
  what goes into the blank: no punctuation, no neighbouring words.
- `hint` (optional): shown to the learner next to the blank. Lemma or feature
  tag, never the answer. No brackets: the UI adds them.
- `explanation` (optional): one short sentence on why this form, in English or
  Greek. Shown after a wrong attempt.

Checking: the learner's input and each answer are NFC-normalised, trimmed, inner
spaces collapsed and lowercased, then compared exactly. Case and spacing do not
matter; accents, dieresis and final ς do.

## Hint conventions

Pick one format per set (see `workflow`, spec table):

| format      | example       | use when                                          |
| ----------- | ------------- | ------------------------------------------------- |
| lemma       | `γράφω`       | the instruction line fixes the tense or case      |
| lemma + tag | `γράφω, αόρ.` | the set mixes tenses, or aspect needs forcing     |
| tag only    | `γεν. πληθ.`  | the word is in the sentence's context, e.g. nouns |

Tags: `ενεστ.` (ενεστώτας), `παρατ.` (παρατατικός), `αόρ.` (αόριστος),
`στιγμ. μέλλ.`, `εξακ. μέλλ.`, `προστ.` (προστακτική), `ονομ.`, `γεν.`, `αιτ.`,
`κλητ.`, `εν.`, `πληθ.`. Lemmas: verbs in 1st sg. present, nouns in nominative
singular, adjectives in masculine nominative singular.

## Set (`create_set` arguments)

```json
{
  "title": "Αόριστος ρημάτων / Simple past of verbs",
  "topic": "aorist",
  "level": "B1",
  "instructions": "Συμπληρώστε με τον αόριστο του ρήματος.",
  "source_ref": "<Book title>, unit 5, ex. 3",
  "visibility": "private",
  "tasks": []
}
```

`source_ref`, `slug`, `visibility` and `tasks` are optional. `visibility` is
`private` (default: only the owner sees the set), `friends` (every logged-in
user) or `public` (everyone, also without login). Change it later with
`update_set`. `add_tasks` takes
`{"set": "<id or slug>", "tasks": [...]}`. At most 50 tasks per
`create_set` or `add_tasks` call; `validate_tasks` takes `{"tasks": [...]}`
and saves nothing.

## Examples

One blank:

```json
{
  "text": "Χθες το βράδυ η Μαρία ___ δύο κεφάλαια από το νέο της βιβλίο.",
  "blanks": [{ "answers": ["διάβασε"], "hint": "διαβάζω" }],
  "explanation": "Aorist for a finished action yesterday; 3rd sg. ending -ε, accent on the third syllable from the end."
}
```

Two blanks:

```json
{
  "text": "Το περασμένο Σάββατο η μαμά ___ στη λαϊκή και ___ φρούτα και λαχανικά.",
  "blanks": [
    { "answers": ["πήγε"], "hint": "πηγαίνω" },
    { "answers": ["αγόρασε"], "hint": "αγοράζω" }
  ],
  "explanation": "Aorist, 3rd sg.: πηγαίνω → πήγε (irregular), αγοράζω → αγόρασε."
}
```

One blank with two accepted answers:

```json
{
  "text": "Χθες ___ σπίτι πολύ αργά, γιατί είχα πολλή δουλειά στο γραφείο.",
  "blanks": [{ "answers": ["ήρθα", "ήλθα"], "hint": "έρχομαι" }],
  "explanation": "έρχομαι has an active aorist: ήρθα (everyday) or ήλθα (formal); 1st sg. like είχα."
}
```

## Lint

The server lints every `create_set`, `add_tasks`, `update_task` and
`validate_tasks` call. All text is NFC-normalised.

Errors (the whole call is rejected, nothing is saved; the message names the
task index):

- number of `___` differs from the number of blanks;
- empty text, or a blank with no answers;
- Latin letters or digits mixed into a Greek word (lookalikes such as Latin o);
- polytonic characters (U+1F00–U+1FFF);
- medial sigma (U+03C3) at the end of a word, or final ς inside a word, in text or answers;
- an answer containing `___`.

Warnings (the call succeeds; review them):

- the same sentence already exists in the set;
- an answer word of two or more syllables without an accent;
- text longer than 20 words.
