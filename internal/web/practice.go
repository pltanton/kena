package web

import (
	"cmp"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

type params struct {
	Mode    string
	Filter  string
	Shuffle bool
	Seed    uint64
	I, N    int
	Streak  int
	Tries   int
}

var errMissingSeed = errors.New("shuffle without seed")

func invalid(what string, v any) error {
	return fmt.Errorf("%s %q: %w", what, v, store.ErrInvalid)
}

func parseParams(v url.Values, defaultMode string) (params, error) {
	p := params{Mode: cmp.Or(v.Get("mode"), defaultMode), Filter: cmp.Or(v.Get("filter"), "all")}
	if p.Mode != "cards" && p.Mode != "list" {
		return p, invalid("mode", p.Mode)
	}
	if p.Filter != "all" && p.Filter != "unsolved" && p.Filter != "mistakes" {
		return p, invalid("filter", p.Filter)
	}
	for name, dst := range map[string]*int{"i": &p.I, "n": &p.N, "streak": &p.Streak, "tries": &p.Tries} {
		if s := v.Get(name); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				return p, invalid(name, s)
			}
			*dst = n
		}
	}
	switch v.Get("shuffle") {
	case "", "0":
		return p, nil
	case "1":
		p.Shuffle = true
	default:
		return p, invalid("shuffle", v.Get("shuffle"))
	}
	s := v.Get("seed")
	if s == "" {
		return p, errMissingSeed
	}
	seed, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return p, invalid("seed", s)
	}
	p.Seed = seed
	return p, nil
}

func (h *handler) pageParams(w http.ResponseWriter, r *http.Request, mode string) (params, bool) {
	q := r.URL.Query()
	p, err := parseParams(q, mode)
	if errors.Is(err, errMissingSeed) {
		q.Set("seed", strconv.FormatUint(uint64(mrand.Uint32()), 10))
		http.Redirect(w, r, r.URL.Path+"?"+q.Encode(), http.StatusFound)
		return p, false
	}
	if err != nil {
		h.fail(w, r, err)
		return p, false
	}
	p.Mode = mode
	return p, true
}

func (p params) query() url.Values {
	v := url.Values{}
	if p.Mode != "" {
		v.Set("mode", p.Mode)
	}
	if p.Filter != "all" {
		v.Set("filter", p.Filter)
	}
	if p.Shuffle {
		v.Set("shuffle", "1")
		v.Set("seed", strconv.FormatUint(p.Seed, 10))
	}
	return v
}

func itoa[T int | int64](n T) string { return strconv.FormatInt(int64(n), 10) }

func deckPath(slug string) string { return "/s/" + url.PathEscape(slug) }

func deckURL(slug, path string, p params, extra ...string) string {
	v := p.query()
	v.Del("mode")
	for i := 0; i < len(extra); i += 2 {
		if extra[i+1] != "0" {
			v.Set(extra[i], extra[i+1])
		}
	}
	u := deckPath(slug) + path
	if q := v.Encode(); q != "" {
		u += "?" + q
	}
	return u
}

func nextURL(slug string, after int64, p params) string {
	return deckURL(slug, "/next", p, "after", itoa(after), "n", itoa(p.N), "streak", itoa(p.Streak))
}

func itemURL(id int64, p params) string {
	return fmt.Sprintf("/t/%d/item?%s", id, p.query().Encode())
}

func ordered(tasks []store.Task, p params) []store.Task {
	out := append([]store.Task(nil), tasks...)
	if p.Shuffle {
		mrand.New(mrand.NewPCG(p.Seed, 0)).Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	}
	return out
}

func filtered(tasks []store.Task, filter string, last map[int64]bool) []store.Task {
	var out []store.Task
	for _, t := range tasks {
		correct, tried := last[t.ID]
		if filter == "all" || filter == "unsolved" && !correct || filter == "mistakes" && tried && !correct {
			out = append(out, t)
		}
	}
	return out
}

type segment struct {
	Text     string
	Blank    bool
	Index    int
	Hint     string
	Checked  bool
	Correct  bool
	Given    string
	Expected string
	Value    string
	Locked   bool
	Wrong    bool
}

type taskView struct {
	ID        int64
	Autofocus bool
	Segments  []segment
}

func newTaskView(t store.Task, results []greek.BlankResult) taskView {
	parts := strings.Split(t.Text, greek.Marker)
	v := taskView{ID: t.ID}
	for i, part := range parts {
		if part != "" {
			v.Segments = append(v.Segments, segment{Text: part})
		}
		if i == len(parts)-1 {
			break
		}
		s := segment{Blank: true, Index: i + 1}
		if i < len(t.Blanks) {
			s.Hint = t.Blanks[i].Hint
		}
		if i < len(results) {
			s.Checked = true
			s.Correct = results[i].Correct
			s.Given = strings.TrimSpace(results[i].Given)
			s.Expected = results[i].Expected
		}
		v.Segments = append(v.Segments, s)
	}
	return v
}

func retryTaskView(t store.Task, results []greek.BlankResult) taskView {
	v := newTaskView(t, nil)
	for i := range v.Segments {
		s := &v.Segments[i]
		if !s.Blank || s.Index-1 >= len(results) {
			continue
		}
		res := results[s.Index-1]
		s.Value = strings.TrimSpace(res.Given)
		s.Locked = res.Correct
		s.Wrong = !res.Correct
	}
	return v
}

var nudges = map[greek.ErrorKind]string{
	greek.KindAccent:     "nudge.accent",
	greek.KindFinalSigma: "nudge.final_sigma",
	greek.KindNearMiss:   "nudge.near_miss",
}

var nudgeRank = map[greek.ErrorKind]int{greek.KindAccent: 1, greek.KindFinalSigma: 2, greek.KindNearMiss: 3}

func nudge(t store.Task, results []greek.BlankResult) string {
	worst := greek.ErrorKind("")
	for i, res := range results {
		if res.Correct || i >= len(t.Blanks) {
			continue
		}
		k := greek.Classify(res.Given, t.Blanks[i].Answers)
		if _, ok := nudges[k]; !ok {
			return ""
		}
		if nudgeRank[k] > nudgeRank[worst] {
			worst = k
		}
	}
	return nudges[worst]
}

type progressBar struct {
	Index, Total int
	Nav          *navView
}

func (b progressBar) Pct() int {
	if b.Total == 0 {
		return 0
	}
	return b.Index * 100 / b.Total
}

type navView struct {
	ID      int64
	PrevID  int64
	PrevURL string
	SkipURL string
	NextURL string
	CardURL string
}

func newNav(slug string, id, prev int64, p params) *navView {
	streak := itoa(p.Streak)
	nv := &navView{ID: id, PrevID: prev, CardURL: deckURL(slug, "/play/card", p, "n", itoa(p.N), "streak", streak)}
	if prev != 0 {
		nv.PrevURL = deckURL(slug, "/play", p, "at", itoa(prev), "streak", streak)
	}
	if id != 0 {
		nv.SkipURL = deckURL(slug, "/play", p, "after", itoa(id), "streak", streak)
		nv.NextURL = nextURL(slug, id, p)
	}
	return nv
}

func indexOf(tasks []store.Task, id int64) int {
	for i, t := range tasks {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func prevInOrder(order []store.Task, filter string, last map[int64]bool, id int64) int64 {
	pos := indexOf(order, id)
	if pos < 0 {
		pos = len(order)
	}
	before := filtered(order[:pos], filter, last)
	if len(before) == 0 {
		return 0
	}
	return before[len(before)-1].ID
}

func positionIn(order []store.Task, filter string, last map[int64]bool, id int64, n int) int {
	i := n - len(filtered(order[indexOf(order, id)+1:], filter, last))
	return max(1, min(i, n))
}

type notes struct {
	Explanation string
	Rule        *ruleView
	Full        bool
}

type cardView struct {
	Info    *cardInfo
	Task    taskView
	Bar     progressBar
	InPage  bool
	Params  params
	Retry   bool
	Nudge   string
	Checked bool
	Correct bool
	Streak  int
	Notes   notes
	NextURL string
	ItemURL string
}

func newCardView(set store.Set, t store.Task, p params, results []greek.BlankResult, reveal, correct bool) *cardView {
	v := &cardView{Info: infoFor(set, t), Params: p}
	switch {
	case results == nil:
		v.Task = newTaskView(t, nil)
	case !reveal:
		v.Task, v.Retry, v.Nudge = retryTaskView(t, results), true, nudge(t, results)
	default:
		v.Task, v.Checked, v.Correct = newTaskView(t, results), true, correct
		v.Notes = notes{Explanation: t.Explanation, Rule: ruleFor(set, t.Rule)}
		v.NextURL, v.ItemURL = nextURL(set.Slug, t.ID, p), itemURL(t.ID, p)
	}
	return v
}

func lastView(set store.Set, t store.Task, p params, la store.LastAttempt, tried, reveal bool) *cardView {
	if !tried {
		return newCardView(set, t, p, nil, false, false)
	}
	results, _ := greek.Check(t.Blanks, la.Answers)
	if !reveal {
		p.Tries = 1
	}
	return newCardView(set, t, p, results, reveal, la.Correct)
}

func (v *cardView) inPage() *cardView {
	v.InPage = true
	v.Task.Autofocus = !v.Retry && !v.Checked
	return v
}

type doneView struct {
	Bar         *progressBar
	InPage      bool
	Empty       bool
	Progress    store.Progress
	AgainURL    string
	MistakesURL string
	DeckURL     string
	LoggedIn    bool
}

func newDone(r *http.Request, slug string, empty bool, pr store.Progress) *doneView {
	_, loggedIn := currentUser(r)
	return &doneView{
		LoggedIn:    loggedIn,
		Empty:       empty,
		Progress:    pr,
		AgainURL:    deckPath(slug) + "/play",
		MistakesURL: deckPath(slug) + "/play?filter=mistakes",
		DeckURL:     deckPath(slug),
	}
}

func firstUnsolved(tasks []store.Task, last map[int64]bool) (int64, bool) {
	for _, t := range tasks {
		if !last[t.ID] {
			return t.ID, true
		}
	}
	return 0, false
}

func progressOf(tasks []store.Task, last map[int64]bool) store.Progress {
	pr := store.Progress{Total: len(tasks)}
	for _, t := range tasks {
		correct, tried := last[t.ID]
		switch {
		case correct:
			pr.Solved++
		case tried:
			pr.Mistakes++
		}
	}
	return pr
}

type setData struct {
	set      store.Set
	tasks    []store.Task
	attempts map[int64]store.LastAttempt
	last     map[int64]bool
}

func (h *handler) loadSet(r *http.Request, slug string) (setData, error) {
	var d setData
	var err error
	if d.set, err = h.visibleSet(r, slug); err != nil {
		return d, err
	}
	if d.tasks, err = h.st.ListTasks(r.Context(), d.set.ID); err != nil {
		return d, err
	}
	if d.attempts, err = h.st.LastAttempts(r.Context(), progressID(r), d.set.ID); err != nil {
		return d, err
	}
	d.last = make(map[int64]bool, len(d.attempts))
	for id, la := range d.attempts {
		d.last[id] = la.Correct
	}
	return d, nil
}

type link struct {
	Label   string
	URL     string
	Current bool
}

type choice struct {
	Label   string
	Count   int
	URL     string
	Current bool
}

type chooser struct {
	Choices []choice
	Shuffle link
}

type deckPage struct {
	Set          store.Set
	Owner        bool
	Saved        bool
	Warnings     []string
	CanStar      bool
	Starred      bool
	Here         string
	Progress     store.Progress
	Chooser      chooser
	PracticeURL  string
	WorksheetURL string
	Rules        []*ruleView
	General      []*ruleView
}

func pickFilter(want string, pr store.Progress) string {
	switch {
	case want == "all",
		want == "unsolved" && pr.Total-pr.Solved > 0,
		want == "mistakes" && pr.Mistakes > 0:
		return want
	case pr.Solved+pr.Mistakes > 0 && pr.Total-pr.Solved > 0:
		return "unsolved"
	}
	return "all"
}

func pickURL(base, filter string, shuffle bool) string {
	v := url.Values{"filter": {filter}}
	if shuffle {
		v.Set("shuffle", "1")
	}
	return base + "?" + v.Encode()
}

func (h *handler) deckHead(w http.ResponseWriter, r *http.Request, d setData, base, filter string, shuffle bool) (deckPage, bool) {
	set := d.set
	data := deckPage{Set: set, Owner: viewerID(r) != 0 && viewerID(r) == set.OwnerID, Progress: progressOf(d.tasks, d.last), Here: r.URL.RequestURI()}
	if data.Owner && r.URL.Query().Get("saved") == "1" {
		data.Saved = true
		data.Warnings = deckWarnings(d.set, d.tasks)
	}
	if u, ok := currentUser(r); ok && !data.Owner {
		data.CanStar = true
		var err error
		if data.Starred, err = h.st.IsStarred(r.Context(), u.ID, set.ID); err != nil {
			h.fail(w, r, err)
			return data, false
		}
	}
	filter = pickFilter(filter, data.Progress)
	pr, path := data.Progress, deckPath(set.Slug)
	for _, o := range []struct {
		filter, label string
		count         int
	}{{"unsolved", "filter.unsolved", pr.Total - pr.Solved}, {"mistakes", "filter.mistakes", pr.Mistakes}, {"all", "filter.all", pr.Total}} {
		if o.count == 0 && o.filter != "all" {
			continue
		}
		data.Chooser.Choices = append(data.Chooser.Choices, choice{o.label, o.count, pickURL(path+base, o.filter, shuffle), o.filter == filter})
	}
	data.Chooser.Shuffle = link{"filter.shuffle", pickURL(path+base, filter, !shuffle), shuffle}
	data.PracticeURL = pickURL(path+"/play", filter, shuffle)
	data.WorksheetURL = pickURL(path+"/list", filter, shuffle)
	return data, true
}

func (h *handler) set(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("mode") == "list" {
		q.Del("mode")
		u := deckPath(r.PathValue("slug")) + "/list"
		if e := q.Encode(); e != "" {
			u += "?" + e
		}
		http.Redirect(w, r, u, http.StatusMovedPermanently)
		return
	}
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data, ok := h.deckHead(w, r, d, "", q.Get("filter"), q.Get("shuffle") == "1")
	if !ok {
		return
	}
	data.Rules = append(ruleViews(d.set, true), ruleViews(d.set, false)...)
	h.page(w, r, http.StatusOK, "set", data)
}

type listPage struct {
	deckPage
	Params params
	Items  []*cardView
	First  int64
	Done   *doneView
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := h.pageParams(w, r, "list")
	if !ok {
		return
	}
	p.I, p.N = 0, 0
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	head, ok := h.deckHead(w, r, d, "/list", r.URL.Query().Get("filter"), p.Shuffle)
	if !ok {
		return
	}
	p.Filter = pickFilter(r.URL.Query().Get("filter"), head.Progress)
	data := listPage{deckPage: head, Params: p}
	data.General = ruleViews(d.set, true)
	todo := filtered(ordered(d.tasks, p), p.Filter, d.last)
	if len(todo) == 0 {
		data.Done = newDone(r, d.set.Slug, true, head.Progress)
	}
	for _, t := range todo {
		la, tried := d.attempts[t.ID]
		data.Items = append(data.Items, lastView(d.set, t, p, la, tried, la.Correct))
	}
	if id, ok := firstUnsolved(todo, d.last); ok && id != todo[0].ID {
		data.First = id
	}
	h.page(w, r, http.StatusOK, "list", data)
}

type playPage struct {
	Set      store.Set
	DeckURL  string
	Card     *cardView
	Done     *doneView
	Filters  []link
	Order    []link
	ListURL  string
	FirstURL string
	ResetURL string
	Bar      progressBar
	General  []*ruleView
}

func freshCard(d setData, order []store.Task, t store.Task, i, n int, p params) *cardView {
	p.I, p.N = i, n
	v := newCardView(d.set, t, p, nil, false, false)
	v.Bar = progressBar{Index: i, Total: n, Nav: newNav(d.set.Slug, t.ID, prevInOrder(order, p.Filter, d.last, t.ID), p)}
	return v
}

func doneBar(slug string, todo []store.Task, p params, n int) progressBar {
	p.N = n
	var prev int64
	if len(todo) > 0 {
		prev = todo[len(todo)-1].ID
	}
	return progressBar{Index: n, Total: n, Nav: newNav(slug, 0, prev, p)}
}

func parseInt(what, s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, invalid(what, s)
	}
	return id, nil
}

func (h *handler) play(w http.ResponseWriter, r *http.Request) {
	p, ok := h.pageParams(w, r, "cards")
	if !ok {
		return
	}
	p.I, p.N, p.Tries = 0, 0, 0
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	set, last := d.set, d.last
	order := ordered(d.tasks, p)
	todo := filtered(order, p.Filter, last)
	pr := progressOf(d.tasks, last)

	data := playPage{Set: set, DeckURL: deckPath(set.Slug), General: ruleViews(set, true)}
	for _, f := range []struct{ filter, label string }{{"all", "filter.all"}, {"unsolved", "filter.unsolved"}, {"mistakes", "filter.mistakes"}} {
		lp := p
		lp.Filter = f.filter
		data.Filters = append(data.Filters, link{f.label, deckURL(set.Slug, "/play", lp), f.filter == p.Filter})
	}
	inOrder, shuffled := p, p
	inOrder.Shuffle, inOrder.Seed = false, 0
	shuffled.Shuffle = true
	if !p.Shuffle {
		shuffled.Seed = uint64(mrand.Uint32())
	}
	data.Order = []link{{"order.in_order", deckURL(set.Slug, "/play", inOrder), !p.Shuffle}, {"order.shuffled", deckURL(set.Slug, "/play", shuffled), p.Shuffle}}
	data.ListURL = deckURL(set.Slug, "/list", p)
	if _, ok := firstUnsolved(todo, last); ok {
		data.FirstURL = deckURL(set.Slug, "/play", p)
	}
	if pr.Solved+pr.Mistakes > 0 {
		data.ResetURL = deckPath(set.Slug) + "/reset"
	}

	q := r.URL.Query()
	rest := todo
	if id, ok := firstUnsolved(todo, last); ok {
		rest = todo[indexOf(todo, id):]
	}
	switch {
	case q.Get("at") != "":
		id, err := parseInt("at", q.Get("at"))
		if err == nil {
			data.Card, err = playState(d, order, p, id, len(todo))
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
		data.Card.inPage()
	case len(todo) == 0:
		data.Done = newDone(r, set.Slug, true, pr)
	case q.Get("after") != "":
		after, err := parseInt("after", q.Get("after"))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		rest = filtered(order[indexOf(order, after)+1:], p.Filter, last)
	}
	if data.Card == nil && data.Done == nil {
		if len(rest) == 0 {
			data.Done = newDone(r, set.Slug, false, pr)
			data.Bar = doneBar(set.Slug, todo, p, len(todo))
		} else {
			data.Card = freshCard(d, order, rest[0], indexOf(todo, rest[0].ID)+1, len(todo), p).inPage()
		}
	}
	if data.Card != nil {
		data.Bar = data.Card.Bar
	} else {
		data.Done.InPage = true
	}
	h.pageAs(w, r, http.StatusOK, "play", "focus", data)
}

func (h *handler) next(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, err := parseParams(q, "cards")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p.Mode = "cards"
	after, err := parseInt("after", q.Get("after"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	order := ordered(d.tasks, p)
	rest := filtered(order[indexOf(order, after)+1:], p.Filter, d.last)
	todo := filtered(order, p.Filter, d.last)
	if len(rest) == 0 {
		done := newDone(r, d.set.Slug, false, progressOf(d.tasks, d.last))
		bar := doneBar(d.set.Slug, todo, p, cmp.Or(p.N, len(todo)))
		done.Bar = &bar
		h.fragment(w, r, "done", done)
		return
	}
	i, n := p.N-len(rest)+1, p.N
	if p.N == 0 || i < 1 {
		i, n = indexOf(todo, rest[0].ID)+1, len(todo)
	}
	h.fragment(w, r, "card", freshCard(d, order, rest[0], i, n, p))
}

func playState(d setData, order []store.Task, p params, id int64, n int) (*cardView, error) {
	pos := indexOf(order, id)
	if pos < 0 {
		return nil, fmt.Errorf("task %d in set %d: %w", id, d.set.ID, store.ErrNotFound)
	}
	p.I, p.N = positionIn(order, p.Filter, d.last, id, n), n
	la, tried := d.attempts[id]
	v := lastView(d.set, order[pos], p, la, tried, la.Correct || la.GaveUp)
	v.Bar = progressBar{Index: p.I, Total: n, Nav: newNav(d.set.Slug, id, prevInOrder(order, p.Filter, d.last, id), p)}
	return v, nil
}

func (h *handler) playCard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, err := parseParams(q, "cards")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p.Mode = "cards"
	p.I, p.Tries = 0, 0
	id, err := parseInt("id", q.Get("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	order := ordered(d.tasks, p)
	card, err := playState(d, order, p, id, cmp.Or(p.N, len(filtered(order, p.Filter, d.last))))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.fragment(w, r, "card", card)
}

func (h *handler) task(r *http.Request, id int64) (store.Task, store.Set, error) {
	task, err := h.st.GetTask(r.Context(), id)
	if err != nil {
		return task, store.Set{}, err
	}
	set, err := h.visibleSet(r, strconv.FormatInt(task.SetID, 10))
	return task, set, err
}

func (h *handler) check(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt("task id", r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if !h.parseForm(w, r) {
		return
	}
	p, err := parseParams(r.PostForm, "cards")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	task, set, err := h.task(r, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	answers := r.PostForm["a"]
	if len(answers) != len(task.Blanks) {
		h.fail(w, r, fmt.Errorf("got %d answers for %d blanks: %w", len(answers), len(task.Blanks), store.ErrInvalid))
		return
	}

	cid := r.PostForm.Get("cid")
	if cid != "" && !uuidRe.MatchString(cid) {
		h.fail(w, r, invalid("cid", cid))
		return
	}

	results, correct := greek.Check(task.Blanks, answers)
	giveUp := !correct && r.PostForm.Get("giveup") == "1"
	switch {
	case !correct:
		p.Streak = 0
	case p.Tries == 0:
		p.Streak++
	}
	if _, err := h.st.Record(r.Context(), progressID(r), store.Attempt{ClientID: cid, TaskID: task.ID, Answers: answers, Correct: correct, GaveUp: giveUp}); err != nil {
		h.fail(w, r, err)
		return
	}

	if r.Header.Get("HX-Request") == "" {
		loc := deckURL(set.Slug, "/list", p)
		if p.Mode == "cards" {
			loc = deckURL(set.Slug, "/play", p, "after", itoa(task.ID), "streak", itoa(p.Streak))
		}
		http.Redirect(w, r, loc, http.StatusSeeOther)
		return
	}

	reveal := correct || giveUp
	if reveal {
		p.Tries = 0
	} else {
		p.Tries++
	}
	v := newCardView(set, task, p, results, reveal, correct)
	if p.Mode == "list" {
		h.fragment(w, r, "list_item", v)
		return
	}
	v.Streak = p.Streak
	v.Bar = progressBar{Index: p.I, Total: p.N, Nav: newNav(set.Slug, task.ID, 0, p)}
	h.fragment(w, r, "card", v)
}

func (h *handler) item(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt("task id", r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p, err := parseParams(r.URL.Query(), "list")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	task, set, err := h.task(r, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.fragment(w, r, "list_item", newCardView(set, task, p, nil, false, false))
}

func (h *handler) resetPage(w http.ResponseWriter, r *http.Request) {
	set, err := h.visibleSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, r, http.StatusOK, "reset", set)
}

func (h *handler) reset(w http.ResponseWriter, r *http.Request) {
	set, err := h.visibleSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.st.ResetProgress(r.Context(), progressID(r), set.ID); err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, deckPath(set.Slug), http.StatusSeeOther)
}
