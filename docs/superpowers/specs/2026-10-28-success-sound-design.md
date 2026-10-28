# Success sound

Date: 2026-10-28. Status: accepted. Extends `2026-10-17-i18n-theme-csp-design.md`
(preferences in cookies) and `2026-10-17-practice-flow-design.md` (focus mode).

## 1. What plays

- A short sound when a check comes back correct in focus mode.
- A longer one (an arpeggio of three or four notes in the same timbre) when the
  done card with `complete` appears.
- Nothing on a wrong check, a partial check, Give up, or a revisited card
  (`/play/card`, `?at=`) that is already answered.

## 2. Preference

- Cookie `kena_sound`: `chime` (default), `coin`, `pop`, `off`. An invalid or
  missing cookie means `chime`.
- `POST /sound` (field `sound`, `next`) goes through `setPref` like `/theme` and
  `/lang`, behind `session.CrossOrigin`; htmx requests get 204.
- `prefs` gains `Sound` and `Sounds []themeOption` (value, label key, current).
- Template `sound_switch`: a form of buttons with `aria-pressed`, the same shape
  as `theme_switch`. It appears in the focus-mode ⋯ menu (a row next to the
  theme row) and on `/account` under its own heading.
- `<body data-sound="…">` in `layout.html` and `play.html` (body, so the
  `<html>` tag stays as tests expect it); the script reads the current value
  from there and updates it when the switch is used.
- Picking a sound in the switch plays it once as a preview (that click is the
  user gesture that unlocks audio). Picking `off` is silent.
- Labels in all three catalogs: `sound.chime`, `sound.coin`, `sound.pop`,
  `sound.off`, plus row/heading keys.

## 3. Sounds

`static/sound.js`, synthesised with the Web Audio API; no audio files, so no
precache entries, no CSP `media-src` and nothing to license.

- `chime`: two soft sine notes rising (bell-like, quick decay).
- `coin`: square-wave blip, two fast notes up (8-bit coin).
- `pop`: short percussive triangle/sine with a fast pitch drop (marimba knock).
- Each has a `done` variant: a rising arpeggio in the same timbre.
- Volume is moderate and fixed; there is no volume setting.

One `AudioContext`, created lazily and resumed on the first `pointerdown` or
`keydown`, so the sound that plays after the htmx response is not blocked by
autoplay rules. If the API is missing or the context cannot start, nothing
plays and nothing is reported.

## 4. Trigger

`sound.js` listens to `htmx:afterSettle` on the play page:

- The request was a POST to a check URL (`/t/{id}/check`) and the settled card
  contains the correct verdict (`.verdict` with the tick) → correct sound.
- The settled card is `.card.done.complete` → done sound, at most once per page
  load.

Detection works on the DOM, so offline practice (`offline.js` answers the same
htmx requests with the same markup) needs no change and the offline golden file
stays as it is. The script must be listed among the precached static files like
the others (every file under `static/` already is). `sound.js` loads on every
page with the other scripts, so the switch on `/account` previews too.

## 5. Known limits

- iOS mutes Web Audio while the ring/silent switch is on silent.
- The cookie is per browser, not per account, like theme and language.

## 6. Testing

- Go: `/sound` refuses unknown values and cross-origin posts, sets the cookie,
  redirects to a safe `next`, answers 204 to htmx; `data-sound="chime"` by
  default and the chosen value after the cookie is set; the switch renders on
  `/account` and in the play menu with the current option pressed; catalog keys
  exist in en/ru/el (existing i18n tests).
- The sounds themselves are checked by ear.
