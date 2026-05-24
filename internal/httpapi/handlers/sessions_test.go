package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

func (e *apiEnv) tokenFor(t *testing.T, userID, sessionID int64) string {
	t.Helper()
	tok, _, err := e.jwt.Issue(userID, sessionID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func (e *apiEnv) sessionReq(t *testing.T, method, url string, sessionID int64, body any) *http.Request {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, url, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.tokenFor(t, 1, sessionID))
	return req
}

// seedSession inserts an active session for user=1 with a unique token_hash.
func (e *apiEnv) seedSession(t *testing.T, kind model.ClientKind, ua string) int64 {
	t.Helper()
	return e.seedSessionWithIP(t, kind, ua, "")
}

func (e *apiEnv) seedSessionWithIP(t *testing.T, kind model.ClientKind, ua, ip string) int64 {
	t.Helper()
	s, err := e.sessions.Create(context.Background(), repo.CreateSessionParams{
		UserID:     1,
		TokenHash:  fmt.Sprintf("hash-%d-%s", time.Now().UnixNano(), kind),
		ClientKind: kind,
		UserAgent:  ua,
		IPAddress:  ip,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return s.ID
}

func TestSessionsHandler_List_MarksCurrent(t *testing.T) {
	e := setupAPIEnv(t)
	rawUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"
	currentID := e.seedSessionWithIP(t, model.ClientWeb, rawUA, "203.0.113.10")
	otherID := e.seedSessionWithIP(t, model.ClientIOS, "Turboist-iOS/1.0 (iPhone; iOS 17.2)", "")

	resp, err := e.app.Test(e.sessionReq(t, http.MethodGet, "/api/v1/sessions/", currentID, nil))
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var got []dto.SessionDTO
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("sessions: got %d, want 2", len(got))
	}
	byID := map[int64]dto.SessionDTO{got[0].ID: got[0], got[1].ID: got[1]}
	cur, ok := byID[currentID]
	if !ok {
		t.Fatalf("current session missing from response")
	}
	if !cur.IsCurrent {
		t.Errorf("isCurrent for current session: got false, want true")
	}
	if cur.DisplayName != "Chrome on macOS" {
		t.Errorf("displayName: got %q, want Chrome on macOS", cur.DisplayName)
	}
	if cur.UserAgent != rawUA {
		t.Errorf("raw userAgent: got %q, want %q", cur.UserAgent, rawUA)
	}
	if cur.IPAddress != "203.0.113.10" {
		t.Errorf("ipAddress: got %q, want 203.0.113.10", cur.IPAddress)
	}
	other := byID[otherID]
	if other.IsCurrent {
		t.Errorf("isCurrent for other session: got true, want false")
	}
	if other.DisplayName != "Turboist iOS on iOS" {
		t.Errorf("other displayName: got %q, want Turboist iOS on iOS", other.DisplayName)
	}
}

func TestSessionsHandler_List_SkipsRevokedAndExpired(t *testing.T) {
	e := setupAPIEnv(t)
	active := e.seedSession(t, model.ClientWeb, "ua-active")
	revoked := e.seedSession(t, model.ClientCLI, "ua-revoked")
	if err := e.sessions.Revoke(context.Background(), revoked); err != nil {
		t.Fatalf("revoke seed: %v", err)
	}

	resp, err := e.app.Test(e.sessionReq(t, http.MethodGet, "/api/v1/sessions/", active, nil))
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	var got []dto.SessionDTO
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != active {
		t.Errorf("sessions: got %+v, want only id=%d", got, active)
	}
}

func TestSessionsHandler_Revoke_Ok(t *testing.T) {
	e := setupAPIEnv(t)
	current := e.seedSession(t, model.ClientWeb, "ua-current")
	target := e.seedSession(t, model.ClientIOS, "ua-target")

	resp, err := e.app.Test(e.sessionReq(t, http.MethodDelete, fmt.Sprintf("/api/v1/sessions/%d", target), current, nil))
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204", resp.StatusCode)
	}

	got, err := e.sessions.Get(context.Background(), target)
	if err != nil {
		t.Fatalf("get target: %v", err)
	}
	if got.RevokedAt == nil {
		t.Errorf("revoked_at: got nil, want non-nil")
	}

	// Current session must remain active.
	cur, err := e.sessions.Get(context.Background(), current)
	if err != nil {
		t.Fatalf("get current: %v", err)
	}
	if cur.RevokedAt != nil {
		t.Errorf("current session revoked unexpectedly")
	}
}

func TestSessionsHandler_Revoke_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	current := e.seedSession(t, model.ClientWeb, "ua")
	resp, err := e.app.Test(e.sessionReq(t, http.MethodDelete, "/api/v1/sessions/99999", current, nil))
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", resp.StatusCode)
	}
}

func TestSessionsHandler_RejectsAPITokenAuth(t *testing.T) {
	e := setupAPIEnv(t)
	e.seedSession(t, model.ClientWeb, "ua")

	plain, err := auth.GenerateAPIToken()
	if err != nil {
		t.Fatalf("generate api token: %v", err)
	}
	hash := auth.HashAPIToken(plain, e.apiTokenSalt)
	if _, err := e.apiTokens.Create(context.Background(), 1, "test", hash); err != nil {
		t.Fatalf("create api token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Errorf("status: got %d, want 401/403", resp.StatusCode)
	}
}
