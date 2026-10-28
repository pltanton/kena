# Single sign-on through OIDC

Date: 2026-10-28. Status: accepted.

## Problem

The household's services log in through Kanidm (`id.pltanton.dev`). kena has
its own usernames and passphrases, so it is the odd one out, and an agent's
OAuth connect asks for a kena passphrase nobody remembers.

## Decisions

- kena is an OIDC client (authorization code + PKCE S256, `openid profile`
  scopes); optional: without `KENA_OIDC_ISSUER` nothing changes.
- SSO only links to existing kena accounts: the first login matches the ID
  token's `preferred_username` (the part before `@`, Kanidm's short name) to a
  kena username and stores the token's `sub` in `users.oidc_subject`; later
  logins go by `sub`, so a rename on either side keeps the link. No account
  for that name → an error asking an admin to create it. A kena account linked
  to another `sub` is refused.
- Passphrase login stays as the fallback (collapsed under the SSO button); QR
  login stays. Admin flags stay managed in kena.

## Config

`KENA_OIDC_ISSUER` (e.g. `https://home.id.pltanton.dev/oauth2/openid/kena`),
`KENA_OIDC_CLIENT_ID`, `KENA_OIDC_CLIENT_SECRET`, `KENA_OIDC_NAME` (button
label). Discovery runs on first use, so kena starts while the IdP is down. The
redirect URI is `KENA_PUBLIC_URL/login/sso/callback`.

## Flow

- `GET /login/sso?next=` (404 when SSO is off): state, nonce and PKCE verifier
  in a 10-minute `kena_sso` cookie (HttpOnly, SameSite=Lax, Secure on https,
  path `/login/sso`), then a redirect to the IdP.
- `GET /login/sso/callback`: the state must match the cookie (which is then
  cleared); the code is exchanged with the verifier; the ID token is verified
  (issuer, audience, expiry, signature via JWKS) and its nonce must match;
  then the account is found or linked as above and the web session starts
  exactly as after a passphrase login (`loggedIn`: device progress merge,
  redirect to the safe `next`).
- IdP errors, a missing or mismatched state and verification failures show the
  login page with a generic "SSO login failed" message; details go to the log.

## Pages

- `/login`: "Log in with <name>" button first, the passphrase form in a
  `<details>` below (open when SSO is off, or after a failed passphrase try).
- OAuth authorize page (agents connecting): the same button with `next` =
  the authorize request, so after SSO the page shows Allow.

## Deploy

- `services.kena.oidc.{issuer, clientId, name}` and
  `services.kena.environmentFiles` in `nix/module.nix`.
- home-server: `sso.clients.kena` (redirect `/login/sso/callback`,
  `envVar = "KENA_OIDC_CLIENT_SECRET"`, secret group `kena`) and the kena
  service reads `/var/lib/sso-secrets/kena.env`.
- Yulia needs a kena account named `julsa` (Kanidm short name) before her first
  SSO login.

## Tests

A fake IdP (httptest: discovery, JWKS, token endpoint, RS256 ID tokens):
link by name, login by `sub` after a rename, unknown name, account linked to
another `sub`, bad state, bad nonce, IdP error, SSO off → 404, buttons on
`/login` and the authorize page, `next` honoured.
