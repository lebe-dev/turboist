package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service/inboxproc"
)

// scriptedClassifier answers every task with the same content.
type scriptedClassifier struct {
	mu      sync.Mutex
	content string
	calls   int
}

func (s *scriptedClassifier) Classify(context.Context, string, string) (inboxproc.Completion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return inboxproc.Completion{Content: s.content, Model: "test/model"}, nil
}

func (s *scriptedClassifier) set(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.content = content
}

type inboxStatusResp struct {
	Enabled        bool    `json:"enabled"`
	Model          string  `json:"model"`
	APIHost        string  `json:"apiHost"`
	Interval       string  `json:"interval"`
	BatchLimit     int     `json:"batchLimit"`
	Running        bool    `json:"running"`
	PendingCount   int     `json:"pendingCount"`
	LastRunAt      *string `json:"lastRunAt"`
	LastError      *string `json:"lastError"`
	DefaultPrompt  string  `json:"defaultPrompt"`
	LastRunSummary *struct {
		Sorted int `json:"sorted"`
		Kept   int `json:"kept"`
		Failed int `json:"failed"`
	} `json:"lastRunSummary"`
}

type inboxLogEntryResp struct {
	ID         int64   `json:"id"`
	TaskID     *int64  `json:"taskId"`
	TaskTitle  string  `json:"taskTitle"`
	Outcome    string  `json:"outcome"`
	Reason     string  `json:"reason"`
	RevertedAt *string `json:"revertedAt"`
	CreatedAt  string  `json:"createdAt"`
	Before     struct {
		LabelIDs []int64 `json:"labelIds"`
		Priority string  `json:"priority"`
	} `json:"before"`
	After *struct {
		ProjectID int64   `json:"projectId"`
		ContextID int64   `json:"contextId"`
		LabelIDs  []int64 `json:"labelIds"`
	} `json:"after"`
}

func seedProcessingInboxTask(t *testing.T, e *apiEnv, title string) *model.Task {
	t.Helper()
	inboxID := int64(1)
	task, err := e.tasks.Create(context.Background(), repo.CreateTask{Placement: repo.Placement{InboxID: &inboxID}, Title: title})
	if err != nil {
		t.Fatalf("seed inbox task: %v", err)
	}
	return task
}

func TestInboxProcessing_StatusWhenDisabled(t *testing.T) {
	e := setupAPIEnv(t)
	seedProcessingInboxTask(t, e, "waiting")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/inbox/processing", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, body %s", resp.StatusCode, body)
	}
	var got inboxStatusResp
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled || got.Running {
		t.Errorf("got %+v, want disabled and idle", got)
	}
	if got.DefaultPrompt != inboxproc.DefaultPrompt {
		t.Error("defaultPrompt must be the built-in prompt")
	}
	if got.APIHost != "openrouter.ai" || got.Interval != "1h" || got.BatchLimit != 10 {
		t.Errorf("config view: got host=%q interval=%q batch=%d", got.APIHost, got.Interval, got.BatchLimit)
	}
	if got.PendingCount != 1 {
		t.Errorf("pendingCount: got %d, want 1", got.PendingCount)
	}
	if strings.Contains(string(body), "apiKey") {
		t.Errorf("status must never carry a key field: %s", body)
	}
}

func TestInboxProcessing_RunWhenDisabled(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/processing/run", nil))
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(body), "inbox_processing_disabled") {
		t.Fatalf("run disabled: got %d %s, want 409 inbox_processing_disabled", resp.StatusCode, body)
	}
}

func TestInboxProcessing_RunSortsAndRevert(t *testing.T) {
	llm := &scriptedClassifier{content: `{"action":"keep","confidence":1}`}
	e := buildAPIEnvWithInboxProcessing(t, llm)
	ctx := context.Background()
	c, err := e.ctxs.Create(ctx, "home", "green", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := e.projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "Garden", Color: "green"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task := seedProcessingInboxTask(t, e, "Plant tulips")
	llm.set(`{"action":"sort","projectId":` + strconv.FormatInt(p.ID, 10) + `,"confidence":0.9,"reason":"garden"}`)
	ch, cancel := e.eventsHub.Subscribe(1)
	defer cancel()

	// Queued before the loop exists, so the answer is deterministic: the start-up
	// pass files the task and the queued manual run then finds nothing to do.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/processing/run", nil))
	if resp.StatusCode != http.StatusAccepted || !strings.Contains(string(body), `"running":true`) {
		t.Fatalf("run: got %d %s, want 202 running", resp.StatusCode, body)
	}
	startInboxLoop(t, e)
	waitUntil(t, func() bool {
		got, err := e.tasks.Get(ctx, task.ID)
		return err == nil && got.AutoSortedAt != nil
	})
	waitUntil(t, func() bool { return !e.processor.Status().Running })
	if got := drain(ch, 200*time.Millisecond); len(got) == 0 {
		t.Error("a filed task must be published by the processor")
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/"+strconv.FormatInt(task.ID, 10), nil))
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"autoSortedAt":"`) {
		t.Fatalf("task: got %d %s, want autoSortedAt set", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/inbox/processing/log", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("log: got %d %s", resp.StatusCode, body)
	}
	var page struct {
		Items []inboxLogEntryResp `json:"items"`
		Total int                 `json:"total"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if page.Total != 1 || page.Items[0].Outcome != "sorted" || page.Items[0].After == nil ||
		page.Items[0].After.ProjectID != p.ID || page.Items[0].Before.LabelIDs == nil {
		t.Fatalf("log: got %s", body)
	}

	revertPath := "/api/v1/inbox/processing/log/" + strconv.FormatInt(page.Items[0].ID, 10) + "/revert"
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPost, revertPath, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revert: got %d %s", resp.StatusCode, body)
	}
	var reverted struct {
		InboxID      *int64  `json:"inboxId"`
		AutoSortedAt *string `json:"autoSortedAt"`
	}
	if err := json.Unmarshal(body, &reverted); err != nil {
		t.Fatalf("decode revert: %v", err)
	}
	if reverted.InboxID == nil || reverted.AutoSortedAt != nil {
		t.Errorf("revert: got %s, want the task back in the Inbox without the marker", body)
	}
	if got := drain(ch, 200*time.Millisecond); len(got) == 0 {
		t.Error("revert must publish")
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPost, revertPath, nil))
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("second revert: got %d %s, want 409", resp.StatusCode, body)
	}
}

func TestInboxProcessing_RevertNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/processing/log/999/revert", nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revert missing: got %d %s, want 404", resp.StatusCode, body)
	}
}

func TestInboxProcessing_LogPagination(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := e.inboxProc.AppendLog(ctx, model.InboxProcessingLogEntry{
			TaskTitle: "t" + strconv.Itoa(i), Outcome: model.InboxOutcomeKept, Model: "m", CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/inbox/processing/log?limit=2&offset=1", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("log: got %d %s", resp.StatusCode, body)
	}
	var page struct {
		Items  []inboxLogEntryResp `json:"items"`
		Total  int                 `json:"total"`
		Limit  int                 `json:"limit"`
		Offset int                 `json:"offset"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Total != 3 || page.Limit != 2 || page.Offset != 1 || len(page.Items) != 2 || page.Items[0].TaskTitle != "t1" {
		t.Errorf("page: got %s", body)
	}
	if page.Items[0].CreatedAt != "2026-09-13T10:01:00.000Z" {
		t.Errorf("createdAt: got %q, want millisecond UTC", page.Items[0].CreatedAt)
	}
}

func TestInboxProcessing_PutPrompt(t *testing.T) {
	e := setupAPIEnv(t)
	ch, cancel := e.eventsHub.Subscribe(1)
	defer cancel()

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPut, "/api/v1/app-settings/inbox-processing",
		map[string]string{"prompt": "Sort {{.Task.Title}} into {{len .Projects}} projects"}))
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"inboxProcessing":{"prompt":"Sort {{.Task.Title}}`) {
		t.Fatalf("put valid: got %d %s", resp.StatusCode, body)
	}
	if got := drain(ch, 150*time.Millisecond); len(got) != 0 {
		t.Errorf("saving the prompt must not publish, got %v", got)
	}
	saved, err := e.appSettings.Get(context.Background())
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if !strings.HasPrefix(saved.InboxProcessing.Prompt, "Sort ") {
		t.Errorf("stored prompt: got %q", saved.InboxProcessing.Prompt)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/app-settings", nil))
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"inboxProcessing":{"prompt":"Sort`) {
		t.Errorf("get app settings: got %d %s", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPut, "/api/v1/app-settings/inbox-processing",
		map[string]string{"prompt": "{{range .Projcets}}{{end}}"}))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(body), "Projcets") {
		t.Fatalf("put broken: got %d %s, want 422 naming the field", resp.StatusCode, body)
	}
	if saved, _ := e.appSettings.Get(context.Background()); !strings.HasPrefix(saved.InboxProcessing.Prompt, "Sort ") {
		t.Errorf("a refused prompt must not be stored, got %q", saved.InboxProcessing.Prompt)
	}

	for name, prompt := range map[string]string{"empty": "", "unchanged default": inboxproc.DefaultPrompt} {
		resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPut, "/api/v1/app-settings/inbox-processing",
			map[string]string{"prompt": prompt}))
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"inboxProcessing":{"prompt":"","paused":false}`) {
			t.Errorf("put %s: got %d %s, want the default stored as an empty prompt", name, resp.StatusCode, body)
		}
	}
}

func TestInboxProcessing_Preview(t *testing.T) {
	e := setupAPIEnv(t)
	seedProcessingInboxTask(t, e, "Buy seeds")
	ch, cancel := e.eventsHub.Subscribe(1)
	defer cancel()

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/processing/preview",
		map[string]string{"prompt": "Task: {{.Task.Title}}"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview: got %d %s", resp.StatusCode, body)
	}
	var got struct {
		Rendered string `json:"rendered"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(got.Rendered, "Task: Buy seeds\n\n## Output") {
		t.Errorf("rendered: got %q", got.Rendered)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/processing/preview", nil))
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "triage assistant") {
		t.Errorf("preview saved (default): got %d %s", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/processing/preview",
		map[string]string{"prompt": "{{.Missing}}"}))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(body), `"error":`) {
		t.Errorf("preview broken: got %d %s, want 422 with details.error", resp.StatusCode, body)
	}
	if got := drain(ch, 150*time.Millisecond); len(got) != 0 {
		t.Errorf("preview must not publish, got %v", got)
	}
}

func TestInboxProcessing_Scopes(t *testing.T) {
	e := setupAPIEnv(t)
	readSettings := issueAPIToken(t, e, "settings-read", []string{"settings:read"})
	readTasks := issueAPIToken(t, e, "tasks-read", []string{"tasks:read"})

	cases := []struct {
		method, path, token string
		want                int
	}{
		{http.MethodGet, "/api/v1/inbox/processing", readSettings, http.StatusOK},
		{http.MethodGet, "/api/v1/inbox/processing", readTasks, http.StatusForbidden},
		{http.MethodGet, "/api/v1/inbox/processing/log", readTasks, http.StatusOK},
		{http.MethodGet, "/api/v1/inbox/processing/log", readSettings, http.StatusForbidden},
		{http.MethodPost, "/api/v1/inbox/processing/run", readTasks, http.StatusForbidden},
		{http.MethodPost, "/api/v1/inbox/processing/log/1/revert", readTasks, http.StatusForbidden},
		{http.MethodPut, "/api/v1/app-settings/inbox-processing", readSettings, http.StatusForbidden},
	}
	for _, tc := range cases {
		resp, body := runRequest(t, e.app, tokenReq(tc.method, tc.path, tc.token, nil))
		if resp.StatusCode != tc.want {
			t.Errorf("%s %s: got %d %s, want %d", tc.method, tc.path, resp.StatusCode, body, tc.want)
		}
	}
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestInboxProcessing_PutPaused(t *testing.T) {
	e := setupAPIEnv(t)
	ch, cancel := e.eventsHub.Subscribe(1)
	defer cancel()

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPut, "/api/v1/app-settings/inbox-processing",
		map[string]string{"prompt": "Sort {{.Task.Title}}"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put prompt: got %d %s", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPut, "/api/v1/app-settings/inbox-processing",
		map[string]bool{"paused": true}))
	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(string(body), `"inboxProcessing":{"prompt":"Sort {{.Task.Title}}","paused":true}`) {
		t.Fatalf("put paused: got %d %s, want paused with the prompt kept", resp.StatusCode, body)
	}
	if got := drain(ch, 150*time.Millisecond); len(got) != 0 {
		t.Errorf("pausing must not publish, got %v", got)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/inbox/processing", nil))
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"paused":true`) {
		t.Errorf("status: got %d %s, want paused", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPut, "/api/v1/app-settings/inbox-processing",
		map[string]any{"paused": false, "prompt": ""}))
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"inboxProcessing":{"prompt":"","paused":false}`) {
		t.Errorf("resume with prompt reset: got %d %s", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPut, "/api/v1/app-settings/inbox-processing", map[string]any{}))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty body: got %d %s, want 400", resp.StatusCode, body)
	}
}

func TestInboxProcessing_RunWithNothingPending(t *testing.T) {
	e := buildAPIEnvWithInboxProcessing(t, &scriptedClassifier{content: `{"action":"keep","confidence":1}`})
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/processing/run", nil))
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(body), "inbox_processing_nothing_pending") {
		t.Fatalf("run on an empty Inbox: got %d %s, want 409 inbox_processing_nothing_pending", resp.StatusCode, body)
	}

	task := seedProcessingInboxTask(t, e, "hmm")
	at := time.Now()
	if _, err := e.tasks.Update(context.Background(), task.ID, repo.TaskUpdate{AutoSortUndecidedAt: &at}); err != nil {
		t.Fatalf("mark: %v", err)
	}
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/processing/run", nil))
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("run with only undecided tasks: got %d %s, want 409", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/inbox/processing", nil))
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"pendingCount":0`) ||
		!strings.Contains(string(body), `"undecidedCount":1`) {
		t.Errorf("status: got %d %s, want pending 0 and undecided 1", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/"+strconv.FormatInt(task.ID, 10), nil))
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"autoSortUndecidedAt":"`) {
		t.Errorf("task: got %d %s, want autoSortUndecidedAt set", resp.StatusCode, body)
	}
}

func TestInboxProcessing_ManualRunIgnoresPause(t *testing.T) {
	llm := &scriptedClassifier{content: `{"action":"keep","confidence":1}`}
	e := buildAPIEnvWithInboxProcessing(t, llm)
	ctx := context.Background()
	c, err := e.ctxs.Create(ctx, "home", "green", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := e.projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "Garden", Color: "green"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task := seedProcessingInboxTask(t, e, "Plant tulips")
	llm.set(`{"action":"sort","projectId":` + strconv.FormatInt(p.ID, 10) + `,"confidence":0.9,"reason":"garden"}`)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPut, "/api/v1/app-settings/inbox-processing",
		map[string]bool{"paused": true}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pause: got %d %s", resp.StatusCode, body)
	}

	// Queued before the loop exists: the start-up pass is skipped because of the
	// pause, and the queued manual run files the task anyway.
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/processing/run", nil))
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("run while paused: got %d %s, want 202", resp.StatusCode, body)
	}
	startInboxLoop(t, e)
	waitUntil(t, func() bool {
		got, err := e.tasks.Get(ctx, task.ID)
		return err == nil && got.AutoSortedAt != nil
	})
}

func TestInboxProcessing_ConfigCarriesAvailability(t *testing.T) {
	for _, tc := range []struct {
		name string
		llm  inboxproc.Classifier
		want bool
	}{
		{"disabled", nil, false},
		{"enabled", &scriptedClassifier{content: `{"action":"keep","confidence":1}`}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := buildAPIEnvWithInboxProcessing(t, tc.llm)
			resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/config", nil))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("config: got %d %s", resp.StatusCode, body)
			}
			var got struct {
				Inbox struct {
					ProcessingEnabled bool `json:"processingEnabled"`
				} `json:"inbox"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Inbox.ProcessingEnabled != tc.want {
				t.Errorf("inbox.processingEnabled: got %v, want %v", got.Inbox.ProcessingEnabled, tc.want)
			}
		})
	}
}
