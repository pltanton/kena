package web

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/markdown"
	"github.com/pltanton/kena/internal/store"
)

const (
	rulePreviewLen  = 90
	maxRulesBody    = 64 << 10
	snippetLen      = 40
	ruleGoneMessage = "rules.gone"
)

type ruleView struct {
	Key     string
	Title   string
	Label   string
	Preview string
	General bool
	HTML    template.HTML
}

func newRuleView(r greek.Rule) *ruleView {
	v := &ruleView{Key: r.Key, Title: r.Title, Label: r.Label(), General: r.General, HTML: markdown.Render(r.Body)}
	if r.Title != "" {
		v.Preview = markdown.Preview(r.Body, rulePreviewLen)
	}
	return v
}

func ruleFor(set store.Set, key string) *ruleView {
	if key == "" {
		return nil
	}
	for _, r := range set.Rules {
		if r.Key == key {
			return newRuleView(r)
		}
	}
	return nil
}

func hasGeneral(set store.Set) bool {
	return slices.ContainsFunc(set.Rules, func(r greek.Rule) bool { return r.General })
}

func ruleViews(set store.Set, general bool) []*ruleView {
	var out []*ruleView
	for _, r := range set.Rules {
		if r.General == general {
			out = append(out, newRuleView(r))
		}
	}
	return out
}

type cardInfo struct {
	Rule *ruleView
}

func infoFor(set store.Set, t store.Task) *cardInfo {
	rule := ruleFor(set, t.Rule)
	if rule == nil && !hasGeneral(set) {
		return nil
	}
	return &cardInfo{Rule: rule}
}

type ruleForm struct {
	Slug     string
	Version  string
	Orig     string
	Label    string
	Key      string
	Title    string
	Body     string
	General  bool
	Uses     int
	Errors   []string
	Warnings []string
	Preview  template.HTML
	Saved    bool
}

type rulesPage struct {
	Set     store.Set
	Version string
	Rules   []ruleForm
	New     ruleForm
	Errors  []string
}

func rulesView(set store.Set, tasks []store.Task, version string) rulesPage {
	uses := map[string]int{}
	for _, t := range tasks {
		uses[t.Rule]++
	}
	v := rulesPage{Set: set, Version: version, New: ruleForm{Slug: set.Slug, Version: version}}
	for _, r := range set.Rules {
		v.Rules = append(v.Rules, ruleForm{Slug: set.Slug, Version: version, Orig: r.Key, Label: r.Label(), Key: r.Key, Title: r.Title, Body: r.Body, General: r.General,
			Uses: uses[r.Key], Warnings: markdown.Issues(r.Body)})
	}
	return v
}

func rulesURL(slug string) string {
	return deckPath(slug) + "/rules"
}

func (h *handler) rulesPage(w http.ResponseWriter, r *http.Request, u store.User) {
	set, err := h.ownedSet(r, u)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	tasks, err := h.st.ListTasks(r.Context(), set.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	v := rulesView(set, tasks, versionOf(set))
	if saved := r.URL.Query().Get("saved"); saved != "" {
		for i := range v.Rules {
			v.Rules[i].Saved = v.Rules[i].Key == saved
		}
	}
	h.page(w, r, http.StatusOK, "rules", v)
}

var ruleFieldLabels = map[string]string{"key": "rules.field_key", "title": "rules.field_title", "body": "rules.field_body"}

func ruleIssues(r *http.Request, issues []greek.Issue, i int) (errs, warnings []string) {
	prefix := fmt.Sprintf("rules[%d].", i)
	for _, iss := range issues {
		field, ok := strings.CutPrefix(iss.Field, prefix)
		if !ok && iss.Field != "rules" {
			continue
		}
		msg := iss.Message
		if label := ruleFieldLabels[field]; label != "" {
			msg = tr(r, label) + ": " + msg
		}
		if iss.Severity == greek.Error {
			errs = append(errs, msg)
		} else {
			warnings = append(warnings, msg)
		}
	}
	return errs, warnings
}

func inUseMessage(r *http.Request, e *store.RulesInUseError, tasks []store.Task) string {
	byID := make(map[int64]store.Task, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}
	var parts []string
	for _, use := range e.Uses {
		for _, ref := range use.Tasks {
			s := tr(r, "rules.task_n", ref.Position)
			if t, ok := byID[ref.ID]; ok {
				s += " «" + snippet(t.Text) + "»"
			}
			parts = append(parts, s)
		}
	}
	return tr(r, "rules.in_use", strings.Join(parts, ", "))
}

func snippet(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	r := []rune(text)
	if len(r) <= snippetLen {
		return text
	}
	return strings.TrimSpace(string(r[:snippetLen])) + "…"
}

func (h *handler) saveRules(w http.ResponseWriter, r *http.Request, u store.User) {
	ctx := r.Context()
	set, err := h.ownedSet(r, u)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRulesBody)
	if !h.parseForm(w, r) {
		return
	}
	f := r.PostForm
	version := f.Get("version")
	at, err := parseVersion(version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	tasks, err := h.st.ListTasks(ctx, set.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	posted := ruleForm{Orig: f.Get("orig"), Key: f.Get("key"), Title: f.Get("title"), Body: f.Get("body"), General: f.Get("general") == "1"}
	idx := -1
	if posted.Orig != "" {
		idx = slices.IndexFunc(set.Rules, func(x greek.Rule) bool { return x.Key == posted.Orig })
	}
	render := func(status int, form ruleForm, pageErrs []string) {
		v := rulesView(set, tasks, version)
		v.Errors = pageErrs
		form.Slug, form.Version = set.Slug, version
		if idx >= 0 {
			form.Uses, form.Label = v.Rules[idx].Uses, v.Rules[idx].Label
			v.Rules[idx] = form
		} else {
			v.New = form
		}
		h.page(w, r, status, "rules", v)
	}
	if posted.Orig != "" && idx < 0 {
		render(http.StatusConflict, posted, []string{tr(r, ruleGoneMessage)})
		return
	}

	save := func(rules []greek.Rule, renames map[string]string, loc string) {
		_, err := h.st.ReplaceRules(ctx, set.ID, at, rules, renames)
		var inUse *store.RulesInUseError
		switch {
		case errors.As(err, &inUse):
			posted.Errors = []string{inUseMessage(r, inUse, tasks)}
			render(http.StatusUnprocessableEntity, posted, nil)
		case errors.Is(err, store.ErrConflict):
			render(http.StatusConflict, posted, []string{tr(r, conflictMessage)})
		case err != nil:
			h.fail(w, r, err)
		default:
			http.Redirect(w, r, loc, http.StatusSeeOther)
		}
	}

	op := f.Get("op")
	if op == "delete" {
		if idx < 0 {
			h.fail(w, r, fmt.Errorf("delete without a rule: %w", store.ErrInvalid))
			return
		}
		save(slices.Delete(slices.Clone(set.Rules), idx, idx+1), nil, rulesURL(set.Slug))
		return
	}
	if op != "save" && op != "preview" {
		h.fail(w, r, invalid("op", op))
		return
	}

	rules := slices.Clone(set.Rules)
	i := idx
	rule := greek.Rule{Key: posted.Key, Title: posted.Title, Body: posted.Body, General: posted.General}
	if i < 0 {
		i = len(rules)
		rules = append(rules, rule)
	} else {
		rules[i] = rule
	}
	prepared, issues := greek.PrepareRules(rules)
	posted.Errors, posted.Warnings = ruleIssues(r, issues, i)
	if len(posted.Errors) > 0 {
		render(http.StatusUnprocessableEntity, posted, nil)
		return
	}
	if op == "preview" {
		posted.Preview = markdown.Render(prepared[i].Body)
		render(http.StatusOK, posted, nil)
		return
	}
	var renames map[string]string
	if idx >= 0 && posted.Orig != prepared[i].Key {
		renames = map[string]string{posted.Orig: prepared[i].Key}
	}
	key := prepared[i].Key
	save(prepared, renames, rulesURL(set.Slug)+"?"+url.Values{"saved": {key}}.Encode()+"#rule-"+key)
}
