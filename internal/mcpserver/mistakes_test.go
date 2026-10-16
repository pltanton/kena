package mcpserver

import (
	"context"
	"strings"
	"testing"
)

func (e *env) attempt(t *testing.T, user string, taskID int64, correct bool, answers ...string) {
	t.Helper()
	ctx := context.Background()
	u, err := e.st.UserByName(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.RecordAttempt(ctx, u.ProgressID, taskID, answers, correct); err != nil {
		t.Fatal(err)
	}
}

func TestMistakesReport(t *testing.T) {
	e := newEnv(t)
	alice := e.connect(t, "alice")
	bob := e.connect(t, "bob")
	out := ok[WriteResult](t, alice, "create_set", setArgs("Αόριστος", validTasks()))
	detail := ok[SetDetail](t, alice, "get_set", map[string]any{"set": out.Slug})
	t0, t1 := detail.Tasks[0].ID, detail.Tasks[1].ID
	other := ok[WriteResult](t, bob, "create_set", setArgs("Bob set", validTasks()))

	e.attempt(t, "alice", t0, false, "εγραψε")
	e.attempt(t, "alice", t0, false, "")
	e.attempt(t, "alice", t1, false, "γράψατε")
	e.attempt(t, "alice", t1, true, "γράψαμε")
	e.attempt(t, "bob", t0, false, "λάθος")

	rep := ok[MistakesReport](t, alice, "get_mistakes_report", map[string]any{})
	if rep.WindowDays != 30 || rep.Note != "" || len(rep.Sets) != 1 {
		t.Fatalf("report = %+v", rep)
	}
	set := rep.Sets[0]
	if set.Slug != out.Slug || set.URL != out.URL || set.TasksTried != 2 || set.Accuracy != 0.25 || len(set.Tasks) != 2 {
		t.Fatalf("set = %+v", set)
	}
	first := set.Tasks[0]
	if first.TaskID != t0 || first.WrongCount != 2 || first.AttemptCount != 2 || first.Fixed {
		t.Errorf("first = %+v", first)
	}
	if len(first.Blanks) != 1 || first.Blanks[0].Answers[0] != "έγραψε" || first.Blanks[0].Hint != "γράφω" {
		t.Errorf("blanks = %+v", first.Blanks)
	}
	if len(first.RecentWrong) != 2 || first.RecentWrong[0].Kinds[0] != "empty" || first.RecentWrong[1].Kinds[0] != "accent" ||
		first.RecentWrong[1].Given[0] != "εγραψε" || first.RecentWrong[0].At == "" {
		t.Errorf("recent = %+v", first.RecentWrong)
	}
	second := set.Tasks[1]
	if second.TaskID != t1 || !second.Fixed || second.RecentWrong[0].Kinds[0] != "near_miss" {
		t.Errorf("second = %+v", second)
	}
	if k := rep.Summary.ByKind; k.Accent != 1 || k.Empty != 1 || k.NearMiss != 1 || k.FinalSigma != 0 || k.Other != 0 {
		t.Errorf("by_kind = %+v", k)
	}
	if len(rep.Summary.TopUnfixed) != 1 || rep.Summary.TopUnfixed[0].TaskID != t0 || rep.Summary.TopUnfixed[0].Set != out.Slug {
		t.Errorf("top_unfixed = %+v", rep.Summary.TopUnfixed)
	}

	bobRep := ok[MistakesReport](t, bob, "get_mistakes_report", map[string]any{})
	if len(bobRep.Sets) != 1 || len(bobRep.Sets[0].Tasks) != 1 || bobRep.Sets[0].Tasks[0].RecentWrong[0].Given[0] != "λάθος" {
		t.Fatalf("bob sees %+v", bobRep)
	}
	if k := bobRep.Summary.ByKind; k.Other != 1 || k.Accent != 0 {
		t.Errorf("bob by_kind = %+v", k)
	}

	filtered := ok[MistakesReport](t, alice, "get_mistakes_report", map[string]any{"set": other.Slug, "days": 7})
	if len(filtered.Sets) != 0 || filtered.WindowDays != 7 || !strings.Contains(filtered.Note, "nothing to analyse") {
		t.Fatalf("filtered = %+v", filtered)
	}
	if bySlug := ok[MistakesReport](t, alice, "get_mistakes_report", map[string]any{"set": out.Slug}); len(bySlug.Sets) != 1 {
		t.Fatalf("by slug = %+v", bySlug)
	}

	empty := ok[MistakesReport](t, e.connect(t, "root"), "get_mistakes_report", map[string]any{})
	if empty.Sets == nil || len(empty.Sets) != 0 || empty.Note == "" || empty.Summary.TopUnfixed == nil {
		t.Fatalf("empty = %+v", empty)
	}

	fails(t, alice, "get_mistakes_report", map[string]any{"set": "nope"}, `set "nope" not found`)
	fails(t, alice, "get_mistakes_report", map[string]any{"days": 400}, "365")
	fails(t, alice, "get_mistakes_report", map[string]any{"days": -1}, "365")
}

func TestMistakesReportReadOnly(t *testing.T) {
	e := newEnv(t)
	res, err := e.connect(t, "alice").ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "get_mistakes_report" {
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !strings.Contains(tool.Description, "analyze-mistakes") {
				t.Fatalf("tool = %+v", tool)
			}
			return
		}
	}
	t.Fatal("get_mistakes_report not listed")
}
