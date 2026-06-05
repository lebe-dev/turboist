package federation_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// captureAuditor records the audit entries the service emits for control-plane
// trust actions (Federation v1 F6.3, US-7.4 AC1).
type captureAuditor struct {
	mu      sync.Mutex
	entries []repo.AuditEntry
}

func (c *captureAuditor) Record(e repo.AuditEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
}

func (c *captureAuditor) only(kind repo.AuditKind) *repo.AuditEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.entries {
		if c.entries[i].Kind == kind {
			return &c.entries[i]
		}
	}
	return nil
}

// TestRevokePeer_RecordsAudit asserts a revoke records one accepted revoke audit
// row carrying the peer (Federation v1 F6.3, US-7.4 AC1).
func TestRevokePeer_RecordsAudit(t *testing.T) {
	svc, projects, fp, instances, _ := newRevokeSvc(t, "https://me.example")
	auditor := &captureAuditor{}
	svc = svc.WithAuditor(auditor).WithRevokeSender(func(context.Context, string, []string) error { return nil })
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	if err := svc.RevokePeer(ctx, pid, "https://bob.example"); err != nil {
		t.Fatalf("RevokePeer: %v", err)
	}

	got := auditor.only(repo.AuditKindRevoke)
	if got == nil {
		t.Fatalf("expected a revoke audit row")
	}
	if got.Outcome != repo.AuditOutcomeAccepted {
		t.Errorf("revoke outcome: got %q, want accepted", got.Outcome)
	}
	if got.PeerInstanceURL != "https://bob.example" {
		t.Errorf("revoke peer: got %q, want https://bob.example", got.PeerInstanceURL)
	}
}

// TestTrustPeerKey_RecordsAudit asserts the manual key-trust records one accepted
// trust_key audit row whose detail carries NO key bytes (Federation v1 F6.3,
// US-7.4 AC1; §7 "never persist secrets/signatures/tokens").
func TestTrustPeerKey_RecordsAudit(t *testing.T) {
	newKey := newKeyB64(t)
	fetch := func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: newKey, DisplayName: "Bob"}, nil
	}
	auditor := &captureAuditor{}
	svc, projects, fp, instances, incidents, _ := newTrustKeySvc(t, "https://me.example", fetch)
	svc = svc.WithAuditor(auditor)
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)
	if _, err := fp.MarkKeyMismatch(ctx, pid, "https://bob.example", "2026-06-03T10:00:00.000Z"); err != nil {
		t.Fatalf("mark mismatch: %v", err)
	}
	if _, err := incidents.RecordKeyChange(ctx, pid, "https://bob.example", "pk", time.Now()); err != nil {
		t.Fatalf("record incident: %v", err)
	}

	if err := svc.TrustPeerKey(ctx, pid, "https://bob.example"); err != nil {
		t.Fatalf("TrustPeerKey: %v", err)
	}

	got := auditor.only(repo.AuditKindTrustKey)
	if got == nil {
		t.Fatalf("expected a trust_key audit row")
	}
	if got.Outcome != repo.AuditOutcomeAccepted {
		t.Errorf("trust_key outcome: got %q, want accepted", got.Outcome)
	}
	if got.Detail == "" {
		t.Errorf("trust_key detail should carry a coded reason, got empty")
	}
	if strings.Contains(got.Detail, newKey) {
		t.Errorf("trust_key detail must NOT contain the key bytes (§7), got %q", got.Detail)
	}
}

// TestService_AuditListAndAlert asserts the service read side: Audit lists rows
// newest-first with the peer filter, and SignatureFailureAlerts flags a peer that
// crosses the threshold within the window while leaving a quiet peer unflagged
// (Federation v1 F6.3, US-7.4 AC1/AC3).
func TestService_AuditListAndAlert(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	_ = projects
	_ = fedProjects
	auditRepo := repo.NewFederationAuditLogRepo(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), "https://me.example").
		WithAuditReader(auditRepo, 3, time.Hour)

	ctx := context.Background()
	now := time.Now()
	// alice + bob are KNOWN federated peers. The spoof mitigation (F6.3 review C)
	// counts ONLY known peers toward the alert, so a stranger claiming a random
	// X-Federation-Instance header cannot raise a bogus "attack on peer X".
	instances := repo.NewFederatedInstanceRepo(d)
	for _, url := range []string{"https://alice.example", "https://bob.example"} {
		if err := instances.Upsert(ctx, model.FederatedInstance{InstanceURL: url, PublicKey: "pk", DisplayName: url, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatalf("seed instance %s: %v", url, err)
		}
	}
	// alice: 3 signature failures (== threshold) within the window → flagged.
	for i := 0; i < 3; i++ {
		if err := auditRepo.Insert(ctx, repo.AuditEntry{
			Kind: repo.AuditKindSignatureInvalid, Outcome: repo.AuditOutcomeRejected,
			PeerInstanceURL: "https://alice.example", CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("insert alice %d: %v", i, err)
		}
	}
	// bob: 1 signature failure → below threshold, not flagged.
	if err := auditRepo.Insert(ctx, repo.AuditEntry{
		Kind: repo.AuditKindSignatureInvalid, Outcome: repo.AuditOutcomeRejected,
		PeerInstanceURL: "https://bob.example", CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert bob: %v", err)
	}
	// stranger: 5 signature failures (WELL over threshold) but NOT a known peer (no
	// federated_instances row) — a spoofed X-Federation-Instance header. Must NEVER
	// be flagged (F6.3 review C known-peer filter). Still appears in the audit LIST.
	for i := 0; i < 5; i++ {
		if err := auditRepo.Insert(ctx, repo.AuditEntry{
			Kind: repo.AuditKindSignatureInvalid, Outcome: repo.AuditOutcomeRejected,
			PeerInstanceURL: "https://stranger.example", CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("insert stranger %d: %v", i, err)
		}
	}

	rows, err := svc.Audit(ctx, fedsvc.AuditQuery{Limit: 50})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(rows) != 9 {
		t.Fatalf("audit rows: got %d, want 9 (alice 3 + bob 1 + stranger 5; the LIST is not filtered, only the alert)", len(rows))
	}

	filtered, err := svc.Audit(ctx, fedsvc.AuditQuery{PeerInstanceURL: "https://alice.example", Limit: 50})
	if err != nil {
		t.Fatalf("Audit filtered: %v", err)
	}
	if len(filtered) != 3 {
		t.Errorf("alice rows: got %d, want 3", len(filtered))
	}

	alerts, err := svc.SignatureFailureAlerts(ctx)
	if err != nil {
		t.Fatalf("SignatureFailureAlerts: %v", err)
	}
	var aliceFlagged, bobFlagged bool
	for _, a := range alerts {
		if a.PeerInstanceURL == "https://alice.example" {
			aliceFlagged = true
			if a.Count < 3 {
				t.Errorf("alice alert count: got %d, want >=3", a.Count)
			}
		}
		if a.PeerInstanceURL == "https://bob.example" {
			bobFlagged = true
		}
		if a.PeerInstanceURL == "https://stranger.example" {
			t.Errorf("unknown (spoofed) peer must NOT be flagged despite crossing the threshold (F6.3 known-peer filter)")
		}
	}
	if !aliceFlagged {
		t.Errorf("alice crossed the threshold and must be flagged (US-7.4 AC3)")
	}
	if bobFlagged {
		t.Errorf("bob is below the threshold and must NOT be flagged")
	}
}
