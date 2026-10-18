package mcpserver

import "testing"

func TestPublicRulesOverMCP(t *testing.T) {
	e := newEnv(t)
	alice := e.connect(t, "alice")
	bob := e.connect(t, "bob")
	root := e.connect(t, "root")

	fails(t, alice, "create_rule_category", map[string]any{"name": "ellenika"}, "only admins")
	ok[PublicCategory](t, root, "create_rule_category", map[string]any{"name": "ellenika"})

	fails(t, alice, "set_public_rule", map[string]any{"id": "nope/posos", "body": "x"}, "does not exist")
	fails(t, alice, "set_public_rule", map[string]any{"id": "ellenika/posos", "body": ""}, "body")
	out := ok[PublicRuleResult](t, alice, "set_public_rule", map[string]any{"id": "ellenika/posos", "title": "Πόσος", "body": "Το πόσος συμφωνεί."})
	if !out.Rule.CanEdit || !out.Created || out.Rule.Creator != "alice" || out.Rule.ID != "ellenika/posos" {
		t.Fatalf("set_public_rule = %+v", out)
	}
	fails(t, bob, "set_public_rule", map[string]any{"id": "ellenika/posos", "body": "x"}, "creator or an admin")
	if got := ok[PublicRuleResult](t, root, "set_public_rule", map[string]any{"id": "ellenika/posos", "body": "Νέο."}); got.Created || got.Rule.Title != "Πόσος" || got.Rule.Body != "Νέο." {
		t.Fatalf("admin update keeps title: %+v", got)
	}

	list := ok[PublicRuleList](t, bob, "list_public_rules", map[string]any{})
	if len(list.Categories) != 1 || len(list.Categories[0].Rules) != 1 || list.Categories[0].Rules[0].ID != "ellenika/posos" {
		t.Fatalf("list_public_rules = %+v", list)
	}

	res := ok[WriteResult](t, bob, "create_set", map[string]any{"title": "R", "topic": "t", "level": "B1", "instructions": "Συμπληρώστε.",
		"tasks": []any{ruleTask("Χθες η Μαρία ___ γράμμα.", "έγραψε", "ellenika/posos")}})
	fails(t, bob, "add_tasks", map[string]any{"set": res.Slug, "tasks": []any{ruleTask("Τότε ___ πολύ.", "έγραψε", "ellenika/missing")}}, "task[0].rule")
	if rep := ok[LintReport](t, bob, "validate_tasks", map[string]any{"tasks": []any{ruleTask("Μετά ___ λίγο.", "έγραψε", "ellenika/posos")}}); len(rep.Errors) != 0 {
		t.Fatalf("validate_tasks with a public rule: %+v", rep)
	}
	fails(t, alice, "attach_public_rule", map[string]any{"set": res.Slug, "id": "ellenika/posos"}, "not found")
	att := ok[AttachResult](t, bob, "attach_public_rule", map[string]any{"set": res.Slug, "id": "ellenika/posos"})
	if len(att.PublicRules) != 1 || !att.PublicRules[0].Attached {
		t.Fatalf("attach = %+v", att)
	}
	d := ok[SetDetail](t, bob, "get_set", map[string]any{"set": res.Slug})
	if len(d.PublicRules) != 1 || d.PublicRules[0].Body != "Νέο." || d.Tasks[0].Rule != "ellenika/posos" {
		t.Fatalf("get_set = %+v", d)
	}
	if got := ok[PublicRuleOut](t, bob, "get_public_rule", map[string]any{"id": "ellenika/posos"}); got.UsedDecks != 1 || got.UsedTasks != 1 || got.CanEdit {
		t.Fatalf("get_public_rule = %+v", got)
	}

	fails(t, alice, "delete_public_rule", map[string]any{"id": "ellenika/posos"}, "used by 1 tasks in 1 decks")
	fails(t, root, "admin_delete_user", map[string]any{"username": "alice", "confirm_username": "alice"}, "transfer")
	if moved := ok[TransferResult](t, root, "admin_transfer_public_rules", map[string]any{"from": "alice", "to": "bob"}); moved.Moved != 1 {
		t.Fatalf("transfer = %+v", moved)
	}

	ok[AttachResult](t, bob, "detach_public_rule", map[string]any{"set": res.Slug, "id": "ellenika/posos"})
	ok[TaskResult](t, bob, "update_task", map[string]any{"task_id": d.Tasks[0].ID, "rule": ""})
	ok[DeleteResult](t, bob, "delete_public_rule", map[string]any{"id": "ellenika/posos"})
	fails(t, bob, "get_public_rule", map[string]any{"id": "ellenika/posos"}, "not found")
}
