package federation_test

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// TestDeadLetter_ListsParkedEvents asserts the service surfaces parked dead-letter
// rows newest-first for the owner diagnostics view (Federation v1 F4.4, US-4.4
// AC3). Without a sync store wired, it returns an empty list rather than erroring.
func TestDeadLetter_ListsParkedEvents(t *testing.T) {
	svc, _, projects, _, _, st := newStatusSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)

	for _, ev := range []struct{ id, at string }{
		{"e1", "2026-06-03T10:00:00.000Z"},
		{"e2", "2026-06-03T10:00:05.000Z"},
	} {
		if err := st.InsertDeadLetter(ctx, store.DeadLetterRow{
			EventID: ev.id, PeerURL: "https://peer.example", LocalProjectID: pid,
			Payload: "{}", StatusCode: 403, Reason: "federation_read_only", FailedAt: ev.at,
		}); err != nil {
			t.Fatalf("park %s: %v", ev.id, err)
		}
	}

	views, err := svc.DeadLetter(ctx, 0)
	if err != nil {
		t.Fatalf("DeadLetter: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("dead-letter views: got %d, want 2", len(views))
	}
	// Newest-first ordering.
	if views[0].EventID != "e2" || views[1].EventID != "e1" {
		t.Errorf("dead-letter order: got [%s, %s], want [e2, e1]", views[0].EventID, views[1].EventID)
	}
	if views[0].PeerInstanceURL != "https://peer.example" || views[0].StatusCode != 403 || views[0].Reason != "federation_read_only" {
		t.Errorf("dead-letter view: got %+v", views[0])
	}
	if views[0].LocalProjectID != pid {
		t.Errorf("dead-letter view local project: got %d, want %d", views[0].LocalProjectID, pid)
	}
}

// TestDeadLetter_EmptyWithoutSyncStore asserts a service built without the sync
// store (a federation-off / partial build) returns an empty list, never an error,
// so the diagnostics endpoint stays a stable empty array.
func TestDeadLetter_EmptyWithoutSyncStore(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	// A service built WITHOUT WithSyncStore (a federation-off / partial build).
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d),
		repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), "https://me.example")

	views, err := svc.DeadLetter(context.Background(), 0)
	if err != nil {
		t.Fatalf("DeadLetter without sync store: %v", err)
	}
	if len(views) != 0 {
		t.Errorf("dead-letter views without store: got %d, want 0", len(views))
	}
}
