package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/pltanton/kena/internal/deckfmt"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

const maxEditorBody = 1 << 20

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

func checkSource(src string) (inputs []greek.TaskInput, errs, warnings []string) {
	items, perrs := deckfmt.Parse(src)
	for _, e := range perrs {
		errs = append(errs, e.Error())
	}
	inputs, issues := greek.Prepare(deckfmt.Tasks(items), nil)
	for _, iss := range issues {
		msg := fmt.Sprintf("line %d: %s%s", items[iss.Task].Line, describeField(iss.Field), iss.Message)
		if iss.Severity == greek.Error {
			errs = append(errs, msg)
		} else {
			warnings = append(warnings, msg)
		}
	}
	return inputs, errs, warnings
}

func taskInputs(tasks []store.Task) []greek.TaskInput {
	out := make([]greek.TaskInput, len(tasks))
	for i, t := range tasks {
		out[i] = greek.TaskInput{Text: t.Text, Blanks: t.Blanks, Explanation: t.Explanation}
	}
	return out
}

func deckWarnings(tasks []store.Task) []string {
	_, _, warnings := checkSource(deckfmt.Format(taskInputs(tasks)))
	return warnings
}

type deckForm struct {
	title, topic, level, instructions, sourceRef, source string
	visibility                                           store.Visibility
}

func parseDeckForm(w http.ResponseWriter, r *http.Request) (deckForm, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEditorBody)
	if err := r.ParseForm(); err != nil {
		return deckForm{}, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid)
	}
	f := r.PostForm
	vis, ok := store.ParseVisibility(f.Get("visibility"))
	if !ok {
		return deckForm{}, fmt.Errorf("visibility %q: %w", f.Get("visibility"), store.ErrInvalid)
	}
	clean := func(k string) string { return strings.TrimSpace(greek.NFC(f.Get(k))) }
	return deckForm{
		title: clean("title"), topic: clean("topic"), level: clean("level"),
		instructions: clean("instructions"), sourceRef: clean("source_ref"),
		source: f.Get("source"), visibility: vis,
	}, nil
}

func (f deckForm) view(v editView) editView {
	v.Title, v.Topic, v.Level, v.Instructions, v.SourceRef, v.Source = f.title, f.topic, f.level, f.instructions, f.sourceRef, f.source
	v.Visibility = visibilityChoices(f.visibility)
	return v
}

func (f deckForm) check() ([]greek.TaskInput, []string) {
	inputs, errs, _ := checkSource(f.source)
	if f.title == "" {
		errs = append([]string{"Title is required."}, errs...)
	}
	return inputs, errs
}

func savedURL(slug string) string {
	return "/s/" + url.PathEscape(slug) + "?saved=1"
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
		Action: "/s/" + url.PathEscape(set.Slug) + "/edit", Slug: set.Slug,
		Title: set.Title, Topic: set.Topic, Level: set.Level, Instructions: set.Instructions, SourceRef: set.SourceRef,
		Source: deckfmt.Format(taskInputs(tasks)),
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
	inputs, errs := f.check()
	if len(errs) > 0 {
		v := f.view(editView{Action: "/s/" + url.PathEscape(set.Slug) + "/edit", Slug: set.Slug, Errors: errs})
		h.page(w, r, http.StatusUnprocessableEntity, "edit", v)
		return
	}
	patch := store.SetPatch{Title: &f.title, Topic: &f.topic, Level: &f.level, Instructions: &f.instructions,
		SourceRef: &f.sourceRef, Visibility: &f.visibility}
	if _, err := h.st.SaveDeck(r.Context(), set.ID, patch, inputs); err != nil {
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
	inputs, errs := f.check()
	if len(errs) > 0 {
		h.page(w, r, http.StatusUnprocessableEntity, "edit", f.view(editView{New: true, Action: "/new", Errors: errs}))
		return
	}
	ctx := r.Context()
	set, err := h.st.CreateSet(ctx, u.ID, store.SetInput{Title: f.title, Topic: f.topic, Level: f.level,
		Instructions: f.instructions, SourceRef: f.sourceRef, Visibility: f.visibility})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.st.SaveDeck(ctx, set.ID, store.SetPatch{}, inputs); err != nil {
		h.fail(w, r, errors.Join(err, h.st.DeleteSet(ctx, set.ID)))
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
