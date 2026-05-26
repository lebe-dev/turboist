package repo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func setupCalendarTest(t *testing.T) (*sql.DB, *CalendarRepo, int64) {
	t.Helper()
	d := setupTestDB(t)
	u, err := NewUserRepo(d).Create(context.Background(), "admin", "h")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return d, NewCalendarRepo(d), u.ID
}

func TestCalendarRepo_OAuthConfig_GetNotFound(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	_, err := r.GetOAuthConfig(context.Background(), uid, model.CalendarProviderGoogle)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCalendarRepo_OAuthConfig_UpsertCreate(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	in := &model.CalendarOAuthConfig{
		UserID:       uid,
		Provider:     model.CalendarProviderGoogle,
		ClientID:     "cid",
		ClientSecret: "secret",
	}
	got, err := r.UpsertOAuthConfig(ctx, in)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got.ClientID != "cid" || got.ClientSecret != "secret" {
		t.Errorf("got %+v", got)
	}
	if got.ID == 0 {
		t.Errorf("expected ID assigned")
	}
}

func TestCalendarRepo_OAuthConfig_UpsertPreservesExistingOnEmptyFields(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	_, err := r.UpsertOAuthConfig(ctx, &model.CalendarOAuthConfig{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		ClientID: "original-id", ClientSecret: "original-secret",
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	// Update with empty ClientID (e.g. partial update from UI) — should keep original.
	got, err := r.UpsertOAuthConfig(ctx, &model.CalendarOAuthConfig{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		ClientID: "", ClientSecret: "new-secret",
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if got.ClientID != "original-id" {
		t.Errorf("ClientID should be preserved when empty in input, got %q", got.ClientID)
	}
	if got.ClientSecret != "new-secret" {
		t.Errorf("ClientSecret should be updated, got %q", got.ClientSecret)
	}
}

func TestCalendarRepo_OAuthConfig_Delete(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	if _, err := r.UpsertOAuthConfig(ctx, &model.CalendarOAuthConfig{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		ClientID: "cid", ClientSecret: "s",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := r.DeleteOAuthConfig(ctx, uid, model.CalendarProviderGoogle); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.GetOAuthConfig(ctx, uid, model.CalendarProviderGoogle); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestCalendarRepo_OAuthConfig_DeleteNotFound(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	if err := r.DeleteOAuthConfig(context.Background(), uid, model.CalendarProviderGoogle); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound when nothing to delete, got %v", err)
	}
}

func TestCalendarRepo_OAuthState_ConsumeRoundTrip(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	state := "random-state-token"
	if err := r.CreateOAuthState(ctx, state, uid, 0, model.CalendarProviderGoogle, time.Minute); err != nil {
		t.Fatalf("create state: %v", err)
	}

	gotUID, err := r.ConsumeOAuthState(ctx, state, model.CalendarProviderGoogle)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if gotUID != uid {
		t.Errorf("user id: got %d, want %d", gotUID, uid)
	}

	// Second consume must fail — state is single-use.
	if _, err := r.ConsumeOAuthState(ctx, state, model.CalendarProviderGoogle); !errors.Is(err, ErrNotFound) {
		t.Errorf("second consume must fail: got %v", err)
	}
}

func TestCalendarRepo_OAuthState_ConsumeExpired(t *testing.T) {
	d, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	state := "expired-state"
	// Insert directly with expiry in the past so DeleteExpiredOAuthStates doesn't
	// nuke it before we observe the expired path.
	now := time.Now()
	if _, err := d.ExecContext(ctx,
		`INSERT INTO calendar_oauth_states (state, user_id, session_id, provider, expires_at, created_at)
		 VALUES (?, ?, 0, ?, ?, ?)`,
		state, uid, string(model.CalendarProviderGoogle),
		model.FormatUTC(now.Add(-1*time.Hour)), model.FormatUTC(now.Add(-2*time.Hour))); err != nil {
		t.Fatalf("inject expired state: %v", err)
	}
	if _, err := r.ConsumeOAuthState(ctx, state, model.CalendarProviderGoogle); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for expired state, got %v", err)
	}
}

func TestCalendarRepo_OAuthState_ConsumeWrongProvider(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	state := "wrong-provider"
	if err := r.CreateOAuthState(ctx, state, uid, 0, model.CalendarProviderGoogle, time.Minute); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := r.ConsumeOAuthState(ctx, state, model.CalendarProvider("apple")); !errors.Is(err, ErrNotFound) {
		t.Errorf("wrong provider must not consume: got %v", err)
	}
}

func TestCalendarRepo_OAuthState_DeleteExpired(t *testing.T) {
	d, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	now := time.Now()
	if _, err := d.ExecContext(ctx,
		`INSERT INTO calendar_oauth_states (state, user_id, session_id, provider, expires_at, created_at)
		 VALUES (?, ?, 0, ?, ?, ?)`,
		"expired-1", uid, string(model.CalendarProviderGoogle),
		model.FormatUTC(now.Add(-1*time.Hour)), model.FormatUTC(now.Add(-2*time.Hour))); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if err := r.CreateOAuthState(ctx, "fresh", uid, 0, model.CalendarProviderGoogle, time.Minute); err != nil {
		t.Fatalf("create fresh: %v", err)
	}
	// CreateOAuthState calls DeleteExpiredOAuthStates internally.
	var n int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM calendar_oauth_states WHERE state = ?`, "expired-1").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("expired state should be cleaned up, got count=%d", n)
	}
}

func TestCalendarRepo_Account_UpsertCreate(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	in := &model.CalendarAccount{
		UserID:       uid,
		Provider:     model.CalendarProviderGoogle,
		Email:        "user@example.com",
		DisplayName:  "User",
		AccessToken:  "at",
		RefreshToken: "rt",
		Expiry:       time.Now().UTC().Add(time.Hour),
	}
	got, err := r.UpsertAccount(ctx, in)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got.Email != "user@example.com" || got.AccessToken != "at" || got.RefreshToken != "rt" {
		t.Errorf("got %+v", got)
	}
	if got.ID == 0 {
		t.Errorf("expected ID assigned")
	}
}

func TestCalendarRepo_Account_UpsertPreservesRefreshToken(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	if _, err := r.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		Email: "u@example.com", AccessToken: "at1", RefreshToken: "rt-original",
		Expiry: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Subsequent upsert from OAuth refresh — only access_token comes back, refresh is empty.
	got, err := r.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		Email: "u@example.com", AccessToken: "at2", RefreshToken: "",
		Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if got.AccessToken != "at2" {
		t.Errorf("access token should update, got %q", got.AccessToken)
	}
	if got.RefreshToken != "rt-original" {
		t.Errorf("refresh token should be preserved when empty in input, got %q", got.RefreshToken)
	}
}

func TestCalendarRepo_Account_GetNotFound(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	if _, err := r.GetAccountByProvider(context.Background(), uid, model.CalendarProviderGoogle); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCalendarRepo_Account_UpdateToken(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	if _, err := r.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		Email: "u@e", AccessToken: "at1", RefreshToken: "rt1",
		Expiry: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	newExpiry := time.Now().Add(2 * time.Hour).UTC()
	got, err := r.UpdateAccountToken(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		AccessToken: "at2", RefreshToken: "rt2", Expiry: newExpiry,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.AccessToken != "at2" || got.RefreshToken != "rt2" {
		t.Errorf("got %+v", got)
	}
}

func TestCalendarRepo_Account_UpdateTokenNotFound(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	_, err := r.UpdateAccountToken(context.Background(), &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		AccessToken: "at", RefreshToken: "rt", Expiry: time.Now(),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCalendarRepo_Account_ListEmpty(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	got, err := r.ListAccounts(context.Background(), uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestCalendarRepo_Account_Delete(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	a, err := r.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		Email: "u@e", AccessToken: "at", RefreshToken: "rt",
		Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := r.DeleteAccount(ctx, uid, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.GetAccountByProvider(ctx, uid, model.CalendarProviderGoogle); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestCalendarRepo_Account_DeleteNotFound(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	if err := r.DeleteAccount(context.Background(), uid, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCalendarRepo_Sources_UpsertAndList(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	acc, err := r.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		Email: "u@e", AccessToken: "at", RefreshToken: "rt", Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("acc: %v", err)
	}
	sources := []model.CalendarSource{
		{ExternalID: "cal-1", Summary: "Primary", Color: "blue", Selected: true, IsPrimary: true},
		{ExternalID: "cal-2", Summary: "Work", Color: "red", Selected: false},
	}
	if err := r.UpsertSources(ctx, acc, sources); err != nil {
		t.Fatalf("upsert sources: %v", err)
	}
	got, err := r.ListSources(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("count: got %d, want 2", len(got))
	}
}

func TestCalendarRepo_Sources_UpsertDeletesStale(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	acc, err := r.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		Email: "u@e", AccessToken: "at", RefreshToken: "rt", Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("acc: %v", err)
	}
	if err := r.UpsertSources(ctx, acc, []model.CalendarSource{
		{ExternalID: "a", Summary: "A"},
		{ExternalID: "b", Summary: "B"},
		{ExternalID: "c", Summary: "C"},
	}); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Subsequent upsert returns fewer sources — stale ones must be removed.
	if err := r.UpsertSources(ctx, acc, []model.CalendarSource{
		{ExternalID: "b", Summary: "B updated"},
	}); err != nil {
		t.Fatalf("second: %v", err)
	}
	got, err := r.ListSources(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ExternalID != "b" || got[0].Summary != "B updated" {
		t.Errorf("expected only updated b, got %+v", got)
	}
}

func TestCalendarRepo_Sources_UpsertEmptyDeletesAll(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	acc, err := r.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		Email: "u@e", AccessToken: "at", RefreshToken: "rt", Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("acc: %v", err)
	}
	if err := r.UpsertSources(ctx, acc, []model.CalendarSource{{ExternalID: "x", Summary: "X"}}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := r.UpsertSources(ctx, acc, nil); err != nil {
		t.Fatalf("empty: %v", err)
	}
	got, err := r.ListSources(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected zero sources, got %d", len(got))
	}
}

func TestCalendarRepo_Sources_UpsertSummaryFallsBackToExternalID(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	acc, err := r.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		Email: "u@e", AccessToken: "at", RefreshToken: "rt", Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("acc: %v", err)
	}
	if err := r.UpsertSources(ctx, acc, []model.CalendarSource{{ExternalID: "cal-x", Summary: ""}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := r.ListSources(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Summary != "cal-x" {
		t.Errorf("empty summary should fall back to external_id, got %+v", got)
	}
}

func TestCalendarRepo_Sources_ListSelected(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	acc, err := r.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		Email: "u@e", AccessToken: "at", RefreshToken: "rt", Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("acc: %v", err)
	}
	if err := r.UpsertSources(ctx, acc, []model.CalendarSource{
		{ExternalID: "yes", Summary: "yes", Selected: true},
		{ExternalID: "no", Summary: "no", Selected: false},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := r.ListSelectedSources(ctx, uid, model.CalendarProviderGoogle)
	if err != nil {
		t.Fatalf("list selected: %v", err)
	}
	if len(got) != 1 || got[0].ExternalID != "yes" {
		t.Errorf("expected one selected source, got %+v", got)
	}
}

func TestCalendarRepo_Sources_SetSourceSelected(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	ctx := context.Background()
	acc, err := r.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		Email: "u@e", AccessToken: "at", RefreshToken: "rt", Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("acc: %v", err)
	}
	if err := r.UpsertSources(ctx, acc, []model.CalendarSource{
		{ExternalID: "cal-x", Summary: "X", Selected: true},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	srcs, err := r.ListSources(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	id := srcs[0].ID
	updated, err := r.SetSourceSelected(ctx, uid, id, false)
	if err != nil {
		t.Fatalf("set selected: %v", err)
	}
	if updated.Selected {
		t.Errorf("expected selected=false")
	}
}

func TestCalendarRepo_Sources_SetSourceSelectedNotFound(t *testing.T) {
	_, r, uid := setupCalendarTest(t)
	_, err := r.SetSourceSelected(context.Background(), uid, 9999, true)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
