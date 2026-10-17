# Swipe navigation in focus mode

Date: 2026-10-17. Status: accepted. Extends `2026-10-17-focus-mode-design.md`
and `2026-10-17-practice-flow-design.md`.

## Problem

On the phone a learner can only leave a card by solving it or giving up. They
want to skip a card they're not ready for and look back at the one before,
without the keyboard closing or the page jumping.

## Behaviour

- **Forward (skip)**: swipe left, ›, or ArrowRight. Shows the next card of the
  current order and filter, exactly what Next would show (`/s/{slug}/next?after=<id>`).
  Nothing is recorded: the skipped task stays untried (or as it was), the
  streak stays as it is. On the last card forward opens the done screen.
  When the learner went back first, forward walks the session history to the
  card they came from (browser-like); at the end of history it is a skip.
- **Back**: swipe right, ‹, or ArrowLeft. Shows the previous card of this
  session's history in its current state; from the done screen, the last card.
  With no history (first card after a reload) it falls back to the previous
  card of the current order and filter; ‹ is disabled when neither exists.
- **Card state** (server-side, from the viewer's last attempt):
  - untried → the fresh form;
  - last attempt wrong and not a give-up → the retry form (typed text, right
    blanks locked, nudge, `tries=1`); no answers;
  - last attempt correct → the result: "Correct", the typed answers, Why? and Next;
  - last attempt a give-up → the result: given → expected, explanation, rule, Next.
- **Streak**: skipping and going back never change it. A revisited retry card
  carries `tries=1`, so solving it can't grow the streak; a wrong check resets
  it as usual. A revisited result doesn't show "N in a row".

## Give-ups are recorded

Migration 010 adds `attempts.gave_up boolean NOT NULL DEFAULT false`. A Give up
whose answers are wrong records `gave_up = true` (`store.RecordGiveUp`);
`LastAttempt.GaveUp` reports it. Only that flag lets a revisited card show the
answer: older wrong attempts (before 010) and unfinished retries come back as
the retry form. The mistakes report is unchanged (a give-up is still a wrong
attempt). The worksheet keeps rendering wrong tasks as retries.

## Routes

- `GET /s/{slug}/play/card?id=<task>&filter&shuffle&seed&n&streak`: the card
  fragment (`card` or `card_result`) in its state plus the out-of-band
  `#focus-pos`. 404 when the set is not visible or the task is not in it
  (`id` of a task from another set included). Never records anything.
- `GET /s/{slug}/play?at=<task>`: the focus page showing that card in its state
  (the no-JS ‹ target). Same 404 rule.
- Progress position for a shown card: `n − (cards of the filter after it in the
order)`, clamped to `1..n`, `n` from the session (`n` param) or the current
  filter's size; for a card that still passes the filter this is what `/next`
  shows.

## Top bar

`✕ · ‹ · progress n/N · › · ⋯`. ‹ and › are 44px links inside `#focus-pos`, so
every fragment's out-of-band bar updates them:

- ‹ `href` = `/s/{slug}/play?at=<previous in order>&…`, `aria-disabled` without
  href when there is none; `data-prev` (that id), `data-card` (the card-route
  base URL with filter, order, `n`, `streak`).
- › `href` = `/s/{slug}/play?after=<id>&…` (the no-JS skip), `data-id`,
  `data-next` (the `/next` fragment URL with `n` and `streak`); disabled on the
  done screen.
- Check responses (retry, result) carry the same bar with the updated streak;
  they don't know the previous card, the script remembers it per id.

Without JS ‹ › are plain links.

## Script (inline in `play.html`)

- History: `sessionStorage["kena:nav:<path>?filter&shuffle&seed"] = {ids, at}`,
  at most 200 ids; every card shown (load, Next, skip, back) is recorded; a new
  card after going back truncates the forward part. Storage errors are ignored
  (history then lives only in memory).
- Navigation goes through `htmx.ajax` into `#card` (`show:none`); one request
  at a time. Before it, focus moves to the keyboard keeper; after the swap the
  existing `htmx:afterSettle` handler focuses the first editable input with
  `preventScroll` (or the keeper on a result). The keyboard stays open.
- Animation: the new card slides in from the direction of travel, 180 ms
  translate 40px + fade (Web Animations); none under `prefers-reduced-motion`.
- Gesture, on `#focus-main` only (not the bar, not the sheets): passive
  `touchstart`/`touchmove`/`touchend`, nothing prevented; decided on
  `touchend`: one finger, |dx| ≥ 60px, |dx| > 2·|dy|, under 600 ms. Ignored when
  the touch starts in an input or textarea, within 20px of the screen edge
  (the browser's own back gesture), or while the ⋯ menu or ⓘ sheet is open.
- Keys: ArrowLeft / ArrowRight without modifiers, unless focus is in an
  editable text field (the keeper doesn't count) or a sheet is open.

## Verification

Server tests: card route states (untried, retry without answers, solved,
given up; a pre-010-style wrong attempt never reveals), invisible set and
foreign task 404, ‹ › links and their disabled states, skipping records no
attempt, `?at=`. Headless Chrome (390×844, touch, DPR 3) over CDP:
`Input.dispatchTouchEvent` swipes left/right; assert the card changes, the
bar index follows, `scrollY` stays 0, the card top is unchanged and
`document.activeElement` is an input; a vertical drag and a swipe starting in
an input do nothing.
