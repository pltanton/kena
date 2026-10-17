package mcpserver

import (
	"context"
	"errors"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

const ruleGuidance = "Rules are short Markdown (a few lines, **bold**, lists; a table for a paradigm, e.g. πόσος/πόση/πόσο by case and number; no HTML, no images). " +
	"general=true: deck-wide theory every card's info sheet shows (the paradigm, the agreement rule); at most one or two per deck. " +
	"general=false: the point a group of tasks practises; tasks name it with rule = key and learners see it with those tasks, also after a wrong answer."

type SetRuleIn struct {
	Set     string  `json:"set" jsonschema:"set id or slug"`
	Key     string  `json:"key" jsonschema:"rule key: a-z, 0-9 and -, at most 32 characters; an existing key is updated, a new one is added"`
	Title   *string `json:"title,omitempty" jsonschema:"short heading, at most 80 characters; omit to keep the current title"`
	Body    string  `json:"body" jsonschema:"the rule in Markdown, at most 4000 characters"`
	General *bool   `json:"general,omitempty" jsonschema:"true = deck-wide theory; omit to keep the current value (false for a new rule)"`
}

type RuleKeyIn struct {
	Set string `json:"set" jsonschema:"set id or slug"`
	Key string `json:"key" jsonschema:"rule key"`
}

type RuleResult struct {
	SetID    int64    `json:"set_id"`
	URL      string   `json:"url"`
	Rule     RuleOut  `json:"rule"`
	Rules    []string `json:"rules"`
	Warnings []string `json:"warnings,omitempty"`
}

func ruleKeys(rules []greek.Rule) []string {
	keys := make([]string, len(rules))
	for i, r := range rules {
		keys[i] = r.Key
	}
	return keys
}

func (s *server) replaceRules(ctx context.Context, st store.Set, rules []greek.Rule) (store.Set, error) {
	updated, err := s.st.ReplaceRules(ctx, st.ID, st.UpdatedAt, rules, nil)
	var inUse *store.RulesInUseError
	switch {
	case errors.As(err, &inUse):
		return store.Set{}, fail("%v; point those tasks at another rule or clear it first (update_task with rule), then retry", inUse)
	case errors.Is(err, store.ErrConflict):
		return store.Set{}, fail("set %q changed while saving; call the tool again", st.Slug)
	}
	return updated, err
}

func (s *server) addRuleTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "set_rule",
		Description: "Add or update one deck rule of a set you own (admins: any set): set, key, body, optional title and general. " +
			"An existing key keeps its place and, when title or general is omitted, their values. " +
			"Returns the saved rule, the deck's rule keys in order and warnings (e.g. HTML that is not shown). " + ruleGuidance,
		Annotations: destructive("Set rule", true),
	}, handle(func(ctx context.Context, u store.User, in SetRuleIn) (RuleResult, error) {
		st, err := s.editableSet(ctx, u, in.Set)
		if err != nil {
			return RuleResult{}, err
		}
		rules := slices.Clone(st.Rules)
		r := greek.Rule{Key: in.Key, Body: in.Body}
		i := slices.IndexFunc(rules, func(x greek.Rule) bool { return x.Key == in.Key })
		if i >= 0 {
			r.Title, r.General = rules[i].Title, rules[i].General
		} else {
			i = len(rules)
			rules = append(rules, r)
		}
		if in.Title != nil {
			r.Title = *in.Title
		}
		if in.General != nil {
			r.General = *in.General
		}
		rules[i] = r
		prepared, rep := prepareGreekRules(rules)
		if len(rep.Errors) > 0 {
			return RuleResult{}, rep.failure()
		}
		updated, err := s.replaceRules(ctx, st, prepared)
		if err != nil {
			return RuleResult{}, err
		}
		return RuleResult{SetID: st.ID, URL: s.url(st.Slug), Rule: ruleOut(prepared[i]), Rules: ruleKeys(updated.Rules), Warnings: rep.Warnings}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_rule",
		Description: "Delete one deck rule of a set you own (admins: any set). Refused while tasks use it: the error lists their ids; move them to another rule or clear it with update_task first.",
		Annotations: destructive("Delete rule", true),
	}, handle(func(ctx context.Context, u store.User, in RuleKeyIn) (DeleteResult, error) {
		st, err := s.editableSet(ctx, u, in.Set)
		if err != nil {
			return DeleteResult{}, err
		}
		i := slices.IndexFunc(st.Rules, func(x greek.Rule) bool { return x.Key == in.Key })
		if i < 0 {
			return DeleteResult{}, fail("rule %q not found in set %q; get_set lists the rules", in.Key, st.Slug)
		}
		if _, err := s.replaceRules(ctx, st, slices.Delete(slices.Clone(st.Rules), i, i+1)); err != nil {
			return DeleteResult{}, err
		}
		return DeleteResult{Deleted: "rule " + in.Key}, nil
	}))
}
