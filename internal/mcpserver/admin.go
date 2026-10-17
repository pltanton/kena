package mcpserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pltanton/kena/internal/passphrase"
	"github.com/pltanton/kena/internal/store"
)

type CreateUserIn struct {
	Username string `json:"username" jsonschema:"2-32 chars: lowercase latin letters, digits, _ or -, starting with a letter or digit; not \"me\""`
	IsAdmin  bool   `json:"is_admin,omitempty" jsonschema:"grant admin rights"`
}

type IssueTokenIn struct {
	Username string `json:"username" jsonschema:"existing username"`
	Label    string `json:"label,omitempty" jsonschema:"where the token will be used, e.g. laptop claude code"`
}

type RevokeTokenIn struct {
	TokenID int64 `json:"token_id" jsonschema:"token id from admin_list_users or admin_issue_token"`
}

type ListUsersIn struct{}

type SetAdminIn struct {
	Username string `json:"username" jsonschema:"existing username"`
	IsAdmin  bool   `json:"is_admin" jsonschema:"true grants admin rights, false removes them"`
}

type UserNameIn struct {
	Username string `json:"username" jsonschema:"existing username"`
}

type DeleteUserIn struct {
	Username        string `json:"username" jsonschema:"existing username"`
	ConfirmUsername string `json:"confirm_username" jsonschema:"the same username again; ask the user to confirm before deleting"`
}

type LogoutResult struct {
	Username string `json:"username"`
	Note     string `json:"note"`
}

type TokenInfo struct {
	ID       int64  `json:"id"`
	Label    string `json:"label"`
	Created  string `json:"created"`
	LastUsed string `json:"last_used,omitempty"`
	Revoked  string `json:"revoked,omitempty"`
}

type UserInfo struct {
	ID       int64       `json:"id"`
	Username string      `json:"username"`
	IsAdmin  bool        `json:"is_admin"`
	Created  string      `json:"created"`
	Tokens   []TokenInfo `json:"tokens,omitempty"`
}

type CreatedUser struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	IsAdmin    bool   `json:"is_admin"`
	Created    string `json:"created"`
	Passphrase string `json:"passphrase"`
	Note       string `json:"note"`
}

type ResetPassword struct {
	Username   string `json:"username"`
	Passphrase string `json:"passphrase"`
	Note       string `json:"note"`
}

const passphraseNote = "shown once; send the username and passphrase to the user, who logs in on the kena site"

type UserList struct {
	Users []UserInfo `json:"users"`
}

type IssuedToken struct {
	Token    string `json:"token"`
	TokenID  int64  `json:"token_id"`
	Username string `json:"username"`
	Note     string `json:"note"`
}

type RevokeResult struct {
	TokenID int64 `json:"token_id"`
	Revoked bool  `json:"revoked"`
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func optStamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return stamp(*t)
}

func userInfo(u store.User) UserInfo {
	return UserInfo{ID: u.ID, Username: u.Username, IsAdmin: u.IsAdmin, Created: stamp(u.CreatedAt)}
}

func (s *server) setNewPassphrase(ctx context.Context, userID int64) (string, error) {
	pass, err := passphrase.Generate()
	if err != nil {
		return "", err
	}
	return pass, s.st.SetPassword(ctx, userID, pass, "")
}

func (s *server) userByName(ctx context.Context, name string) (store.User, error) {
	u, err := s.st.UserByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return store.User{}, fail("user %q not found; see admin_list_users", name)
	}
	return u, err
}

func guard(err error) error {
	if errors.Is(err, store.ErrForbidden) {
		return fail("%s", strings.TrimSuffix(err.Error(), ": "+store.ErrForbidden.Error()))
	}
	return err
}

func adminOnly[In, Out any](fn func(context.Context, store.User, In) (Out, error)) mcp.ToolHandlerFor[In, Out] {
	return handle(func(ctx context.Context, u store.User, in In) (Out, error) {
		if !u.IsAdmin {
			var zero Out
			return zero, fail("admin only")
		}
		return fn(ctx, u, in)
	})
}

func (s *server) addAdminTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "admin_create_user",
		Description: "Admin only. Create a user account with a generated passphrase. The passphrase appears only in this result; send the username and passphrase to the user. They log in on the kena website, connect the Claude app or ChatGPT by logging in on the kena page those apps open, and create personal tokens for Claude Code and other agents on their Account page.",
		Annotations: additive("Create user"),
	}, adminOnly(func(ctx context.Context, _ store.User, in CreateUserIn) (CreatedUser, error) {
		u, err := s.st.CreateUser(ctx, in.Username, in.IsAdmin)
		switch {
		case errors.Is(err, store.ErrConflict):
			return CreatedUser{}, fail("user %q already exists; use admin_reset_password for a new passphrase", in.Username)
		case errors.Is(err, store.ErrInvalid):
			return CreatedUser{}, fail("invalid username %q: use 2-32 lowercase latin letters, digits, _ or -, starting with a letter or digit", in.Username)
		case err != nil:
			return CreatedUser{}, err
		}
		pass, err := s.setNewPassphrase(ctx, u.ID)
		if err != nil {
			return CreatedUser{}, err
		}
		return CreatedUser{ID: u.ID, Username: u.Username, IsAdmin: u.IsAdmin, Created: stamp(u.CreatedAt), Passphrase: pass, Note: passphraseNote}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "admin_reset_password",
		Description: "Admin only. Give a user a new generated passphrase, e.g. when they forgot theirs. Logs the user out everywhere and disconnects their OAuth apps (Claude app, ChatGPT); personal tokens keep working. The passphrase appears only in this result.",
		Annotations: destructive("Reset passphrase", false),
	}, adminOnly(func(ctx context.Context, _ store.User, in UserNameIn) (ResetPassword, error) {
		u, err := s.userByName(ctx, in.Username)
		if err != nil {
			return ResetPassword{}, err
		}
		pass, err := s.setNewPassphrase(ctx, u.ID)
		if err != nil {
			return ResetPassword{}, err
		}
		return ResetPassword{Username: u.Username, Passphrase: pass, Note: passphraseNote}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "admin_issue_token",
		Description: "Admin only. Issue a personal bearer token for a user (for MCP clients without OAuth, e.g. Claude Code, Hermes). Users can also create their own on their Account page. The secret appears only in this result and cannot be retrieved later; pass it to the user right away.",
		Annotations: additive("Issue token"),
	}, adminOnly(func(ctx context.Context, _ store.User, in IssueTokenIn) (IssuedToken, error) {
		u, err := s.st.UserByName(ctx, in.Username)
		if errors.Is(err, store.ErrNotFound) {
			return IssuedToken{}, fail("user %q not found; create it with admin_create_user", in.Username)
		}
		if err != nil {
			return IssuedToken{}, err
		}
		secret, tok, err := s.st.IssueToken(ctx, u.ID, in.Label)
		if err != nil {
			return IssuedToken{}, err
		}
		return IssuedToken{Token: secret, TokenID: tok.ID, Username: u.Username, Note: "shown once; store it now"}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "admin_revoke_token",
		Description: "Admin only. Revoke a personal token by id; clients using it get 401 from then on. Also disconnects OAuth connectors (claude.ai, ChatGPT) authorised with it.",
		Annotations: destructive("Revoke token", true),
	}, adminOnly(func(ctx context.Context, _ store.User, in RevokeTokenIn) (RevokeResult, error) {
		err := s.st.RevokeToken(ctx, in.TokenID)
		if errors.Is(err, store.ErrNotFound) {
			return RevokeResult{}, fail("token %d not found; see admin_list_users", in.TokenID)
		}
		if err != nil {
			return RevokeResult{}, err
		}
		return RevokeResult{TokenID: in.TokenID, Revoked: true}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "admin_list_users",
		Description: "Admin only. List users with their personal tokens: id, label, created, last used and revoked times (token secrets are never shown).",
		Annotations: readOnly("List users"),
	}, adminOnly(func(ctx context.Context, _ store.User, _ ListUsersIn) (UserList, error) {
		users, err := s.st.ListUsers(ctx)
		if err != nil {
			return UserList{}, err
		}
		out := UserList{Users: []UserInfo{}}
		for _, u := range users {
			tokens, err := s.st.ListTokens(ctx, u.ID)
			if err != nil {
				return UserList{}, err
			}
			info := userInfo(u)
			for _, t := range tokens {
				info.Tokens = append(info.Tokens, TokenInfo{ID: t.ID, Label: t.Label, Created: stamp(t.CreatedAt),
					LastUsed: optStamp(t.LastUsedAt), Revoked: optStamp(t.RevokedAt)})
			}
			out.Users = append(out.Users, info)
		}
		return out, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "admin_set_admin",
		Description: "Admin only. Grant or remove admin rights. You cannot change your own, and the last admin keeps theirs.",
		Annotations: destructive("Set admin", true),
	}, adminOnly(func(ctx context.Context, me store.User, in SetAdminIn) (UserInfo, error) {
		u, err := s.userByName(ctx, in.Username)
		if err != nil {
			return UserInfo{}, err
		}
		if err := s.st.SetAdmin(ctx, me.ID, u.ID, in.IsAdmin); err != nil {
			return UserInfo{}, guard(err)
		}
		u.IsAdmin = in.IsAdmin
		return userInfo(u), nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "admin_logout_user",
		Description: "Admin only. Log a user out on every device: ends their web sessions and disconnects their OAuth apps (Claude app, ChatGPT). Personal tokens keep working; revoke them with admin_revoke_token.",
		Annotations: destructive("Log out user", true),
	}, adminOnly(func(ctx context.Context, _ store.User, in UserNameIn) (LogoutResult, error) {
		u, err := s.userByName(ctx, in.Username)
		if err != nil {
			return LogoutResult{}, err
		}
		if err := s.st.LogoutEverywhere(ctx, u.ID); err != nil {
			return LogoutResult{}, err
		}
		return LogoutResult{Username: u.Username, Note: "sessions ended and apps disconnected; personal tokens still work"}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "admin_delete_user",
		Description: "Admin only. Delete a user with all their decks, tokens and progress. Cannot be undone: confirm with the person first and pass the username twice. You cannot delete yourself or the last admin.",
		Annotations: destructive("Delete user", false),
	}, adminOnly(func(ctx context.Context, me store.User, in DeleteUserIn) (DeleteResult, error) {
		if in.ConfirmUsername != in.Username {
			return DeleteResult{}, fail("confirm_username %q does not match username %q; nothing was deleted", in.ConfirmUsername, in.Username)
		}
		u, err := s.userByName(ctx, in.Username)
		if err != nil {
			return DeleteResult{}, err
		}
		if err := s.st.DeleteUser(ctx, me.ID, u.ID); err != nil {
			return DeleteResult{}, guard(err)
		}
		return DeleteResult{Deleted: "user " + u.Username}, nil
	}))
}
