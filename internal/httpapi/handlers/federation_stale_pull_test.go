package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/federation/events"
)

// pullEvents issues a signed-style pull (the test group injects the peer) against
// the project's events endpoint with the given since_hlc, returning the response.
func (e *fedEventsEnv) pull(t *testing.T, projectID int64, sinceHLC string) *http.Response {
	t.Helper()
	target := "/federation/projects/" + itoa(projectID) + "/events?since_hlc=" + sinceHLC
	req := httptest.NewRequest(http.MethodGet, target, nil)
	resp, err := e.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("pull request: %v", err)
	}
	return resp
}

// TestPull_StaleSinceHLCReturns410 asserts US-3.7 AC4 (EMIT half): a pull whose
// since_hlc is OLDER than the oldest retained outbox event for the project — i.e.
// the events between since_hlc and the oldest retained have been GC'd — returns
// 410 Gone with {snapshot_url, as_of_hlc} so the peer re-snapshots (the consume
// half lands in F4.2). A since_hlc at-or-after the oldest retained is a normal 200.
func TestPull_StaleSinceHLCReturns410(t *testing.T) {
	env := newFedEventsEnv(t)

	// Retain one event at HLC 00000000000500 (the oldest retained); anything before
	// it has been pruned by the retention GC.
	insertOutboxEvent(t, env, "ev-retained", "00000000000500-0000-nodeA")

	// since_hlc strictly older than the oldest retained → events were GC'd → 410.
	resp := env.pull(t, env.localProject, "00000000000100-0000-nodeA")
	if resp.StatusCode != http.StatusGone {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("stale pull status: got %d, want 410; body %s", resp.StatusCode, b)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				SnapshotURL string `json:"snapshot_url"`
				AsOfHLC     string `json:"as_of_hlc"`
			} `json:"details"`
		} `json:"error"`
	}
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode 410 body: %v (%s)", err, raw)
	}
	if body.Error.Details.SnapshotURL == "" {
		t.Errorf("410 body must carry snapshot_url: %s", raw)
	}
	if body.Error.Details.AsOfHLC == "" {
		t.Errorf("410 body must carry as_of_hlc: %s", raw)
	}
}

// TestPull_FreshSinceHLCReturns200 asserts a since_hlc that is at-or-after the
// oldest retained event is served normally (200) — the 410 is reserved for the
// genuine fell-out-of-retention case (no false re-snapshots).
func TestPull_FreshSinceHLCReturns200(t *testing.T) {
	env := newFedEventsEnv(t)
	insertOutboxEvent(t, env, "ev-retained", "00000000000500-0000-nodeA")

	resp := env.pull(t, env.localProject, "00000000000500-0000-nodeA")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("fresh pull status: got %d, want 200; body %s", resp.StatusCode, b)
	}
}

// TestPull_EmptySinceHLCReturns200 asserts a fresh peer with NO cursor (empty
// since_hlc) is never 410'd — it is served the full retained log (the initial
// bootstrap path), not pushed to re-snapshot.
func TestPull_EmptySinceHLCReturns200(t *testing.T) {
	env := newFedEventsEnv(t)
	insertOutboxEvent(t, env, "ev-retained", "00000000000500-0000-nodeA")

	resp := env.pull(t, env.localProject, "")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("empty-cursor pull status: got %d, want 200; body %s", resp.StatusCode, b)
	}
}

// TestPull_PrunedFloorAfterOutboxGCReturns410 asserts the US-3.7 AC4 review fix:
// once the retention GC has purged a quiet project's outbox ENTIRELY (no retained
// rows) but recorded a durable pruned-floor HLC, a peer whose since_hlc predates
// that floor still gets 410 — it must NOT be falsely served 200 + an empty body
// and told it is caught up. The 410 body carries the floor as as_of_hlc.
func TestPull_PrunedFloorAfterOutboxGCReturns410(t *testing.T) {
	env := newFedEventsEnv(t)
	ctx := context.Background()

	// Simulate the GC having purged the outbox to empty after recording the floor:
	// no outbox rows remain, but the pruned floor is at 00000000000500.
	if _, err := env.store.AdvancePrunedFloor(ctx, env.localProject, "00000000000500-0000-nodeA", "2026-06-01T00:00:00.000Z"); err != nil {
		t.Fatalf("advance pruned floor: %v", err)
	}

	resp := env.pull(t, env.localProject, "00000000000100-0000-nodeA")
	if resp.StatusCode != http.StatusGone {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("pruned-floor pull status: got %d, want 410; body %s", resp.StatusCode, b)
	}
	var body struct {
		Error struct {
			Details struct {
				SnapshotURL string `json:"snapshot_url"`
				AsOfHLC     string `json:"as_of_hlc"`
			} `json:"details"`
		} `json:"error"`
	}
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode 410 body: %v (%s)", err, raw)
	}
	if body.Error.Details.SnapshotURL == "" {
		t.Errorf("410 body must carry snapshot_url: %s", raw)
	}
	// With the outbox empty, as_of_hlc falls back to the durable pruned floor.
	if body.Error.Details.AsOfHLC != "00000000000500-0000-nodeA" {
		t.Errorf("410 as_of_hlc: got %q, want the pruned floor 00000000000500-0000-nodeA", body.Error.Details.AsOfHLC)
	}
}

// TestPull_NonEmptyCursorNoRetainedOutboxReturns410 asserts the minimum-safety
// rule: a federated project with a non-empty since_hlc but NO retained outbox at
// all (a long-quiet project the GC may have emptied) cannot be safely told it is
// caught up, so the pull returns 410 (re-snapshot) rather than 200 + empty body.
func TestPull_NonEmptyCursorNoRetainedOutboxReturns410(t *testing.T) {
	env := newFedEventsEnv(t)

	// No outbox rows, no pruned floor recorded — but a non-empty cursor.
	resp := env.pull(t, env.localProject, "00000000000300-0000-nodeA")
	if resp.StatusCode != http.StatusGone {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("empty-outbox non-empty-cursor status: got %d, want 410; body %s", resp.StatusCode, b)
	}
}

// insertOutboxEvent inserts a canonical-ish outbox payload carrying one field at
// the given HLC so OldestRetainedHLC can decode its max field HLC.
func insertOutboxEvent(t *testing.T, env *fedEventsEnv, eventID, fieldHLC string) {
	t.Helper()
	evt := events.Event{
		EventID:         eventID,
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        "task-1",
		ProjectClientID: env.projClientID,
		Author:          env.peerURL,
		OriginInstance:  env.peerURL,
		Fields:          map[string]events.Field{"title": {Value: "v", HLC: fieldHLC}},
	}
	payload, _ := events.Marshal(evt)
	if _, err := env.db.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at) VALUES (?, ?, ?, '', '2026-06-01T00:00:00.000Z')`,
		eventID, env.localProject, string(payload)); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
}
