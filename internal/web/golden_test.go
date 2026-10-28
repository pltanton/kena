package web_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

const goldenFile = "testdata/offline_golden.json"

type goldenStep struct {
	Method string     `json:"method"`
	URL    string     `json:"url"`
	Form   url.Values `json:"form,omitempty"`
	HTML   string     `json:"html,omitempty"`
	Card   string     `json:"card,omitempty"`
	Bar    string     `json:"bar,omitempty"`
	Menu   []string   `json:"menu,omitempty"`
}

type golden struct {
	Pack  json.RawMessage `json:"pack"`
	Steps []goldenStep    `json:"steps"`
}

var (
	cardRe     = regexp.MustCompile(`(?s)<div id="card">\n(.*)\n</div>\n</div>\n</main>`)
	barRe      = regexp.MustCompile(`(?s)<div id="focus-pos" class="focus-pos">(.*?)</div>\n<button type="button" class="sync-ind"`)
	segRe      = regexp.MustCompile(`<span class="seg">(.*?)</span></div>`)
	sheetRe    = regexp.MustCompile(`<div class="sheet-links">(.*?)</div>`)
	anySeedRe  = regexp.MustCompile(`seed=\d+`)
	goldenDeck = []greek.TaskInput{
		{Text: "Εσύ ___ πολύ.", Blanks: []greek.Blank{{Answers: []string{"γράφεις"}, Hint: "γράφω"}}, Explanation: "β΄ πρόσωπο & <ενικός> «Α+Β»", Rule: "pres"},
		{Text: "— Τι ___;\n— ___ καφέ.", Blanks: []greek.Blank{{Answers: []string{"πίνεις", "θα πιεις"}}, {Answers: []string{"Πίνω"}, Hint: "πίνω", Choices: []string{"Έπινα", "Θα πίνω"}}}},
		{Text: "Ο ___ μου ήρθε 'χθες'.", Blanks: []greek.Blank{{Answers: []string{"φίλος"}}}, Explanation: "τελικό ς", Rule: "sig"},
		{Text: "Αυτοί ___ αύριο.", Blanks: []greek.Blank{{Answers: []string{"θα έρθουν"}, Hint: "έρχομαι"}}, Rule: "ellenika/mellon"},
		{Text: "Εμείς ___ τώρα.", Blanks: []greek.Blank{{Answers: []string{"τρώμε"}}}, Rule: "gen"},
		{Text: "Η Άννα ___ σήμερα.", Blanks: []greek.Blank{{Answers: []string{"διαβάζει"}, Choices: []string{"διάβαζε", "θα διαβάσει & <ξανά>"}}}},
		{Kind: greek.ChoiceTask, Text: "Χθες ___ ένα γράμμα & <x>. Ποιος χρόνος είναι το «έγραψα»;", Blanks: []greek.Blank{{Answers: []string{"Αόριστος"}, Choices: []string{"Ενεστώτας", "Παρατατικός & <x>"}}}, Explanation: "Ολοκληρωμένη πράξη.", Rule: "pres"},
	}
	goldenRules = []greek.Rule{
		{Key: "pres", Title: "Ενεστώτας", Body: "Κλίση:\n\n| | ενικός |\n|---|---|\n| β΄ | γράφεις |"},
		{Key: "sig", Body: "Στο τέλος της λέξης γράφουμε **ς**."},
		{Key: "gen", Title: "Γενικά", Body: "Προσοχή στους *τόνους* & στα [άρθρα](https://example.org).", General: true},
	}
)

func (e *env) goldenStep(lang string, s goldenStep) goldenStep {
	e.t.Helper()
	var req *http.Request
	var err error
	if s.Method == http.MethodPost {
		req, err = http.NewRequest(http.MethodPost, e.srv.URL+s.URL, strings.NewReader(s.Form.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequest(http.MethodGet, e.srv.URL+s.URL, nil)
	}
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Accept-Language", lang)
	if s.Method != "PLAY" {
		req.Header.Set("HX-Request", "true")
	}
	resp, body := e.do(req)
	wantStatus(e.t, resp, http.StatusOK)
	if s.Method != "PLAY" {
		s.HTML = body
		return s
	}
	card, bar := cardRe.FindStringSubmatch(body), barRe.FindStringSubmatch(body)
	if card == nil || bar == nil {
		e.t.Fatalf("%s: no card or bar in\n%s", s.URL, body)
	}
	s.Card, s.Bar = card[1], bar[1]
	segs := segRe.FindAllStringSubmatch(body, 2)
	links := sheetRe.FindStringSubmatch(body)
	if len(segs) != 2 || links == nil {
		e.t.Fatalf("%s: no menu in\n%s", s.URL, body)
	}
	order := segs[1][1]
	if !strings.Contains(s.URL, "shuffle=1") {
		order = anySeedRe.ReplaceAllString(order, "seed=*")
	}
	s.Menu = []string{segs[0][1], order, links[1]}
	return s
}

// The JS engine replays these steps (internal/web/jstest) and must render the same markup.
func TestOfflineGolden(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	set, err := e.st.CreateSet(ctx, e.maria().ID, store.SetInput{Title: "Golden deck", Visibility: store.Public, Rules: goldenRules})
	if err != nil {
		t.Fatal(err)
	}
	e.publicRule("ellenika/mellon", "Μέλλοντας", "**θα** + υποτακτική: *θα έρθουν*.")
	e.publicRule("ellenika/arthra", "", "Τα άρθρα: ο, η, το.")
	if _, err := e.st.AttachPublicRule(ctx, set.ID, "ellenika/arthra"); err != nil {
		t.Fatal(err)
	}
	tasks, err := e.st.AddTasks(ctx, set.ID, goldenDeck)
	if err != nil {
		t.Fatal(err)
	}
	id := func(i int) string { return fmt.Sprint(tasks[i-1].ID) }
	g := e.as("", false)
	const lang = "ru"
	g.goldenStep(lang, goldenStep{Method: http.MethodPost, URL: "/t/" + id(1) + "/check", Form: url.Values{"a": {"γραφεις"}}})
	_, _, packBody := g.pack(set.Slug, "Accept-Language", lang)

	play := func(q string) goldenStep { return goldenStep{Method: "PLAY", URL: "/s/" + set.Slug + "/play" + q} }
	get := func(path string) goldenStep { return goldenStep{Method: http.MethodGet, URL: "/s/" + set.Slug + path} }
	check := func(task int, form url.Values) goldenStep {
		if form.Get("mode") == "" {
			form.Set("mode", "cards")
		}
		if form.Get("filter") == "" {
			form.Set("filter", "all")
		}
		return goldenStep{Method: http.MethodPost, URL: "/t/" + id(task) + "/check", Form: form}
	}
	n6 := url.Values{"i": {"1"}, "n": {"6"}}
	with := func(base url.Values, kv ...string) url.Values {
		v := url.Values{}
		for k, vs := range base {
			v[k] = append([]string(nil), vs...)
		}
		for i := 0; i < len(kv); i += 2 {
			v.Add(kv[i], kv[i+1])
		}
		return v
	}
	steps := []goldenStep{
		play(""),
		check(1, with(n6, "a", "γραφεις", "cid", cid(101))),
		check(1, with(n6, "a", "γράφεις", "tries", "1")),
		get("/next?after=" + id(1) + "&n=6"),
		check(2, url.Values{"a": {"πίνεις"}, "i": {"2"}, "n": {"6"}, "streak": {"1"}, "cid": {cid(103)}}),
		check(2, url.Values{"a": {""}, "c2": {"Έπινα"}, "i": {"2"}, "n": {"6"}, "streak": {"1"}}),
		get("/play/card?id=" + id(2) + "&n=6"),
		check(2, url.Values{"a": {"πινεις"}, "c2": {"Πίνω"}, "x2": {"Έπινα"}, "i": {"2"}, "n": {"6"}}),
		check(2, url.Values{"a": {"πινεις"}, "c2": {"Πίνω"}, "i": {"2"}, "n": {"6"}, "tries": {"1"}, "giveup": {"1"}}),
		get("/next?after=" + id(2) + "&n=6"),
		check(3, url.Values{"a": {"φίλοσ"}, "i": {"3"}, "n": {"6"}}),
		check(3, url.Values{"a": {" Φίλος "}, "i": {"3"}, "n": {"6"}, "tries": {"1"}}),
		get("/next?after=" + id(3) + "&n=6"),
		check(4, url.Values{"a": {"θα έρθουνε"}, "i": {"4"}, "n": {"6"}}),
		check(4, url.Values{"a": {""}, "i": {"4"}, "n": {"6"}, "tries": {"1"}}),
		get("/play/card?id=" + id(4) + "&n=6"),
		play("?at=" + id(4)),
		check(4, url.Values{"a": {"θα  έρθουν"}, "i": {"4"}, "n": {"6"}, "tries": {"2"}}),
		get("/next?after=" + id(4) + "&n=6"),
		check(5, url.Values{"a": {"τρώμε"}, "i": {"5"}, "n": {"6"}}),
		get("/next?after=" + id(5) + "&n=6&streak=1"),
		check(6, url.Values{"c1": {"θα διαβάσει & <ξανά>"}, "i": {"6"}, "n": {"6"}, "streak": {"1"}}),
		check(6, url.Values{"c1": {"διάβαζε"}, "x1": {"θα διαβάσει & <ξανά>"}, "i": {"6"}, "n": {"6"}, "tries": {"1"}}),
		check(6, url.Values{"c1": {"διαβάζει"}, "x1": {"θα διαβάσει & <ξανά>", "διάβαζε"}, "i": {"6"}, "n": {"6"}, "tries": {"2"}, "cid": {cid(102)}}),
		get("/next?after=" + id(6) + "&n=6&streak=2"),
		get("/play/card?id=" + id(2) + "&n=6&streak=2"),
		get("/play/card?id=" + id(1) + "&n=6&streak=2"),
		get("/play/card?id=" + id(6) + "&n=6"),
		play("?filter=mistakes"),
		play("?filter=unsolved&streak=3"),
		get("/next?after=0&filter=mistakes"),
		check(2, url.Values{"a": {"θα πιεις"}, "c2": {"Πίνω"}, "filter": {"unsolved"}, "i": {"1"}, "n": {"1"}, "streak": {"3"}}),
		get("/next?after=" + id(2) + "&filter=unsolved&n=1&streak=4"),
		play("?filter=unsolved"),
		play("?filter=mistakes"),
		play("?at=" + id(3)),
		play("?after=" + id(6)),
		play("?after=" + id(3) + "&filter=all"),
		play("?shuffle=1&seed=42"),
		get("/next?after=" + id(2) + "&shuffle=1&seed=42&n=6"),
		get("/play/card?id=" + id(5) + "&shuffle=1&seed=42&n=6"),
		play("?shuffle=1&seed=42&at=" + id(5)),
		get("/next?after=" + id(6) + "&n=6"),
		get("/next?after=" + id(6)),
		get("/play/card?id=" + id(3) + "&filter=mistakes"),
		play("?at=" + id(7)),
		check(7, url.Values{"c1": {"Παρατατικός & <x>"}, "i": {"7"}, "n": {"7"}}),
		check(7, url.Values{"c1": {"Ενεστώτας"}, "x1": {"Παρατατικός & <x>"}, "i": {"7"}, "n": {"7"}, "tries": {"1"}}),
		get("/play/card?id=" + id(7) + "&n=7"),
		check(7, url.Values{"c1": {"Αόριστος"}, "x1": {"Παρατατικός & <x>", "Ενεστώτας"}, "i": {"7"}, "n": {"7"}, "tries": {"2"}}),
		get("/play/card?id=" + id(7) + "&n=7"),
		check(7, url.Values{"c1": {"Ενεστώτας"}, "i": {"7"}, "n": {"7"}, "giveup": {"1"}}),
	}
	got := golden{Pack: json.RawMessage(packBody)}
	for _, s := range steps {
		got.Steps = append(got.Steps, g.goldenStep(lang, s))
	}

	if os.Getenv("KENA_UPDATE_GOLDEN") != "" {
		b, err := json.MarshalIndent(got, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenFile, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	var want golden
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(comparablePack(t, got.Pack), comparablePack(t, want.Pack)) {
		t.Errorf("%s: pack is stale; regenerate with KENA_UPDATE_GOLDEN=1", goldenFile)
	}
	if len(got.Steps) != len(want.Steps) {
		t.Fatalf("%s: %d steps, want %d; regenerate with KENA_UPDATE_GOLDEN=1", goldenFile, len(want.Steps), len(got.Steps))
	}
	for i := range got.Steps {
		if !reflect.DeepEqual(got.Steps[i], want.Steps[i]) {
			t.Errorf("%s: step %d (%s %s) is stale; regenerate with KENA_UPDATE_GOLDEN=1", goldenFile, i, got.Steps[i].Method, got.Steps[i].URL)
		}
	}
}

func comparablePack(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var p packJSON
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	secrets := make([]secret, len(p.Tasks))
	for i, task := range p.Tasks {
		secrets[i] = reveal(t, p.Key, task.X)
		m["tasks"].([]any)[i].(map[string]any)["x"] = nil
	}
	m["secrets"] = fmt.Sprint(secrets)
	delete(m, "key")
	delete(m, "version")
	return m
}
