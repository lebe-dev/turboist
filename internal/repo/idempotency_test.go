package repo

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// seedIdempotencyUser inserts the single user (id=1) that idempotency rows
// reference via their user_id FK.
func seedIdempotencyUser(t *testing.T, d *sql.DB) {
	t.Helper()
	if _, err := NewUserRepo(d).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func TestIdempotencyRepo_Reserve_Creates(t *testing.T) {
	d := setupTestDB(t)
	seedIdempotencyUser(t, d)
	r := NewIdempotencyRepo(d)
	ctx := context.Background()

	// A fresh key is created: existing=false. Per the contract callers ignore
	// the record on this path (the middleware proceeds to the handler).
	_, existing, err := r.Reserve(ctx, "key-abc123", 1, "POST", "/api/v1/inbox/tasks")
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if existing {
		t.Fatalf("existing: got true, want false for a fresh key")
	}

	// Re-reserving the same key returns the stored row (still in flight) with
	// the reservation's method/path/user preserved.
	rec, existing, err := r.Reserve(ctx, "key-abc123", 1, "POST", "/api/v1/inbox/tasks")
	if err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	if !existing {
		t.Fatalf("existing: got false, want true on re-reserve")
	}
	if rec.Key != "key-abc123" {
		t.Errorf("key: got %q, want %q", rec.Key, "key-abc123")
	}
	if rec.UserID != 1 {
		t.Errorf("user_id: got %d, want 1", rec.UserID)
	}
	if rec.Method != "POST" {
		t.Errorf("method: got %q, want POST", rec.Method)
	}
	if rec.Path != "/api/v1/inbox/tasks" {
		t.Errorf("path: got %q, want /api/v1/inbox/tasks", rec.Path)
	}
	if rec.Status != 0 {
		t.Errorf("status: got %d, want 0 (in flight)", rec.Status)
	}
	if rec.CreatedAt.IsZero() {
		t.Errorf("created_at: got zero, want a timestamp")
	}
}

func TestIdempotencyRepo_Reserve_ConflictInFlight(t *testing.T) {
	d := setupTestDB(t)
	seedIdempotencyUser(t, d)
	r := NewIdempotencyRepo(d)
	ctx := context.Background()

	if _, existing, err := r.Reserve(ctx, "dup-key-1", 1, "POST", "/api/v1/tasks/5/complete"); err != nil || existing {
		t.Fatalf("first reserve: existing=%v err=%v", existing, err)
	}

	rec, existing, err := r.Reserve(ctx, "dup-key-1", 1, "POST", "/api/v1/tasks/5/complete")
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if !existing {
		t.Fatalf("existing: got false, want true for a duplicate key")
	}
	if rec.Key != "dup-key-1" {
		t.Errorf("key: got %q, want dup-key-1", rec.Key)
	}
	if rec.Status != 0 {
		t.Errorf("status: got %d, want 0 (still in flight)", rec.Status)
	}
}

func TestIdempotencyRepo_Reserve_ConflictCompleted(t *testing.T) {
	d := setupTestDB(t)
	seedIdempotencyUser(t, d)
	r := NewIdempotencyRepo(d)
	ctx := context.Background()

	if _, existing, err := r.Reserve(ctx, "done-key", 1, "POST", "/api/v1/inbox/tasks"); err != nil || existing {
		t.Fatalf("first reserve: existing=%v err=%v", existing, err)
	}
	if err := r.Complete(ctx, "done-key", 200, []byte(`{"id":42}`)); err != nil {
		t.Fatalf("complete: %v", err)
	}

	rec, existing, err := r.Reserve(ctx, "done-key", 1, "POST", "/api/v1/inbox/tasks")
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if !existing {
		t.Fatalf("existing: got false, want true for a completed key")
	}
	if rec.Status != 200 {
		t.Errorf("status: got %d, want 200", rec.Status)
	}
	if rec.Response != `{"id":42}` {
		t.Errorf("response: got %q, want %q", rec.Response, `{"id":42}`)
	}
}

func TestIdempotencyRepo_Complete_StoresStatusAndBody(t *testing.T) {
	d := setupTestDB(t)
	seedIdempotencyUser(t, d)
	r := NewIdempotencyRepo(d)
	ctx := context.Background()

	if _, _, err := r.Reserve(ctx, "complete-key", 1, "POST", "/api/v1/inbox/tasks"); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := r.Complete(ctx, "complete-key", 201, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("complete: %v", err)
	}

	rec, existing, err := r.Reserve(ctx, "complete-key", 1, "POST", "/api/v1/inbox/tasks")
	if err != nil || !existing {
		t.Fatalf("reserve after complete: existing=%v err=%v", existing, err)
	}
	if rec.Status != 201 {
		t.Errorf("status: got %d, want 201", rec.Status)
	}
	if rec.Response != `{"ok":true}` {
		t.Errorf("response: got %q, want %q", rec.Response, `{"ok":true}`)
	}
}

func TestIdempotencyRepo_Release_DeletesPending(t *testing.T) {
	d := setupTestDB(t)
	seedIdempotencyUser(t, d)
	r := NewIdempotencyRepo(d)
	ctx := context.Background()

	if _, _, err := r.Reserve(ctx, "release-key", 1, "POST", "/api/v1/tasks/5/complete"); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := r.Release(ctx, "release-key"); err != nil {
		t.Fatalf("release: %v", err)
	}

	// After release the key is free again — a re-reserve creates a fresh row.
	_, existing, err := r.Reserve(ctx, "release-key", 1, "POST", "/api/v1/tasks/5/complete")
	if err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	if existing {
		t.Fatalf("existing: got true, want false after release")
	}
}

func TestIdempotencyRepo_DeleteOlderThan_PrunesByCutoff(t *testing.T) {
	d := setupTestDB(t)
	seedIdempotencyUser(t, d)
	r := NewIdempotencyRepo(d)
	ctx := context.Background()

	// Two keys created "now", one stale key backdated well before the cutoff.
	if _, _, err := r.Reserve(ctx, "fresh-1", 1, "POST", "/api/v1/inbox/tasks"); err != nil {
		t.Fatalf("reserve fresh-1: %v", err)
	}
	if _, _, err := r.Reserve(ctx, "fresh-2", 1, "POST", "/api/v1/inbox/tasks"); err != nil {
		t.Fatalf("reserve fresh-2: %v", err)
	}
	old := model.FormatUTC(time.Now().Add(-72 * time.Hour))
	if _, err := d.ExecContext(ctx,
		`INSERT INTO idempotency_keys (key, user_id, method, path, status, response, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"stale-1", 1, "POST", "/api/v1/inbox/tasks", 200, "{}", old); err != nil {
		t.Fatalf("insert stale: %v", err)
	}

	cutoff := time.Now().Add(-48 * time.Hour)
	n, err := r.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("delete older than: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted: got %d, want 1", n)
	}

	// The stale key is gone; fresh keys survive and still replay.
	if _, existing, err := r.Reserve(ctx, "stale-1", 1, "POST", "/api/v1/inbox/tasks"); err != nil || existing {
		t.Errorf("stale-1: existing=%v err=%v, want fresh reservation", existing, err)
	}
	if _, existing, err := r.Reserve(ctx, "fresh-1", 1, "POST", "/api/v1/inbox/tasks"); err != nil || !existing {
		t.Errorf("fresh-1: existing=%v err=%v, want it to still exist", existing, err)
	}
}

func TestIdempotencyRepo_DeleteOlderThan_NoRows(t *testing.T) {
	d := setupTestDB(t)
	seedIdempotencyUser(t, d)
	r := NewIdempotencyRepo(d)
	ctx := context.Background()

	if _, _, err := r.Reserve(ctx, "keep-me", 1, "POST", "/api/v1/inbox/tasks"); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	n, err := r.DeleteOlderThan(ctx, time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("delete older than: %v", err)
	}
	if n != 0 {
		t.Errorf("deleted: got %d, want 0", n)
	}
}
