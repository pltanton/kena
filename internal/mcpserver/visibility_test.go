package mcpserver

import (
	"fmt"
	"strings"
	"testing"
)

func TestVisibility(t *testing.T) {
	e := newEnv(t)
	alice, bob, root := e.connect(t, "alice"), e.connect(t, "bob"), e.connect(t, "root")

	priv := ok[WriteResult](t, alice, "create_set", setArgs("Private one", validTasks()))
	if priv.Visibility != "private" {
		t.Fatalf("create_set default visibility = %q", priv.Visibility)
	}
	friends := ok[WriteResult](t, alice, "create_set", visibility(setArgs("Friends one", nil), "friends"))
	pub := ok[WriteResult](t, alice, "create_set", visibility(setArgs("Public one", nil), "public"))
	if friends.Visibility != "friends" || pub.Visibility != "public" {
		t.Fatalf("create_set ignored visibility: %q %q", friends.Visibility, pub.Visibility)
	}
	fails(t, alice, "create_set", visibility(setArgs("Bad", nil), "everyone"), "visibility", "private, friends or public")

	slugs := func(list SetList) map[string]string {
		m := map[string]string{}
		for _, s := range list.Sets {
			m[s.Slug] = s.Visibility
		}
		return m
	}
	if got := slugs(ok[SetList](t, bob, "list_sets", map[string]any{})); len(got) != 2 || got[pub.Slug] != "public" || got[friends.Slug] != "friends" {
		t.Fatalf("bob lists %v", got)
	}
	if got := slugs(ok[SetList](t, alice, "list_sets", map[string]any{})); len(got) != 3 || got[priv.Slug] != "private" {
		t.Fatalf("alice lists %v", got)
	}
	if got := slugs(ok[SetList](t, root, "list_sets", map[string]any{})); len(got) != 3 {
		t.Fatalf("admin lists %v", got)
	}
	if d := ok[SetDetail](t, bob, "get_set", map[string]any{"set": friends.Slug}); d.Visibility != "friends" {
		t.Fatalf("bob get_set friends = %+v", d)
	}

	ref := map[string]any{"set": priv.Slug}
	fails(t, bob, "get_set", ref, "not found")
	fails(t, bob, "update_set", map[string]any{"set": priv.Slug, "title": "x"}, "not found")
	fails(t, bob, "add_tasks", map[string]any{"set": priv.Slug, "tasks": validTasks()}, "not found")
	fails(t, bob, "validate_tasks", map[string]any{"set": priv.Slug, "tasks": validTasks()}, "not found")
	fails(t, bob, "get_mistakes_report", ref, "not found")
	fails(t, bob, "delete_set", ref, "not found")
	privTask := ok[SetDetail](t, alice, "get_set", ref).Tasks[0].ID
	for _, tool := range []string{"update_task", "delete_task"} {
		args := map[string]any{"task_id": privTask, "explanation": "x"}
		if tool == "delete_task" {
			args = map[string]any{"task_id": privTask}
		}
		msg := fails(t, bob, tool, args, fmt.Sprintf("task %d not found", privTask))
		if strings.Contains(msg, "set") {
			t.Errorf("%s leaks the set: %q", tool, msg)
		}
	}
	if d := ok[SetDetail](t, root, "get_set", ref); d.Visibility != "private" || len(d.Tasks) != 2 {
		t.Fatalf("admin get_set = %+v", d)
	}
	if d := ok[SetDetail](t, alice, "get_set", ref); d.Owner != "alice" {
		t.Fatalf("owner get_set = %+v", d)
	}

	fails(t, alice, "update_set", map[string]any{"set": priv.Slug, "visibility": "shared"}, "private, friends or public")
	upd := ok[SetDetail](t, alice, "update_set", map[string]any{"set": priv.Slug, "visibility": "friends"})
	if upd.Visibility != "friends" {
		t.Fatalf("update_set visibility = %+v", upd)
	}
	if d := ok[SetDetail](t, bob, "get_set", ref); d.Visibility != "friends" {
		t.Fatalf("bob get_set after sharing = %+v", d)
	}
	fails(t, bob, "update_set", map[string]any{"set": priv.Slug, "visibility": "private"}, "belongs to alice")
}
