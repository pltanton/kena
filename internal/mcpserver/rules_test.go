package mcpserver

import (
	"testing"
)

func ruleTask(text, answer, rule string) map[string]any {
	m := task(text, answer)
	if rule != "" {
		m["rule"] = rule
	}
	return m
}

func TestDeckRulesOverMCP(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	rules := []map[string]any{{"key": "past", "text": "Αόριστος για μία ολοκληρωμένη πράξη στο παρελθόν."}}

	fails(t, cs, "create_set", map[string]any{"title": "R", "topic": "t", "level": "B1", "instructions": "Συμπληρώστε.", "tasks": []any{ruleTask("Χθες η Μαρία ___ γράμμα.", "έγραψε", "nope")}},
		"task[0].rule", `"nope"`)
	fails(t, cs, "create_set", map[string]any{"title": "R", "topic": "t", "level": "B1", "instructions": "Συμπληρώστε.", "rules": []map[string]any{{"key": "Bad Key", "text": "x"}}}, "rules[0].key")

	res := ok[WriteResult](t, cs, "create_set", map[string]any{"title": "R", "topic": "t", "level": "B1", "instructions": "Συμπληρώστε.", "rules": rules,
		"tasks": []any{ruleTask("Χθες η Μαρία ___ γράμμα.", "έγραψε", "past"), ruleTask("Πέρσι ___ βιβλία.", "γράψαμε", "")}})
	d := ok[SetDetail](t, cs, "get_set", map[string]any{"set": res.Slug})
	if len(d.Rules) != 1 || d.Rules[0].Key != "past" || d.Tasks[0].Rule != "past" || d.Tasks[1].Rule != "" {
		t.Fatalf("get_set = %+v", d)
	}

	fails(t, cs, "add_tasks", map[string]any{"set": res.Slug, "tasks": []any{ruleTask("Τότε ___ πολύ.", "έγραψε", "habit")}}, "task[0].rule")
	ok[WriteResult](t, cs, "add_tasks", map[string]any{"set": res.Slug, "tasks": []any{ruleTask("Τότε ___ πολύ.", "έγραψε", "past")}})

	rep := ok[LintReport](t, cs, "validate_tasks", map[string]any{"set": res.Slug, "tasks": []any{ruleTask("Μετά ___ λίγο.", "έγραψε", "past")}})
	if len(rep.Errors) != 0 {
		t.Fatalf("validate_tasks against the set's rules: %+v", rep)
	}
	rep = ok[LintReport](t, cs, "validate_tasks", map[string]any{"rules": []map[string]any{{"key": "x", "text": "y"}},
		"tasks": []any{ruleTask("Μετά ___ λίγο.", "έγραψε", "past")}})
	if len(rep.Errors) != 1 {
		t.Fatalf("validate_tasks with explicit rules: %+v", rep)
	}

	fails(t, cs, "update_set", map[string]any{"set": res.Slug, "rules": []map[string]any{{"key": "other", "text": "z"}}},
		`"past"`, "still used", "task")
	more := append(rules, map[string]any{"key": "habit", "text": "Παρατατικός για συνήθεια."})
	if got := ok[SetDetail](t, cs, "update_set", map[string]any{"set": res.Slug, "rules": more}); len(got.Rules) != 2 {
		t.Fatalf("update_set rules = %+v", got.Rules)
	}

	upd := ok[TaskResult](t, cs, "update_task", map[string]any{"task_id": d.Tasks[1].ID, "rule": "habit"})
	if upd.Task.Rule != "habit" {
		t.Fatalf("update_task rule = %+v", upd.Task)
	}
	fails(t, cs, "update_task", map[string]any{"task_id": d.Tasks[1].ID, "rule": "nope"}, "task[0].rule")
	upd = ok[TaskResult](t, cs, "update_task", map[string]any{"task_id": d.Tasks[1].ID, "rule": ""})
	if upd.Task.Rule != "" {
		t.Fatalf("clearing rule: %+v", upd.Task)
	}
}
