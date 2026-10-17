package store_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func TestDeckRulesRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	u, err := s.CreateUser(ctx, "anna", false)
	if err != nil {
		t.Fatal(err)
	}
	rules := []greek.Rule{{Key: "agree", Text: "Το πόσος συμφωνεί με το ουσιαστικό."}}
	set, err := s.CreateSet(ctx, u.ID, store.SetInput{Title: "Rules", Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetSet(ctx, set.Slug); !reflect.DeepEqual(got.Rules, rules) {
		t.Fatalf("rules = %+v", got.Rules)
	}
	plain, _ := s.CreateSet(ctx, u.ID, store.SetInput{Title: "Plain"})
	if plain.Rules == nil || len(plain.Rules) != 0 {
		t.Fatalf("default rules = %#v", plain.Rules)
	}

	tasks, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{
		{Text: "Πόσους ___;", Blanks: []greek.Blank{{Answers: []string{"φίλους"}}}, Rule: "agree"},
		{Text: "Πόσες ___;", Blanks: []greek.Blank{{Answers: []string{"φίλες"}}}},
	})
	if err != nil || tasks[0].Rule != "agree" || tasks[1].Rule != "" {
		t.Fatalf("AddTasks = %+v, %v", tasks, err)
	}
	none := ""
	if got, err := s.UpdateTask(ctx, tasks[0].ID, store.TaskPatch{Rule: &none}); err != nil || got.Rule != "" {
		t.Fatalf("clear rule: %+v, %v", got, err)
	}
	key := "agree"
	if got, err := s.UpdateTask(ctx, tasks[1].ID, store.TaskPatch{Rule: &key}); err != nil || got.Rule != "agree" {
		t.Fatalf("set rule: %+v, %v", got, err)
	}
	if got, _ := s.GetTask(ctx, tasks[1].ID); got.Rule != "agree" {
		t.Fatalf("GetTask rule = %q", got.Rule)
	}

	more := append(rules, greek.Rule{Key: "polys", Text: "πολλοί, πολλές, πολλά"})
	if got, err := s.UpdateSet(ctx, set.ID, store.SetPatch{Rules: &more}); err != nil || len(got.Rules) != 2 {
		t.Fatalf("UpdateSet rules: %+v, %v", got.Rules, err)
	}
	current, _ := s.GetSet(ctx, set.Slug)
	only := more[1:]
	ch, err := s.SaveDeck(ctx, set.ID, current.UpdatedAt, store.SetPatch{Rules: &only}, []greek.TaskInput{
		{Text: "Πόσους ___;", Blanks: []greek.Blank{{Answers: []string{"φίλους"}}}, Rule: "polys"},
		{Text: "Πόσες ___;", Blanks: []greek.Blank{{Answers: []string{"φίλες"}}}, Rule: "polys"},
	})
	if err != nil || ch.Updated != 2 {
		t.Fatalf("SaveDeck = %+v, %v", ch, err)
	}
	list, _ := s.ListTasks(ctx, set.ID)
	if list[0].Rule != "polys" || list[1].Rule != "polys" {
		t.Fatalf("tasks after SaveDeck = %+v", list)
	}
	if got, _ := s.GetSet(ctx, set.Slug); !reflect.DeepEqual(got.Rules, only) {
		t.Fatalf("rules after SaveDeck = %+v", got.Rules)
	}

	device := "11111111-1111-1111-1111-111111111111"
	if err := s.RecordAttempt(ctx, device, list[0].ID, []string{"λάθος"}, false); err != nil {
		t.Fatal(err)
	}
	rep, err := s.MistakesReport(ctx, u.ID, device, nil, time.Now().Add(-time.Hour))
	if err != nil || len(rep.Sets) != 1 || rep.Sets[0].Tasks[0].RuleText != "πολλοί, πολλές, πολλά" {
		t.Fatalf("mistakes report rule = %+v, %v", rep, err)
	}
}
