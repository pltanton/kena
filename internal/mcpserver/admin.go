package mcpserver

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pltanton/kena/internal/store"
)

type CreateUserIn struct {
	Username string `json:"username" jsonschema:"2-32 chars: lowercase latin letters, digits, _ or -, starting with a letter or digit"`
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
		Description: "Admin only. Create a user account. Then give them access with admin_issue_token.",
		Annotations: additive("Create user"),
	}, adminOnly(func(ctx context.Context, _ store.User, in CreateUserIn) (UserInfo, error) {
		u, err := s.st.CreateUser(ctx, in.Username, in.IsAdmin)
		switch {
		case errors.Is(err, store.ErrConflict):
			return UserInfo{}, fail("user %q already exists", in.Username)
		case errors.Is(err, store.ErrInvalid):
			return UserInfo{}, fail("invalid username %q: use 2-32 lowercase latin letters, digits, _ or -, starting with a letter or digit", in.Username)
		case err != nil:
			return UserInfo{}, err
		}
		return userInfo(u), nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "admin_issue_token",
		Description: "Admin only. Issue a personal bearer token for a user (for MCP clients without OAuth). The secret appears only in this result and cannot be retrieved later; pass it to the user right away.",
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
		Description: "Admin only. Revoke a personal token by id; clients using it get 401 from then on.",
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
}
