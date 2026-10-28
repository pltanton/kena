package mcpserver

import "testing"

func TestChoicesOverMCP(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	choice := map[string]any{"text": "Χθες ___ στο σινεμά.", "blanks": []map[string]any{{"answers": []string{"πήγα"}, "hint": "πάω", "choices": []string{"πηγαίνω", "θα πάω"}}}}
	bad := map[string]any{"text": "Χθες ___ στο σινεμά.", "blanks": []map[string]any{{"answers": []string{"πήγα"}, "choices": []string{"Πήγα"}}}}

	rep := ok[LintReport](t, cs, "validate_tasks", map[string]any{"tasks": []any{bad}})
	if len(rep.Errors) != 1 {
		t.Fatalf("validate_tasks = %+v", rep)
	}
	fails(t, cs, "create_set", map[string]any{"title": "C", "topic": "t", "level": "A2", "instructions": "Διαλέξτε.", "tasks": []any{bad}}, "task[0].blanks[0].choices[0]")
	res := ok[WriteResult](t, cs, "create_set", map[string]any{"title": "C", "topic": "t", "level": "A2", "instructions": "Διαλέξτε.", "tasks": []any{choice}})
	d := ok[SetDetail](t, cs, "get_set", map[string]any{"set": res.Slug})
	if got := d.Tasks[0].Blanks[0].Choices; len(got) != 2 || got[0] != "πηγαίνω" {
		t.Fatalf("choices = %q", got)
	}
}

func TestQuestionOverMCP(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	q := map[string]any{"kind": "choice", "text": "Ποιος χρόνος είναι το «έγραψα»;", "blanks": []map[string]any{{"answers": []string{"Αόριστος"}, "choices": []string{"Ενεστώτας", "Παρατατικός"}}}}
	bad := map[string]any{"kind": "choice", "text": "Ποιος χρόνος είναι το ___;", "blanks": []map[string]any{{"answers": []string{"Αόριστος"}}}}
	fails(t, cs, "create_set", map[string]any{"title": "Q", "topic": "t", "level": "A2", "instructions": "Διαλέξτε.", "tasks": []any{bad}}, "task[0].text", "task[0].blanks[0].choices")
	res := ok[WriteResult](t, cs, "create_set", map[string]any{"title": "Q", "topic": "t", "level": "A2", "instructions": "Διαλέξτε.",
		"tasks": []any{q, task("Χθες ___ γράμμα.", "έγραψα")}})
	d := ok[SetDetail](t, cs, "get_set", map[string]any{"set": res.Slug})
	if d.Tasks[0].Kind != "choice" || d.Tasks[1].Kind != "" {
		t.Fatalf("kinds = %q, %q", d.Tasks[0].Kind, d.Tasks[1].Kind)
	}
	fails(t, cs, "update_task", map[string]any{"task_id": d.Tasks[1].ID, "kind": "choice"}, "task[0]")
	upd := ok[TaskResult](t, cs, "update_task", map[string]any{"task_id": d.Tasks[0].ID, "text": "Ποιος χρόνος είναι το «έγραφα»;",
		"blanks": []map[string]any{{"answers": []string{"Παρατατικός"}, "choices": []string{"Αόριστος"}}}})
	if upd.Task.Kind != "choice" || upd.Task.Blanks[0].Answers[0] != "Παρατατικός" {
		t.Fatalf("update_task = %+v", upd.Task)
	}
}
