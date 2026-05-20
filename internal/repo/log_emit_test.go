package repo

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestTaskRepo_LogsDebugOnGetAndErrorAbsentOnNotFound(t *testing.T) {
	d := setupTestDB(t)
	tlabels := NewTaskLabelsRepo(d)
	tr := NewTaskRepo(d, tlabels)
	ctx, cap := ctxWithCapture(t)

	_, err := tr.Get(ctx, 999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	records := cap.snapshot()
	r, ok := findOp(records, "repo.tasks.Get")
	if !ok {
		t.Fatalf("no record with op=repo.tasks.Get; got %d records", len(records))
	}
	if r.Level != slog.LevelDebug {
		t.Errorf("entry log level: got %v, want DEBUG", r.Level)
	}
	// sql.ErrNoRows / ErrNotFound must be classified as DEBUG, not ERROR.
	if got := countLevel(records, slog.LevelError); got != 0 {
		t.Errorf("error records on ErrNotFound: got %d, want 0", got)
	}
}

func TestTaskRepo_LogsDebugOnHappyPathCreate(t *testing.T) {
	d := setupTestDB(t)
	cr := NewContextRepo(d)
	tlabels := NewTaskLabelsRepo(d)
	tr := NewTaskRepo(d, tlabels)
	ctx, cap := ctxWithCapture(t)

	c, err := cr.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	_, err = tr.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &c.ID},
		Title:     "hello",
		Priority:  model.PriorityNone,
		DayPart:   model.DayPartNone,
		PlanState: model.PlanStateNone,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	records := cap.snapshot()
	if _, ok := findOp(records, "repo.tasks.Create"); !ok {
		t.Fatalf("missing DEBUG record for repo.tasks.Create")
	}
	if got := countLevel(records, slog.LevelError); got != 0 {
		t.Errorf("happy path emitted ERROR records: got %d, want 0", got)
	}
}

func TestProjectRepo_LogsDebugOnList(t *testing.T) {
	_, pr, _, _, ctxID := newProjectFixtures(t)
	ctx, cap := ctxWithCapture(t)

	if _, _, err := pr.List(ctx, ProjectListFilter{ContextID: &ctxID}, Page{}); err != nil {
		t.Fatalf("list: %v", err)
	}
	records := cap.snapshot()
	r, ok := findOp(records, "repo.projects.List")
	if !ok {
		t.Fatalf("missing repo.projects.List DEBUG record (got %d records)", len(records))
	}
	if r.Level != slog.LevelDebug {
		t.Errorf("list log level: got %v, want DEBUG", r.Level)
	}
}

func TestProjectRepo_LogsDebugOnGetNotFound(t *testing.T) {
	_, pr, _, _, _ := newProjectFixtures(t)
	ctx, cap := ctxWithCapture(t)

	_, err := pr.Get(ctx, 999999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	records := cap.snapshot()
	if _, ok := findOp(records, "repo.projects.Get"); !ok {
		t.Fatalf("missing repo.projects.Get DEBUG record")
	}
	if got := countLevel(records, slog.LevelError); got != 0 {
		t.Errorf("ErrNotFound should not emit ERROR: got %d", got)
	}
}

func TestSessionRepo_LogsDebugOnCreateAndGet(t *testing.T) {
	d := setupTestDB(t)
	ur := NewUserRepo(d)
	sr := NewSessionRepo(d)
	if _, err := ur.Create(t.Context(), "alice", "pw"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	ctx, cap := ctxWithCapture(t)

	s, err := sr.Create(ctx, CreateSessionParams{
		UserID:     1,
		TokenHash:  "hash-a",
		ClientKind: model.ClientWeb,
		UserAgent:  "test",
		ExpiresAt:  time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := sr.Get(ctx, s.ID); err != nil {
		t.Fatalf("get session: %v", err)
	}
	records := cap.snapshot()
	if _, ok := findOp(records, "repo.sessions.Create"); !ok {
		t.Fatalf("missing repo.sessions.Create DEBUG record")
	}
	if _, ok := findOp(records, "repo.sessions.Get"); !ok {
		t.Fatalf("missing repo.sessions.Get DEBUG record")
	}
}

func TestSessionRepo_LogsErrorOnGetMissing(t *testing.T) {
	d := setupTestDB(t)
	sr := NewSessionRepo(d)
	ctx, cap := ctxWithCapture(t)

	_, err := sr.Get(ctx, 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	records := cap.snapshot()
	if got := countLevel(records, slog.LevelError); got != 0 {
		t.Errorf("ErrNotFound emitted ERROR: got %d", got)
	}
}

func TestAPITokenRepo_LogsDebugOnHappyPath(t *testing.T) {
	d := setupTestDB(t)
	ur := NewUserRepo(d)
	if _, err := ur.Create(t.Context(), "alice", "pw"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	ar := NewAPITokenRepo(d)
	ctx, cap := ctxWithCapture(t)

	tok, err := ar.Create(ctx, 1, "ci", "hash-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := ar.GetByTokenHash(ctx, "hash-1"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if _, err := ar.ListByUser(ctx, 1); err != nil {
		t.Fatalf("list: %v", err)
	}
	if err := ar.Delete(ctx, tok.ID, 1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	records := cap.snapshot()
	for _, op := range []string{
		"repo.api_tokens.Create",
		"repo.api_tokens.GetByTokenHash",
		"repo.api_tokens.ListByUser",
		"repo.api_tokens.Delete",
	} {
		if _, ok := findOp(records, op); !ok {
			t.Errorf("missing DEBUG record for op=%s", op)
		}
	}
	if got := countLevel(records, slog.LevelError); got != 0 {
		t.Errorf("happy path emitted ERROR records: got %d, want 0", got)
	}
}

func TestAPITokenRepo_LogsDebugOnGetByHashMiss(t *testing.T) {
	d := setupTestDB(t)
	ar := NewAPITokenRepo(d)
	ctx, cap := ctxWithCapture(t)

	_, err := ar.GetByTokenHash(ctx, "no-such-hash")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	records := cap.snapshot()
	if got := countLevel(records, slog.LevelError); got != 0 {
		t.Errorf("lookup miss emitted ERROR: got %d", got)
	}
}
