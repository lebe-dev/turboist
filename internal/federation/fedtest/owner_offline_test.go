// Federation v1 F5.6a — consolidated owner-death (owner-offline) end-to-end. The
// unit legs are covered elsewhere (owner-offline derivation in recovery, the outbox
// queue, the recovery flush, per-field LWW convergence); this is the single
// end-to-end assertion the consolidated path was missing: a WRITE joiner's local
// edits are ALLOWED and QUEUED while the owner is unreachable (US-6.5 AC2 — not
// blocked), and FLUSH + LWW-resolve on the owner the moment it returns (US-6.5 AC3).
package fedtest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/outbox"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// TestTwoInstance_OwnerOffline_JoinerEditsQueueThenFlushOnReturn drives the full
// owner-offline flow between two real in-process instances:
//
//  1. Owner A creates a shared task; it converges on joiner B.
//  2. Owner A goes OFFLINE (modelled by not draining B→A). Joiner B — a WRITE peer —
//     edits the task. The edit is ALLOWED locally and QUEUED in B's outbox, never
//     blocked (US-6.5 AC2).
//  3. Owner A RETURNS. B's outbox flushes to A; A applies the queued edit and
//     per-field LWW resolves A's copy to B's newer title (US-6.5 AC3).
func TestTwoInstance_OwnerOffline_JoinerEditsQueueThenFlushOnReturn(t *testing.T) {
	const aURL, bURL = "https://a.example", "https://b.example"

	pubKeys := map[string]string{}
	resolver := func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: pubKeys[instanceURL], DisplayName: instanceURL}, nil
	}
	a := newInstance(t, aURL, peerkeys.NewCache(resolver))
	b := newInstance(t, bURL, peerkeys.NewCache(resolver))
	pubKeys[aURL] = a.pubKeyB64
	pubKeys[bURL] = b.pubKeyB64

	ctx := context.Background()

	projClientID := model.NewClientID()
	aProj := seedFederatedProject(t, a, projClientID, aURL, true, bURL, model.FederationPermissionWrite)
	bProj := seedFederatedProject(t, b, projClientID, aURL, false, aURL, model.FederationPermissionWrite)

	// Both apply queues run: B applies the owner's create; A applies B's flushed edit.
	actx, acancel := context.WithCancel(ctx)
	defer acancel()
	a.queue.Start(actx)
	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	b.queue.Start(bctx)

	sender := newRoutingSender(map[string]*fiber.App{aURL: a.app, bURL: b.app})

	// 1) Owner A creates the shared task and propagates it to joiner B.
	aMutator := newMutator(t, a, aURL)
	aPublisher := fedsvc.NewPublisher(a.fedProjects, a.keys, crypto.NewTokenCipher(cipherKey), sender, aURL, nil)
	aWorker := outbox.NewWorker(a.store, aPublisher, aPublisher, nil)

	cx := int64(1)
	taskClientID := model.NewClientID()
	if _, err := aMutator.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &aProj}, Title: "Owner task"}, taskClientID); err != nil {
		t.Fatalf("owner create: %v", err)
	}
	pushAndDrain(t, ctx, a, aProj, aPublisher, bURL, aWorker)
	if !converged(b, func() bool { return taskExistsOnB(b, taskClientID, "Owner task") }) {
		t.Fatalf("owner's create did not converge on joiner within 5s")
	}

	// 2) OWNER OFFLINE. The joiner (a WRITE peer) edits its local copy. The edit
	// must be ALLOWED and applied locally, and QUEUED in the joiner's outbox for
	// delivery when the owner returns — never blocked (US-6.5 AC2). "Offline" is
	// modelled by NOT draining B→A yet.
	bMutator := newMutator(t, b, bURL)
	var bTaskID int64
	if err := b.db.QueryRow(`SELECT id FROM tasks WHERE client_id = ?`, taskClientID).Scan(&bTaskID); err != nil {
		t.Fatalf("resolve joiner task id: %v", err)
	}
	bTask, err := b.tasks.Get(ctx, bTaskID)
	if err != nil {
		t.Fatalf("get joiner task: %v", err)
	}
	editedTitle := "Edited while owner offline"
	if err := bMutator.Update(ctx, bTask, repo.TaskUpdate{Title: &editedTitle}); err != nil {
		t.Fatalf("joiner edit while owner offline must be ALLOWED, not blocked: %v", err)
	}
	// The local edit is applied on the joiner immediately (US-6.5 AC2 — not blocked).
	if !taskExistsOnB(b, taskClientID, editedTitle) {
		t.Fatalf("joiner's offline edit was not applied locally (US-6.5 AC2)")
	}
	// ...and it is QUEUED in the joiner's outbox for the offline owner.
	if n := outboxRowCount(t, b.db, bProj); n == 0 {
		t.Fatalf("joiner's offline edit must be queued in its outbox, got 0 rows")
	}

	// 3) OWNER RETURNS. The joiner's outbox flushes to A; A applies the queued edit
	// and per-field LWW resolves A's copy to the joiner's newer title (US-6.5 AC3).
	bPublisher := fedsvc.NewPublisher(b.fedProjects, b.keys, crypto.NewTokenCipher(cipherKey), sender, bURL, nil)
	bWorker := outbox.NewWorker(b.store, bPublisher, bPublisher, nil)
	pushAndDrain(t, ctx, b, bProj, bPublisher, aURL, bWorker)
	if !converged(a, func() bool { return taskExistsOnB(a, taskClientID, editedTitle) }) {
		t.Fatalf("owner did not converge to the joiner's flushed edit within 5s (LWW)")
	}
}

// outboxRowCount counts a project's federation_outbox rows (the joiner's queued,
// not-yet-flushed edits while the owner is offline).
func outboxRowCount(t *testing.T, d *sql.DB, projectID int64) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federation_outbox WHERE local_project_id = ?`, projectID).Scan(&n); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return n
}
