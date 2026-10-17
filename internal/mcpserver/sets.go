package mcpserver

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

const authoringRules = "Rules: call get_authoring_guide first if you have not yet. " +
	"Write new sentences only; never copy, translate back or lightly edit the book's sentences. " +
	"One ___ (three underscores) per blank; blanks listed in the order the ___ appear. " +
	"answers = every accepted form, accents exact: checking is strict, a missing accent or wrong final ς marks the learner wrong. " +
	"Monotonic Greek only, ς at word end, σ elsewhere. " +
	"hint = lemma or tag, never the answer. " +
	"explanation = one short sentence specific to its item (what makes this form, and the cue for this tense or case), never the same generic text on every task. " +
	"At most 50 tasks per create_set/add_tasks call; send larger sets in batches."

const lintNote = "Every task is linted: any error rejects the whole call and nothing is saved (errors name task[i] and the field); warnings are returned with the result."

type SetRef struct {
	Set string `json:"set" jsonschema:"set id or slug"`
}

type ListSetsIn struct {
	Owner string `json:"owner,omitempty" jsonschema:"only sets owned by this username; omit for all sets"`
}

type CreateSetIn struct {
	Title        string   `json:"title" jsonschema:"set title, Greek / English, e.g. Αόριστος ρημάτων / Simple past of verbs"`
	Topic        string   `json:"topic" jsonschema:"short grammar topic label, e.g. aorist"`
	Level        string   `json:"level" jsonschema:"CEFR level, e.g. B1"`
	Instructions string   `json:"instructions" jsonschema:"one Greek line telling the learner what to do"`
	SourceRef    string   `json:"source_ref,omitempty" jsonschema:"book title, unit, exercise number the spec was taken from"`
	Slug         string   `json:"slug,omitempty" jsonschema:"URL slug; derived from the title when omitted"`
	Visibility   string   `json:"visibility,omitempty" jsonschema:"private (default: only you), friends (every logged-in user) or public (everyone, also without login); ask the user"`
	Tasks        []TaskIn `json:"tasks,omitempty" jsonschema:"first tasks of the set, at most 50; add the rest with add_tasks"`
}

type AddTasksIn struct {
	Set   string   `json:"set" jsonschema:"set id or slug"`
	Tasks []TaskIn `json:"tasks" jsonschema:"1 to 50 tasks to append"`
}

type UpdateSetIn struct {
	Set          string  `json:"set" jsonschema:"set id or slug"`
	Title        *string `json:"title,omitempty" jsonschema:"new title"`
	Topic        *string `json:"topic,omitempty" jsonschema:"new topic"`
	Level        *string `json:"level,omitempty" jsonschema:"new CEFR level"`
	Instructions *string `json:"instructions,omitempty" jsonschema:"new Greek instruction line"`
	SourceRef    *string `json:"source_ref,omitempty" jsonschema:"new source reference"`
	Visibility   *string `json:"visibility,omitempty" jsonschema:"private (only you), friends (every logged-in user) or public (everyone, also without login)"`
}

type UpdateTaskIn struct {
	TaskID      int64      `json:"task_id" jsonschema:"task id from get_set"`
	Text        *string    `json:"text,omitempty" jsonschema:"new sentence with one ___ per blank"`
	Blanks      *[]BlankIn `json:"blanks,omitempty" jsonschema:"new blanks, replacing all old ones; one per ___, in order"`
	Explanation *string    `json:"explanation,omitempty" jsonschema:"new explanation"`
}

type TaskRef struct {
	TaskID int64 `json:"task_id" jsonschema:"task id from get_set"`
}

type ValidateIn struct {
	Tasks []TaskIn `json:"tasks" jsonschema:"1 to 50 tasks to check"`
	Set   string   `json:"set,omitempty" jsonschema:"existing set id or slug; when given, tasks are also checked for duplicates against that set's tasks"`
}

type SetSummary struct {
	ID         int64  `json:"id"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Topic      string `json:"topic"`
	Level      string `json:"level"`
	Owner      string `json:"owner"`
	TaskCount  int    `json:"task_count"`
	Visibility string `json:"visibility"`
	URL        string `json:"url"`
}

type SetList struct {
	Sets []SetSummary `json:"sets"`
}

type TaskOut struct {
	ID          int64     `json:"id"`
	Position    int       `json:"position"`
	Text        string    `json:"text"`
	Blanks      []BlankIn `json:"blanks"`
	Explanation string    `json:"explanation,omitempty"`
}

type SetDetail struct {
	ID           int64     `json:"id"`
	Slug         string    `json:"slug"`
	Title        string    `json:"title"`
	Topic        string    `json:"topic"`
	Level        string    `json:"level"`
	Instructions string    `json:"instructions"`
	SourceRef    string    `json:"source_ref,omitempty"`
	Owner        string    `json:"owner"`
	TaskCount    int       `json:"task_count"`
	Visibility   string    `json:"visibility"`
	URL          string    `json:"url"`
	Tasks        []TaskOut `json:"tasks,omitempty"`
}

type WriteResult struct {
	SetID      int64    `json:"set_id"`
	Slug       string   `json:"slug"`
	URL        string   `json:"url"`
	Visibility string   `json:"visibility"`
	Added      int      `json:"added"`
	Total      int      `json:"total"`
	Warnings   []string `json:"warnings,omitempty"`
}

type TaskResult struct {
	SetID    int64    `json:"set_id"`
	URL      string   `json:"url"`
	Task     TaskOut  `json:"task"`
	Warnings []string `json:"warnings,omitempty"`
}

type DeleteResult struct {
	Deleted string `json:"deleted"`
}

func parseVisibility(v string) (store.Visibility, error) {
	vis, ok := store.ParseVisibility(v)
	if !ok {
		return "", fail("visibility %q: use private, friends or public", v)
	}
	return vis, nil
}

func (s *server) url(slug string) string { return s.publicURL + "/s/" + slug }

func (s *server) summary(st store.Set) SetSummary {
	return SetSummary{ID: st.ID, Slug: st.Slug, Title: st.Title, Topic: st.Topic, Level: st.Level,
		Owner: st.OwnerName, TaskCount: st.TaskCount, Visibility: string(st.Visibility), URL: s.url(st.Slug)}
}

func (s *server) detail(st store.Set, tasks []store.Task) SetDetail {
	d := SetDetail{ID: st.ID, Slug: st.Slug, Title: st.Title, Topic: st.Topic, Level: st.Level,
		Instructions: st.Instructions, SourceRef: st.SourceRef, Owner: st.OwnerName,
		TaskCount: st.TaskCount, Visibility: string(st.Visibility), URL: s.url(st.Slug)}
	for _, t := range tasks {
		d.Tasks = append(d.Tasks, taskOut(t))
	}
	return d
}

func taskOut(t store.Task) TaskOut {
	return TaskOut{ID: t.ID, Position: t.Position, Text: t.Text, Blanks: fromGreekBlanks(t.Blanks), Explanation: t.Explanation}
}

func (s *server) loadSet(ctx context.Context, ref string) (store.Set, error) {
	st, err := s.st.GetSet(ctx, ref)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrInvalid) {
		return store.Set{}, fail("set %q not found; use list_sets to find the id or slug", ref)
	}
	return st, err
}

func (s *server) visibleSet(ctx context.Context, u store.User, ref string) (store.Set, error) {
	st, err := s.loadSet(ctx, ref)
	if err != nil {
		return store.Set{}, err
	}
	if !u.IsAdmin && !store.CanView(u.ID, st) {
		return store.Set{}, fail("set %q not found; use list_sets to find the id or slug", ref)
	}
	return st, nil
}

func (s *server) editableSet(ctx context.Context, u store.User, ref string) (store.Set, error) {
	st, err := s.visibleSet(ctx, u, ref)
	if err != nil {
		return store.Set{}, err
	}
	if !store.CanEdit(u, st) {
		return store.Set{}, fail("set %q belongs to %s; you can only edit your own sets", st.Slug, st.OwnerName)
	}
	return st, nil
}

func (s *server) editableTask(ctx context.Context, u store.User, id int64) (store.Task, store.Set, error) {
	t, err := s.st.GetTask(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Task{}, store.Set{}, fail("task %d not found", id)
	}
	if err != nil {
		return store.Task{}, store.Set{}, err
	}
	st, err := s.st.GetSet(ctx, strconv.FormatInt(t.SetID, 10))
	if errors.Is(err, store.ErrNotFound) || (err == nil && !u.IsAdmin && !store.CanView(u.ID, st)) {
		return store.Task{}, store.Set{}, fail("task %d not found", id)
	}
	if err != nil {
		return store.Task{}, store.Set{}, err
	}
	if !store.CanEdit(u, st) {
		return store.Task{}, store.Set{}, fail("set %q belongs to %s; you can only edit your own sets", st.Slug, st.OwnerName)
	}
	return t, st, nil
}

func (s *server) existingTasks(ctx context.Context, setID int64, skipTask int64) ([]greek.TaskInput, error) {
	tasks, err := s.st.ListTasks(ctx, setID)
	if err != nil {
		return nil, err
	}
	out := make([]greek.TaskInput, 0, len(tasks))
	for _, t := range tasks {
		if t.ID != skipTask {
			out = append(out, greek.TaskInput{Text: t.Text, Explanation: t.Explanation})
		}
	}
	return out, nil
}

func (s *server) addSetTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_sets",
		Description: "List task sets you can see: your own sets plus other users' friends and public sets (admins: all). Each has id, slug, title, topic, level, owner, task_count, visibility and the learner-facing url. Filter by owner username.",
		Annotations: readOnly("List sets"),
	}, handle(func(ctx context.Context, u store.User, in ListSetsIn) (SetList, error) {
		sets, err := s.st.ListSets(ctx, store.ListFilter{Owner: in.Owner, ViewerID: u.ID, All: u.IsAdmin})
		if err != nil {
			return SetList{}, err
		}
		out := SetList{Sets: []SetSummary{}}
		for _, st := range sets {
			out.Sets = append(out.Sets, s.summary(st))
		}
		return out, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_set",
		Description: "Get one set by id or slug: metadata (owner, url, instructions, source_ref) and all tasks in order with their ids, text, blanks with accepted answers and hints, and explanation. Use the task ids with update_task / delete_task.",
		Annotations: readOnly("Get set"),
	}, handle(func(ctx context.Context, u store.User, in SetRef) (SetDetail, error) {
		st, err := s.visibleSet(ctx, u, in.Set)
		if err != nil {
			return SetDetail{}, err
		}
		tasks, err := s.st.ListTasks(ctx, st.ID)
		if err != nil {
			return SetDetail{}, err
		}
		return s.detail(st, tasks), nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "create_set",
		Description: "Create a new set owned by you, optionally with its first tasks. " +
			"Private by default (only you see it); once it is complete, ask the user who should see it and call update_set with visibility=friends (every logged-in user) or public (everyone). " +
			"Returns set_id, slug, url (give the url to the user), visibility, added, total and warnings. " +
			"Run validate_tasks on each batch first. " + lintNote + " " + authoringRules,
		Annotations: additive("Create set"),
	}, handle(func(ctx context.Context, u store.User, in CreateSetIn) (WriteResult, error) {
		if strings.TrimSpace(in.Title) == "" {
			return WriteResult{}, fail("title is required")
		}
		if len(in.Tasks) > maxTasksPerCall {
			return WriteResult{}, fail("max %d tasks per call; send batches (create_set with up to %d, then add_tasks)", maxTasksPerCall, maxTasksPerCall)
		}
		vis := store.Private
		if in.Visibility != "" {
			v, err := parseVisibility(in.Visibility)
			if err != nil {
				return WriteResult{}, err
			}
			vis = v
		}
		inputs, rep := prepare(in.Tasks, nil)
		if len(rep.Errors) > 0 {
			return WriteResult{}, rep.failure()
		}
		st, err := s.st.CreateSet(ctx, u.ID, store.SetInput{
			Title: greek.NFC(in.Title), Topic: greek.NFC(in.Topic), Level: in.Level,
			Instructions: greek.NFC(in.Instructions), SourceRef: greek.NFC(in.SourceRef), Slug: in.Slug,
			Visibility: vis,
		})
		if errors.Is(err, store.ErrInvalid) || errors.Is(err, store.ErrConflict) {
			return WriteResult{}, fail("cannot create set: %v", err)
		}
		if err != nil {
			return WriteResult{}, err
		}
		if len(inputs) > 0 {
			if _, err := s.st.AddTasks(ctx, st.ID, inputs); err != nil {
				if delErr := s.st.DeleteSet(ctx, st.ID); delErr != nil {
					slog.ErrorContext(ctx, "removing set after failed add", "set", st.ID, "err", delErr)
				}
				return WriteResult{}, err
			}
		}
		return WriteResult{SetID: st.ID, Slug: st.Slug, URL: s.url(st.Slug), Visibility: string(st.Visibility), Added: len(inputs), Total: len(inputs), Warnings: rep.Warnings}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "add_tasks",
		Description: "Append 1-50 tasks to a set you own (admins: any set); set = id or slug. " +
			"Run validate_tasks first. Sentences already in the set come back as duplicate warnings. " +
			lintNote + " Returns added, total, url and warnings. " + authoringRules,
		Annotations: additive("Add tasks"),
	}, handle(func(ctx context.Context, u store.User, in AddTasksIn) (WriteResult, error) {
		if len(in.Tasks) == 0 {
			return WriteResult{}, fail("tasks is empty; send 1 to %d tasks", maxTasksPerCall)
		}
		if len(in.Tasks) > maxTasksPerCall {
			return WriteResult{}, fail("max %d tasks per call; send batches", maxTasksPerCall)
		}
		st, err := s.editableSet(ctx, u, in.Set)
		if err != nil {
			return WriteResult{}, err
		}
		existing, err := s.existingTasks(ctx, st.ID, 0)
		if err != nil {
			return WriteResult{}, err
		}
		inputs, rep := prepare(in.Tasks, existing)
		if len(rep.Errors) > 0 {
			return WriteResult{}, rep.failure()
		}
		if _, err := s.st.AddTasks(ctx, st.ID, inputs); err != nil {
			return WriteResult{}, err
		}
		return WriteResult{SetID: st.ID, Slug: st.Slug, URL: s.url(st.Slug), Visibility: string(st.Visibility), Added: len(inputs),
			Total: len(existing) + len(inputs), Warnings: rep.Warnings}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_set",
		Description: "Change metadata of a set you own (admins: any set): title, topic, level, instructions, source_ref, visibility (private: only you; friends: every logged-in user; public: everyone). Only the fields you pass change. Returns the updated set without tasks.",
		Annotations: destructive("Update set", true),
	}, handle(func(ctx context.Context, u store.User, in UpdateSetIn) (SetDetail, error) {
		if in.Title == nil && in.Topic == nil && in.Level == nil && in.Instructions == nil && in.SourceRef == nil && in.Visibility == nil {
			return SetDetail{}, fail("nothing to update; pass at least one of title, topic, level, instructions, source_ref, visibility")
		}
		st, err := s.editableSet(ctx, u, in.Set)
		if err != nil {
			return SetDetail{}, err
		}
		if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
			return SetDetail{}, fail("title cannot be empty")
		}
		var vis *store.Visibility
		if in.Visibility != nil {
			v, err := parseVisibility(*in.Visibility)
			if err != nil {
				return SetDetail{}, err
			}
			vis = &v
		}
		nfc := func(p *string) *string {
			if p == nil {
				return nil
			}
			return ptr(greek.NFC(*p))
		}
		updated, err := s.st.UpdateSet(ctx, st.ID, store.SetPatch{
			Title: nfc(in.Title), Topic: nfc(in.Topic), Level: in.Level,
			Instructions: nfc(in.Instructions), SourceRef: nfc(in.SourceRef), Visibility: vis,
		})
		if err != nil {
			return SetDetail{}, err
		}
		return s.detail(updated, nil), nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "update_task",
		Description: "Change one task in a set you own (admins: any set). Only the fields you pass change; blanks replaces all blanks. " +
			"The merged task is linted as task[0]: errors reject the change. " + authoringRules,
		Annotations: destructive("Update task", true),
	}, handle(func(ctx context.Context, u store.User, in UpdateTaskIn) (TaskResult, error) {
		if in.Text == nil && in.Blanks == nil && in.Explanation == nil {
			return TaskResult{}, fail("nothing to update; pass at least one of text, blanks, explanation")
		}
		t, st, err := s.editableTask(ctx, u, in.TaskID)
		if err != nil {
			return TaskResult{}, err
		}
		merged := TaskIn{Text: t.Text, Blanks: fromGreekBlanks(t.Blanks), Explanation: t.Explanation}
		if in.Text != nil {
			merged.Text = *in.Text
		}
		if in.Blanks != nil {
			merged.Blanks = *in.Blanks
		}
		if in.Explanation != nil {
			merged.Explanation = *in.Explanation
		}
		existing, err := s.existingTasks(ctx, st.ID, t.ID)
		if err != nil {
			return TaskResult{}, err
		}
		inputs, rep := prepare([]TaskIn{merged}, existing)
		if len(rep.Errors) > 0 {
			return TaskResult{}, rep.failure()
		}
		norm := inputs[0]
		updated, err := s.st.UpdateTask(ctx, t.ID, store.TaskPatch{Text: &norm.Text, Blanks: &norm.Blanks, Explanation: &norm.Explanation})
		if err != nil {
			return TaskResult{}, err
		}
		return TaskResult{SetID: st.ID, URL: s.url(st.Slug), Task: taskOut(updated), Warnings: rep.Warnings}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_task",
		Description: "Delete one task by id from a set you own (admins: any set). Cannot be undone.",
		Annotations: destructive("Delete task", true),
	}, handle(func(ctx context.Context, u store.User, in TaskRef) (DeleteResult, error) {
		if _, _, err := s.editableTask(ctx, u, in.TaskID); err != nil {
			return DeleteResult{}, err
		}
		if err := s.st.DeleteTask(ctx, in.TaskID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return DeleteResult{}, fail("task %d not found", in.TaskID)
			}
			return DeleteResult{}, err
		}
		return DeleteResult{Deleted: "task " + strconv.FormatInt(in.TaskID, 10)}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_set",
		Description: "Delete a set you own (admins: any set) with all its tasks and learner progress. Cannot be undone; confirm with the user first.",
		Annotations: destructive("Delete set", true),
	}, handle(func(ctx context.Context, u store.User, in SetRef) (DeleteResult, error) {
		st, err := s.editableSet(ctx, u, in.Set)
		if err != nil {
			return DeleteResult{}, err
		}
		if err := s.st.DeleteSet(ctx, st.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return DeleteResult{}, fail("set %q not found", in.Set)
			}
			return DeleteResult{}, err
		}
		return DeleteResult{Deleted: "set " + st.Slug}, nil
	}))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "validate_tasks",
		Description: "Dry run: lint tasks exactly as create_set/add_tasks would, without saving anything. " +
			"Checks: the 50-task-per-call cap; per-task lint (text, blanks, hints, Greek spelling and accents); " +
			"duplicate sentence text within the batch; and, when set is given, duplicates against that set's existing tasks too; " +
			"explanations repeated on more than 2 tasks, and in batches of 10+ too many missing explanations or subject pronouns. " +
			"Returns errors (would reject the write) and warnings, each as task[i].field: message. " +
			"Run it on every batch, fix all errors, review warnings, then upload. " + authoringRules,
		Annotations: readOnly("Validate tasks"),
	}, handle(func(ctx context.Context, u store.User, in ValidateIn) (LintReport, error) {
		if len(in.Tasks) > maxTasksPerCall {
			return LintReport{}, fail("max %d tasks per call; send batches", maxTasksPerCall)
		}
		var existing []greek.TaskInput
		if in.Set != "" {
			st, err := s.visibleSet(ctx, u, in.Set)
			if err != nil {
				return LintReport{}, err
			}
			existing, err = s.existingTasks(ctx, st.ID, 0)
			if err != nil {
				return LintReport{}, err
			}
		}
		_, rep := prepare(in.Tasks, existing)
		return rep, nil
	}))
}
