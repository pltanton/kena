package mcpserver

import "testing"

func TestSharing(t *testing.T) {
	e := newEnv(t)
	alice, bob, root := e.connect(t, "alice"), e.connect(t, "bob"), e.connect(t, "root")

	priv := ok[WriteResult](t, alice, "create_set", setArgs("Private one", validTasks()))
	if priv.Shared {
		t.Fatal("create_set shares by default")
	}
	pub := ok[WriteResult](t, alice, "create_set", shared(setArgs("Public one", nil)))
	if !pub.Shared {
		t.Fatal("create_set ignored shared")
	}

	slugs := func(list SetList) map[string]bool {
		m := map[string]bool{}
		for _, s := range list.Sets {
			m[s.Slug] = s.Shared
		}
		return m
	}
	if got := slugs(ok[SetList](t, bob, "list_sets", map[string]any{})); len(got) != 1 || !got[pub.Slug] {
		t.Fatalf("bob lists %v", got)
	}
	if got := slugs(ok[SetList](t, alice, "list_sets", map[string]any{})); len(got) != 2 || got[priv.Slug] {
		t.Fatalf("alice lists %v", got)
	}
	if got := slugs(ok[SetList](t, root, "list_sets", map[string]any{})); len(got) != 2 {
		t.Fatalf("admin lists %v", got)
	}

	ref := map[string]any{"set": priv.Slug}
	fails(t, bob, "get_set", ref, "not found")
	fails(t, bob, "update_set", map[string]any{"set": priv.Slug, "title": "x"}, "not found")
	fails(t, bob, "add_tasks", map[string]any{"set": priv.Slug, "tasks": validTasks()}, "not found")
	fails(t, bob, "validate_tasks", map[string]any{"set": priv.Slug, "tasks": validTasks()}, "not found")
	fails(t, bob, "get_mistakes_report", ref, "not found")
	if d := ok[SetDetail](t, root, "get_set", ref); d.Shared || len(d.Tasks) != 2 {
		t.Fatalf("admin get_set = %+v", d)
	}
	if d := ok[SetDetail](t, alice, "get_set", ref); d.Owner != "alice" {
		t.Fatalf("owner get_set = %+v", d)
	}

	upd := ok[SetDetail](t, alice, "update_set", map[string]any{"set": priv.Slug, "shared": true})
	if !upd.Shared {
		t.Fatalf("update_set shared = %+v", upd)
	}
	if d := ok[SetDetail](t, bob, "get_set", ref); !d.Shared {
		t.Fatalf("bob get_set after sharing = %+v", d)
	}
	fails(t, bob, "update_set", map[string]any{"set": priv.Slug, "shared": false}, "belongs to alice")
}
