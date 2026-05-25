package repo

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedUserForIdempotency(t *testing.T, ctx context.Context, r *UserRepo, name string) int64 {
	t.Helper()
	u, err := r.Create(ctx, name, "h")
	if err != nil {
		t.Fatalf("seed user %s: %v", name, err)
	}
	return u.ID
}

func TestIdempotencyRepo_PutAndGet(t *testing.T) {
	ctx := context.Background()
	d := setupTestDB(t)
	users := NewUserRepo(d)
	uid := seedUserForIdempotency(t, ctx, users, "alice")
	r := NewIdempotencyRepo(d)

	rec := IdempotencyRecord{
		UserID:       uid,
		Key:          "abc-123",
		StatusCode:   201,
		ContentType:  "application/json",
		ResponseBody: []byte(`{"id":7}`),
		CreatedAt:    time.Now(),
	}
	if err := r.Put(ctx, rec); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := r.Get(ctx, uid, "abc-123", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.StatusCode != 201 {
		t.Errorf("status: got %d, want 201", got.StatusCode)
	}
	if string(got.ResponseBody) != `{"id":7}` {
		t.Errorf("body: got %q, want %q", got.ResponseBody, `{"id":7}`)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type: got %q, want application/json", got.ContentType)
	}
}

func TestIdempotencyRepo_Get_MissingReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	d := setupTestDB(t)
	users := NewUserRepo(d)
	uid := seedUserForIdempotency(t, ctx, users, "alice")
	r := NewIdempotencyRepo(d)

	_, err := r.Get(ctx, uid, "nope", time.Now().Add(-time.Hour))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err: got %v, want ErrNotFound", err)
	}
}

func TestIdempotencyRepo_Get_ExpiredTreatedAsMiss(t *testing.T) {
	ctx := context.Background()
	d := setupTestDB(t)
	users := NewUserRepo(d)
	uid := seedUserForIdempotency(t, ctx, users, "alice")
	r := NewIdempotencyRepo(d)

	stale := time.Now().Add(-48 * time.Hour)
	if err := r.Put(ctx, IdempotencyRecord{
		UserID:       uid,
		Key:          "old",
		StatusCode:   200,
		ContentType:  "application/json",
		ResponseBody: []byte(`{}`),
		CreatedAt:    stale,
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	_, err := r.Get(ctx, uid, "old", time.Now().Add(-24*time.Hour))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expired row should be ignored: got err %v, want ErrNotFound", err)
	}
}

func TestIdempotencyRepo_Put_OverwritesExisting(t *testing.T) {
	ctx := context.Background()
	d := setupTestDB(t)
	users := NewUserRepo(d)
	uid := seedUserForIdempotency(t, ctx, users, "alice")
	r := NewIdempotencyRepo(d)

	if err := r.Put(ctx, IdempotencyRecord{
		UserID: uid, Key: "k", StatusCode: 200, ContentType: "application/json",
		ResponseBody: []byte(`{"v":1}`), CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put1: %v", err)
	}
	if err := r.Put(ctx, IdempotencyRecord{
		UserID: uid, Key: "k", StatusCode: 201, ContentType: "application/json",
		ResponseBody: []byte(`{"v":2}`), CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put2: %v", err)
	}
	got, err := r.Get(ctx, uid, "k", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.StatusCode != 201 || string(got.ResponseBody) != `{"v":2}` {
		t.Errorf("overwrite: got status=%d body=%q, want 201 / {\"v\":2}", got.StatusCode, got.ResponseBody)
	}
}

func TestIdempotencyRepo_Get_PerUserIsolation(t *testing.T) {
	ctx := context.Background()
	d := setupTestDB(t)
	users := NewUserRepo(d)
	alice := seedUserForIdempotency(t, ctx, users, "alice")
	// The app is single-user (users.id is locked to 1 by CHECK), but the
	// idempotency table is keyed by user_id without a FK so we can still
	// exercise the (user_id, key) scoping with a synthetic second id.
	const bob int64 = 2
	r := NewIdempotencyRepo(d)

	if err := r.Put(ctx, IdempotencyRecord{
		UserID: alice, Key: "shared", StatusCode: 200, ContentType: "application/json",
		ResponseBody: []byte(`alice`), CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put alice: %v", err)
	}
	_, err := r.Get(ctx, bob, "shared", time.Now().Add(-time.Hour))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("bob saw alice's key: err %v, want ErrNotFound", err)
	}
}

func TestIdempotencyRepo_DeleteExpired(t *testing.T) {
	ctx := context.Background()
	d := setupTestDB(t)
	users := NewUserRepo(d)
	uid := seedUserForIdempotency(t, ctx, users, "alice")
	r := NewIdempotencyRepo(d)

	if err := r.Put(ctx, IdempotencyRecord{
		UserID: uid, Key: "old", StatusCode: 200, ContentType: "application/json",
		ResponseBody: []byte(`{}`), CreatedAt: time.Now().Add(-48 * time.Hour),
	}); err != nil {
		t.Fatalf("put old: %v", err)
	}
	if err := r.Put(ctx, IdempotencyRecord{
		UserID: uid, Key: "fresh", StatusCode: 200, ContentType: "application/json",
		ResponseBody: []byte(`{}`), CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put fresh: %v", err)
	}

	n, err := r.DeleteExpired(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted: got %d, want 1", n)
	}
	if _, err := r.Get(ctx, uid, "fresh", time.Now().Add(-time.Hour)); err != nil {
		t.Errorf("fresh row should survive: %v", err)
	}
}
