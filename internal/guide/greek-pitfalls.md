---
name: greek-pitfalls
description: Modern Greek traps for task authors (accent shift, enclitic double accent, final sigma, monotonic spelling, aspect after θα/να, clitics, σε + article, final ν, variant forms, rare lemmas), each with a concrete rule.
---

# Greek pitfalls

Checking is strict: after trimming, collapsing spaces and lowercasing, the
learner's answer must match one of `answers` letter for letter, accents and final
sigma included. A key with a wrong accent marks every correct learner wrong.

## 1. Accent shift

The accent can fall only on one of the last three syllables. When an ending adds
a syllable or a long ending appears, the accent moves.

- Nouns: ο άνθρωπος → του ανθρώπου, των ανθρώπων, τους ανθρώπους (but οι άνθρωποι).
  το πρόβλημα → του προβλήματος, τα προβλήματα, των προβλημάτων.
- Feminine nouns in unstressed -α/-η mostly stress the genitive plural ending:
  η θάλασσα → των θαλασσών, η ώρα → των ωρών, η γυναίκα → των γυναικών
  (exception: η πόλη → των πόλεων).
- Verbs, past tenses: the accent sits on the third syllable from the end. Two-syllable
  stems get a stressed augment έ- only when the accent needs it:
  έγραψα, έγραψες, έγραψε, γράψαμε, γράψατε, έγραψαν; έγραφα → γράφαμε.
  Longer stems take no augment: διάβασα → διαβάσαμε, αγόρασα → αγοράσαμε.
- Medio-passive present: έρχομαι, έρχεσαι, έρχεται, ερχόμαστε, έρχεστε, έρχονται.

Rule: for every inflected answer, count syllables and place the accent by the
paradigm (see `self-check`, step 3). Never place it by memory of a similar word.

## 2. Enclitic double accent

A word stressed on the third syllable from the end takes a second accent on its
last syllable when an enclitic (μου, σου, του, της, μας, σας, τους, το, τα, τον)
follows it:

- ο δάσκαλός μου, το τηλέφωνό σου, η γειτόνισσά μας, διάβασέ το
- but no change on words stressed later: το σπίτι μου, ο φίλος σου, το παιδί της

Rules:

- If the answer is such a word and the enclitic follows the blank, the key must
  carry both accents: `Ο ___ μου μένει στην Πάτρα.` → `δάσκαλός`.
- Learners often miss this. Use it only when the set teaches it; otherwise move
  the enclitic away or choose a word stressed on the last two syllables.

## 3. Final sigma

- ς at the end of a word, medial sigma (U+03C3) everywhere else: ο δρόμος, της θάλασσας, σήμερα.
- This applies to answers and hints too. The server rejects a call with a medial
  sigma at the end of a word or a ς inside one.
- An enclitic does not change it: ο φίλος μου.

## 4. Monotonic spelling only

- One accent mark (τόνος) on every word of two or more syllables, plus
  dieresis where needed (ρολόι, κοροϊδεύω, λαϊκή).
- Monosyllables carry no accent (θα, να, μου, το, και, φως), except ή (or) and the
  question words πού and πώς (vs. the conjunctions που, πως).
- Capital initial: write the accent before the capital (Έγραψα, Όταν).
- Never output polytonic characters (breathings, grave, circumflex, or the Greek
  Extended block U+1F00–U+1FFF, including oxia forms that look like τόνος).
  The server rejects them. Do not copy text from old sources or polytonic fonts.
- Never type Latin lookalikes inside Greek words (Latin o, v, A, E, H, K, M, T, X, p, y).
- The Greek question mark is `;`. Never use `?` in Greek text.

## 5. Aspect after θα and να (the main ambiguity)

θα and να take either the perfective (θα γράψω, να γράψω) or the imperfective
(θα γράφω, να γράφω) stem. Both are usually grammatical; only the meaning differs.

Rules:

- Force the aspect with a strong cue:
  - imperfective: κάθε μέρα, συνέχεια, πάντα, κάθε πρωί, όλη τη μέρα
    `Από αύριο η Σοφία θα ___ κάθε μέρα μία ώρα αγγλικά.` (διαβάζω) → `διαβάζει`
  - perfective: μόλις, μέχρι + deadline, πριν + event, a single counted event
    `Μόλις η Σοφία ___ το κεφάλαιο, θα βγει για καφέ.` (διαβάζω) → `διαβάσει`
    `Η Ελένη πρέπει να ___ το γράμμα σήμερα, πριν κλείσει το ταχυδρομείο.` (γράφω) → `γράψει`
- A bare `αύριο` is not enough: `Αύριο θα γράφω όλη τη μέρα` is correct Greek.
- If you cannot make the cue decisive, put the aspect or tense in the hint
  (`γράφω, στιγμ. μέλλ.` / `γράφω, εξακ. μέλλ.`) or list both forms in `answers`.
- Watch the hint = answer trap: the imperfective 1st sg. (θα γράφω, να γράφω)
  equals the lemma. Use another person.

## 6. Clitic placement

- Before finite verbs, also after θα, να, δεν, μην: θα το γράψω, δεν το έγραψα,
  να τον δεις, θα σου το δώσω (indirect before direct).
- After imperatives and gerunds: γράψε το / γράψ' το, δώσε μου το, φέρνοντάς το.

Rules:

- Keep the clitic outside the blank; blank only the verb.
- If the blank is an imperative before a clitic, list both full and elided
  forms where both exist: `___ το αμέσως!` (γράφω) → `γράψε`, `γράψ'`.
  Better still, avoid imperative + clitic blanks unless the set teaches them.
- If the target is the pronoun itself, make the referent explicit:
  `Πού είναι τα κλειδιά μου; Δεν ___ βρίσκω πουθενά.` → `τα`

## 7. σε + article

σε merges with the definite article: στον, στη(ν), στο, στους, στις, στα.
Never write σε τον, σε το.

- If the article is the target after σε, the blank covers the whole merged form:
  `Πάμε ___ σινεμά απόψε;` → `στο`
- Otherwise keep στο/στη/στον in the text, outside the blank.

## 8. Final ν: τη(ν), δε(ν), μη(ν), ένα(ν)

The ν is required before a vowel and before κ, π, τ, ξ, ψ, γκ, μπ, ντ (so also τζ and τ + sigma);
before other consonants it is usually dropped, but writers vary.
τον and στον always keep their ν.

Rules:

- Do not blank these words unless the target is exactly this rule.
- If you must, before a vowel or one of those consonants list only the form with ν
  (`δεν έχω`, `την Παρασκευή`); before any other consonant list both
  (`δε θέλω` and `δεν θέλω` → answers `δε`, `δεν`).
- In your own text, follow the rule consistently.

## 9. Variant forms

Many forms have accepted variants. List every variant the learner may know in
`answers`:

- ήρθα / ήλθα (έρχομαι), ήρθαμε / ήλθαμε
- αγαπούσα / αγάπαγα (imperfect of -άω verbs; αγαπούσα is the standard one)
- 3rd pl. long forms: έγραψαν / γράψανε, γράφουν / γράφουνε, ήρθαν / ήρθανε
- πηγαίνω / πάω: with hint `πηγαίνω`, the present 3rd sg. is both `πηγαίνει` and `πάει`
- γράψε / γράψ' before a clitic (see 6)

If an item needs more than two or three variants, change the slot or the lemma.

## 10. Lemmas to avoid

- Rare or bookish lemmas: their forms are the ones language models invent.
- Lemmas whose form in your slot is disputed, rare or missing (many nouns have
  no genitive plural in use; many verbs have no common passive aorist).
- Defective or heavily irregular verbs outside the set's target.

Rule: if you are not certain a form is in everyday use and spelled exactly so,
pick another lemma. The pool is large enough.
