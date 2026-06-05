package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// getAudit GETs the JWT audit endpoint and decodes the response.
func getAudit(t *testing.T, e *apiEnv, query string) (int, dto.AuditResponseDTO) {
	t.Helper()
	url := "/api/v1/federation/audit"
	if query != "" {
		url += "?" + query
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, url, nil))
	var out dto.AuditResponseDTO
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("parse: %v; body: %s", err, body)
		}
	}
	return resp.StatusCode, out
}

// TestFederationAudit_ListsRowsWithRequiredFields asserts the audit endpoint
// returns one row per recorded event with timestamp, peer, kind, and outcome
// (Federation v1 F6.3, US-7.4 AC1), newest-first.
func TestFederationAudit_ListsRowsWithRequiredFields(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()
	base := time.Now()
	if err := e.fedAudit.Insert(ctx, repo.AuditEntry{
		Kind: repo.AuditKindHandshake, Outcome: repo.AuditOutcomeAccepted,
		PeerInstanceURL: "https://alice.example", Detail: "handshake accepted", CreatedAt: base,
	}); err != nil {
		t.Fatalf("insert handshake: %v", err)
	}
	if err := e.fedAudit.Insert(ctx, repo.AuditEntry{
		Kind: repo.AuditKindReplay, Outcome: repo.AuditOutcomeRejected,
		PeerInstanceURL: "https://bob.example", Detail: "nonce replay", CreatedAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("insert replay: %v", err)
	}

	status, out := getAudit(t, e, "")
	if status != http.StatusOK {
		t.Fatalf("audit: got %d, want 200", status)
	}
	if len(out.Entries) != 2 {
		t.Fatalf("entries: got %d, want 2", len(out.Entries))
	}
	// Newest-first.
	first := out.Entries[0]
	if first.Kind != "replay" {
		t.Errorf("entries[0].kind: got %q, want replay (newest-first)", first.Kind)
	}
	if first.PeerInstanceUrl != "https://bob.example" {
		t.Errorf("entries[0].peerInstanceUrl: got %q, want https://bob.example", first.PeerInstanceUrl)
	}
	if first.Outcome != "rejected" {
		t.Errorf("entries[0].outcome: got %q, want rejected", first.Outcome)
	}
	if first.CreatedAt == "" {
		t.Errorf("entries[0].createdAt is empty")
	}
}

// TestFederationAudit_FiltersByPeerAndPaginates asserts the ?peer= filter and the
// limit/offset pagination narrow the result (Federation v1 F6.3, US-7.4 AC1).
func TestFederationAudit_FiltersByPeerAndPaginates(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()
	base := time.Now()
	for i, peer := range []string{"https://alice.example", "https://bob.example", "https://alice.example"} {
		if err := e.fedAudit.Insert(ctx, repo.AuditEntry{
			Kind: repo.AuditKindReplay, Outcome: repo.AuditOutcomeRejected,
			PeerInstanceURL: peer, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	_, byPeer := getAudit(t, e, "peer=https://alice.example")
	if len(byPeer.Entries) != 2 {
		t.Fatalf("alice entries: got %d, want 2", len(byPeer.Entries))
	}
	for _, row := range byPeer.Entries {
		if row.PeerInstanceUrl != "https://alice.example" {
			t.Errorf("filtered peer: got %q, want only alice", row.PeerInstanceUrl)
		}
	}

	_, limited := getAudit(t, e, "limit=1")
	if len(limited.Entries) != 1 {
		t.Errorf("limit=1 entries: got %d, want 1", len(limited.Entries))
	}
}

// TestFederationAudit_AttackAlert asserts a burst of signature failures from one
// peer surfaces a "possible attack on peer X" alert with the count + threshold
// (Federation v1 F6.3, US-7.4 AC3). The harness wires threshold=3 / window=1h.
func TestFederationAudit_AttackAlert(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()
	now := time.Now()
	// The flagged peer must be a KNOWN federated peer (F6.3 review C: the alert
	// counts only known peers, so a stranger spoofing the X-Federation-Instance
	// header cannot raise a bogus alert). "attacker.example" is a real joined peer
	// whose burst of signature failures is the anomaly the alert exists to surface.
	if err := e.fedInstances.Upsert(ctx, model.FederatedInstance{
		InstanceURL: "https://attacker.example", PublicKey: "pk", DisplayName: "Attacker", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed attacker instance: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := e.fedAudit.Insert(ctx, repo.AuditEntry{
			Kind: repo.AuditKindSignatureInvalid, Outcome: repo.AuditOutcomeRejected,
			PeerInstanceURL: "https://attacker.example", CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	status, out := getAudit(t, e, "")
	if status != http.StatusOK {
		t.Fatalf("audit: got %d, want 200", status)
	}
	if len(out.Alerts) != 1 {
		t.Fatalf("alerts: got %d, want 1", len(out.Alerts))
	}
	alert := out.Alerts[0]
	if alert.PeerInstanceUrl != "https://attacker.example" {
		t.Errorf("alert peer: got %q, want https://attacker.example", alert.PeerInstanceUrl)
	}
	if alert.Count < 3 {
		t.Errorf("alert count: got %d, want >=3", alert.Count)
	}
	if alert.Threshold != 3 {
		t.Errorf("alert threshold: got %d, want 3", alert.Threshold)
	}
}

// TestFederationAudit_DetailNeverLeaksSecret asserts the audit detail field never
// echoes the raw signature/secret — only a short coded reason (§7 F6.3 "never
// persist secrets/signatures/tokens").
func TestFederationAudit_DetailNeverLeaksSecret(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()
	if err := e.fedAudit.Insert(ctx, repo.AuditEntry{
		Kind: repo.AuditKindSignatureInvalid, Outcome: repo.AuditOutcomeRejected,
		PeerInstanceURL: "https://peer.example", Detail: "transport signature invalid", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, out := getAudit(t, e, "")
	if len(out.Entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(out.Entries))
	}
	if out.Entries[0].Detail != "transport signature invalid" {
		t.Errorf("detail: got %q, want the coded reason", out.Entries[0].Detail)
	}
}
