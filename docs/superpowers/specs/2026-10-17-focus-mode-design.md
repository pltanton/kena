# kena: deck overview + focus mode for practice

Date: 2026-10-17. Status: approved. Replaces the cards layout inside the deck page.

## Problems (phone, real use)

1. The practice card shares the page with header, strip, title, stats and
   filters: ~40% of the screen is chrome the learner doesn't use while practising.
2. The page scrolls, so focus changes, the keyboard opening/closing and htmx
   swaps move the card.
3. Tapping anything except the input (Why?, Give up, hint) blurs it, the
   keyboard hides, the viewport grows and everything jumps.

## User stories → behaviour

- **Overview.** `/s/{slug}` becomes the deck overview: title, instruction,
  progress (solved/total, mistakes), owner's Edit, star; options Cards/List,
  filter (All/Unsolved/Mistakes), Shuffle, Reset progress; one primary button
  **Start** (or **Continue** when some tasks are solved; label shows the number
  left). List mode stays a scrolling page reached from the overview.
- **Focus mode.** `/s/{slug}/play?filter=&shuffle=&seed=` renders a dedicated
  layout without the site header, strip, footer and options:
  - Thin top bar: ✕ (back to overview) · slim progress bar with `3/9` · ⋯ menu.
  - The card is pinned under the bar (top-aligned, not centred). Sentence,
    inputs, nudge, Check / Give up / Next live inside the card, directly under
    the sentence.
  - The document never scrolls in focus mode (`overflow: hidden` on html/body,
    layout height bound to the visual viewport via `100dvh` plus a
    `visualViewport` resize listener setting `--vvh`;
    viewport meta `interactive-widget=resizes-content`). If a long sentence
    doesn't fit, only the card's inner area scrolls.
  - Tapping non-input controls inside the card (Why?, Give up, hint, Next)
    must not move focus away from the active input or the keyboard keeper:
    `pointerdown` → `preventDefault()` on those controls, actions on `click`.
    The keyboard stays open throughout a session.
  - ⋯ opens a bottom sheet (or small popover) with Mode (Cards ↔ List, which
    leaves focus mode), Filter, Shuffle, First unsolved, Reset progress.
    Opening it may blur the input; closing returns focus to the input.
  - Done screen inside the same layout: "All solved" / "Done", Practise again,
    Mistakes only (if any), Back to deck.
  - Desktop: same focus mode, centred column (max ~40rem), still no page chrome
    except the thin bar.
- Existing behaviours stay: retry with shake + nudge, Give up, Why? after
  correct, streak (first tries), Enter = Check / Next, no answers before give-up
  or correct, no-JS fallback (forms post and redirect).

## Verification

Headless Chrome with mobile emulation (390×844, touch, DPR 3). Emulate the
keyboard by shrinking the visible viewport (`Emulation.setDeviceMetricsOverride`
height 844 → 450) while an input is focused. Assert the card's
`getBoundingClientRect().top` and `window.scrollY` are unchanged across: focus
input, type, Check (wrong), tap Why?/hint, Give up, Next, correct answer, Next.
Assert `document.activeElement` stays an input or the keeper after tapping
Why?/Give up (dispatch pointerdown+click).
