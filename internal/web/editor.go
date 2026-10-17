package web

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pltanton/kena/internal/deckfmt"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

const (
	maxEditorBody   = 1 << 20
	maxDeckTasks    = 500
	maxShownErrors  = 50
	conflictMessage = "edit.conflict"
)

type editView struct {
	New          bool
	Action       string
	Slug         string
	Title        string
	Topic        string
	Level        string
	Instructions string
	SourceRef    string
	Visibility   []visibilityOption
	Source       string
	Errors       []string
	Lossy        *lossy
	Version      string
	RuleCount    int
}

var fieldRe = regexp.MustCompile(`^blanks\[(\d+)\](?:\.answers\[(\d+)\])?$`)

func describeField(field string) string {
	m := fieldRe.FindStringSubmatch(field)
	if m == nil {
		return ""
	}
	n := func(s string) int { i, _ := strconv.Atoi(s); return i + 1 }
	if m[2] == "" {
		return fmt.Sprintf("blank %d: ", n(m[1]))
	}
	return fmt.Sprintf("blank %d, answer %d: ", n(m[1]), n(m[2]))
}

type lineMsg struct {
	line int
	msg  string
}

func byLine(ms []lineMsg) []string {
	if len(ms) == 0 {
		return nil
	}
	slices.SortStableFunc(ms, func(a, b lineMsg) int { return cmp.Compare(a.line, b.line) })
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.msg
	}
	return out
}

func checkSource(src string, existing []greek.Rule) (rules []greek.Rule, inputs []greek.TaskInput, errs, warnings []string) {
	d, perrs := deckfmt.ParseDeck(src)
	items := d.Items
	var es, ws []lineMsg
	add := func(line int, sev greek.Severity, text string) {
		m := lineMsg{line, fmt.Sprintf("line %d: %s", line, text)}
		if sev == greek.Error {
			es = append(es, m)
		} else {
			ws = append(ws, m)
		}
	}
	for _, e := range perrs {
		es = append(es, lineMsg{e.Line, e.Error()})
	}
	if len(items) > maxDeckTasks {
		add(items[maxDeckTasks].Line, greek.Error, fmt.Sprintf("a deck holds at most %d tasks", maxDeckTasks))
	}
	merged := slices.Clone(existing)
	lineOf := map[int]int{}
	for k, r := range d.Rules {
		norm, _ := greek.PrepareRules([]greek.Rule{r})
		j := slices.IndexFunc(merged, func(x greek.Rule) bool { return x.Key == norm[0].Key })
		switch {
		case j < 0:
			lineOf[len(merged)] = d.RuleLines[k]
			merged = append(merged, r)
		case merged[j].Body != norm[0].Body:
			add(d.RuleLines[k], greek.Error, fmt.Sprintf("rule %q already exists with other text; edit it on the Rules page and drop this line", norm[0].Key))
		}
	}
	prepared := existing
	if len(lineOf) > 0 {
		var rissues []greek.Issue
		prepared, rissues = greek.PrepareRules(merged)
		rules = prepared
		for _, iss := range rissues {
			line := 1
			if iss.Field != "rules" {
				var i int
				var ok bool
				if _, err := fmt.Sscanf(iss.Field, "rules[%d]", &i); err == nil {
					line, ok = lineOf[i]
				}
				if !ok {
					continue
				}
			}
			add(line, iss.Severity, iss.Message)
		}
	}
	inputs, issues := greek.PrepareDeck(deckfmt.Tasks(items), nil, prepared)
	for _, iss := range issues {
		add(items[iss.Task].Line, iss.Severity, describeField(iss.Field)+iss.Message)
	}
	return rules, inputs, byLine(es), byLine(ws)
}

func taskInputs(tasks []store.Task) []greek.TaskInput {
	out := make([]greek.TaskInput, len(tasks))
	for i, t := range tasks {
		out[i] = greek.TaskInput{Text: t.Text, Blanks: t.Blanks, Explanation: t.Explanation, Rule: t.Rule}
	}
	return out
}

func lossyLines(rules []greek.Rule, inputs []greek.TaskInput) []int {
	var lines []int
	line := 1
	for _, in := range inputs {
		src := deckfmt.Format([]greek.TaskInput{in})
		items, errs := deckfmt.Parse(src)
		var back []greek.TaskInput
		if len(errs) == 0 && len(items) == 1 {
			back, _ = greek.PrepareDeck(deckfmt.Tasks(items), nil, rules)
		}
		if len(back) != 1 || !sameInput(back[0], in) {
			lines = append(lines, line)
		}
		line += strings.Count(src, "\n") + 1
	}
	return lines
}

func sameInput(a, b greek.TaskInput) bool {
	return a.Text == b.Text && a.Explanation == b.Explanation && a.Rule == b.Rule &&
		slices.EqualFunc(a.Blanks, b.Blanks, func(x, y greek.Blank) bool {
			return x.Hint == y.Hint && slices.Equal(x.Answers, y.Answers)
		})
}

type lossy struct {
	N     int
	Lines string
}

func linesList(lines []int) *lossy {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = strconv.Itoa(l)
	}
	return &lossy{N: len(lines), Lines: strings.Join(parts, ", ")}
}

func deckSource(tasks []store.Task) string {
	return deckfmt.Format(taskInputs(tasks))
}

func deckWarnings(set store.Set, tasks []store.Task) []string {
	_, _, _, warnings := checkSource(deckSource(tasks), set.Rules)
	return warnings
}

type deckForm struct {
	title, topic, level, instructions, sourceRef, source string
	visibility                                           store.Visibility
	version                                              string
}

func versionOf(set store.Set) string {
	return strconv.FormatInt(set.UpdatedAt.UnixNano(), 10)
}

func parseVersion(s string) (time.Time, error) {
	nanos, err := parseInt("version", s)
	return time.Unix(0, nanos), err
}

func parseDeckForm(w http.ResponseWriter, r *http.Request) (deckForm, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEditorBody)
	if err := r.ParseForm(); err != nil {
		return deckForm{}, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid)
	}
	f := r.PostForm
	vis, ok := store.ParseVisibility(f.Get("visibility"))
	if !ok {
		return deckForm{}, invalid("visibility", f.Get("visibility"))
	}
	clean := func(k string) string { return strings.TrimSpace(greek.NFC(f.Get(k))) }
	return deckForm{
		title: clean("title"), topic: clean("topic"), level: clean("level"),
		instructions: clean("instructions"), sourceRef: clean("source_ref"),
		source: f.Get("source"), visibility: vis, version: f.Get("version"),
	}, nil
}

func (f deckForm) view(v editView) editView {
	v.Title, v.Topic, v.Level, v.Instructions, v.SourceRef, v.Source = f.title, f.topic, f.level, f.instructions, f.sourceRef, f.source
	v.Visibility = visibilityChoices(f.visibility)
	v.Version = f.version
	return v
}

func (f deckForm) check(r *http.Request, existing []greek.Rule) ([]greek.Rule, []greek.TaskInput, []string) {
	rules, inputs, errs, _ := checkSource(f.source, existing)
	if f.title == "" {
		errs = append([]string{tr(r, "edit.title_required")}, errs...)
	}
	if n := len(errs) - maxShownErrors; n > 0 {
		errs = append(errs[:maxShownErrors], trn(r, "edit.and_more", n))
	}
	return rules, inputs, errs
}

func savedURL(slug string) string {
	return deckPath(slug) + "?saved=1"
}

func (h *handler) editPage(w http.ResponseWriter, r *http.Request, u store.User) {
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
	v := editView{
		Action: deckPath(set.Slug) + "/edit", Slug: set.Slug,
		Title: set.Title, Topic: set.Topic, Level: set.Level, Instructions: set.Instructions, SourceRef: set.SourceRef,
		Source: deckSource(tasks), Version: versionOf(set), RuleCount: len(set.Rules),
	}
	if lines := lossyLines(set.Rules, taskInputs(tasks)); len(lines) > 0 {
		v.Lossy = linesList(lines)
	}
	v.Visibility = visibilityChoices(set.Visibility)
	h.page(w, r, http.StatusOK, "edit", v)
}

func (h *handler) saveDeck(w http.ResponseWriter, r *http.Request, u store.User) {
	set, err := h.ownedSet(r, u)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	f, err := parseDeckForm(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	version, err := parseVersion(f.version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rerender := func(status int, errs []string) {
		v := f.view(editView{Action: deckPath(set.Slug) + "/edit", Slug: set.Slug, Errors: errs, RuleCount: len(set.Rules)})
		h.page(w, r, status, "edit", v)
	}
	rules, inputs, errs := f.check(r, set.Rules)
	if len(errs) > 0 {
		rerender(http.StatusUnprocessableEntity, errs)
		return
	}
	patch := store.SetPatch{Title: &f.title, Topic: &f.topic, Level: &f.level, Instructions: &f.instructions,
		SourceRef: &f.sourceRef, Visibility: &f.visibility}
	if rules != nil {
		patch.Rules = &rules
	}
	_, err = h.st.SaveDeck(r.Context(), set.ID, version, patch, inputs)
	if errors.Is(err, store.ErrConflict) {
		rerender(http.StatusConflict, []string{tr(r, conflictMessage)})
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, savedURL(set.Slug), http.StatusSeeOther)
}

func (h *handler) newPage(w http.ResponseWriter, r *http.Request, u store.User) {
	h.page(w, r, http.StatusOK, "edit", editView{New: true, Action: "/new", Visibility: visibilityChoices(store.Private)})
}

func (h *handler) createDeck(w http.ResponseWriter, r *http.Request, u store.User) {
	f, err := parseDeckForm(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rules, inputs, errs := f.check(r, nil)
	if len(errs) > 0 {
		h.page(w, r, http.StatusUnprocessableEntity, "edit", f.view(editView{New: true, Action: "/new", Errors: errs}))
		return
	}
	ctx := r.Context()
	set, err := h.st.CreateSet(ctx, u.ID, store.SetInput{Title: f.title, Topic: f.topic, Level: f.level,
		Instructions: f.instructions, SourceRef: f.sourceRef, Visibility: f.visibility, Rules: rules})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.st.SaveDeck(ctx, set.ID, time.Time{}, store.SetPatch{}, inputs); err != nil {
		h.fail(w, r, errors.Join(err, h.st.DeleteSet(context.WithoutCancel(ctx), set.ID)))
		return
	}
	http.Redirect(w, r, savedURL(set.Slug), http.StatusSeeOther)
}

func (h *handler) deletePage(w http.ResponseWriter, r *http.Request, u store.User) {
	set, err := h.ownedSet(r, u)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, r, http.StatusOK, "delete", set)
}

func (h *handler) deleteDeck(w http.ResponseWriter, r *http.Request, u store.User) {
	set, err := h.ownedSet(r, u)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.st.DeleteSet(r.Context(), set.ID); err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
