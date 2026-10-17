package mcpserver

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

const (
	defaultMistakeDays = 30
	maxMistakeDays     = 365
	topUnfixedLimit    = 10
)

type MistakesIn struct {
	Set  string `json:"set,omitempty" jsonschema:"only this set (id or slug); omit for all sets"`
	Days int    `json:"days,omitempty" jsonschema:"window in days, 1-365; default 30"`
}

type WrongAttemptOut struct {
	Given []string `json:"given"`
	Kinds []string `json:"kinds"`
	At    string   `json:"at"`
}

type MistakeTask struct {
	TaskID       int64             `json:"task_id"`
	Text         string            `json:"text"`
	Blanks       []BlankIn         `json:"blanks"`
	Explanation  string            `json:"explanation,omitempty"`
	WrongCount   int               `json:"wrong_count"`
	AttemptCount int               `json:"attempt_count"`
	Fixed        bool              `json:"fixed"`
	RecentWrong  []WrongAttemptOut `json:"recent_wrong"`
}

type MistakeSet struct {
	ID         int64         `json:"id"`
	Slug       string        `json:"slug"`
	Title      string        `json:"title"`
	Topic      string        `json:"topic"`
	Level      string        `json:"level"`
	URL        string        `json:"url"`
	TasksTried int           `json:"tasks_tried"`
	Accuracy   float64       `json:"accuracy"`
	Tasks      []MistakeTask `json:"tasks"`
}

type KindCounts struct {
	Accent     int `json:"accent"`
	FinalSigma int `json:"final_sigma"`
	NearMiss   int `json:"near_miss"`
	Other      int `json:"other"`
	Empty      int `json:"empty"`
}

type UnfixedTask struct {
	TaskID     int64  `json:"task_id"`
	Set        string `json:"set"`
	Text       string `json:"text"`
	WrongCount int    `json:"wrong_count"`
}

type MistakesSummary struct {
	ByKind     KindCounts    `json:"by_kind"`
	TopUnfixed []UnfixedTask `json:"top_unfixed"`
}

type MistakesReport struct {
	WindowDays int             `json:"window_days"`
	Sets       []MistakeSet    `json:"sets"`
	Summary    MistakesSummary `json:"summary"`
	Note       string          `json:"note,omitempty"`
}

func (k *KindCounts) add(kind greek.ErrorKind) {
	switch kind {
	case greek.KindAccent:
		k.Accent++
	case greek.KindFinalSigma:
		k.FinalSigma++
	case greek.KindNearMiss:
		k.NearMiss++
	case greek.KindOther:
		k.Other++
	case greek.KindEmpty:
		k.Empty++
	}
}

func (s *server) addMistakeTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "get_mistakes_report",
		Description: "Read your own wrong answers on the kena site (the logged-in learner's progress), to find weak spots and build a practice set. " +
			"Read get_authoring_guide topic analyze-mistakes first. " +
			"Returns per set the tasks with wrong attempts in the window: text, blanks (accepted answers, hint), explanation, wrong_count, attempt_count, " +
			"fixed (latest attempt correct) and up to 5 recent wrong attempts with the given answers and a kind per blank " +
			"(accent, final_sigma, near_miss, other, empty, correct). summary.by_kind counts those blanks; summary.top_unfixed lists the worst unfixed tasks.",
		Annotations: readOnly("Mistakes report"),
	}, handle(func(ctx context.Context, u store.User, in MistakesIn) (MistakesReport, error) {
		days := in.Days
		if days == 0 {
			days = defaultMistakeDays
		}
		if days < 1 || days > maxMistakeDays {
			return MistakesReport{}, fail("days must be 1 to %d", maxMistakeDays)
		}
		var setID *int64
		if in.Set != "" {
			st, err := s.visibleSet(ctx, u, in.Set)
			if err != nil {
				return MistakesReport{}, err
			}
			setID = &st.ID
		}
		if u.ProgressID == "" {
			fresh, err := s.st.UserByID(ctx, u.ID)
			if err != nil {
				return MistakesReport{}, err
			}
			u = fresh
		}
		rep, err := s.st.MistakesReport(ctx, u.ID, u.ProgressID, setID, time.Now().AddDate(0, 0, -days))
		if err != nil {
			return MistakesReport{}, err
		}
		return s.mistakesOut(rep, days), nil
	}))
}

func (s *server) mistakesOut(rep store.Report, days int) MistakesReport {
	out := MistakesReport{WindowDays: days, Sets: []MistakeSet{}, Summary: MistakesSummary{TopUnfixed: []UnfixedTask{}}}
	var unfixed []UnfixedTask
	for _, sm := range rep.Sets {
		ms := MistakeSet{ID: sm.Set.ID, Slug: sm.Set.Slug, Title: sm.Set.Title, Topic: sm.Set.Topic, Level: sm.Set.Level,
			URL: s.url(sm.Set.Slug), TasksTried: sm.TasksTried}
		if sm.Attempts > 0 {
			ms.Accuracy = math.Round(float64(sm.Correct)/float64(sm.Attempts)*100) / 100
		}
		for _, tm := range sm.Tasks {
			mt := MistakeTask{TaskID: tm.Task.ID, Text: tm.Task.Text, Blanks: fromGreekBlanks(tm.Task.Blanks),
				Explanation: tm.Task.Explanation, WrongCount: tm.WrongCount, AttemptCount: tm.AttemptCount,
				Fixed: tm.LastCorrect, RecentWrong: []WrongAttemptOut{}}
			for _, wa := range tm.RecentWrong {
				wo := WrongAttemptOut{Given: wa.Answers, Kinds: make([]string, len(tm.Task.Blanks)), At: stamp(wa.CreatedAt)}
				for i, b := range tm.Task.Blanks {
					var given string
					if i < len(wa.Answers) {
						given = wa.Answers[i]
					}
					kind := greek.Classify(given, b.Answers)
					wo.Kinds[i] = string(kind)
					out.Summary.ByKind.add(kind)
				}
				mt.RecentWrong = append(mt.RecentWrong, wo)
			}
			ms.Tasks = append(ms.Tasks, mt)
			if !tm.LastCorrect {
				unfixed = append(unfixed, UnfixedTask{TaskID: tm.Task.ID, Set: sm.Set.Slug, Text: tm.Task.Text, WrongCount: tm.WrongCount})
			}
		}
		out.Sets = append(out.Sets, ms)
	}
	slices.SortStableFunc(unfixed, func(a, b UnfixedTask) int { return cmp.Compare(b.WrongCount, a.WrongCount) })
	if len(unfixed) > topUnfixedLimit {
		unfixed = unfixed[:topUnfixedLimit]
	}
	out.Summary.TopUnfixed = append(out.Summary.TopUnfixed, unfixed...)
	if len(out.Sets) == 0 {
		out.Note = "No wrong answers in the last " + strconv.Itoa(days) + " days: nothing to analyse yet. " +
			"Suggest the learner practise a set on the site (logged in) first."
	}
	return out
}
