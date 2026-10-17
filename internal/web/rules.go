package web

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/markdown"
	"github.com/pltanton/kena/internal/store"
)

const (
	rulePreviewLen  = 90
	maxRulesBody    = 64 << 10
	snippetLen      = 40
	ruleGoneMessage = "This rule was changed or deleted since you opened the page — reload and try again."
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

func generalRules(set store.Set) []*ruleView {
	var out []*ruleView
	for _, r := range set.Rules {
		if r.General {
			out = append(out, newRuleView(r))
		}
	}
	return out
}

func overviewRules(set store.Set) []*ruleView {
	out := generalRules(set)
	for _, r := range set.Rules {
		if !r.General {
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

func (h *handler) rulesView(set store.Set, tasks []store.Task, version string) rulesPage {
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
	return "/s/" + url.PathEscape(slug) + "/rules"
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
	v := h.rulesView(set, tasks, versionOf(set))
	if saved := r.URL.Query().Get("saved"); saved != "" {
		for i := range v.Rules {
			v.Rules[i].Saved = v.Rules[i].Key == saved
		}
	}
	h.page(w, r, http.StatusOK, "rules", v)
}

var ruleFieldLabels = map[string]string{"key": "Key", "title": "Title", "body": "Body"}

func ruleIssues(issues []greek.Issue, i int) (errs, warnings []string) {
	prefix := fmt.Sprintf("rules[%d].", i)
	for _, iss := range issues {
		field, ok := strings.CutPrefix(iss.Field, prefix)
		if !ok && iss.Field != "rules" {
			continue
		}
		msg := iss.Message
		if label := ruleFieldLabels[field]; label != "" {
			msg = label + ": " + msg
		}
		if iss.Severity == greek.Error {
			errs = append(errs, msg)
		} else {
			warnings = append(warnings, msg)
		}
	}
	return errs, warnings
}

func inUseMessage(e *store.RulesInUseError, tasks []store.Task) string {
	byID := make(map[int64]store.Task, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}
	var parts []string
	for _, use := range e.Uses {
		for _, ref := range use.Tasks {
			s := fmt.Sprintf("task %d", ref.Position)
			if t, ok := byID[ref.ID]; ok {
				s += " «" + snippet(t.Text) + "»"
			}
			parts = append(parts, s)
		}
	}
	return fmt.Sprintf("Still used by %s. Point those tasks at another rule (or remove their @key) in the editor first.", strings.Join(parts, ", "))
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
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	f := r.PostForm
	version := f.Get("version")
	nanos, err := strconv.ParseInt(version, 10, 64)
	if err != nil {
		h.fail(w, r, fmt.Errorf("version %q: %w", version, store.ErrInvalid))
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
		v := h.rulesView(set, tasks, version)
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
		render(http.StatusConflict, posted, []string{ruleGoneMessage})
		return
	}

	op := f.Get("op")
	if op == "delete" {
		if idx < 0 {
			h.fail(w, r, fmt.Errorf("delete without a rule: %w", store.ErrInvalid))
			return
		}
		_, err := h.st.ReplaceRules(ctx, set.ID, time.Unix(0, nanos), slices.Delete(slices.Clone(set.Rules), idx, idx+1), nil)
		var inUse *store.RulesInUseError
		switch {
		case errors.As(err, &inUse):
			posted.Errors = []string{inUseMessage(inUse, tasks)}
			render(http.StatusUnprocessableEntity, posted, nil)
		case errors.Is(err, store.ErrConflict):
			render(http.StatusConflict, posted, []string{conflictMessage})
		case err != nil:
			h.fail(w, r, err)
		default:
			http.Redirect(w, r, rulesURL(set.Slug), http.StatusSeeOther)
		}
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
	posted.Errors, posted.Warnings = ruleIssues(issues, i)
	switch op {
	case "preview":
		posted.Preview = markdown.Render(prepared[i].Body)
		render(http.StatusOK, posted, nil)
		return
	case "save":
	default:
		h.fail(w, r, fmt.Errorf("op %q: %w", op, store.ErrInvalid))
		return
	}
	if len(posted.Errors) > 0 {
		render(http.StatusUnprocessableEntity, posted, nil)
		return
	}
	var renames map[string]string
	if idx >= 0 && posted.Orig != prepared[i].Key {
		renames = map[string]string{posted.Orig: prepared[i].Key}
	}
	_, err = h.st.ReplaceRules(ctx, set.ID, time.Unix(0, nanos), prepared, renames)
	var inUse *store.RulesInUseError
	switch {
	case errors.As(err, &inUse):
		posted.Errors = []string{inUseMessage(inUse, tasks)}
		render(http.StatusUnprocessableEntity, posted, nil)
	case errors.Is(err, store.ErrConflict):
		render(http.StatusConflict, posted, []string{conflictMessage})
	case err != nil:
		h.fail(w, r, err)
	default:
		http.Redirect(w, r, rulesURL(set.Slug)+"?"+url.Values{"saved": {prepared[i].Key}}.Encode()+"#rule-"+prepared[i].Key, http.StatusSeeOther)
	}
}
