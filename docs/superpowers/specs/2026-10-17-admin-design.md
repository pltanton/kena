# Web admin for users

Date: 2026-10-17. Status: accepted.

## Scope

Admins manage accounts in the browser instead of the CLI or MCP. Everything
lives under `/admin`; for anyone who is not a logged-in admin every `/admin`
route answers 404 (anonymous visitors included, no login redirect). Admins
see an "Admin" link on their Account page.

## Pages

- `GET /admin`: create-user form (username, admin checkbox) and the user
  list, newest activity first in each row: username, admin badge, decks,
  last activity (latest of session start, attempt, token use), active sessions
  and active tokens. Each row opens the user page. Mobile: stacked rows.
- `POST /admin/users`: creates the user with a generated passphrase and shows
  it once (`Cache-Control: no-store`, copy button).
- `GET /admin/users/{name}`: the same facts plus actions:
  - Reset passphrase → confirm page → `POST …/passphrase`: new passphrase shown
    once (no-store); `store.SetPassword` revokes sessions, OAuth grants and codes.
  - Make admin / Remove admin → `POST …/admin` (`is_admin=1|0`), no confirm.
  - Log out everywhere → confirm → `POST …/logout`: deletes sessions, revokes
    OAuth grants, deletes OAuth and QR login codes; personal tokens stay.
  - Delete user → confirm page that asks to type the username → `POST
…/delete`: deletes the user (decks, tasks, stars, tokens, grants cascade)
    and their progress (attempts under their `progress_id`).
- Guards (store level, `ErrForbidden` with a message the page shows): an admin
  cannot change their own admin flag or delete themselves; the last admin can
  neither lose the flag nor be deleted. Guards lock the admin rows, so two
  concurrent demotions cannot remove every admin.
- All POSTs go through `session.CrossOrigin` and answer 404 to non-admins.

## Store

`ListUserSummaries`, `SetAdmin(actorID, userID, isAdmin)`,
`LogoutEverywhere(userID)`, `DeleteUser(actorID, userID)`.

## MCP

`admin_set_admin {username, is_admin}`, `admin_logout_user {username}`,
`admin_delete_user {username, confirm_username}` (the two must match), with
the same guards; `admin_list_users` keeps working.
