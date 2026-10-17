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
  "explanation": "Optional: what is specific to this item.",
  "rule": "optional-rule-key"
}
```

- `text`: the sentence. Each blank is exactly three underscores `___`, with
  spaces or punctuation around it as in normal text. It may span several lines
  (`\n`): a dialogue puts each turn on its own line, starting with `— ` (the
  Greek dialogue dash) or `Α: ` / `Β: `. No empty lines, and no line may start
  with `>`.
- `blanks`: one entry per `___`, in the order they appear.
- `answers`: every accepted form, exact spelling and accents, lowercase. Only
  what goes into the blank: no punctuation, no neighbouring words.
- `hint` (optional): shown to the learner next to the blank. Lemma or feature
  tag, never the answer. No brackets: the UI adds them.
- `rule` (optional): the key of a deck rule this item practises (see Deck
  rules below). Shown with the explanation.
- `explanation` (optional): one short sentence (≤ ~120 characters) shown after
  a wrong attempt, only for what is specific to this item: an irregular form,
  an accent shift, the cue in a deck that mixes tenses or cases. Never the same
  text on two tasks — that is a deck rule. One language per deck, short English
  by default up to B1. See `item-rules`, rules 10 and 11.

## Deck rules

The grammar of a deck, written once and shown to the learner behind the ⓘ on
each card: `rules` on `create_set` / `update_set` (the whole list), or one at a
time with `set_rule` / `delete_rule`. Each rule:

- `key`: a–z, 0–9 and `-`, at most 32 characters; tasks name it in `rule`;
- `title` (optional): one line, at most 80 characters;
- `body`: Markdown, at most 4000 characters: short paragraphs, `**bold**`,
  lists, and a table for a paradigm. No HTML, no images (they are not shown);
- `general` (optional): `true` for deck-wide theory, shown on every card and
  first on the deck page; `false` (default) for the point a group of tasks
  practises, shown only with the tasks whose `rule` is this key.

At most 30 rules. A key a task still uses cannot be removed (`update_set`,
`delete_rule`); the error lists the tasks. The old shape `{key, text}` is still
read: `text` becomes the body.

```json
{
  "rules": [
    {
      "key": "posos-forms",
      "title": "Πόσος: γένος, αριθμός, πτώση",
      "general": true,
      "body": "Το **πόσος** συμφωνεί με το ουσιαστικό σε γένος, αριθμό και πτώση.\n\n| | αρσενικό | θηλυκό | ουδέτερο |\n|---|---|---|---|\n| ονομ. εν. | πόσος | πόση | πόσο |\n| αιτ. εν. | πόσο | πόση | πόσο |\n| γεν. εν. | πόσου | πόσης | πόσου |\n| ονομ. πληθ. | πόσοι | πόσες | πόσα |\n| αιτ. πληθ. | πόσους | πόσες | πόσα |\n| γεν. πληθ. | πόσων | πόσων | πόσων |"
    },
    {
      "key": "masc-pl-acc",
      "title": "Αρσενικό πληθυντικού, αιτιατική: πόσους",
      "body": "Το ουσιαστικό είναι αντικείμενο: **πόσους** φίλους; — πολλούς, λίγους, αρκετούς."
    }
  ]
}
```

The table above, as the learner sees it:

|             | αρσενικό | θηλυκό | ουδέτερο |
|-------------|----------|--------|----------|
| ονομ. εν.   | πόσος    | πόση   | πόσο     |
| αιτ. εν.    | πόσο     | πόση   | πόσο     |
| γεν. εν.    | πόσου    | πόσης  | πόσου    |
| ονομ. πληθ. | πόσοι    | πόσες  | πόσα     |
| αιτ. πληθ.  | πόσους   | πόσες  | πόσα     |
| γεν. πληθ.  | πόσων    | πόσων  | πόσων    |

In the deck editor (Edit deck) a task names its rule with an `@key` line under
its sentence; the rules themselves are edited on the deck's Rules page:

```
Χθες η Μαρία {έγραψε | γράφω} δύο γράμματα.
@aorist-stem
```

Checking: the learner's input and each answer are NFC-normalised, trimmed, inner
spaces collapsed and lowercased, then compared exactly. Case and spacing do not
matter; accents, dieresis and final ς do.

## Hint conventions

Pick one format per set (see `workflow`, spec table):

| format      | example       | use when                                          |
| ----------- | ------------- | ------------------------------------------------- |
| lemma       | `γράφω`       | the instruction line or a cue fixes the tense     |
| lemma + tag | `γράφω, αόρ.` | the set mixes tenses and no cue settles the item  |
| tag only    | `γεν. πληθ.`  | the word is in the sentence's context, e.g. nouns |

Tags: `ενεστ.` (ενεστώτας), `παρατ.` (παρατατικός), `αόρ.` (αόριστος),
`στιγμ. μέλλ.`, `εξακ. μέλλ.`, `προστ.` (προστακτική), `ονομ.`, `γεν.`, `αιτ.`,
`κλητ.`, `εν.`, `πληθ.`. Lemmas: verbs in 1st sg. present, nouns in nominative
singular, adjectives in masculine nominative singular.

In a deck that mixes aorist and imperfect (or other tenses, aspects, cases),
a bare lemma is allowed only where the sentence forces the choice. Otherwise add
the tag to that item's hint.

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

A dialogue, one turn per line:

```json
{
  "text": "— Πόσους φίλους ___ στο πάρτι σου;\n— Πολλούς, σχεδόν όλη την τάξη.",
  "blanks": [{ "answers": ["κάλεσες"], "hint": "καλώ" }],
  "explanation": "2nd sg. aorist of καλώ: κάλεσα → κάλεσες; the question is about one finished party."
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
- an empty line inside the text, or a text line starting with `>`, `@` or `=`;
- a `rule` that is not a key of the deck's rules; a malformed rule (key,
  title over 80 characters or with a line break, empty body or over 4000
  characters, more than 30 rules);
- Latin letters or digits mixed into a Greek word (lookalikes such as Latin o);
- polytonic characters (U+1F00–U+1FFF);
- medial sigma (U+03C3) at the end of a word, or final ς inside a word, in text or answers;
- an answer containing `___`.

Warnings (the call succeeds; review them):

- the same sentence already exists in the set;
- a rule body with raw HTML, an image or a link that is not http(s): not shown;
- an answer word of two or more syllables without an accent;
- text longer than 20 words;
- the same explanation (ignoring case, spacing and final punctuation) on more
  than two tasks of the batch and the set — put it in a deck rule;
- in a batch of 10 or more: over 60% of the tasks with the same sequence of
  hints (vary lemmas, cases and frames);
- in a batch of 10 or more: over half the tasks with neither an explanation nor a `rule`;
- in a batch of 10 or more: over 60% of sentences with a subject pronoun
  (εγώ, εσύ, αυτός, αυτή, αυτό, εμείς, εσείς, αυτοί, αυτές, αυτά).

The batch warnings are reported on the first task they concern.
