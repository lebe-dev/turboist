package federation

import (
	"bufio"
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/snapshot"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ndjsonSender is a HandshakeSender that, for the re-bootstrap snapshot GET,
// replies with a pre-built NDJSON snapshot body (the owner's re-bootstrap
// snapshot). It lets the joiner-side ReBootstrap run without a network round-trip.
type ndjsonSender struct {
	body []byte
}

func (s *ndjsonSender) Send(_ context.Context, _ SignedRequest) (*SignedResponse, error) {
	return &SignedResponse{StatusCode: 200, Body: s.body}, nil
}

// newJoinerServiceFromSnapshot builds a joiner-side federation service that
// already holds a federated project mapped to the owner (carrying the owner's
// project client_id), with an ndjsonSender that serves the given snapshot body on
// the re-bootstrap fetch. Returns the service and the local project id.
func newJoinerServiceFromSnapshot(t *testing.T, ownerURL string, snapshotBody []byte, projClientID string) (*Service, int64) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "joiner.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	plabels := repo.NewProjectLabelsRepo(d)
	tlabels := repo.NewTaskLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	tasks := repo.NewTaskRepo(d, tlabels)
	contexts := repo.NewContextRepo(d)
	sections := repo.NewProjectSectionRepo(d)
	fedProjects := repo.NewFederatedProjectRepo(d)
	fedInvites := repo.NewFederationInviteRepo(d)
	fedInstances := repo.NewFederatedInstanceRepo(d)
	fedKeys := repo.NewFederationKeysRepo(d)
	cipher := crypto.NewTokenCipher(snapTestKey)

	svc := NewService(d, projects, fedProjects, fedKeys, fedInvites, fedInstances, cipher, "https://bob.example")
	svc.WithSnapshotDeps(tasks, sections, contexts, repo.NewFederationSnapshotRepo(d))
	fetch := func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, DisplayName: instanceURL}, nil
	}
	svc.WithJoinDeps(&ndjsonSender{body: snapshotBody}, fetch, peerkeys.NewCache(fetch), time.Now)

	ctx := context.Background()
	cx, err := contexts.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := projects.Create(ctx, repo.CreateProject{ContextID: cx.ID, Title: "Stale copy", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := d.Exec(`UPDATE projects SET client_id = ?, is_federated = 1 WHERE id = ?`, projClientID, p.ID); err != nil {
		t.Fatalf("federate joiner project: %v", err)
	}
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    p.ID,
		PeerInstanceURL:   ownerURL,
		IsOwner:           false,
		OriginInstanceURL: ownerURL,
		Permissions:       model.FederationPermissionWrite,
		LastReceivedHLC:   "00000000000100-0000-owner",
	}); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}
	return svc, p.ID
}

// TestReBootstrap_PreservesOutboxAndStampsCutoff drives the service-level F4.2
// consume: a joiner that fell behind retention re-fetches the owner snapshot,
// overwrites its local project in one tx, and (R3) does NOT clear the
// federation_outbox; the returned cutoff X (as_of_hlc + wall-clock) is persisted
// on the mapping row (US-4.2 AC2/AC3/AC4).
func TestReBootstrap_PreservesOutboxAndStampsCutoff(t *testing.T) {
	const ownerURL = "https://alice.example"

	// Build a real owner snapshot to serve as the re-bootstrap body.
	ownerSvc, ownerProjectID := newOwnerService(t)
	ctx := context.Background()
	tok := mintToken(t, ownerSvc, ownerProjectID, time.Now())
	ownerSnap, err := ownerSvc.BuildSnapshot(ctx, ownerProjectID, tok, time.Now())
	if err != nil {
		t.Fatalf("owner build: %v", err)
	}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := snapshot.WriteNDJSON(w, ownerSnap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()

	joiner, localProjectID := newJoinerServiceFromSnapshot(t, ownerURL, buf.Bytes(), ownerSnap.Project.ClientID)

	// The joiner holds two UNSENT outbox events (local edits awaiting delivery).
	for _, id := range []string{"unsent-a", "unsent-b"} {
		if _, err := joiner.db.Exec(
			`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
			 VALUES (?, ?, '{}', '', '2026-06-03T00:00:00.000Z')`, id, localProjectID); err != nil {
			t.Fatalf("seed outbox %s: %v", id, err)
		}
	}

	at := time.Date(2026, 6, 3, 9, 30, 0, 0, time.UTC)
	joiner.now = func() time.Time { return at }
	res, err := joiner.ReBootstrap(ctx, localProjectID, ownerURL,
		ownerURL+"/federation/projects/"+itoa(ownerProjectID)+"/snapshot", ownerSnap.AsOfHLC)
	if err != nil {
		t.Fatalf("re-bootstrap: %v", err)
	}

	// Cutoff X surfaced and is the real as_of + wall-clock (not a placeholder).
	if res.CutoffHLC != ownerSnap.AsOfHLC {
		t.Errorf("cutoff hlc: got %q, want %q", res.CutoffHLC, ownerSnap.AsOfHLC)
	}
	if res.RebootstrappedAt != model.FormatUTC(at) {
		t.Errorf("cutoff X wall-clock: got %q, want %q", res.RebootstrappedAt, model.FormatUTC(at))
	}

	// Outbox preserved (R3 — the headline F4.2 bug).
	var outbox int
	if err := joiner.db.QueryRow(`SELECT COUNT(*) FROM federation_outbox`).Scan(&outbox); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if outbox != 2 {
		t.Errorf("outbox count after re-bootstrap: got %d, want 2 (unsent edits must survive)", outbox)
	}

	// The mapping row carries the cutoff so the joiner UI can render the banner.
	fp, err := joiner.fedProjects.Get(ctx, localProjectID, ownerURL)
	if err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	if fp.LastReceivedHLC != ownerSnap.AsOfHLC {
		t.Errorf("last_received_hlc: got %q, want %q", fp.LastReceivedHLC, ownerSnap.AsOfHLC)
	}
}

// TestStalePullCodeDriftGuard pins the duplicated stale-pull error code to the
// exact string the owner's pull endpoint emits (httpapi.CodeFederationStalePull).
// The service layer cannot import httpapi (wrong direction), so the constant is
// duplicated; this guard fails loudly if the wire code ever changes on one side.
func TestStalePullCodeDriftGuard(t *testing.T) {
	if codeFederationStalePull != "federation_stale_pull" {
		t.Errorf("stale-pull code drift: got %q, want %q (must match httpapi.CodeFederationStalePull)", codeFederationStalePull, "federation_stale_pull")
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
