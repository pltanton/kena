# Installed app: system bars and native feel

Date: 2026-10-28. Status: accepted. Extends `2026-10-17-i18n-theme-csp-design.md`
(theme) and `2026-10-18-offline-design.md` (the installed app).

## 1. System bars

An installed Android app (WebAPK) paints the navigation bar with the manifest
`theme_color`; the manifest had the light page colour only, so a dark page sat
on a near-white bar.

- Both viewports get `viewport-fit=cover`: where Chrome draws the page edge to
  edge, the page background fills the area behind the gesture bar. The site
  header and `main`, the focus-mode app, `#flash` and the existing bottom sheet
  and toast keep out of the bars with `env(safe-area-inset-*)`; the insets are
  0 elsewhere, so nothing moves on desktop.
- `/static/manifest.webmanifest?scheme=dark` serves the same manifest with
  `theme_color` and `background_color` set to the dark page colour (`#181b20`);
  without the parameter (or with any other value) it is the static light file.
  The path stays the precached one. `<link rel="manifest">` carries
  `?scheme=dark` for the `dark` theme; under `auto`, `app.js` sets it from
  `prefers-color-scheme` and updates it when the theme switch or the system
  scheme changes. Chrome picks up manifest changes on its own update check, so
  an installed app follows a theme change with a delay.

## 2. Native feel

- Controls (`a`, `button`, `summary`, `select`, `label`, `input`) have no tap
  highlight and `touch-action: manipulation` (no double-tap zoom delay).
- App chrome (buttons, `summary`, the site header, the focus bar, segmented
  links, switches) is `user-select: none`, so a long press does not select it.
  Deck content stays selectable.
- `@view-transition { navigation: auto; }` under
  `prefers-reduced-motion: no-preference`: page navigations cross-fade.
- Manifest `launch_handler: {client_mode: "navigate-existing"}`: a link into
  kena reuses the open app window.
- In standalone display mode, `overscroll-behavior-y: none` on `html` and
  `body`: no pull-to-refresh, no overscroll glow. The browser tab keeps them.

## 3. Keyboard in focus mode

The rule that picking never opens the keyboard stays. In addition, on Chrome
(VirtualKeyboard API, needs sticky user activation):

- When a card with a typed blank settles and the page has had a user gesture,
  focus goes to the blank and `navigator.virtualKeyboard.show()` opens the
  keyboard, also when the card before it was answered by a pick.
- A tap on the card outside any control focuses the first open blank and opens
  the keyboard.

`show()` needs the focused input to have `virtualkeyboardpolicy="manual"`; the
attribute is set for the call and removed right after, so tapping the input
still opens the keyboard the usual way.

Not possible: opening the keyboard on the first card after a page load. User
activation does not survive a navigation, and neither Android nor iOS shows the
keyboard for a focus without it. iOS has no VirtualKeyboard API; there only the
existing keeper trick applies.

## 4. Testing

- Go: viewport metas carry `viewport-fit=cover`; the manifest serves both
  schemes with the right colours, `launch_handler`, the same `id`, and falls
  back to light for unknown values; the layout links the scheme of the theme
  cookie.
- Bars, transitions and the keyboard are checked on an Android phone with the
  installed app.
