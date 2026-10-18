package mcpserver

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/markdown"
	"github.com/pltanton/kena/internal/store"
)

const publicRulePreviewLen = 120

type PublicRuleRef struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Attached bool   `json:"attached"`
}

type ListPublicRulesIn struct {
	Category string `json:"category,omitempty" jsonschema:"only this category; omit for all"`
}

type PublicRuleSummary struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Preview string `json:"preview"`
	Creator string `json:"creator"`
}

type PublicCategory struct {
	Name  string              `json:"name"`
	Rules []PublicRuleSummary `json:"rules"`
}

type PublicRuleList struct {
	Categories []PublicCategory `json:"categories"`
}

type PublicRuleIDIn struct {
	ID string `json:"id" jsonschema:"public rule id: category/name, e.g. ellenika/posos"`
}

type PublicRuleOut struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Creator   string `json:"creator"`
	UsedTasks int    `json:"used_by_tasks"`
	UsedDecks int    `json:"used_by_decks"`
	CanEdit   bool   `json:"can_edit"`
	URL       string `json:"url"`
}

type SetPublicRuleIn struct {
	ID    string  `json:"id" jsonschema:"category/name; the category must exist (admins create categories), name: a-z, 0-9 and -, at most 32 characters"`
	Title *string `json:"title,omitempty" jsonschema:"short heading, at most 80 characters; omit to keep the current title"`
	Body  string  `json:"body" jsonschema:"the rule in Markdown, at most 4000 characters"`
}

type PublicRuleResult struct {
	Rule     PublicRuleOut `json:"rule"`
	Created  bool          `json:"created"`
	Warnings []string      `json:"warnings,omitempty"`
}

type CategoryIn struct {
	Name string `json:"name" jsonschema:"category name: a-z, 0-9 and -, at most 32 characters, e.g. ellenika"`
}

type AttachIn struct {
	Set string `json:"set" jsonschema:"set id or slug"`
	ID  string `json:"id" jsonschema:"public rule id: category/name"`
}

type AttachResult struct {
	SetID       int64           `json:"set_id"`
	URL         string          `json:"url"`
	PublicRules []PublicRuleRef `json:"public_rules"`
}

type TransferIn struct {
	From string `json:"from" jsonschema:"username whose public rules move"`
	To   string `json:"to" jsonschema:"username who becomes their creator"`
}

type TransferResult struct {
	Moved int64 `json:"moved"`
}

func (s *server) ruleURL(key string) string { return s.publicURL + "/rules/" + key }

func publicErr(err error) error {
	var inUse *store.PublicRuleInUseError
	switch {
	case errors.As(err, &inUse):
		return fail("%v; point those tasks at another rule (update_task) and detach it from the decks first", inUse)
	case errors.Is(err, store.ErrNotFound):
		return fail("not found; list_public_rules shows the ids")
	case errors.Is(err, store.ErrInvalid), errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrForbidden):
		return fail("%v", err)
	}
	return err
}

func (s *server) publicRuleOut(ctx context.Context, u store.User, p store.PublicRule) (PublicRuleOut, error) {
	use, err := s.st.PublicRuleUse(ctx, p.Key())
	if err != nil {
		return PublicRuleOut{}, err
	}
	return PublicRuleOut{ID: p.Key(), Title: p.Title, Body: p.Body, Creator: p.OwnerName,
		UsedTasks: use.Tasks, UsedDecks: use.Decks, CanEdit: store.CanEditPublicRule(u, p), URL: s.ruleURL(p.Key())}, nil
}

func publicRefs(st store.Set) []PublicRuleRef {
	out := []PublicRuleRef{}
	for _, r := range st.Public {
		out = append(out, PublicRuleRef{ID: r.Key, Title: r.Title, Body: r.Body, Attached: r.General})
	}
	return out
}

func (s *server) addPublicRuleTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_public_rules",
		Description: "List public rules: grammar rules shared by every deck, grouped by category, each with id (category/name), title, preview and creator. " +
			"Check here before writing a deck rule: a task uses a public rule with rule = id, and attach_public_rule shows one on every card of a deck.",
		Annotations: readOnly("List public rules"),
	}, handle(func(ctx context.Context, u store.User, in ListPublicRulesIn) (PublicRuleList, error) {
		cats, err := s.st.ListRuleCategories(ctx)
		if err != nil {
			return PublicRuleList{}, err
		}
		rules, err := s.st.ListPublicRules(ctx, in.Category)
		if err != nil {
			return PublicRuleList{}, err
		}
		out := PublicRuleList{Categories: []PublicCategory{}}
		idx := map[string]int{}
		for _, c := range cats {
			if in.Category != "" && c.Name != in.Category {
				continue
			}
			idx[c.Name] = len(out.Categories)
			out.Categories = append(out.Categories, PublicCategory{Name: c.Name, Rules: []PublicRuleSummary{}})
		}
		for _, p := range rules {
			c := &out.Categories[idx[p.Category]]
			c.Rules = append(c.Rules, PublicRuleSummary{ID: p.Key(), Title: p.Title, Preview: markdown.Preview(p.Body, publicRulePreviewLen), Creator: p.OwnerName})
		}
		return out, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_public_rule",
		Description: "Get one public rule by id (category/name): title, Markdown body, creator, how many tasks and decks use it, and whether you can edit it.",
		Annotations: readOnly("Get public rule"),
	}, handle(func(ctx context.Context, u store.User, in PublicRuleIDIn) (PublicRuleOut, error) {
		p, err := s.st.GetPublicRule(ctx, in.ID)
		if err != nil {
			return PublicRuleOut{}, publicErr(err)
		}
		return s.publicRuleOut(ctx, u, p)
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "set_public_rule",
		Description: "Create or update a public rule (id = category/name). Anyone can create one in an existing category; only its creator or an admin can change it later. " +
			"Create one only for grammar that applies beyond a single deck; deck-specific notes stay deck rules (set_rule). " + ruleGuidance,
		Annotations: destructive("Set public rule", true),
	}, handle(func(ctx context.Context, u store.User, in SetPublicRuleIn) (PublicRuleResult, error) {
		if _, _, err := store.SplitPublicRuleKey(in.ID); err != nil {
			return PublicRuleResult{}, publicErr(err)
		}
		title := ""
		if old, err := s.st.GetPublicRule(ctx, in.ID); err == nil {
			title = old.Title
		} else if !errors.Is(err, store.ErrNotFound) {
			return PublicRuleResult{}, err
		}
		if in.Title != nil {
			title = *in.Title
		}
		prepared, rep := prepareRules([]greek.Rule{{Key: "rule", Title: title, Body: in.Body}})
		if len(rep.Errors) > 0 {
			return PublicRuleResult{}, rep.failure()
		}
		p, created, err := s.st.SavePublicRule(ctx, u, in.ID, time.Time{}, store.PublicRuleInput{Title: prepared[0].Title, Body: prepared[0].Body})
		if err != nil {
			return PublicRuleResult{}, publicErr(err)
		}
		out, err := s.publicRuleOut(ctx, u, p)
		if err != nil {
			return PublicRuleResult{}, err
		}
		return PublicRuleResult{Rule: out, Created: created, Warnings: rep.Warnings}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_public_rule",
		Description: "Delete a public rule you created (admins: any). Refused while any task or deck uses it. Cannot be undone.",
		Annotations: destructive("Delete public rule", true),
	}, handle(func(ctx context.Context, u store.User, in PublicRuleIDIn) (DeleteResult, error) {
		if err := s.st.DeletePublicRule(ctx, u, in.ID); err != nil {
			return DeleteResult{}, publicErr(err)
		}
		return DeleteResult{Deleted: "public rule " + in.ID}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_rule_category",
		Description: "Admin only. Create a public rule category (a-z, 0-9 and -), e.g. ellenika.",
		Annotations: additive("Create rule category"),
	}, handle(func(ctx context.Context, u store.User, in CategoryIn) (PublicCategory, error) {
		c, err := s.st.CreateRuleCategory(ctx, u, in.Name)
		if err != nil {
			return PublicCategory{}, publicErr(err)
		}
		return PublicCategory{Name: c.Name, Rules: []PublicRuleSummary{}}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "attach_public_rule",
		Description: "Attach a public rule to a set you own (admins: any set) as deck-wide theory: it shows on every card's info sheet. " +
			"At most 10 per deck. To use a public rule for some tasks only, set their rule = id instead.",
		Annotations: additive("Attach public rule"),
	}, handle(func(ctx context.Context, u store.User, in AttachIn) (AttachResult, error) {
		st, err := s.editableSet(ctx, u, in.Set)
		if err != nil {
			return AttachResult{}, err
		}
		updated, err := s.st.AttachPublicRule(ctx, st.ID, in.ID)
		if err != nil {
			return AttachResult{}, publicErr(err)
		}
		return AttachResult{SetID: st.ID, URL: s.url(st.Slug), PublicRules: publicRefs(updated)}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "detach_public_rule",
		Description: "Stop showing an attached public rule on every card of a set you own (admins: any set). Tasks that use it with rule = id keep it.",
		Annotations: destructive("Detach public rule", true),
	}, handle(func(ctx context.Context, u store.User, in AttachIn) (AttachResult, error) {
		st, err := s.editableSet(ctx, u, in.Set)
		if err != nil {
			return AttachResult{}, err
		}
		updated, err := s.st.DetachPublicRule(ctx, st.ID, in.ID)
		if err != nil {
			return AttachResult{}, publicErr(err)
		}
		return AttachResult{SetID: st.ID, URL: s.url(st.Slug), PublicRules: publicRefs(updated)}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "admin_transfer_public_rules",
		Description: "Admin only. Make another user the creator of all public rules a user created, e.g. before deleting that user.",
		Annotations: destructive("Transfer public rules", true),
	}, adminOnly(func(ctx context.Context, me store.User, in TransferIn) (TransferResult, error) {
		from, err := s.userByName(ctx, in.From)
		if err != nil {
			return TransferResult{}, err
		}
		if _, err := s.userByName(ctx, in.To); err != nil {
			return TransferResult{}, err
		}
		n, err := s.st.TransferPublicRules(ctx, me, from.ID, in.To)
		if err != nil {
			return TransferResult{}, publicErr(err)
		}
		return TransferResult{Moved: n}, nil
	}))
}
