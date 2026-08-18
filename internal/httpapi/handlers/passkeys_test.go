package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lebe-dev/turboist/internal/repo"
)

type passkeyDTO struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt *string `json:"lastUsedAt"`
}

type ceremonyResp struct {
	CeremonyID string `json:"ceremonyId"`
	Options    struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			RP        struct {
				ID string `json:"id"`
			} `json:"rp"`
			ExcludeCredentials []struct {
				ID string `json:"id"`
			} `json:"excludeCredentials"`
		} `json:"publicKey"`
	} `json:"options"`
}

func seedPasskey(t *testing.T, e *apiEnv, credentialID, name string) int64 {
	t.Helper()
	created, err := e.passkeys.Create(context.Background(), repo.CreateCredentialParams{
		UserID:       1,
		CredentialID: credentialID,
		Name:         name,
		// The blob is the go-webauthn Credential Record; only the id has to
		// decode cleanly for the paths these tests exercise.
		Credential: []byte(`{"id":"AQ=="}`),
	})
	if err != nil {
		t.Fatalf("seed passkey: %v", err)
	}
	return created.ID
}

func TestPasskeysHandler_List(t *testing.T) {
	e := setupAPIEnv(t)
	seedPasskey(t, e, "cred-1", "MacBook")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/passkeys", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var got []passkeyDTO
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v; body: %s", err, body)
	}
	if len(got) != 1 {
		t.Fatalf("list: got %d entries, want 1", len(got))
	}
	if got[0].Name != "MacBook" {
		t.Errorf("name: got %q, want MacBook", got[0].Name)
	}
	if got[0].LastUsedAt != nil {
		t.Errorf("lastUsedAt: got %v, want null for an unused passkey", *got[0].LastUsedAt)
	}
}

func TestPasskeysHandler_RegisterBegin(t *testing.T) {
	e := setupAPIEnv(t)
	seedPasskey(t, e, "cred-1", "MacBook")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/passkeys/register/begin", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var got ceremonyResp
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v; body: %s", err, body)
	}
	if got.CeremonyID == "" {
		t.Error("ceremonyId: got empty string, want an opaque id")
	}
	if got.Options.PublicKey.Challenge == "" {
		t.Error("challenge: got empty string, want a generated challenge")
	}
	if got.Options.PublicKey.RP.ID != "localhost" {
		t.Errorf("rp id: got %q, want localhost", got.Options.PublicKey.RP.ID)
	}
	if len(got.Options.PublicKey.ExcludeCredentials) != 1 {
		t.Errorf("excludeCredentials: got %d, want the already-registered credential",
			len(got.Options.PublicKey.ExcludeCredentials))
	}
}

func TestPasskeysHandler_RegisterFinish_UnknownCeremony(t *testing.T) {
	e := setupAPIEnv(t)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/passkeys/register/finish",
		map[string]any{"ceremonyId": "nope", "credential": map[string]any{"id": "x"}}))
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400; body: %s", resp.StatusCode, body)
	}
	if code := parseErr(t, body).Error.Code; code != "passkey_ceremony_invalid" {
		t.Errorf("code: got %q, want passkey_ceremony_invalid", code)
	}
}

func TestPasskeysHandler_RegisterFinish_ValidatesBody(t *testing.T) {
	e := setupAPIEnv(t)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/passkeys/register/finish",
		map[string]any{"credential": map[string]any{"id": "x"}}))
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400; body: %s", resp.StatusCode, body)
	}
	if code := parseErr(t, body).Error.Code; code != "validation_failed" {
		t.Errorf("code: got %q, want validation_failed", code)
	}
}

func TestPasskeysHandler_Rename(t *testing.T) {
	e := setupAPIEnv(t)
	id := seedPasskey(t, e, "cred-1", "MacBook")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		"/api/v1/passkeys/"+itoa(id), map[string]any{"name": "  iPhone  "}))
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var got passkeyDTO
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v; body: %s", err, body)
	}
	if got.Name != "iPhone" {
		t.Errorf("name: got %q, want iPhone", got.Name)
	}
}

func TestPasskeysHandler_Rename_NotFound(t *testing.T) {
	e := setupAPIEnv(t)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		"/api/v1/passkeys/999", map[string]any{"name": "x"}))
	if resp.StatusCode != 404 {
		t.Fatalf("status: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

func TestPasskeysHandler_Delete(t *testing.T) {
	e := setupAPIEnv(t)
	id := seedPasskey(t, e, "cred-1", "MacBook")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/passkeys/"+itoa(id), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("status: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	rows, err := e.passkeys.ListByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("remaining passkeys: got %d, want 0", len(rows))
	}
}

// A leaked API token must not be able to enrol or drop a passkey: the group is
// mounted behind RequireJWTAuth precisely to keep that door shut.
func TestPasskeysHandler_RejectsAPITokenAuth(t *testing.T) {
	e := setupAPIEnv(t)
	token := issueAPIToken(t, e, "full", []string{"*"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/passkeys", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != 401 && resp.StatusCode != 403 {
		t.Fatalf("status: got %d, want 401 or 403; body: %s", resp.StatusCode, body)
	}
}

func TestPasskeysHandler_RequiresAuth(t *testing.T) {
	e := setupAPIEnv(t)

	resp, body := doReq(t, e.app, httptest.NewRequest(http.MethodGet, "/api/v1/passkeys", nil))
	if resp.StatusCode != 401 {
		t.Fatalf("status: got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

// GET /api/config is unauthenticated: the login page reads it before it has any
// token, to decide whether a passkey button would lead anywhere.
func TestPublicConfig_PasskeyAvailability(t *testing.T) {
	e := setupAPIEnv(t)

	var got struct {
		Passkeys struct {
			Enabled   bool `json:"enabled"`
			Available bool `json:"available"`
		} `json:"passkeys"`
	}
	resp, body := doReq(t, e.app, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v; body: %s", err, body)
	}
	if !got.Passkeys.Enabled {
		t.Error("enabled: got false, want true when the service is wired")
	}
	if got.Passkeys.Available {
		t.Error("available: got true, want false before any passkey is registered")
	}

	seedPasskey(t, e, "cred-1", "MacBook")
	resp, body = doReq(t, e.app, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v; body: %s", err, body)
	}
	if !got.Passkeys.Available {
		t.Error("available: got false, want true once a passkey is registered")
	}
}
