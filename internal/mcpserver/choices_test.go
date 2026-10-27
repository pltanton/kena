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
