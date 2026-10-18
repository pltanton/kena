package web

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/markdown"
	"github.com/pltanton/kena/internal/store"
)

type publicRuleRow struct {
	Key     string
	Name    string
	Label   string
	Preview string
	Creator string
}

type publicCategory struct {
	Name  string
	Rules []publicRuleRow
}

type publicRuleForm struct {
	Action     string
	Key        string
	Category   string
	Name       string
	Title      string
	Body       string
	Version    string
	Categories []string
	Errors     []string
	Warnings   []string
	Preview    template.HTML
	Saved      bool
}

type publicRulesPage struct {
	Categories    []publicCategory
	LoggedIn      bool
	IsAdmin       bool
	LoginURL      string
	New           publicRuleForm
	CategoryName  string
	CategoryError string
}

type publicRulePage struct {
	Rule    store.PublicRule
	HTML    template.HTML
	Use     store.PublicRuleUse
	CanEdit bool
	Form    publicRuleForm
	Error   string
}

func publicRulePath(key string) string {
	return "/rules/" + key
}

func ruleVersion(p store.PublicRule) string {
	return strconv.FormatInt(p.UpdatedAt.UnixNano(), 10)
}

func (h *handler) publicRulesView(r *http.Request) (publicRulesPage, error) {
	ctx := r.Context()
	cats, err := h.st.ListRuleCategories(ctx)
	if err != nil {
		return publicRulesPage{}, err
	}
	rules, err := h.st.ListPublicRules(ctx, "")
	if err != nil {
		return publicRulesPage{}, err
	}
	u, logged := currentUser(r)
	v := publicRulesPage{LoggedIn: logged, IsAdmin: logged && u.IsAdmin, New: publicRuleForm{Action: "/rules"}, LoginURL: loginURL("/rules")}
	idx := map[string]int{}
	for _, c := range cats {
		idx[c.Name] = len(v.Categories)
		v.Categories = append(v.Categories, publicCategory{Name: c.Name})
		v.New.Categories = append(v.New.Categories, c.Name)
	}
	for _, p := range rules {
		c := &v.Categories[idx[p.Category]]
		c.Rules = append(c.Rules, publicRuleRow{Key: p.Key(), Name: p.Name, Label: p.Rule().Label(),
			Preview: markdown.Preview(p.Body, rulePreviewLen), Creator: p.OwnerName})
	}
	return v, nil
}

func (h *handler) publicRules(w http.ResponseWriter, r *http.Request) {
	v, err := h.publicRulesView(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, r, http.StatusOK, "public_rules", v)
}

func (h *handler) createCategory(w http.ResponseWriter, r *http.Request, u store.User) {
	if !u.IsAdmin {
		h.notFound(w, r)
		return
	}
	if !h.parseForm(w, r) {
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("name"))
	_, err := h.st.CreateRuleCategory(r.Context(), u, name)
	var msg string
	switch {
	case errors.Is(err, store.ErrInvalid):
		msg = tr(r, "public.bad_category")
	case errors.Is(err, store.ErrConflict):
		msg = tr(r, "public.category_exists", name)
	case err != nil:
		h.fail(w, r, err)
		return
	default:
		http.Redirect(w, r, "/rules#cat-"+name, http.StatusSeeOther)
		return
	}
	v, err := h.publicRulesView(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	v.CategoryName, v.CategoryError = name, msg
	h.page(w, r, http.StatusUnprocessableEntity, "public_rules", v)
}

func (h *handler) deleteCategory(w http.ResponseWriter, r *http.Request, u store.User) {
	if !u.IsAdmin {
		h.notFound(w, r)
		return
	}
	err := h.st.DeleteRuleCategory(r.Context(), u, r.PathValue("category"))
	if errors.Is(err, store.ErrConflict) {
		v, verr := h.publicRulesView(r)
		if verr != nil {
			h.fail(w, r, verr)
			return
		}
		v.CategoryError = tr(r, "public.category_not_empty")
		h.page(w, r, http.StatusConflict, "public_rules", v)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/rules", http.StatusSeeOther)
}

func postedRuleForm(r *http.Request) publicRuleForm {
	f := r.PostForm
	return publicRuleForm{Category: f.Get("category"), Name: strings.TrimSpace(f.Get("name")), Title: f.Get("title"),
		Body: f.Get("body"), Version: f.Get("version")}
}

func lintPublicRule(r *http.Request, form *publicRuleForm) (greek.Rule, bool) {
	prepared, issues := greek.PrepareRules([]greek.Rule{{Key: "rule", Title: form.Title, Body: form.Body}})
	form.Errors, form.Warnings = ruleIssues(r, issues, 0)
	return prepared[0], len(form.Errors) == 0
}

func (h *handler) createPublicRule(w http.ResponseWriter, r *http.Request, u store.User) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRulesBody)
	if !h.parseForm(w, r) {
		return
	}
	form := postedRuleForm(r)
	render := func(status int) {
		v, err := h.publicRulesView(r)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		form.Action, form.Categories = v.New.Action, v.New.Categories
		v.New = form
		h.page(w, r, status, "public_rules", v)
	}
	op := r.PostForm.Get("op")
	if op != "save" && op != "preview" {
		h.fail(w, r, invalid("op", op))
		return
	}
	key := form.Category + "/" + form.Name
	if _, _, err := store.SplitPublicRuleKey(key); err != nil {
		form.Errors = []string{tr(r, "public.bad_name")}
		render(http.StatusUnprocessableEntity)
		return
	}
	rule, ok := lintPublicRule(r, &form)
	if !ok {
		render(http.StatusUnprocessableEntity)
		return
	}
	if op == "preview" {
		form.Preview = markdown.Render(rule.Body)
		render(http.StatusOK)
		return
	}
	if _, err := h.st.GetPublicRule(r.Context(), key); err == nil {
		form.Errors = []string{tr(r, "public.exists", key)}
		render(http.StatusConflict)
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		h.fail(w, r, err)
		return
	}
	_, _, err := h.st.SavePublicRule(r.Context(), u, key, time.Time{}, store.PublicRuleInput{Title: rule.Title, Body: rule.Body})
	switch {
	case errors.Is(err, store.ErrConflict):
		form.Errors = []string{tr(r, "public.exists", key)}
		render(http.StatusConflict)
	case errors.Is(err, store.ErrInvalid):
		form.Errors = []string{tr(r, "public.no_category")}
		render(http.StatusUnprocessableEntity)
	case err != nil:
		h.fail(w, r, err)
	default:
		http.Redirect(w, r, publicRulePath(key)+"?saved=1", http.StatusSeeOther)
	}
}

func ruleKeyOf(r *http.Request) string {
	return r.PathValue("category") + "/" + r.PathValue("name")
}

func (h *handler) publicRuleView(r *http.Request) (publicRulePage, error) {
	key := ruleKeyOf(r)
	p, err := h.st.GetPublicRule(r.Context(), key)
	if err != nil {
		return publicRulePage{}, err
	}
	use, err := h.st.PublicRuleUse(r.Context(), key)
	if err != nil {
		return publicRulePage{}, err
	}
	u, _ := currentUser(r)
	return publicRulePage{Rule: p, HTML: markdown.Render(p.Body), Use: use, CanEdit: store.CanEditPublicRule(u, p),
		Form: publicRuleForm{Action: publicRulePath(key), Key: key, Title: p.Title, Body: p.Body, Version: ruleVersion(p),
			Warnings: markdown.Issues(p.Body)}}, nil
}

func (h *handler) publicRule(w http.ResponseWriter, r *http.Request) {
	v, err := h.publicRuleView(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	v.Form.Saved = r.URL.Query().Get("saved") != ""
	h.page(w, r, http.StatusOK, "public_rule", v)
}

func (h *handler) savePublicRule(w http.ResponseWriter, r *http.Request, u store.User) {
	v, err := h.publicRuleView(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !v.CanEdit {
		h.notFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRulesBody)
	if !h.parseForm(w, r) {
		return
	}
	key := v.Rule.Key()
	posted := postedRuleForm(r)
	posted.Action, posted.Key = v.Form.Action, key
	render := func(status int, pageErr string) {
		v.Form, v.Error = posted, pageErr
		h.page(w, r, status, "public_rule", v)
	}
	switch op := r.PostForm.Get("op"); op {
	case "delete":
		err := h.st.DeletePublicRule(r.Context(), u, key)
		var inUse *store.PublicRuleInUseError
		switch {
		case errors.As(err, &inUse):
			render(http.StatusConflict, tr(r, "public.in_use", trn(r, "count.tasks", inUse.Tasks), trn(r, "count.decks", inUse.Decks)))
		case err != nil:
			h.fail(w, r, err)
		default:
			http.Redirect(w, r, "/rules", http.StatusSeeOther)
		}
	case "save", "preview":
		at, err := parseVersion(posted.Version)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		rule, ok := lintPublicRule(r, &posted)
		if !ok {
			render(http.StatusUnprocessableEntity, "")
			return
		}
		if op == "preview" {
			posted.Preview = markdown.Render(rule.Body)
			render(http.StatusOK, "")
			return
		}
		_, _, err = h.st.SavePublicRule(r.Context(), u, key, at, store.PublicRuleInput{Title: rule.Title, Body: rule.Body})
		switch {
		case errors.Is(err, store.ErrConflict):
			render(http.StatusConflict, tr(r, conflictMessage))
		case err != nil:
			h.fail(w, r, err)
		default:
			http.Redirect(w, r, publicRulePath(key)+"?"+url.Values{"saved": {"1"}}.Encode(), http.StatusSeeOther)
		}
	default:
		h.fail(w, r, invalid("op", op))
	}
}

type publicOption struct {
	Key   string
	Label string
}

type publicGroup struct {
	Name    string
	Options []publicOption
}

type deckPublic struct {
	Attached []*ruleView
	Groups   []publicGroup
	Full     bool
	Error    string
}

func (h *handler) deckPublic(r *http.Request, set store.Set) (deckPublic, error) {
	rules, err := h.st.ListPublicRules(r.Context(), "")
	if err != nil {
		return deckPublic{}, err
	}
	var d deckPublic
	attached := map[string]bool{}
	for _, p := range set.Public {
		if p.General {
			attached[p.Key] = true
			d.Attached = append(d.Attached, newRuleView(p))
		}
	}
	d.Full = len(d.Attached) >= store.MaxAttachedPublicRules
	for _, p := range rules {
		if attached[p.Key()] {
			continue
		}
		if n := len(d.Groups); n == 0 || d.Groups[n-1].Name != p.Category {
			d.Groups = append(d.Groups, publicGroup{Name: p.Category})
		}
		g := &d.Groups[len(d.Groups)-1]
		g.Options = append(g.Options, publicOption{Key: p.Key(), Label: p.Name + " — " + p.Rule().Label()})
	}
	return d, nil
}

func (h *handler) deckPublicRule(w http.ResponseWriter, r *http.Request, u store.User) {
	set, err := h.ownedSet(r, u)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !h.parseForm(w, r) {
		return
	}
	key := r.PostForm.Get("id")
	switch op := r.PostForm.Get("op"); op {
	case "attach":
		_, err = h.st.AttachPublicRule(r.Context(), set.ID, key)
	case "detach":
		_, err = h.st.DetachPublicRule(r.Context(), set.ID, key)
	default:
		err = invalid("op", op)
	}
	if errors.Is(err, store.ErrTooManyAttached) || errors.Is(err, store.ErrInvalid) && r.PostForm.Get("op") == "attach" {
		tasks, terr := h.st.ListTasks(r.Context(), set.ID)
		if terr != nil {
			h.fail(w, r, terr)
			return
		}
		v := rulesView(set, tasks, versionOf(set))
		if v.Public, terr = h.deckPublic(r, set); terr != nil {
			h.fail(w, r, terr)
			return
		}
		v.Public.Error = tr(r, "public.attach_failed", store.MaxAttachedPublicRules)
		h.page(w, r, http.StatusUnprocessableEntity, "rules", v)
		return
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, rulesURL(set.Slug)+"#public", http.StatusSeeOther)
}
