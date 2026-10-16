# kena accounts: passphrase login, synced progress

Date: 2026-10-16. Status: approved. Extends `2026-10-16-kena-design.md`.

## Goal

A user logs in on any device with username + passphrase and sees the same
progress. Claude app / ChatGPT connect without the user ever seeing a token.
Personal tokens stay for Claude Code / Hermes, created on an Account page.
Anonymous practice keeps working without login.

## Data (migration 003)

- `users.password_hash text` (nullable; argon2id PHC string).
- `users.progress_id uuid NOT NULL DEFAULT gen_random_uuid()` — the device id
  used for attempts while logged in.
- `sessions(hash bytea PK, user_id → users ON DELETE CASCADE, created_at, expires_at)`;
  secret `kena_s_…`, cookie `kena_session` (HttpOnly, SameSite=Lax, Secure
  behind TLS, 1 year).

## Behaviour

- Passphrase: 4 random words from the embedded EFF short wordlist, joined with
  `-` (e.g. `olive-tiger-harbor-seven`). Users may set their own (min 10 chars).
- Login: `GET/POST /login` (username, passphrase, `next`). Wrong credentials →
  1 s delay, generic error. Success: create session; move attempts of the current
  anonymous `kena_device` to the user's `progress_id`
  (`UPDATE attempts SET device_id = progress_id WHERE device_id = anon`); redirect
  to `next` (local paths only).
- While logged in, the web uses `progress_id` instead of the device cookie.
- `POST /logout` deletes the session.
- `GET /account`: username; change passphrase (current + new); personal tokens
  (label, created, last used; "New token" shows it once; Revoke); connected apps
  (active OAuth grants by client name; Disconnect).
- Header: "Log in" or "<username> · Account".
- All state-changing POSTs check `Origin`/`Referer` host = public host (CSRF);
  cookies are SameSite=Lax.

## OAuth authorize page

Replaces "paste your token": logged in → "Allow <app> to create and edit
<username>'s sets? [Allow] [Cancel]" plus the existing "you'll be sent back to
<host>" line; not logged in → login form on the same page (then Allow). Grants
issued this way have no `token_id`; they are revoked from the Account page or
when the passphrase changes (all grants and other sessions of that user).

## Admin

- CLI: `kena user create NAME [--admin]` and `kena user passwd NAME` print a new
  passphrase (both migrate first). `token create` stays.
- MCP: `admin_create_user` returns a generated passphrase;
  new `admin_reset_password{username}` returns a new one and revokes that user's
  sessions and OAuth grants.

## Out of scope

Self sign-up, email, passkeys, rate limiting beyond the delay.
