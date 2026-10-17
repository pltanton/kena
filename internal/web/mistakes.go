package web

import (
	"net/http"
	"net/url"
	"time"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

const (
	mistakesDays = 30
	askPrompt    = "Analyze my kena mistakes and make a practice set for my weak spots."
)

var kindLabels = map[greek.ErrorKind]string{
	greek.KindAccent:     "accent",
	greek.KindFinalSigma: "ς",
	greek.KindNearMiss:   "near miss",
}

type mistakeRow struct {
	Task       taskView
	Kinds      []string
	WrongCount int
	Fixed      bool
	Notes      notes
}

type mistakeSet struct {
	Title string
	URL   string
	Tasks []mistakeRow
}

type mistakesView struct {
	Days   int
	Prompt string
	Sets   []mistakeSet
}

func newMistakeRow(tm store.TaskMistakes) mistakeRow {
	row := mistakeRow{WrongCount: tm.WrongCount, Fixed: tm.LastCorrect, Notes: notes{Explanation: tm.Task.Explanation, Rule: tm.RuleText}}
	var given []string
	if len(tm.RecentWrong) > 0 {
		given = tm.RecentWrong[0].Answers
	}
	results, _ := greek.Check(tm.Task.Blanks, given)
	row.Task = newTaskView(tm.Task, false, results)
	seen := map[string]bool{}
	for i, b := range tm.Task.Blanks {
		if results[i].Correct {
			continue
		}
		if label, ok := kindLabels[greek.Classify(results[i].Given, b.Answers)]; ok && !seen[label] {
			seen[label] = true
			row.Kinds = append(row.Kinds, label)
		}
	}
	return row
}

func (h *handler) mistakes(w http.ResponseWriter, r *http.Request, u store.User) {
	rep, err := h.st.MistakesReport(r.Context(), u.ID, u.ProgressID, nil, time.Now().AddDate(0, 0, -mistakesDays))
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	v := mistakesView{Days: mistakesDays, Prompt: askPrompt}
	for _, sm := range rep.Sets {
		ms := mistakeSet{Title: sm.Set.Title, URL: "/s/" + url.PathEscape(sm.Set.Slug)}
		for _, tm := range sm.Tasks {
			ms.Tasks = append(ms.Tasks, newMistakeRow(tm))
		}
		v.Sets = append(v.Sets, ms)
	}
	h.page(w, r, http.StatusOK, "mistakes", v)
}
