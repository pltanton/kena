package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pltanton/kena/internal/guide"
	"github.com/pltanton/kena/internal/oauth"
	"github.com/pltanton/kena/internal/store"
)

const maxTasksPerCall = 50

type server struct {
	st        *store.Store
	publicURL string
}

func New(st *store.Store, publicURL string) *mcp.Server {
	brand := publicURL + "/static/brand/"
	srv := mcp.NewServer(&mcp.Implementation{Name: "kena", Title: "kena", Version: "0.1.0", WebsiteURL: publicURL,
		Icons: []mcp.Icon{
			{Source: brand + "icon-128.png", MIMEType: "image/png", Sizes: []string{"128x128"}},
			{Source: brand + "icon-512.png", MIMEType: "image/png", Sizes: []string{"512x512"}},
			{Source: brand + "icon.svg", MIMEType: "image/svg+xml", Sizes: []string{"any"}},
		}},
		&mcp.ServerOptions{Instructions: guide.Instructions})
	s := &server{st: st, publicURL: publicURL}
	s.addGuide(srv)
	s.addSetTools(srv)
	s.addRuleTools(srv)
	s.addPublicRuleTools(srv)
	s.addMistakeTools(srv)
	s.addAdminTools(srv)
	return srv
}

func Handler(srv *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			// Behind a reverse proxy the peer is 127.0.0.1 with a public Host, which the SDK rejects with 403; bearer auth guards access.
			DisableLocalhostProtection: true,
		})
}

type toolError struct{ msg string }

func (e *toolError) Error() string { return e.msg }

func fail(format string, args ...any) error {
	return &toolError{fmt.Sprintf(format, args...)}
}

// Tool errors go back as Go errors: the SDK then omits structuredContent, which it would otherwise fill with a zero Out.
func handle[In, Out any](fn func(context.Context, store.User, In) (Out, error)) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		var zero Out
		var user store.User
		authed := false
		if req.Extra != nil {
			user, authed = oauth.UserFrom(req.Extra.TokenInfo)
		}
		if !authed {
			return nil, zero, fail("unauthenticated")
		}
		out, err := fn(ctx, user, in)
		if err == nil {
			return nil, out, nil
		}
		var te *toolError
		if errors.As(err, &te) {
			return nil, zero, te
		}
		slog.ErrorContext(ctx, "mcp tool failed", "tool", req.Params.Name, "user", user.Username, "err", err)
		return nil, zero, &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "internal error"}
	}
}

func ptr[T any](v T) *T { return &v }

var closedWorld = ptr(false)

func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: closedWorld}
}

func additive(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: ptr(false), OpenWorldHint: closedWorld}
}

func destructive(title string, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: ptr(true), IdempotentHint: idempotent, OpenWorldHint: closedWorld}
}

type GuideIn struct {
	Topic string `json:"topic,omitempty" jsonschema:"guide topic; omit for the index plus the workflow"`
}

type GuideResult struct {
	Topic   string `json:"topic"`
	Content string `json:"content"`
}

func (s *server) addGuide(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "get_authoring_guide",
		Description: "Read the kena authoring guide. Call it with no topic before writing any tasks: " +
			"you get the topic index and the step-by-step workflow (photo of a book exercise -> spec -> new sentences -> self-check -> validate_tasks -> create_set/add_tasks). " +
			"Topics: " + topicList() + ".",
		Annotations: readOnly("Authoring guide"),
	}, handle(func(_ context.Context, _ store.User, in GuideIn) (GuideResult, error) {
		content, err := guide.Get(in.Topic)
		if err != nil {
			return GuideResult{}, fail("%v", err)
		}
		topic := in.Topic
		if topic == "" {
			topic = "index"
		}
		return GuideResult{Topic: topic, Content: content}, nil
	}))

	for _, t := range guide.Topics() {
		srv.AddResource(&mcp.Resource{
			URI:         "skill://kena/" + t.Name,
			Name:        t.Name,
			Title:       t.Title,
			MIMEType:    "text/markdown",
			Description: t.Summary,
		}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			content, err := guide.Get(t.Name)
			if err != nil {
				return nil, err
			}
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
				URI: req.Params.URI, MIMEType: "text/markdown", Text: content,
			}}}, nil
		})
	}
}

func topicList() string {
	var names []string
	for _, t := range guide.Topics() {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}
