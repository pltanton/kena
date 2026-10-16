---
name: item-rules
description: Rules for a single gap-fill item (one target per blank, context forces the form, hints, length, level, variety, no giveaways) with good and bad Greek examples.
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
- Good: `Το γράμμα για τη γιαγιά; Εγώ το ___ χθες το βράδυ.` (γράφω) → `έγραψα`

## 2. Context forces the form

The learner must be able to derive exactly one form, or you list every form that
fits in `answers`. Force each feature with something visible:

- person and number: a subject noun or pronoun, or another verb with the same subject;
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

## 3. Hint = lemma or feature tag, never the answer

- Use the format from the spec, the same for the whole set: lemma (`γράφω`),
  lemma + tag (`γράφω, αόρ.`) or tag only (`γεν. πληθ.`).
- Never put the answer, a part of it, or its stem in the hint.
- If the answer equals the lemma (1st sg. present: `γράφω`), change the slot:
  use another person, or another target.

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

## 7. Variety across the set

Follow the grid from `workflow`. Within any 20 consecutive items:

- several persons and both numbers; masculine, feminine and neuter where relevant;
- several lexical classes of the target;
- some negatives (`δεν`, `μην`) and some questions (Greek question mark is `;`);
- the blank at the start, middle and end;
- no two items with the same frame ("Χθες ο Νίκος ___ …", "Χθες η Μαρία ___ …").

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
