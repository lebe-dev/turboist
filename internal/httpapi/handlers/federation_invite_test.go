package handlers_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

// TestCreateInvite_ReturnsFragmentSecretLink asserts that POST .../invites on a
// federated project returns a one-time link of the form
// https://{BASE_URL}/federation/join#invite=<id>.<secret> with the secret in the
// URL FRAGMENT (US-1.2 AC1, AC6), and that the stored row keeps only the SHA-256
// hash of the secret (US-1.2 AC2).
func TestCreateInvite_ReturnsFragmentSecretLink(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")

	// Enable federation first (precondition for invite creation).
	if resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/enable", p.ID), nil)); resp.StatusCode != http.StatusOK {
		t.Fatalf("enable: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/invites", p.ID),
		map[string]any{"permissions": "write"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create invite: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	var got dto.CreateInviteResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.InviteID == "" {
		t.Errorf("inviteId empty")
	}
	if got.Secret == "" {
		t.Errorf("secret empty (must be returned once to the owner UI)")
	}
	if got.Permissions != "write" {
		t.Errorf("permissions: got %q, want write", got.Permissions)
	}
	if got.ExpiresAt == "" {
		t.Errorf("expiresAt empty (default 7d expected)")
	}
	if got.MaxUses != 1 {
		t.Errorf("maxUses: got %d, want 1 (default)", got.MaxUses)
	}

	// US-1.2 AC1 + AC6: the link is .../federation/join#invite=<id>.<secret> with
	// the secret in the fragment (after the '#'), so it never reaches the server
	// in the request line and never lands in access logs.
	wantPrefix := testBaseURL + "/federation/join#invite="
	if !strings.HasPrefix(got.Link, wantPrefix) {
		t.Fatalf("link prefix: got %q, want prefix %q", got.Link, wantPrefix)
	}
	frag := strings.TrimPrefix(got.Link, testBaseURL+"/federation/join#")
	if !strings.HasPrefix(frag, "invite=") {
		t.Fatalf("fragment must start with invite=, got %q", frag)
	}
	if strings.Contains(got.Link, "?") {
		t.Errorf("link must not carry the secret in a query string: %q", got.Link)
	}
	frag = strings.TrimPrefix(frag, "invite=")
	parts := strings.SplitN(frag, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("fragment must be <id>.<secret>, got %q", frag)
	}
	if parts[0] != got.InviteID {
		t.Errorf("link id: got %q, want %q", parts[0], got.InviteID)
	}
	if parts[1] != got.Secret {
		t.Errorf("link secret does not match returned secret")
	}

	// US-1.2 AC2: DB stores only SHA-256(secret), never plaintext.
	stored, err := e.fedInvites.Get(context.Background(), got.InviteID)
	if err != nil {
		t.Fatalf("get stored invite: %v", err)
	}
	sum := sha256.Sum256([]byte(got.Secret))
	if stored.SecretHash != hex.EncodeToString(sum[:]) {
		t.Errorf("stored secret_hash != SHA-256(secret)")
	}
	if stored.SecretHash == got.Secret {
		t.Errorf("stored secret_hash equals plaintext secret — not hashed")
	}
}

// TestCreateInvite_NotEnabled asserts a 400 with CodeFederationNotEnabled when
// federation has not been enabled on the project (US-1.1 AC3).
func TestCreateInvite_NotEnabled(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared") // NOT federated

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/invites", p.ID),
		map[string]any{"permissions": "write"}))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("create invite on non-federated project: got %d, want 400; body: %s", resp.StatusCode, body)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code != httpapi.CodeFederationNotEnabled {
		t.Errorf("error code: got %q, want %q", env.Error.Code, httpapi.CodeFederationNotEnabled)
	}
}

// TestCreateInvite_NonExistentProject asserts a 404 for an unknown project.
func TestCreateInvite_NonExistentProject(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/projects/99999/invites", map[string]any{"permissions": "write"}))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("create invite for unknown project: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

// TestCreateInvite_InvalidPermissions asserts a 400 for a permission outside
// read/write/admin.
func TestCreateInvite_InvalidPermissions(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	if resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/enable", p.ID), nil)); resp.StatusCode != http.StatusOK {
		t.Fatalf("enable: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/invites", p.ID),
		map[string]any{"permissions": "owner"}))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid permissions: got %d, want 400; body: %s", resp.StatusCode, body)
	}
}
