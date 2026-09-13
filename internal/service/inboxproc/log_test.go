package inboxproc

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// captureHandler records every log line so a test can read back its fields.
type captureHandler struct {
	mu      *sync.Mutex
	records *[]slog.Record
	attrs   []slog.Attr
}

func newCaptureHandler() *captureHandler {
	return &captureHandler{mu: &sync.Mutex{}, records: &[]slog.Record{}}
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	rec := r.Clone()
	rec.AddAttrs(h.attrs...)
	*h.records = append(*h.records, rec)
	return nil
}

func (h *captureHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return &captureHandler{mu: h.mu, records: h.records, attrs: append(append([]slog.Attr{}, h.attrs...), a...)}
}

func (h *captureHandler) WithGroup(string) slog.Handler { return h }

// find returns the fields of the first record with the message, or nil.
func (h *captureHandler) find(msg string) map[string]slog.Value {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range *h.records {
		if r.Message != msg {
			continue
		}
		fields := map[string]slog.Value{"level": slog.StringValue(r.Level.String())}
		r.Attrs(func(a slog.Attr) bool {
			fields[a.Key] = a.Value.Resolve()
			return true
		})
		return fields
	}
	return nil
}

func withCapture(f *procFixture) *captureHandler {
	h := newCaptureHandler()
	f.proc.log = slog.New(h)
	return h
}

func stringsOf(t *testing.T, v slog.Value) []string {
	t.Helper()
	got, ok := v.Any().([]string)
	if !ok {
		t.Fatalf("value %v: got %T, want []string", v, v.Any())
	}
	return got
}

func TestProcessorLog_FiledTaskDescribesTheMove(t *testing.T) {
	f := newProcFixture(t)
	logs := withCapture(f)
	ctx := context.Background()
	captured, err := f.labels.Create(ctx, "captured", "blue", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	task := f.inboxTask(t, "Plant tulips")
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{captured.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}
	f.llm.respond = answer(f.sortAnswer())

	f.proc.RunOnce(ctx)

	rec := logs.find("inbox processing filed a task")
	if rec == nil {
		t.Fatal("no log line for the filed task")
	}
	if rec["level"].String() != "INFO" {
		t.Errorf("level: got %s, want INFO", rec["level"])
	}
	entries, _, _ := f.state.ListLog(ctx, repo.Page{})
	want := map[string]string{
		"task_title": "Plant tulips",
		"from":       "inbox",
		"context":    "home",
		"project":    "Garden",
		"priority":   "high",
		"due_date":   "2026-09-20",
		"reason":     "garden work",
		"model":      "fake/model",
		"task_id":    strconv.FormatInt(task.ID, 10),
		"project_id": strconv.FormatInt(f.garden.ID, 10),
		"context_id": strconv.FormatInt(f.contextID, 10),
		"journal_id": strconv.FormatInt(entries[0].ID, 10),
		"confidence": "0.9",
	}
	for key, value := range want {
		got, ok := rec[key]
		if !ok {
			t.Errorf("field %s: missing in %v", key, rec)
			continue
		}
		if got.String() != value {
			t.Errorf("field %s: got %q, want %q", key, got.String(), value)
		}
	}
	if added := stringsOf(t, rec["labels_added"]); len(added) != 1 || added[0] != "outdoor" {
		t.Errorf("labels_added: got %v, want [outdoor]", added)
	}
	if all := stringsOf(t, rec["labels"]); len(all) != 2 {
		t.Errorf("labels: got %v, want both labels of the filed task", all)
	}

	finished := logs.find("inbox processing run finished")
	if finished == nil || finished["sorted"].Int64() != 1 || finished["manual"].Bool() {
		t.Errorf("run summary: got %v, want one sorted in a scheduled run", finished)
	}
}

func TestProcessorLog_KeptAndFailedTasks(t *testing.T) {
	f := newProcFixture(t)
	logs := withCapture(f)
	ctx := context.Background()
	f.inboxTask(t, "hmm")
	f.proc.RunOnce(ctx)

	kept := logs.find("inbox processing kept a task in the inbox")
	if kept == nil || kept["task_title"].String() != "hmm" || kept["reason"].String() != "unsure" ||
		kept["confidence"].Float64() != 1 || kept["journal_id"].Int64() == 0 {
		t.Errorf("kept line: got %v", kept)
	}

	f.inboxTask(t, "garbled")
	f.llm.respond = answer("no json here")
	f.proc.runOnce(ctx, true)

	failed := logs.find("inbox processing could not decide on a task")
	if failed == nil || failed["level"].String() != "WARN" || failed["task_title"].String() != "garbled" ||
		failed["attempts"].Int64() != 1 || failed["next_attempt_at"].String() == "" || failed["err"].String() == "" {
		t.Errorf("failed line: got %v", failed)
	}
	if manual := logs.find("inbox processing run finished"); manual == nil {
		t.Error("no run summary")
	}
}

func TestProcessorLog_RevertDescribesTheWayBack(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	task := f.inboxTask(t, "Plant tulips")
	f.llm.respond = answer(f.sortAnswer())
	f.proc.RunOnce(ctx)
	entries, _, _ := f.state.ListLog(ctx, repo.Page{})

	logs := withCapture(f)
	f.now = f.now.Add(time.Hour)
	if _, err := f.proc.Revert(ctx, entries[0].ID); err != nil {
		t.Fatalf("revert: %v", err)
	}

	rec := logs.find("inbox processing decision reverted")
	if rec == nil {
		t.Fatal("no log line for the revert")
	}
	want := map[string]string{
		"task_id":           strconv.FormatInt(task.ID, 10),
		"task_title":        "Plant tulips",
		"journal_id":        strconv.FormatInt(entries[0].ID, 10),
		"from_project":      "Garden",
		"from_project_id":   strconv.FormatInt(f.garden.ID, 10),
		"to":                "inbox",
		"priority_restored": string(model.PriorityNone),
		"due_restored":      "",
	}
	for key, value := range want {
		got, ok := rec[key]
		if !ok {
			t.Errorf("field %s: missing in %v", key, rec)
			continue
		}
		if got.String() != value {
			t.Errorf("field %s: got %q, want %q", key, got.String(), value)
		}
	}
	if removed := stringsOf(t, rec["labels_removed"]); len(removed) != 1 || removed[0] != "outdoor" {
		t.Errorf("labels_removed: got %v, want [outdoor]", removed)
	}
}
