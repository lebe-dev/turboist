package calendar

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"golang.org/x/oauth2"
	gcal "google.golang.org/api/calendar/v3"
)

const testTokenKey = "test-token-key-32bytes-padding!!"

func setupCalSvc(t *testing.T) (*sql.DB, *Service, int64) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := repo.NewUserRepo(d)
	u, err := users.Create(context.Background(), "admin", "h")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	calRepo := repo.NewCalendarRepo(d)
	svc := NewService(calRepo, users, "http://test/", testTokenKey, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return d, svc, u.ID
}

func TestService_NewService_TrimsTrailingSlash(t *testing.T) {
	_, svc, _ := setupCalSvc(t)
	cfg, ok := svc.OAuthConfig("id", "secret")
	if !ok {
		t.Fatalf("expected ok")
	}
	want := "http://test/api/v1/calendars/google/callback"
	if cfg.RedirectURL != want {
		t.Errorf("RedirectURL: got %q, want %q", cfg.RedirectURL, want)
	}
}

func TestService_CipherAndCache_NonNil(t *testing.T) {
	_, svc, _ := setupCalSvc(t)
	if svc.Cipher() == nil {
		t.Errorf("Cipher must be non-nil")
	}
	if svc.Cache() == nil {
		t.Errorf("Cache must be non-nil")
	}
}

func TestService_OAuthConfig_EmptyReturnsFalse(t *testing.T) {
	_, svc, _ := setupCalSvc(t)
	if cfg, ok := svc.OAuthConfig("", "secret"); ok || cfg != nil {
		t.Errorf("empty client id must return false; got cfg=%v ok=%v", cfg, ok)
	}
	if cfg, ok := svc.OAuthConfig("id", ""); ok || cfg != nil {
		t.Errorf("empty client secret must return false; got cfg=%v ok=%v", cfg, ok)
	}
}

func TestService_OAuthConfig_Scopes(t *testing.T) {
	_, svc, _ := setupCalSvc(t)
	cfg, ok := svc.OAuthConfig("id", "secret")
	if !ok {
		t.Fatalf("expected ok")
	}
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != gcal.CalendarReadonlyScope {
		t.Errorf("scopes: got %v, want [%s]", cfg.Scopes, gcal.CalendarReadonlyScope)
	}
}

func TestService_OAuthConfigForUser_NotConfigured(t *testing.T) {
	_, svc, uid := setupCalSvc(t)
	cfg, ok, err := svc.OAuthConfigForUser(context.Background(), uid)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok || cfg != nil {
		t.Errorf("expected not configured, got cfg=%v ok=%v", cfg, ok)
	}
}

func TestService_OAuthConfigForUser_EncryptedRoundTrip(t *testing.T) {
	d, svc, uid := setupCalSvc(t)
	ctx := context.Background()
	cipher := crypto.NewTokenCipher(testTokenKey)
	encID, _ := cipher.Encrypt("client-id")
	encSecret, _ := cipher.Encrypt("client-secret")
	r := repo.NewCalendarRepo(d)
	if _, err := r.UpsertOAuthConfig(ctx, &model.CalendarOAuthConfig{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		ClientID: encID, ClientSecret: encSecret,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg, ok, err := svc.OAuthConfigForUser(ctx, uid)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok || cfg == nil {
		t.Fatalf("expected configured")
	}
	if cfg.ClientID != "client-id" || cfg.ClientSecret != "client-secret" {
		t.Errorf("decrypted creds: got id=%q secret=%q", cfg.ClientID, cfg.ClientSecret)
	}
}

func TestService_OAuthConfigForUser_MigratesPlainTextToEncrypted(t *testing.T) {
	d, svc, uid := setupCalSvc(t)
	ctx := context.Background()
	r := repo.NewCalendarRepo(d)
	// Seed with plain-text credentials (legacy format).
	if _, err := r.UpsertOAuthConfig(ctx, &model.CalendarOAuthConfig{
		UserID: uid, Provider: model.CalendarProviderGoogle,
		ClientID: "plain-id", ClientSecret: "plain-secret",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg, ok, err := svc.OAuthConfigForUser(ctx, uid)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok || cfg == nil || cfg.ClientID != "plain-id" || cfg.ClientSecret != "plain-secret" {
		t.Errorf("decrypted creds: got cfg=%+v", cfg)
	}

	// Verify the stored values are now encrypted.
	stored, err := r.GetOAuthConfig(ctx, uid, model.CalendarProviderGoogle)
	if err != nil {
		t.Fatalf("get stored: %v", err)
	}
	if !crypto.IsEncrypted(stored.ClientID) || !crypto.IsEncrypted(stored.ClientSecret) {
		t.Errorf("expected stored values to be re-encrypted, got id=%q secret=%q",
			stored.ClientID, stored.ClientSecret)
	}
}

func TestService_EncryptAccountTokens_RoundTrip(t *testing.T) {
	_, svc, _ := setupCalSvc(t)
	a := &model.CalendarAccount{
		AccessToken:  "plain-access",
		RefreshToken: "plain-refresh",
	}
	if err := svc.EncryptAccountTokens(a); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !crypto.IsEncrypted(a.AccessToken) || !crypto.IsEncrypted(a.RefreshToken) {
		t.Errorf("tokens should be encrypted after EncryptAccountTokens")
	}

	tk := &oauth2.Token{AccessToken: a.AccessToken, RefreshToken: a.RefreshToken}
	if err := svc.DecryptToken(tk); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if tk.AccessToken != "plain-access" || tk.RefreshToken != "plain-refresh" {
		t.Errorf("round-trip: got %+v", tk)
	}
}

func TestService_DecryptToken_PlainTextPassesThrough(t *testing.T) {
	_, svc, _ := setupCalSvc(t)
	tk := &oauth2.Token{AccessToken: "plain", RefreshToken: ""}
	if err := svc.DecryptToken(tk); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if tk.AccessToken != "plain" {
		t.Errorf("plain text should pass through, got %q", tk.AccessToken)
	}
}

func TestAccountToken_FieldsCopied(t *testing.T) {
	exp := time.Now().Add(time.Hour).UTC()
	a := &model.CalendarAccount{AccessToken: "at", RefreshToken: "rt", Expiry: exp}
	tk := accountToken(a)
	if tk.AccessToken != "at" || tk.RefreshToken != "rt" || !tk.Expiry.Equal(exp) || tk.TokenType != "Bearer" {
		t.Errorf("got %+v", tk)
	}
}

func TestTokenChanged(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		old  *oauth2.Token
		new  *oauth2.Token
		want bool
	}{
		{
			name: "identical",
			old:  &oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: now},
			new:  &oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: now},
			want: false,
		},
		{
			name: "access token differs",
			old:  &oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: now},
			new:  &oauth2.Token{AccessToken: "b", RefreshToken: "r", Expiry: now},
			want: true,
		},
		{
			name: "refresh token differs (both non-empty)",
			old:  &oauth2.Token{AccessToken: "a", RefreshToken: "r1", Expiry: now},
			new:  &oauth2.Token{AccessToken: "a", RefreshToken: "r2", Expiry: now},
			want: true,
		},
		{
			name: "refresh token empty in fresh ignored",
			old:  &oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: now},
			new:  &oauth2.Token{AccessToken: "a", RefreshToken: "", Expiry: now},
			want: false,
		},
		{
			name: "expiry differs",
			old:  &oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: now},
			new:  &oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: now.Add(time.Minute)},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenChanged(tc.old, tc.new); got != tc.want {
				t.Errorf("tokenChanged: got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGoogleEventToCalendarEvent_TitleFallback(t *testing.T) {
	src := model.CalendarSource{ID: 7, ExternalID: "primary", Summary: "Primary"}
	ev := &gcal.Event{
		Id:      "evt-1",
		Summary: "",
		Start:   &gcal.EventDateTime{DateTime: "2026-05-15T09:00:00Z"},
		End:     &gcal.EventDateTime{DateTime: "2026-05-15T10:00:00Z"},
	}
	out, ok := googleEventToCalendarEvent(ev, src)
	if !ok {
		t.Fatalf("expected ok")
	}
	if out.Title != "(No title)" {
		t.Errorf("title fallback: got %q, want %q", out.Title, "(No title)")
	}
	if !strings.HasPrefix(out.ID, "google:primary:evt-1") {
		t.Errorf("composite ID: got %q", out.ID)
	}
	if out.SourceID != 7 {
		t.Errorf("source id: got %d, want 7", out.SourceID)
	}
}

func TestGoogleEventToCalendarEvent_InvalidTimesReturnsFalse(t *testing.T) {
	src := model.CalendarSource{}
	_, ok := googleEventToCalendarEvent(&gcal.Event{Id: "x"}, src)
	if ok {
		t.Errorf("expected false for event with nil start/end")
	}
}

func TestEventsCacheKey_Stable(t *testing.T) {
	start, _ := time.Parse(time.RFC3339, "2026-05-15T00:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2026-05-22T00:00:00Z")
	sources := []model.CalendarSource{
		{ID: 1, ExternalID: "a"},
		{ID: 2, ExternalID: "b"},
	}
	k1 := EventsCacheKey(42, start, end, sources)
	k2 := EventsCacheKey(42, start, end, sources)
	if k1 != k2 {
		t.Errorf("key must be stable: %q vs %q", k1, k2)
	}
	if !strings.Contains(k1, "42") {
		t.Errorf("key should contain user id, got %q", k1)
	}
}

func TestEventCache_GetMiss(t *testing.T) {
	c := NewEventCache(time.Hour)
	if _, ok := c.Get("missing"); ok {
		t.Errorf("expected miss")
	}
}

func TestEventCache_SetThenGet(t *testing.T) {
	c := NewEventCache(time.Hour)
	items := []CalendarEvent{{ID: "x", Title: "X"}}
	c.Set("k", items)
	got, ok := c.Get("k")
	if !ok {
		t.Fatalf("expected hit")
	}
	if len(got) != 1 || got[0].ID != "x" {
		t.Errorf("got %+v", got)
	}
}

func TestEventCache_GetExpired(t *testing.T) {
	c := NewEventCache(1 * time.Millisecond)
	c.Set("k", []CalendarEvent{{ID: "x"}})
	time.Sleep(10 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Errorf("expected miss on expired entry")
	}
}

func TestEventCache_DeleteUser(t *testing.T) {
	c := NewEventCache(time.Hour)
	c.Set("1|a", []CalendarEvent{{ID: "u1a"}})
	c.Set("1|b", []CalendarEvent{{ID: "u1b"}})
	c.Set("2|a", []CalendarEvent{{ID: "u2a"}})
	c.DeleteUser(1)
	if _, ok := c.Get("1|a"); ok {
		t.Errorf("user 1 entries must be removed")
	}
	if _, ok := c.Get("1|b"); ok {
		t.Errorf("user 1 entries must be removed")
	}
	if _, ok := c.Get("2|a"); !ok {
		t.Errorf("user 2 entries must be preserved")
	}
}

func TestEventCache_GetReturnsClone(t *testing.T) {
	c := NewEventCache(time.Hour)
	original := []CalendarEvent{{ID: "x", Title: "orig"}}
	c.Set("k", original)
	got, _ := c.Get("k")
	got[0].Title = "mutated"
	again, _ := c.Get("k")
	if again[0].Title != "orig" {
		t.Errorf("cache must return a clone; mutation leaked: %q", again[0].Title)
	}
}
