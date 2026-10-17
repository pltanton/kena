# Practice flow, reset, QR login, passphrase reveal, theme

Date: 2026-10-17. Status: accepted.

## 1. Retry on wrong, Give up, Why?

- A wrong check (cards and list) no longer reveals the answer. The same task
  comes back as a form: wrong blanks keep the typed text (`value`), get a red
  underline, `aria-invalid="true"` and a ~300 ms shake (none under
  `prefers-reduced-motion`); blanks that were right become `readonly`, green,
  in the answer font, and still submit their value. The first wrong input is
  focused (cards: by the page script with `preventScroll`; Enter = Check).
- Nudge under the sentence, muted: the worst `greek.Classify` kind among the
  wrong blanks (given vs that blank's accepted answers) — accent → "Almost —
  check the accent", final_sigma → "Almost — final ς", near_miss → "Close — a
  letter or two off"; empty/other anywhere → no nudge.
- "Give up" is a quiet button next to Check, always visible. It submits the
  form with `giveup=1`; the attempt is recorded with the typed answers
  (wrong, unless they happen to be right, in which case it is a normal
  correct check) and the result shows given → expected, the explanation and
  Next — today's wrong result.
- Every check is an attempt, as before. The form carries `tries` (wrong checks
  of this task so far). The first wrong check resets the streak; a later
  correct check solves the task (last attempt correct) without growing the
  streak; the wrong attempts stay in the mistakes report. Only first-try
  correct answers grow the streak.
- Answers reach the browser only after give-up or a correct check. The retry
  form shows which blanks are right, and only the user's own text. List mode
  renders a task whose last attempt is wrong in the retry state (it cannot
  tell a give-up from an unfinished retry), so a reload never reveals. The
  `/mistakes` page still shows expected answers for attempted tasks.
- A correct result with an explanation shows a collapsed `<details>` "Why?"
  with the explanation; focus stays on the keeper so Enter still goes Next.
- Without JS a check still redirects (cards: to the next card), so retry is an
  htmx-only behaviour.

## 2. Reset progress

Quiet "Reset progress" link at the bottom of the practice page when the viewer
has attempts in the deck → `GET /s/{slug}/reset` confirm page → `POST`
deletes the viewer's attempts (`progress_id` logged in, device id otherwise)
for that deck's tasks (`store.ResetProgress`) and redirects to the deck. Any
viewer who can see the deck may reset their own progress; CrossOrigin applies.

## 3. First unsolved

Cards: a quiet "First unsolved" link next to the position counter when the
current card is not the first unsolved task of the current order and filter;
it opens the deck without `after`, which already starts at the first unsolved
card. List: a "First unsolved" link above the list to `#t-<id>`. Hidden when
nothing is unsolved.

## 4. QR login

- Table `login_codes (hash bytea PK, user_id → users ON DELETE CASCADE,
created_at, expires_at, used_at)`. `store.CreateLoginCode(user, ttl)`
  deletes the user's other unused codes and returns a `kena_q_` secret
  (stored hashed); `store.UseLoginCode(secret)` marks it used once and
  returns the user; `PurgeExpired` drops codes a day after expiry or use.
- Account → "Log in on your phone": a "Show code" / "New code" POST
  (`/account/qr`) renders the page with a QR (rsc.io/qr, rendered as inline
  SVG, black on white) for `<publicURL>/login/qr/<secret>`, valid 5 minutes,
  plus "Anyone who scans this logs in as you — don't share screenshots."
- Phone: `GET /login/qr/{secret}` does not consume the code (link previews
  and prefetchers fetch GETs); it shows "Log in as <name> on this device?"
  with one button. `POST /login/qr/{secret}` (CrossOrigin) uses the code,
  replaces any session, merges the device's anonymous attempts like a normal
  login and redirects to `/`. Unknown, used or expired code → 410 page "This
  code expired — make a new one on your computer".

## 5. Show / hide passphrase

Every passphrase input (login, account ×2, oauth login) sits in `.pass` with a
`type="button"` "Show" toggle (`aria-pressed`, hidden until the script runs).
`/static/reveal.js` flips `type` and sets fields back to `password` on submit
so password managers still save them.

## 6. Theme

- Cookie `kena_theme` = auto | light | dark (1 year). The layout renders
  `<html data-theme="…">`, so there is no flash; theme-color metas follow
  (both media-gated for auto, one fixed colour otherwise).
- CSS colours are defined once with `light-dark()`; `color-scheme` picks the
  side: `light dark` for auto, `light` or `dark` for the forced themes. The
  strip image (not a colour) is switched by `[data-theme=dark]` and the dark
  media query under `:not([data-theme=light])`.
- Control: Auto · Light · Dark in a quiet page footer and on the Account
  page; `POST /theme` (CrossOrigin) sets the cookie and redirects back to a
  safe `next`; with JS the switch applies instantly and posts in the
  background. The oauth page does not share the layout and stays auto.
