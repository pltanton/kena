---
name: item-rules
description: Rules for a single gap-fill item (one target per blank, context forces the form, hints in mixed-tense decks, natural pro-drop sentences, varied frames, no giveaways, item-specific explanations, choice blanks) with good and bad Greek examples.
---

# Item rules

Each rule has a bad and a good example. In the examples the hint is shown in
brackets after the blank; in the JSON it goes in the `hint` field (see `format`).
Unless marked otherwise, the examples belong to an aorist set whose instruction
line is "Συμπληρώστε με τον αόριστο του ρήματος.", so the tense is given and
only the other features need a cue.

## 1. One target per blank

The blank holds exactly one form of the target. Everything else the learner must
produce is already in the sentence.

- Bad (aorist set): `Το γράμμα; ___ χθες το βράδυ.` → `το έγραψα` — the blank
  needs a clitic and a verb, and the person is free.
- Good: `— Και το γράμμα στη γιαγιά; — Το ___ χθες και το έστειλα σήμερα.` (γράφω) → `έγραψα`
  (έστειλα fixes the person; no `εγώ` needed)

## 2. Context forces the form

The learner must be able to derive exactly one form, or you list every form that
fits in `answers`. Force each feature with something visible:

- person and number: a name or subject noun, another verb with the same subject,
  a possessive, a vocative or a dialogue turn (`— Πού ήσουν;`); a subject
  pronoun only for contrast (see 6);
- tense and aspect: a time cue (χθες, αύριο, κάθε μέρα, τώρα, μόλις, μέχρι);
- case: a preposition or a verb that governs it;
- gender: an article or an agreeing word.

Bad (διαβάζω): `Η Μαρία ___ το βιβλίο.` — διαβάζει, διάβασε, διάβαζε, θα διαβάσει all fit.

Good (διαβάζω): `Χθες το βράδυ η Μαρία ___ δύο κεφάλαια.` → `διάβασε`

Bad (γυρίζω): `Όταν ___ στο σπίτι, η Άννα έκανε ένα ζεστό μπάνιο.` — γύρισα,
γύρισες, γύρισε… all fit: somebody else may have come home.

Good: `Όταν η Άννα ___ στο σπίτι, έκανε ένα ζεστό μπάνιο.` → `γύρισε`

The set's instruction line counts as context: in a set that says "Συμπληρώστε με
τον αόριστο", the tense is given, but person and number still need a cue.

### Decks that mix aorist and imperfect

The instruction "αόριστος ή παρατατικός" gives no tense, so every item must
force one, by a cue in the sentence or by a tag in the hint (see 3):

- aorist: a single finished event or a result: μία φορά, ξαφνικά, τελικά,
  πριν από δύο χρόνια, a count (`δύο κεφάλαια`), the event that interrupts (`όταν χτύπησε το τηλέφωνο`);
- imperfect: habit or background: κάθε μέρα, κάθε καλοκαίρι, συνήθως, συχνά,
  πάντα, συνέχεια, `ενώ` + ongoing action, `όταν ήμουν μικρός`.
- A bare `χθες` is weak: `Χθες διάβαζα όλο το απόγευμα` is correct Greek. Pair
  it with a result or a count.

Bad (mixed deck): `Χθες το βράδυ η Ελένη ___ στο σπίτι.` (μένω) — έμεινε and έμενε both fit.

Good: `Όταν ήμουν μικρός, ___ κάθε καλοκαίρι στο χωριό με τη γιαγιά.` (πηγαίνω) → `πήγαινα`

Good: `Ενώ η Σοφία ___ το μεσημεριανό, χτύπησε το κουδούνι.` (μαγειρεύω) → `μαγείρευε`

## 3. Hint = lemma or feature tag, never the answer

- Use the format from the spec, the same for the whole set: lemma (`γράφω`),
  lemma + tag (`γράφω, αόρ.`) or tag only (`γεν. πληθ.`). Exception: in a
  mixed deck the tag may go on some items only (last bullet).
- Never put the answer, a part of it, or its stem in the hint.
- If the answer equals the lemma (1st sg. present: `γράφω`), change the slot:
  use another person, or another target.
- In a deck that mixes tenses, aspects or cases, decide per item: either the
  sentence forces the choice with an unambiguous cue (see 2), or the hint
  carries the tag (`γράφω, αόρ.` / `γράφω, παρατ.`). Never leave a bare lemma
  where both tenses fit. If choosing the tense is the point of the deck,
  prefer cues and tag only the items no natural cue can settle.

Bad: `Χθες ___ ένα γράμμα. (έγραψ-)`. Good: `Χθες ο Κώστας ___ ένα γράμμα στον Νίκο. (γράφω)` → `έγραψε`.

## 4. Length: 6–14 words, one or two clauses

- Bad: `Εγώ ___ χθες.` (3 words, no scene)
- Bad: a 25-word sentence with three subordinate clauses.
- Good: `Το περασμένο καλοκαίρι η Σοφία ___ δύο εβδομάδες στη Νάξο με τους φίλους της.` (μένω) → `έμεινε`

## 5. Vocabulary at the level

- A2–B1 vocabulary unless the book is higher. Every word outside the target
  should be one the learner already knows.
- Prefer frequent lemmas. Rare lemmas are where invented inflections come from.
- No idioms, no regional or archaic words, no slang.

## 6. Meaningful sentences, mini-scenes

- Everyday situations: home, work, school, shopping, travel, food, weather,
  friends and family, health, free time.
- The sentence must be something a Greek speaker would say. No translationese,
  no nonsense pairings.
- Bad: `Ο σκύλος ___ το τραπέζι κάθε μέρα.` (διαβάζω) Good (present-tense set): `Ο παππούς ___ την εφημερίδα κάθε πρωί στο καφενείο.` (διαβάζω) → `διαβάζει`
- Two short clauses that build a small scene are better than one bare clause.
- Use varied Greek names (Νίκος, Ελένη, Κώστας, Σοφία, Γιώργος, Άννα, Δημήτρης,
  Μαρία) and common nouns, not the same three people throughout.
- Greek drops subject pronouns: the verb ending already shows the person. Write
  εγώ, εσύ, αυτός… only for contrast (`Εγώ έμεινα σπίτι, αυτός βγήκε.`). Let
  the person come from a name, a vocative, a possessive, another verb or a
  dialogue turn.
- Bad: `Την περασμένη εβδομάδα εσύ ___ ένα γράμμα.` (γράφω) — the pronoun is
  there only to fix the person, and no Greek speaker says it so.
- Good: `— Γιατί δεν ήρθες στο πάρτι; — ___ πυρετό όλο το βράδυ.` (έχω) → `είχα`

## 7. Variety across the set

Follow the grid from `workflow`. Within any 20 consecutive items:

- several persons and both numbers; masculine, feminine and neuter where relevant;
- several lexical classes of the target;
- some negatives (`δεν`, `μην`) and some questions (Greek question mark is `;`);
- the blank at the start, middle and end;
- several frames: statements, questions, negatives, short dialogues, a main
  clause with `όταν`, `ενώ`, `γιατί` or `πριν`;
- a dialogue puts each turn on its own line (`\n` in `text`), each starting
  with `— ` or `Α: ` / `Β: `, never two turns run together on one line:
  - Bad: `Α: Πόσους ___ κάλεσες; Β: Κάλεσα πολλούς.`
  - Good: `— Πόσους φίλους ___ στο πάρτι;` / `— Πολλούς, όλη την τάξη.` (two lines)
- the cue in different places, not always first in the sentence;
- no two items with the same frame ("Χθες ο Νίκος ___ …", "Χθες η Μαρία ___ …"),
  and across the set no frame on more than ~20% of items. "Time cue + subject
  + ___ + object" on every line is a template, not a deck.

## 8. No giveaways

- The answer, or another form of the same lemma, must not appear elsewhere in
  the sentence.
- Bad (γράφω): `Εγώ έγραψα στη μαμά και η Ελένη ___ στον μπαμπά.` — the stem is shown.
- Good (γράφω): `Χθες η Ελένη ___ ένα μεγάλο γράμμα στον μπαμπά της.` → `έγραψε`
- A question that repeats the target verb gives the answer away: avoid
  `— Έφαγες; — Ναι, ___.`

## 9. Answers

- `answers` holds only what the learner types into the blank: no punctuation, no
  neighbouring words.
- List every form that is grammatical in that sentence and fits the hint
  (variants, both aspects if the context allows both). If the list grows past
  two or three, rewrite the sentence instead.

## 10. Explanations: specific to the item

The explanation is what the learner reads after a wrong answer. It must tell
them what they missed in this item, so a generic line is as good as none.

- One short sentence, at most ~120 characters, for this item only.
- Name what makes this form: the stem change (γράφ- → έγραψ-), the augment
  (έ- only when stressed: έγραψα but γράψαμε), an irregular form (βλέπω → είδα),
  the accent shift, the ending for the person or case.
- In a deck that mixes tenses or cases, also say why this one: point to the cue
  in the sentence (`μία φορά` → aorist, `κάθε μέρα` → imperfect).
- One language for the whole deck: short English by default up to B1, Greek if
  the user or the level asks for it.
- Never put the same explanation on two items. If two read interchangeably,
  rewrite them. The server warns when one text repeats on more than two tasks,
  and when explanations are templated: the same sentence with only quoted
  words, numbers or the answer swapped (`… «πολλούς» … πρότασης 1`, `… «πολλές»
  … πρότασης 2`).

Bad (on every item of a deck): `Αόριστος: ολοκληρωμένη πράξη στο παρελθόν.`

Bad: `Past tense of γράφω.` (true of every γράφω item, says nothing about the slot)

Good:

- `μία φορά = one finished event → aorist; γράφ- → έγραψ-, stressed augment έ-.`
- `1st pl.: the accent moves to γράψ-, so no augment: γράψαμε, not έγραψαμε.`
- `Irregular aorist: βλέπω → είδα; 3rd pl. είδαν.`
- `κάθε καλοκαίρι = habit → imperfect; πηγαίνω → πήγαινα, 1st pl. πηγαίναμε.`
- `ενώ + ongoing action → imperfect μαγείρευε; the interruption (χτύπησε) is aorist.`

## 11. Deck rules: the general grammar, once

What applies to many items — how the form is built, agreement, the cue words,
an irregular group — is a deck rule, written once. The learner opens it with
the ⓘ on the card, before or after answering, and sees it after a wrong
answer. The explanation is left for what only this item needs.

- A rule is short Markdown: a title, two or three lines, `**bold**` for the
  forms, and a table where the point is a paradigm. No HTML, no images.
- Mark deck-wide theory `general: true` (the full paradigm, the agreement
  rule): it shows on every card. Give the other rules to the items that
  practise them with `rule`; at most one per item.
- 2–6 item rules per deck, plus one or two general ones, each with a short key:
  `aorist-stem`, `masc-pl-acc`.
- An explanation that would read the same on several items with a word or a
  number swapped is a rule: move it.

Bad (on every item, a word swapped):
`Η απάντηση «πολλούς» συμφωνεί με το ουσιαστικό της πρότασης.`

Good — a general rule `posos-forms`, title `Πόσος: γένος, αριθμός, πτώση`:

```
Το **πόσος** συμφωνεί με το ουσιαστικό σε γένος, αριθμό και πτώση.

|             | αρσενικό | θηλυκό | ουδέτερο |
|-------------|----------|--------|----------|
| ονομ. εν.   | πόσος    | πόση   | πόσο     |
| αιτ. εν.    | πόσο     | πόση   | πόσο     |
| γεν. εν.    | πόσου    | πόσης  | πόσου    |
| ονομ. πληθ. | πόσοι    | πόσες  | πόσα     |
| αιτ. πληθ.  | πόσους   | πόσες  | πόσα     |
| γεν. πληθ.  | πόσων    | πόσων  | πόσων    |
```

an item rule `masc-pl-acc`, title `Αρσενικό πληθυντικού, αιτιατική: πόσους`:
`Το ουσιαστικό είναι αντικείμενο: **πόσους** φίλους; — πολλούς, λίγους, αρκετούς.`
and on one item only, as its explanation: `φίλος is masculine: πολλούς φίλους, not πολλές.`

## 12. Choice blanks: pick one form

A blank can offer options instead of a text field: `choices` lists 1–5 wrong
options, and the learner picks among them and `answers[0]`. Typing stays the
default; use choices for forms that are easy to confuse (aorist and
imperfect, the article's cases), for the first decks of a level, or to vary a
long deck — not for a whole deck.

- Distractors are the same lemma and part of speech, plausible, and clearly
  wrong in this context: `πήγα` against `πηγαίνω`, `θα πάω`.
- No typos, no forms that do not exist, and no accent-only differences unless
  accents are the deck's point.
- None of them may fit the context: a distractor that is also right is a bug.
- The hint still names the lemma or the tag when the options alone don't
  decide.

Bad: `Χθες ___ στο σινεμά.` answers `πήγα`, choices `πήγε`, `πήγαμε` — the
person is the only difference and the sentence has no subject: all three fit.

Good: `Χθες ___ στο σινεμά.` hint `πάω`, answers `πήγα`, choices `πηγαίνω`,
`θα πάω` — χθες forces the past.

## 13. Question cards

A task with `kind: "choice"` is a question card instead of a sentence: `text`
is the question (no `___`), and its one blank holds the right option in
`answers` and 1–5 wrong ones in `choices`; no hint. The learner picks from a
list under the question. Use it for what a gap can't ask: naming a tense or a
case, picking the right translation, which form fits a described situation.
A deck can mix question cards and gap tasks.

- The question names exactly what to choose; the options are short and
  parallel (all tense names, all forms of one verb).
- Exactly one option is right in the question's terms.
- Keep gap tasks the backbone of a deck: a question checks recognition, a gap
  checks production.

Good: `Ποιος χρόνος είναι το «έγραψα»;` answers `Αόριστος`, choices
`Ενεστώτας`, `Παρατατικός`.

## 14. Public rules: grammar shared by every deck

Public rules live outside any deck, grouped by category, with the id
`category/name` (`ellenika/posos`). Any deck can use them.

- Before writing a deck rule, call `list_public_rules`. If a public rule
  already covers the point, use it: an item names it with
  `rule: "ellenika/posos"`, and `attach_public_rule` shows it on every card of
  the deck, like a `general` deck rule.
- Write a public rule (`set_public_rule`) only for grammar that holds beyond
  this deck: a paradigm, an agreement rule, a stem pattern. What belongs to
  this deck's selection or wording stays a deck rule.
- Only admins create categories; anyone can add a rule to one. Only the rule's
  creator or an admin can change it, and the change shows in every deck that
  uses it, so keep it general.
