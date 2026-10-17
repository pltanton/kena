# Partial check of multi-blank tasks

Date: 2026-10-18. Status: accepted. Extends `2026-10-17-practice-flow-design.md`,
`2026-10-17-focus-mode-design.md` and `2026-10-18-offline-design.md`.

## Problem

On the phone a learner types the first word of a two-blank task and presses
Enter. Today that is a full check: the empty blank counts as wrong, the
attempt is recorded as a mistake and the streak resets. They wanted the typed
word checked and the cursor moved to the next blank.

## Behaviour

- **Partial check.** A check (Enter or Check, not Give up) whose answers
  include an empty blank (whitespace only, `greek.NormalizeAnswer` = "") is
  partial: only the filled blanks are evaluated.
  - Correct filled blanks lock: `readonly`, green, answer font, showing the
    typed text (as in the retry form).
  - Wrong filled blanks get the retry styling: typed text, red underline,
    `aria-invalid`, shake, and the nudge (accent / final ς / near miss) over
    the wrong blanks only.
  - Empty blanks stay untouched: editable, no error styling, no value.
  - No expected answer reaches the page; only the user's own text.
  - Screen readers: the hidden status is "Not quite, try again." when a filled
    blank is wrong without a nudge, and "Right so far." when every filled
    blank is right.
- **Full check** when every blank is filled (or on Give up): unchanged —
  scoring, recording, streak, result view.
- **Single-blank tasks** are a special case of the rule: an empty blank is a
  partial check with nothing to evaluate. The page script doesn't send a
  check whose editable blanks are all empty (Enter keeps focus, nothing
  happens); Give up still submits.
- **Recording.** A partial check records no attempt: progress, mistakes and
  `/sync` are untouched, the `cid` is ignored, the offline engine queues
  nothing and leaves its state as it was.
- **Streak.** A wrong filled blank counts like a wrong check: `tries` + 1 and
  the streak resets, so a later full correct check is not a first try. A
  partial check with every filled blank right leaves `tries` and the streak
  as they were.
- **Focus** (page script, `preventScroll`): a wrong filled blank → the first
  wrong one; otherwise the next empty blank after the one that was focused
  when the check was sent, wrapping to the first. Focus mode and worksheet
  items alike (`app.js` remembers the blank, `play.js` / `app.js` focus it
  after the swap).
- **Worksheet** items behave the same (`list_item` fragment).
- **Retry views** generally render an empty blank untouched instead of wrong
  and leave it out of the nudge (it can only appear there from a partial
  check or an attempt recorded before this rule).
- **No JS.** The form posts; a partial check renders the page instead of
  redirecting: focus mode at that card (`/s/{slug}/play` with the bar of
  `?at=<task>`), or the worksheet with that item in its partial state.
  Nothing is recorded.

## Offline engine

`offline.js` `check` mirrors the handler: partial → the same retry markup, no
`attempt`, no state change; the golden file gains a partial-right, a
partial-wrong and a following full check of the two-blank task, and its old
empty single-blank step now renders a partial check.

## Verification

Go: partial with a right/wrong/empty mix → no attempt recorded, locked and
wrong classes, empty blank untouched, no expected answers in the body, `tries`
incremented and streak reset on wrong, unchanged on all-right; full check
afterwards records once; worksheet fragment; no-JS page; Give up with an
empty blank still reveals. Golden + node tests. Headless Chrome on a 2-blank
card: type the first blank, Enter → it locks green, `activeElement` is the
second blank, `scrollY` unchanged; type a wrong word in the second with the
first empty → only the second shakes.
