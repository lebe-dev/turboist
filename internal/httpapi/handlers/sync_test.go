package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/repo"
)

// syncChange mirrors one entry of the delta feed. Data stays raw so a test can
// compare it byte-for-byte against what the entity's own GET endpoint answers.
type syncChange struct {
	Entity string          `json:"entity"`
	Op     string          `json:"op"`
	Seq    int64           `json:"seq"`
	ID     int64           `json:"id"`
	Data   json.RawMessage `json:"data"`
}

type syncPage struct {
	Epoch   int64        `json:"epoch"`
	Cursor  int64        `json:"cursor"`
	HasMore bool         `json:"hasMore"`
	Changes []syncChange `json:"changes"`
}

func syncURL(since int64, extra string) string {
	url := fmt.Sprintf("/api/v1/sync/changes?since=%d", since)
	if extra != "" {
		url += "&" + extra
	}
	return url
}

func fetchChanges(t *testing.T, e *apiEnv, since int64, extra string) syncPage {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, syncURL(since, extra), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("sync changes: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var page syncPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("parse sync page: %v", err)
	}
	return page
}

// drainChanges walks the feed the way a replica does — since=cursor until hasMore is
// false — and returns every change in the order they were served.
func drainChanges(t *testing.T, e *apiEnv, since int64, extra string) []syncChange {
	t.Helper()
	var all []syncChange
	cursor := since
	for range 50 {
		page := fetchChanges(t, e, cursor, extra)
		all = append(all, page.Changes...)
		if page.Cursor < cursor {
			t.Fatalf("cursor went backwards: got %d, was %d", page.Cursor, cursor)
		}
		cursor = page.Cursor
		if !page.HasMore {
			return all
		}
	}
	t.Fatal("feed never reported hasMore=false")
	return nil
}

// currentSeq is the cursor a caller would hold right now: everything logged so
// far is already known to it.
func currentSeq(t *testing.T, e *apiEnv) int64 {
	t.Helper()
	return fetchChanges(t, e, 0, "limit=500").Cursor
}

// lastChangeFor picks the single change for one entity id out of a page.
func lastChangeFor(t *testing.T, changes []syncChange, entity string, id int64) syncChange {
	t.Helper()
	var found *syncChange
	for i := range changes {
		if changes[i].Entity == entity && changes[i].ID == id {
			if found != nil {
				t.Fatalf("%s %d appears twice in one page", entity, id)
			}
			found = &changes[i]
		}
	}
	if found == nil {
		t.Fatalf("no change for %s %d", entity, id)
	}
	return *found
}

func TestSyncChanges_ReportsCurrentEpoch(t *testing.T) {
	e := setupAPIEnv(t)
	page := fetchChanges(t, e, 0, "")
	if page.Epoch != 1 {
		t.Errorf("epoch: got %d, want 1", page.Epoch)
	}
}

func TestSyncChanges_UpsertCarriesEntityAndID(t *testing.T) {
	e := setupAPIEnv(t)
	since := currentSeq(t, e)
	c := createTestContext(t, e, "Work")

	changes := drainChanges(t, e, since, "")
	got := lastChangeFor(t, changes, repo.SyncEntityContext, c.ID)
	if got.Op != repo.SyncOpUpsert {
		t.Errorf("op: got %q, want %q", got.Op, repo.SyncOpUpsert)
	}
	if len(got.Data) == 0 {
		t.Fatal("upsert carried no data")
	}
}

// A task edited repeatedly is one change to apply, not one per edit — the log
// is append-only, so the read side is where the collapsing happens.
func TestSyncChanges_DedupesRepeatedEdits(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	task := createTestTask(t, e, c.ID, "First")
	since := currentSeq(t, e)

	for _, title := range []string{"Second", "Third", "Fourth"} {
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
			fmt.Sprintf("/api/v1/tasks/%d", task.ID), map[string]any{"title": title}))
		if resp.StatusCode != 200 {
			t.Fatalf("patch task: got %d; body: %s", resp.StatusCode, body)
		}
	}

	changes := drainChanges(t, e, since, "")
	got := lastChangeFor(t, changes, repo.SyncEntityTask, task.ID)
	var payload map[string]any
	if err := json.Unmarshal(got.Data, &payload); err != nil {
		t.Fatalf("parse task payload: %v", err)
	}
	if payload["title"] != "Fourth" {
		t.Errorf("title: got %v, want Fourth", payload["title"])
	}
}

// An entity created and then deleted inside one window is just the delete: the
// upsert has no row left to serve, and telling the replica to store one would
// resurrect a task the server no longer has.
func TestSyncChanges_DeleteCollapsesEarlierUpsert(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	since := currentSeq(t, e)
	task := createTestTask(t, e, c.ID, "Doomed")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("delete task: got %d; body: %s", resp.StatusCode, body)
	}

	changes := drainChanges(t, e, since, "")
	got := lastChangeFor(t, changes, repo.SyncEntityTask, task.ID)
	if got.Op != repo.SyncOpDelete {
		t.Errorf("op: got %q, want %q", got.Op, repo.SyncOpDelete)
	}
	if len(got.Data) != 0 {
		t.Errorf("delete carried data: %s", got.Data)
	}
	if got.ID != task.ID {
		t.Errorf("id: got %d, want %d", got.ID, task.ID)
	}
}

// A cascade deletes rows without logging a delete for each of them; the read
// side notices the row is gone and serves a tombstone anyway.
func TestSyncChanges_CascadedDeleteBecomesTombstone(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	task := createTestTask(t, e, c.ID, "Child of a doomed context")
	since := currentSeq(t, e)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/contexts/%d", c.ID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("delete context: got %d; body: %s", resp.StatusCode, body)
	}

	changes := drainChanges(t, e, since, "")
	got := lastChangeFor(t, changes, repo.SyncEntityTask, task.ID)
	if got.Op != repo.SyncOpDelete {
		t.Errorf("cascaded task op: got %q, want %q", got.Op, repo.SyncOpDelete)
	}
}

func TestSyncChanges_PagesInSeqOrder(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	since := currentSeq(t, e)
	var ids []int64
	for i := range 5 {
		ids = append(ids, createTestTask(t, e, c.ID, fmt.Sprintf("Task %d", i)).ID)
	}

	page := fetchChanges(t, e, since, "limit=2")
	if !page.HasMore {
		t.Fatal("first page: hasMore is false with more changes pending")
	}
	if len(page.Changes) != 2 {
		t.Fatalf("first page size: got %d, want 2", len(page.Changes))
	}

	all := drainChanges(t, e, since, "limit=2")
	var prev int64
	for _, ch := range all {
		if ch.Seq <= prev {
			t.Fatalf("seq out of order: %d after %d", ch.Seq, prev)
		}
		prev = ch.Seq
	}
	seen := map[int64]bool{}
	for _, ch := range all {
		if ch.Entity == repo.SyncEntityTask {
			seen[ch.ID] = true
		}
	}
	for _, id := range ids {
		if !seen[id] {
			t.Errorf("task %d never appeared across the pages", id)
		}
	}
}

// The acceptance case: a replica that only ever saw deltas holds exactly what
// the REST API would have handed it, deletions included.
func TestSyncChanges_LoopReconstructsCurrentState(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	kept := createTestTask(t, e, c.ID, "Kept")
	renamed := createTestTask(t, e, c.ID, "Before")
	removed := createTestTask(t, e, c.ID, "Removed")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", renamed.ID), map[string]any{"title": "After"}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch task: got %d; body: %s", resp.StatusCode, body)
	}
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/tasks/%d", removed.ID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("delete task: got %d; body: %s", resp.StatusCode, body)
	}

	replica := map[int64]json.RawMessage{}
	for _, ch := range drainChanges(t, e, 0, "limit=1") {
		if ch.Entity != repo.SyncEntityTask {
			continue
		}
		if ch.Op == repo.SyncOpDelete {
			delete(replica, ch.ID)
			continue
		}
		replica[ch.ID] = ch.Data
	}

	if _, ok := replica[removed.ID]; ok {
		t.Error("deleted task survived in the replica")
	}
	for _, id := range []int64{kept.ID, renamed.ID} {
		stored, ok := replica[id]
		if !ok {
			t.Fatalf("task %d missing from the replica", id)
		}
		_, want := doReq(t, e.app, e.authedReq(t, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d", id), nil))
		assertSameJSON(t, stored, want)
	}
}

// Parity is the whole point of reusing the DTOs: a task served in a delta must
// be indistinguishable from the same task served by its own endpoint, labels,
// blocker counts and all.
func TestSyncChanges_TaskPayloadMatchesSingleGet(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	label := createTestLabel(t, e, "urgent")
	blocker := createTestTask(t, e, c.ID, "Blocker")
	task := createTestTask(t, e, c.ID, "Blocked")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID), map[string]any{"labels": []string{label.Name}}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch labels: got %d; body: %s", resp.StatusCode, body)
	}
	addRelation(t, e, task.ID, map[string]any{
		"targetTaskId": blocker.ID,
		"type":         "blocks",
		"direction":    "incoming",
	})

	got := lastChangeFor(t, drainChanges(t, e, 0, ""), repo.SyncEntityTask, task.ID)
	_, want := doReq(t, e.app, e.authedReq(t, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d", task.ID), nil))
	assertSameJSON(t, got.Data, want)
}

func TestSyncChanges_ProjectPayloadMatchesSingleGet(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	p := createTestProject(t, e, c.ID, "Website")

	got := lastChangeFor(t, drainChanges(t, e, 0, ""), repo.SyncEntityProject, p.ID)
	_, want := doReq(t, e.app, e.authedReq(t, http.MethodGet, fmt.Sprintf("/api/v1/projects/%d", p.ID), nil))
	assertSameJSON(t, got.Data, want)
}

func TestSyncChanges_UserSettingsPayloadMatchesSingleGet(t *testing.T) {
	e := setupAPIEnv(t)
	since := currentSeq(t, e)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{"locale": "ru"}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch settings: got %d; body: %s", resp.StatusCode, body)
	}

	got := lastChangeFor(t, drainChanges(t, e, since, ""), repo.SyncEntityUserSettings, 1)
	_, want := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	assertSameJSON(t, got.Data, want)
}

func TestSyncChanges_UserStatePayloadMatchesSingleGet(t *testing.T) {
	e := setupAPIEnv(t)
	since := currentSeq(t, e)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/state", map[string]any{"sidebar": "open"}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch state: got %d; body: %s", resp.StatusCode, body)
	}

	got := lastChangeFor(t, drainChanges(t, e, since, ""), repo.SyncEntityUserState, 1)
	_, want := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/state", nil))
	assertSameJSON(t, got.Data, want)
}

// The edge is served whole — both endpoints named — because a replica holds the
// relation table itself and works out direction locally for whichever end it is
// drawing, unlike the task-relative shape the detail endpoint returns.
func TestSyncChanges_TaskRelationCarriesBothEndpoints(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	blocker := createTestTask(t, e, c.ID, "Blocker")
	blocked := createTestTask(t, e, c.ID, "Blocked")
	since := currentSeq(t, e)

	updated := addRelation(t, e, blocked.ID, map[string]any{
		"targetTaskId": blocker.ID,
		"type":         "blocks",
		"direction":    "incoming",
	})
	relationID := updated.Relations[0].ID

	got := lastChangeFor(t, drainChanges(t, e, since, ""), repo.SyncEntityTaskRelation, relationID)
	var edge struct {
		ID           int64  `json:"id"`
		SourceTaskID int64  `json:"sourceTaskId"`
		TargetTaskID int64  `json:"targetTaskId"`
		Type         string `json:"type"`
		CreatedAt    string `json:"createdAt"`
	}
	if err := json.Unmarshal(got.Data, &edge); err != nil {
		t.Fatalf("parse relation payload: %v", err)
	}
	if edge.SourceTaskID != blocker.ID {
		t.Errorf("sourceTaskId: got %d, want %d", edge.SourceTaskID, blocker.ID)
	}
	if edge.TargetTaskID != blocked.ID {
		t.Errorf("targetTaskId: got %d, want %d", edge.TargetTaskID, blocked.ID)
	}
	if edge.Type != "blocks" {
		t.Errorf("type: got %q, want blocks", edge.Type)
	}
	if edge.CreatedAt == "" {
		t.Error("createdAt is empty")
	}
}

// A restore rewrites history, so every cursor stamped before it points into a
// past that no longer exists. The client is told to start over rather than
// resume onto data that quietly disagrees.
func TestSyncChanges_EpochMismatch(t *testing.T) {
	e := setupAPIEnv(t)
	appSettings := repo.NewAppSettingsRepo(e.db)
	newEpoch, err := appSettings.BumpSyncEpoch(context.Background())
	if err != nil {
		t.Fatalf("bump epoch: %v", err)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, syncURL(0, "epoch=1"), nil))
	if resp.StatusCode != 409 {
		t.Fatalf("got %d, want 409; body: %s", resp.StatusCode, body)
	}
	var out struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if out.Error.Code != httpapi.CodeSyncEpochMismatch {
		t.Errorf("code: got %q, want %q", out.Error.Code, httpapi.CodeSyncEpochMismatch)
	}
	if got := out.Error.Details["epoch"]; got != float64(newEpoch) {
		t.Errorf("details.epoch: got %v, want %d", got, newEpoch)
	}
}

func TestSyncChanges_MatchingEpochIsAccepted(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, syncURL(0, "epoch=1"), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// Once the changes a cursor still needs have been pruned, no sequence of deltas
// can close the gap, so the feed refuses rather than serving a page that would
// leave the replica quietly incomplete.
func TestSyncChanges_CursorExpired(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	createTestTask(t, e, c.ID, "Old news")
	cutoff := currentSeq(t, e)
	createTestTask(t, e, c.ID, "Recent")

	if _, err := e.db.Exec(`DELETE FROM change_log WHERE seq <= ?`, cutoff); err != nil {
		t.Fatalf("prune log: %v", err)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, syncURL(1, ""), nil))
	if resp.StatusCode != 410 {
		t.Fatalf("got %d, want 410; body: %s", resp.StatusCode, body)
	}
	var out struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if out.Error.Code != httpapi.CodeSyncCursorExpired {
		t.Errorf("code: got %q, want %q", out.Error.Code, httpapi.CodeSyncCursorExpired)
	}
	if out.Error.Details["oldestRetained"] == nil {
		t.Error("details.oldestRetained is missing")
	}
}

// A cursor sitting exactly on the pruning boundary lost nothing and must keep
// working — otherwise routine pruning would force a full re-sync on every
// client that was merely up to date.
func TestSyncChanges_CursorAtPruneBoundarySurvives(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	createTestTask(t, e, c.ID, "Old news")
	cutoff := currentSeq(t, e)
	createTestTask(t, e, c.ID, "Recent")

	if _, err := e.db.Exec(`DELETE FROM change_log WHERE seq <= ?`, cutoff); err != nil {
		t.Fatalf("prune log: %v", err)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, syncURL(cutoff, ""), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

func TestSyncChanges_RejectsNegativeSince(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/sync/changes?since=-3", nil))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestSyncChanges_RejectsOversizedLimit(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/sync/changes?limit=5000", nil))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

// One page can carry every kind of row in the workspace, so a token that may
// read only some of them must not get at the rest through this endpoint.
func TestSyncChanges_RequiresEveryReadScope(t *testing.T) {
	e := setupAPIEnv(t)
	partial := issueAPIToken(t, e, "partial", []string{auth.ScopeTasksRead, auth.ScopeProjectsRead})
	resp, body := runRequest(t, e.app, tokenReq(http.MethodGet, "/api/v1/sync/changes", partial, nil))
	if resp.StatusCode != 403 {
		t.Fatalf("partial token: got %d, want 403; body: %s", resp.StatusCode, body)
	}

	full := issueAPIToken(t, e, "full", []string{auth.ScopeWildcard})
	resp, body = runRequest(t, e.app, tokenReq(http.MethodGet, "/api/v1/sync/changes", full, nil))
	if resp.StatusCode != 200 {
		t.Fatalf("full token: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// assertSameJSON compares two JSON documents by value, so field order and
// whitespace differences between two encoders never make a parity test lie.
func assertSameJSON(t *testing.T, got, want []byte) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("parse got: %v (%s)", err, got)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("parse want: %v (%s)", err, want)
	}
	gs, _ := json.Marshal(g)
	ws, _ := json.Marshal(w)
	if string(gs) != string(ws) {
		t.Errorf("payload mismatch:\n got: %s\nwant: %s", gs, ws)
	}
}

// An upsert whose row is gone by the time the page is read reaches the wire as
// a tombstone with no `data` at all: a replica applies what it is given, so a
// change carrying no payload must not be able to look like one that does.
func TestSyncChanges_DanglingUpsertServedAsTombstone(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	task := createTestTask(t, e, c.ID, "Row that outlives its pointer")
	since := currentSeq(t, e)

	if _, err := e.db.Exec(`INSERT INTO change_log (entity, entity_id, op, changed_at)
	                        VALUES (?, ?, ?, '2026-01-01T00:00:00.000Z')`,
		repo.SyncEntityTask, task.ID, repo.SyncOpUpsert); err != nil {
		t.Fatalf("log upsert: %v", err)
	}
	if _, err := e.db.Exec(`DELETE FROM tasks WHERE id = ?`, task.ID); err != nil {
		t.Fatalf("delete task row: %v", err)
	}
	if _, err := e.db.Exec(`DELETE FROM change_log WHERE entity = ? AND entity_id = ? AND op = ?`,
		repo.SyncEntityTask, task.ID, repo.SyncOpDelete); err != nil {
		t.Fatalf("drop tombstone: %v", err)
	}

	got := lastChangeFor(t, drainChanges(t, e, since, ""), repo.SyncEntityTask, task.ID)
	if got.Op != repo.SyncOpDelete {
		t.Errorf("op: got %q, want %q", got.Op, repo.SyncOpDelete)
	}
	if got.Data != nil {
		t.Errorf("data: got %s, want it absent", got.Data)
	}
}
