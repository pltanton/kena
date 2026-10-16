package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/text/unicode/norm"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/oauth"
	"github.com/pltanton/kena/internal/store"
)

func TestMain(m *testing.M) {
	dbtest.Main(m)
}

const publicURL = "https://kena.example.org"

type env struct {
	st     *store.Store
	ts     *httptest.Server
	tokens map[string]string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st := store.New(dbtest.New(t))
	e := &env{st: st, tokens: map[string]string{}}
	for _, u := range []struct {
		name  string
		admin bool
	}{{"alice", false}, {"bob", false}, {"root", true}} {
		user, err := st.CreateUser(ctx, u.name, u.admin)
		if err != nil {
			t.Fatal(err)
		}
		tok, _, err := st.IssueToken(ctx, user.ID, "test")
		if err != nil {
			t.Fatal(err)
		}
		e.tokens[u.name] = tok
	}
	oauthSrv := &oauth.Server{Store: st, PublicURL: publicURL}
	h := auth.RequireBearerToken(oauthSrv.Verify, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: oauthSrv.ResourceMetadataURL(),
	})(Handler(New(st, publicURL)))
	e.ts = httptest.NewServer(h)
	t.Cleanup(e.ts.Close)
	return e
}

type bearerRT struct{ token string }

func (b bearerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func (e *env) connectToken(t *testing.T, token string) (*mcp.ClientSession, error) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   e.ts.URL,
		HTTPClient: &http.Client{Transport: bearerRT{token}},
	}, nil)
	if err == nil {
		t.Cleanup(func() { cs.Close() })
	}
	return cs, err
}

func (e *env) connect(t *testing.T, user string) *mcp.ClientSession {
	t.Helper()
	cs, err := e.connectToken(t, e.tokens[user])
	if err != nil {
		t.Fatalf("connect as %s: %v", user, err)
	}
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func ok[T any](t *testing.T, cs *mcp.ClientSession, name string, args any) T {
	t.Helper()
	res := call(t, cs, name, args)
	if res.IsError {
		t.Fatalf("%s: unexpected tool error: %s", name, text(res))
	}
	var out T
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: decoding %s: %v", name, raw, err)
	}
	return out
}

func fails(t *testing.T, cs *mcp.ClientSession, name string, args any, want ...string) string {
	t.Helper()
	res := call(t, cs, name, args)
	if !res.IsError {
		t.Fatalf("%s: expected tool error, got %s", name, text(res))
	}
	if res.StructuredContent != nil {
		t.Errorf("%s: error result carries structuredContent %v", name, res.StructuredContent)
	}
	msg := text(res)
	for _, w := range want {
		if !strings.Contains(msg, w) {
			t.Errorf("%s: error %q does not contain %q", name, msg, w)
		}
	}
	return msg
}

func task(text string, answers ...string) map[string]any {
	return map[string]any{
		"text":   text,
		"blanks": []map[string]any{{"answers": answers, "hint": "γράφω"}},
	}
}

func validTasks() []map[string]any {
	return []map[string]any{
		task("Χθες η Μαρία ___ ένα γράμμα στη γιαγιά της.", "έγραψε"),
		task("Πέρσι εμείς ___ πολλά βιβλία για το σχολείο.", "γράψαμε"),
	}
}

func setArgs(title string, tasks any) map[string]any {
	a := map[string]any{
		"title":        title,
		"topic":        "aorist",
		"level":        "B1",
		"instructions": "Συμπληρώστε με τον αόριστο.",
		"source_ref":   "Book, unit 5, ex. 3",
	}
	if tasks != nil {
		a["tasks"] = tasks
	}
	return a
}

func TestNoTokenIs401(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Post(e.ts.URL, "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if h := resp.Header.Get("WWW-Authenticate"); !strings.Contains(h, "resource_metadata=") {
		t.Fatalf("WWW-Authenticate = %q", h)
	}
}

func TestToolsAndInstructions(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	if cs.InitializeResult().Instructions == "" {
		t.Fatal("empty instructions")
	}
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if len(tool.Description) > 1500 {
			t.Errorf("%s description is %d chars", tool.Name, len(tool.Description))
		}
	}
	want := []string{
		"add_tasks", "admin_create_user", "admin_issue_token", "admin_list_users", "admin_reset_password", "admin_revoke_token",
		"create_set", "delete_set", "delete_task", "get_authoring_guide", "get_set", "list_sets",
		"update_set", "update_task", "validate_tasks",
	}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Fatalf("tools = %v\nwant %v", names, want)
	}
}

func TestCreateAndGetSet(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	out := ok[WriteResult](t, cs, "create_set", setArgs("Αόριστος", validTasks()))
	if out.Slug == "" || out.URL != publicURL+"/s/"+out.Slug || out.Added != 2 || out.Total != 2 {
		t.Fatalf("create_set = %+v", out)
	}

	got := ok[SetDetail](t, cs, "get_set", map[string]any{"set": out.Slug})
	if got.Owner != "alice" || got.URL != out.URL || len(got.Tasks) != 2 {
		t.Fatalf("get_set = %+v", got)
	}
	if got.Tasks[0].Blanks[0].Answers[0] != "έγραψε" || got.Tasks[1].Position != 2 {
		t.Fatalf("tasks = %+v", got.Tasks)
	}

	byID := ok[SetDetail](t, cs, "get_set", map[string]any{"set": fmt.Sprint(out.SetID)})
	if byID.Slug != out.Slug {
		t.Fatalf("get_set by id = %+v", byID)
	}
	fails(t, cs, "get_set", map[string]any{"set": "nope"}, `set "nope" not found`)
}

func TestCreateSetLintErrorWritesNothing(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	bad := []map[string]any{
		task("Χθες η Μαρία ___ ένα γράμμα και ___ στη γιαγιά της.", "έγραψε"),
	}
	fails(t, cs, "create_set", setArgs("Bad", bad), "task[0].text")
	list := ok[SetList](t, cs, "list_sets", map[string]any{})
	if len(list.Sets) != 0 {
		t.Fatalf("sets = %+v", list.Sets)
	}
}

func TestBatchLimits(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	out := ok[WriteResult](t, cs, "create_set", setArgs("Limits", nil))
	if out.Added != 0 || out.Total != 0 {
		t.Fatalf("create_set = %+v", out)
	}
	many := make([]map[string]any, 51)
	for i := range many {
		many[i] = validTasks()[0]
	}
	fails(t, cs, "add_tasks", map[string]any{"set": out.Slug, "tasks": many}, "max 50")
	fails(t, cs, "create_set", setArgs("Too many", many), "max 50")
	fails(t, cs, "add_tasks", map[string]any{"set": out.Slug, "tasks": []any{}})
	fails(t, cs, "add_tasks", map[string]any{"set": "missing", "tasks": validTasks()}, `set "missing" not found`)
}

func TestCreateSetRequiresTitle(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	fails(t, cs, "create_set", setArgs("   ", nil), "title is required")
	list := ok[SetList](t, cs, "list_sets", map[string]any{})
	if len(list.Sets) != 0 {
		t.Fatalf("sets = %+v", list.Sets)
	}
}

func TestAddTasksDuplicateWarning(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	out := ok[WriteResult](t, cs, "create_set", setArgs("Dups", validTasks()))
	add := ok[WriteResult](t, cs, "add_tasks", map[string]any{"set": out.Slug, "tasks": validTasks()[:1]})
	if add.Added != 1 || add.Total != 3 {
		t.Fatalf("add_tasks = %+v", add)
	}
	if len(add.Warnings) == 0 || !strings.Contains(strings.Join(add.Warnings, "\n"), "task[0].text: duplicate") {
		t.Fatalf("warnings = %v", add.Warnings)
	}
}

func TestEditPermissions(t *testing.T) {
	e := newEnv(t)
	alice := e.connect(t, "alice")
	out := ok[WriteResult](t, alice, "create_set", setArgs("Alice set", validTasks()))
	detail := ok[SetDetail](t, alice, "get_set", map[string]any{"set": out.Slug})
	taskID := detail.Tasks[0].ID

	bob := e.connect(t, "bob")
	patch := map[string]any{"task_id": taskID, "text": "Χθες ο Νίκος ___ ένα γράμμα στη γιαγιά του."}
	fails(t, bob, "update_task", patch, "belongs to alice")
	fails(t, bob, "add_tasks", map[string]any{"set": out.Slug, "tasks": validTasks()}, "belongs to alice")
	fails(t, bob, "delete_set", map[string]any{"set": out.Slug}, "belongs to alice")
	fails(t, bob, "delete_task", map[string]any{"task_id": taskID}, "belongs to alice")
	fails(t, bob, "update_set", map[string]any{"set": out.Slug, "title": "Bob's title"}, "belongs to alice")

	root := e.connect(t, "root")
	upd := ok[TaskResult](t, root, "update_task", patch)
	if !strings.HasPrefix(upd.Task.Text, "Χθες ο Νίκος") || upd.Task.Blanks[0].Answers[0] != "έγραψε" {
		t.Fatalf("update_task = %+v", upd)
	}
	if upd.URL != out.URL {
		t.Fatalf("update_task url = %q, want %q", upd.URL, out.URL)
	}

	fails(t, alice, "update_task", map[string]any{"task_id": taskID, "blanks": []map[string]any{}}, "task[0].text")
	fails(t, alice, "update_task", map[string]any{"task_id": 999999, "explanation": "x"}, "task 999999 not found")

	meta := ok[SetDetail](t, alice, "update_set", map[string]any{"set": out.Slug, "level": "A2"})
	if meta.Level != "A2" || meta.Title != "Alice set" {
		t.Fatalf("update_set = %+v", meta)
	}

	fails(t, alice, "update_set", map[string]any{"set": out.Slug, "title": "   "}, "title cannot be empty")

	ok[DeleteResult](t, alice, "delete_task", map[string]any{"task_id": taskID})
	after := ok[SetDetail](t, alice, "get_set", map[string]any{"set": out.Slug})
	if len(after.Tasks) != 1 {
		t.Fatalf("tasks after delete = %d", len(after.Tasks))
	}
	ok[DeleteResult](t, alice, "delete_set", map[string]any{"set": out.Slug})
	fails(t, alice, "get_set", map[string]any{"set": out.Slug}, "not found")
}

func TestValidateTasks(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	tasks := []map[string]any{
		task("Χθες η Μαρία ___ ένα γράμμα.", "εγραψε"),
		task("Χθες η Μαρία ___ ένα γράμμα.", "έγραψε"),
		task("Αυτοσ ___ γράμμα.", "έγραψε"),
	}
	rep := ok[LintReport](t, cs, "validate_tasks", map[string]any{"tasks": tasks})
	joined := strings.Join(rep.Warnings, "\n")
	if !strings.Contains(joined, "task[0].blanks[0].answers[0]: no accent") || !strings.Contains(joined, "task[1].text: duplicate") {
		t.Fatalf("warnings = %v", rep.Warnings)
	}
	if len(rep.Errors) != 1 || !strings.HasPrefix(rep.Errors[0], "task[2].text: ") {
		t.Fatalf("errors = %v", rep.Errors)
	}
	list := ok[SetList](t, cs, "list_sets", map[string]any{})
	if len(list.Sets) != 0 {
		t.Fatalf("validate_tasks wrote sets: %+v", list.Sets)
	}
}

func TestValidateTasksAgainstSet(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	out := ok[WriteResult](t, cs, "create_set", setArgs("Set", validTasks()))

	dupe := []map[string]any{validTasks()[0]}
	rep := ok[LintReport](t, cs, "validate_tasks", map[string]any{"tasks": dupe, "set": out.Slug})
	if joined := strings.Join(rep.Warnings, "\n"); !strings.Contains(joined, "task[0].text: duplicate") {
		t.Fatalf("warnings = %v", rep.Warnings)
	}

	noSet := ok[LintReport](t, cs, "validate_tasks", map[string]any{"tasks": dupe})
	if len(noSet.Warnings) != 0 {
		t.Fatalf("warnings without set = %v", noSet.Warnings)
	}

	fails(t, cs, "validate_tasks", map[string]any{"tasks": dupe, "set": "missing"}, `set "missing" not found`)

	many := make([]map[string]any, 51)
	for i := range many {
		many[i] = validTasks()[0]
	}
	fails(t, cs, "validate_tasks", map[string]any{"tasks": many}, "max 50")
}

func TestAdminTokens(t *testing.T) {
	e := newEnv(t)
	alice := e.connect(t, "alice")
	fails(t, alice, "admin_issue_token", map[string]any{"username": "bob"}, "admin only")
	fails(t, alice, "admin_list_users", map[string]any{}, "admin only")

	root := e.connect(t, "root")
	issued := ok[IssuedToken](t, root, "admin_issue_token", map[string]any{"username": "bob", "label": "laptop"})
	if issued.Token == "" || issued.Username != "bob" || issued.Note == "" {
		t.Fatalf("admin_issue_token = %+v", issued)
	}

	fresh, err := e.connectToken(t, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	ok[SetList](t, fresh, "list_sets", map[string]any{})

	users := ok[UserList](t, root, "admin_list_users", map[string]any{})
	var bobTokens []TokenInfo
	for _, u := range users.Users {
		if u.Username == "bob" {
			bobTokens = u.Tokens
		}
	}
	if len(bobTokens) != 2 || bobTokens[1].Label != "laptop" {
		t.Fatalf("bob tokens = %+v", bobTokens)
	}

	ok[RevokeResult](t, root, "admin_revoke_token", map[string]any{"token_id": issued.TokenID})
	fails(t, root, "admin_revoke_token", map[string]any{"token_id": 999999}, "not found")

	req, _ := http.NewRequest(http.MethodPost, e.ts.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Authorization", "Bearer "+issued.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d", resp.StatusCode)
	}
}

func TestAdminCreateUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root := e.connect(t, "root")
	u := ok[CreatedUser](t, root, "admin_create_user", map[string]any{"username": "carol"})
	if u.Username != "carol" || u.IsAdmin || strings.Count(u.Passphrase, "-") < 3 {
		t.Fatalf("admin_create_user = %+v", u)
	}
	got, err := e.st.CheckPassword(ctx, "carol", u.Passphrase)
	if err != nil || got.ID != u.ID {
		t.Fatalf("returned passphrase does not log in: %v", err)
	}
	fails(t, root, "admin_create_user", map[string]any{"username": "carol"}, "already exists")
	fails(t, root, "admin_create_user", map[string]any{"username": "Bad Name"}, "invalid username")
	fails(t, e.connect(t, "alice"), "admin_create_user", map[string]any{"username": "dave"}, "admin only")
}

func TestAdminResetPassword(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root := e.connect(t, "root")
	created := ok[CreatedUser](t, root, "admin_create_user", map[string]any{"username": "carol"})
	sess, err := e.st.CreateSession(ctx, created.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, _, err := e.st.CreateGrant(ctx, created.ID, "kc_x", "res", 0, time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	reset := ok[ResetPassword](t, root, "admin_reset_password", map[string]any{"username": "carol"})
	if reset.Username != "carol" || reset.Passphrase == "" || reset.Passphrase == created.Passphrase {
		t.Fatalf("admin_reset_password = %+v", reset)
	}
	if _, err := e.st.CheckPassword(ctx, "carol", reset.Passphrase); err != nil {
		t.Fatalf("new passphrase does not log in: %v", err)
	}
	if _, err := e.st.CheckPassword(ctx, "carol", created.Passphrase); err == nil {
		t.Fatal("old passphrase still logs in")
	}
	if _, err := e.st.UserBySession(ctx, sess); err == nil {
		t.Error("session survives reset")
	}
	if _, _, err := e.st.GrantByAccess(ctx, access); err == nil {
		t.Error("OAuth grant survives reset")
	}
	fails(t, root, "admin_reset_password", map[string]any{"username": "nobody"}, "not found")
	fails(t, e.connect(t, "alice"), "admin_reset_password", map[string]any{"username": "bob"}, "admin only")
}

func TestListSetsOwnerFilter(t *testing.T) {
	e := newEnv(t)
	ok[WriteResult](t, e.connect(t, "alice"), "create_set", setArgs("A", validTasks()))
	ok[WriteResult](t, e.connect(t, "bob"), "create_set", setArgs("B", nil))
	cs := e.connect(t, "bob")
	all := ok[SetList](t, cs, "list_sets", map[string]any{})
	if len(all.Sets) != 2 {
		t.Fatalf("sets = %+v", all.Sets)
	}
	mine := ok[SetList](t, cs, "list_sets", map[string]any{"owner": "alice"})
	if len(mine.Sets) != 1 || mine.Sets[0].Owner != "alice" || mine.Sets[0].TaskCount != 2 || mine.Sets[0].URL == "" {
		t.Fatalf("alice sets = %+v", mine.Sets)
	}
}

func TestGuide(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	idx := ok[GuideResult](t, cs, "get_authoring_guide", map[string]any{})
	for _, name := range []string{"workflow", "item-rules", "greek-pitfalls", "self-check", "format"} {
		if !strings.Contains(idx.Content, name) {
			t.Errorf("index does not mention %s", name)
		}
	}
	fmtGuide := ok[GuideResult](t, cs, "get_authoring_guide", map[string]any{"topic": "format"})
	if fmtGuide.Topic != "format" || !strings.Contains(fmtGuide.Content, "# Format") {
		t.Fatalf("format guide = %+v", fmtGuide.Topic)
	}
	fails(t, cs, "get_authoring_guide", map[string]any{"topic": "nope"}, "workflow")
}

func TestResources(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, "alice")
	ctx := context.Background()
	list, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range list.Resources {
		if r.URI == "skill://kena/workflow" {
			found = true
			if r.MIMEType != "text/markdown" || r.Description == "" {
				t.Errorf("resource = %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("resources = %+v", list.Resources)
	}
	res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "skill://kena/workflow"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 || !strings.Contains(res.Contents[0].Text, "workflow") {
		t.Fatalf("contents = %+v", res.Contents)
	}
}

func TestPrepareNormalizes(t *testing.T) {
	nfd := norm.NFD.String("Χθες η Μαρία ___ ένα γράμμα.")
	out, rep := prepare([]TaskIn{
		{Text: "  " + nfd + " ", Blanks: []BlankIn{{Answers: []string{norm.NFD.String("έγραψε ")}, Hint: norm.NFD.String("γράφω")}}, Explanation: norm.NFD.String("αόριστος")},
	}, []string{"Χθες η Μαρία ___ ένα γράμμα."})
	if len(rep.Errors) != 0 {
		t.Fatalf("errors = %v", rep.Errors)
	}
	got := out[0]
	if got.Text != "Χθες η Μαρία ___ ένα γράμμα." || got.Blanks[0].Answers[0] != "έγραψε" ||
		got.Blanks[0].Hint != "γράφω" || got.Explanation != "αόριστος" {
		t.Fatalf("not NFC: %+q", got)
	}
	if len(rep.Warnings) != 1 || rep.Warnings[0] != "task[0].text: duplicate of another task's text" {
		t.Fatalf("warnings = %v", rep.Warnings)
	}
}
