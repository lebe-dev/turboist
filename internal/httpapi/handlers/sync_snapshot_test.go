package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// snapshotBody is the bootstrap payload with every collection left raw, so a
// test can compare a row byte-for-byte against what its own GET endpoint answers.
type snapshotBody struct {
	Epoch          int64  `json:"epoch"`
	Cursor         int64  `json:"cursor"`
	CompletedSince string `json:"completedSince"`

	Tasks         []json.RawMessage `json:"tasks"`
	Projects      []json.RawMessage `json:"projects"`
	Sections      []json.RawMessage `json:"sections"`
	Contexts      []json.RawMessage `json:"contexts"`
	Labels        []json.RawMessage `json:"labels"`
	TaskRelations []json.RawMessage `json:"taskRelations"`
	TaskTemplates []json.RawMessage `json:"taskTemplates"`

	UserSettings json.RawMessage `json:"userSettings"`
	AppSettings  json.RawMessage `json:"appSettings"`
	UserState    json.RawMessage `json:"userState"`
}

// collections maps every array in the payload onto the entity name the delta
// feed uses for the same rows. The two vocabularies have to stay the same one:
// a client applies a snapshot and a delta through the same code.
func (s snapshotBody) collections() map[string][]json.RawMessage {
	return map[string][]json.RawMessage{
		repo.SyncEntityTask:         s.Tasks,
		repo.SyncEntityProject:      s.Projects,
		repo.SyncEntitySection:      s.Sections,
		repo.SyncEntityContext:      s.Contexts,
		repo.SyncEntityLabel:        s.Labels,
		repo.SyncEntityTaskRelation: s.TaskRelations,
		repo.SyncEntityTaskTemplate: s.TaskTemplates,
	}
}

// replica is the local store a client ends up with after applying the payload,
// keyed the way an applier keys it: entity name, then row id.
func (s snapshotBody) replica(t *testing.T) map[string]map[int64]json.RawMessage {
	t.Helper()
	out := map[string]map[int64]json.RawMessage{}
	for entity, rows := range s.collections() {
		out[entity] = map[int64]json.RawMessage{}
		for _, row := range rows {
			var head struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(row, &head); err != nil {
				t.Fatalf("parse %s row: %v", entity, err)
			}
			if head.ID == 0 {
				t.Fatalf("%s row carries no id: %s", entity, row)
			}
			out[entity][head.ID] = row
		}
	}
	// The three singletons have no id inside their payload; the log addresses
	// them by the row they live on, which is always 1.
	out[repo.SyncEntityUserSettings] = map[int64]json.RawMessage{1: s.UserSettings}
	out[repo.SyncEntityAppSettings] = map[int64]json.RawMessage{1: s.AppSettings}
	out[repo.SyncEntityUserState] = map[int64]json.RawMessage{1: s.UserState}
	return out
}

func fetchSnapshot(t *testing.T, e *apiEnv) (snapshotBody, []byte) {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/sync/snapshot", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("sync snapshot: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var snap snapshotBody
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	return snap, body
}

// completeTaskAt back-dates a completion straight in the database. The API only
// ever completes a task "now", and the window boundary is 90 days out.
func completeTaskAt(t *testing.T, e *apiEnv, taskID int64, at time.Time) {
	t.Helper()
	if _, err := e.db.Exec(
		`UPDATE tasks SET status = 'completed', completed_at = ? WHERE id = ?`,
		model.FormatUTC(at), taskID); err != nil {
		t.Fatalf("back-date completion of task %d: %v", taskID, err)
	}
}

func TestSyncSnapshot_ReportsEpochAndCursor(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	createTestTask(t, e, c.ID, "Something to log")

	snap, _ := fetchSnapshot(t, e)
	if snap.Epoch != 1 {
		t.Errorf("epoch: got %d, want 1", snap.Epoch)
	}
	if want := currentSeq(t, e); snap.Cursor != want {
		t.Errorf("cursor: got %d, want %d", snap.Cursor, want)
	}
	if snap.CompletedSince == "" {
		t.Error("completedSince is empty")
	}
}

func TestSyncSnapshot_CarriesEveryCollection(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	createTestProject(t, e, c.ID, "Website")
	createTestLabel(t, e, "urgent")
	createTestSection(t, e)
	blocker := createTestTask(t, e, c.ID, "Blocker")
	blocked := createTestTask(t, e, c.ID, "Blocked")
	addRelation(t, e, blocked.ID, map[string]any{
		"targetTaskId": blocker.ID,
		"type":         "blocks",
		"direction":    "incoming",
	})
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/task-templates",
		map[string]any{"name": "Weekly review"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create template: got %d; body: %s", resp.StatusCode, body)
	}

	snap, _ := fetchSnapshot(t, e)
	for entity, rows := range snap.collections() {
		if len(rows) == 0 {
			t.Errorf("%s: got 0 rows, want at least one", entity)
		}
	}
	for name, blob := range map[string]json.RawMessage{
		"userSettings": snap.UserSettings,
		"appSettings":  snap.AppSettings,
		"userState":    snap.UserState,
	} {
		if len(blob) == 0 {
			t.Errorf("%s is missing from the snapshot", name)
		}
	}
}

// A client decoding into non-nullable lists must not have to special-case an
// empty workspace, so every collection is an array even when nothing is in it.
func TestSyncSnapshot_EmptyCollectionsAreArrays(t *testing.T) {
	e := setupAPIEnv(t)
	_, raw := fetchSnapshot(t, e)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	for _, name := range []string{"tasks", "sections", "contexts", "labels", "taskRelations", "taskTemplates"} {
		if string(fields[name]) != "[]" {
			t.Errorf("%s on an empty workspace: got %s, want []", name, fields[name])
		}
	}
}

func TestSyncSnapshot_TaskPayloadMatchesSingleGet(t *testing.T) {
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

	snap, _ := fetchSnapshot(t, e)
	got := snap.replica(t)[repo.SyncEntityTask][task.ID]
	if got == nil {
		t.Fatalf("task %d missing from the snapshot", task.ID)
	}
	_, want := doReq(t, e.app, e.authedReq(t, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d", task.ID), nil))
	assertSameJSON(t, got, want)
}

func TestSyncSnapshot_ProjectPayloadMatchesSingleGet(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	p := createTestProject(t, e, c.ID, "Website")

	snap, _ := fetchSnapshot(t, e)
	got := snap.replica(t)[repo.SyncEntityProject][p.ID]
	if got == nil {
		t.Fatalf("project %d missing from the snapshot", p.ID)
	}
	_, want := doReq(t, e.app, e.authedReq(t, http.MethodGet, fmt.Sprintf("/api/v1/projects/%d", p.ID), nil))
	assertSameJSON(t, got, want)
}

func TestSyncSnapshot_SingletonBlobsMatchTheirOwnEndpoints(t *testing.T) {
	e := setupAPIEnv(t)
	for _, patch := range []struct {
		url  string
		body map[string]any
	}{
		{"/api/v1/settings", map[string]any{"locale": "ru"}},
		{"/api/v1/state", map[string]any{"sidebar": "open"}},
	} {
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, patch.url, patch.body))
		if resp.StatusCode != 200 {
			t.Fatalf("patch %s: got %d; body: %s", patch.url, resp.StatusCode, body)
		}
	}

	snap, _ := fetchSnapshot(t, e)
	_, wantSettings := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	assertSameJSON(t, snap.UserSettings, wantSettings)
	_, wantState := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/state", nil))
	assertSameJSON(t, snap.UserState, wantState)
}

// A snapshot row and the delta row for the same entity must be the same bytes —
// otherwise a replica's contents would depend on which of the two paths
// delivered a row, which is exactly the drift the shared DTOs exist to prevent.
func TestSyncSnapshot_RowsMatchTheDeltaFeed(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	task := createTestTask(t, e, c.ID, "Shared by both paths")

	snap, _ := fetchSnapshot(t, e)
	fromSnapshot := snap.replica(t)[repo.SyncEntityTask][task.ID]
	fromDelta := lastChangeFor(t, drainChanges(t, e, 0, ""), repo.SyncEntityTask, task.ID)
	assertSameJSON(t, fromSnapshot, fromDelta.Data)
}

// Open history is unbounded, completed history is not: a task finished before
// the window stays on the server and is read online when someone asks for it.
func TestSyncSnapshot_OmitsTasksCompletedBeforeTheWindow(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	recent := createTestTask(t, e, c.ID, "Finished last week")
	ancient := createTestTask(t, e, c.ID, "Finished last year")
	open := createTestTask(t, e, c.ID, "Still open")

	now := time.Now()
	completeTaskAt(t, e, recent.ID, now.Add(-7*24*time.Hour))
	completeTaskAt(t, e, ancient.ID, now.Add(-repo.SyncHistoryWindow).Add(-24*time.Hour))

	snap, _ := fetchSnapshot(t, e)
	present := snap.replica(t)[repo.SyncEntityTask]
	for _, want := range []int64{recent.ID, open.ID} {
		if present[want] == nil {
			t.Errorf("task %d missing from the snapshot", want)
		}
	}
	if present[ancient.ID] != nil {
		t.Error("a task completed before the window was carried in the snapshot")
	}
}

// The acceptance case: seed from a snapshot, follow the delta feed from the
// cursor it handed back, and the replica holds exactly what the server holds.
func TestSyncSnapshot_ThenChangesLoopReconstructsServerState(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	kept := createTestTask(t, e, c.ID, "Kept")
	renamed := createTestTask(t, e, c.ID, "Before")
	removed := createTestTask(t, e, c.ID, "Removed")
	createTestProject(t, e, c.ID, "Website")
	createTestLabel(t, e, "urgent")

	seed, _ := fetchSnapshot(t, e)
	replica := seed.replica(t)

	// Everything below happens after the snapshot was taken, so only the delta
	// feed can carry it.
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
	fresh := createTestTask(t, e, c.ID, "Created after the snapshot")
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/settings",
		map[string]any{"locale": "ru"}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch settings: got %d; body: %s", resp.StatusCode, body)
	}

	applyChanges(t, replica, drainChanges(t, e, seed.Cursor, "limit=1"))

	after, _ := fetchSnapshot(t, e)
	assertSameReplica(t, replica, after.replica(t))

	// Spot-check the intent behind the diff, so a comparison that silently
	// stopped comparing anything would not pass.
	if replica[repo.SyncEntityTask][removed.ID] != nil {
		t.Error("deleted task survived in the replica")
	}
	for _, id := range []int64{kept.ID, renamed.ID, fresh.ID} {
		if replica[repo.SyncEntityTask][id] == nil {
			t.Errorf("task %d missing from the replica", id)
		}
	}
}

// The cursor is read inside the same transaction as the rows. A write racing the
// snapshot must therefore either land in the payload or land above the cursor
// and arrive with the next delta — never fall into a gap between the two, which
// is what would happen if the cursor were read in a transaction of its own.
//
// Each round seeds a replica from a snapshot taken while writes are in flight,
// follows the delta feed from the cursor it handed back, and diffs the result
// against the server. A cursor that ran ahead of its own data loses a write, and
// the diff says which one.
func TestSyncSnapshot_CursorStaysConsistentUnderConcurrentWrites(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")

	const rounds = 25
	const writesPerRound = 4

	for round := range rounds {
		reqs := make([]*http.Request, writesPerRound)
		for i := range reqs {
			reqs[i] = e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/contexts/%d/tasks", c.ID),
				map[string]any{"title": fmt.Sprintf("Racing write %d/%d", round, i)})
		}

		var wg sync.WaitGroup
		failures := make(chan string, writesPerRound)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, req := range reqs {
				resp, err := e.app.Test(req)
				if err != nil {
					failures <- err.Error()
					return
				}
				_ = resp.Body.Close()
				if resp.StatusCode != 201 {
					failures <- fmt.Sprintf("create task: got %d, want 201", resp.StatusCode)
					return
				}
			}
		}()

		seed, _ := fetchSnapshot(t, e)
		wg.Wait()
		close(failures)
		for msg := range failures {
			t.Fatalf("concurrent write: %s", msg)
		}

		replica := seed.replica(t)
		applyChanges(t, replica, drainChanges(t, e, seed.Cursor, ""))
		after, _ := fetchSnapshot(t, e)
		assertSameReplica(t, replica, after.replica(t))
		if t.Failed() {
			t.Fatalf("replica diverged on round %d", round)
		}
	}
}

// A write that happens after the snapshot was taken must sit strictly above the
// cursor it handed back, so the very next delta call delivers it. A cursor that
// ran ahead of its own payload would swallow the write silently.
func TestSyncSnapshot_LaterWritesLandAboveTheCursor(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	createTestTask(t, e, c.ID, "Before the snapshot")

	seed, _ := fetchSnapshot(t, e)
	later := createTestTask(t, e, c.ID, "After the snapshot")

	change := lastChangeFor(t, drainChanges(t, e, seed.Cursor, ""), repo.SyncEntityTask, later.ID)
	if change.Seq <= seed.Cursor {
		t.Errorf("seq of a post-snapshot write: got %d, want above the cursor %d", change.Seq, seed.Cursor)
	}
	if seed.replica(t)[repo.SyncEntityTask][later.ID] != nil {
		t.Error("a task created after the snapshot was already in it")
	}
}

// One snapshot exposes every kind of row in the workspace at once, so a token
// that may read only some of them must not get at the rest through it.
func TestSyncSnapshot_RequiresEveryReadScope(t *testing.T) {
	e := setupAPIEnv(t)
	partial := issueAPIToken(t, e, "partial", []string{auth.ScopeTasksRead, auth.ScopeProjectsRead})
	resp, body := runRequest(t, e.app, tokenReq(http.MethodGet, "/api/v1/sync/snapshot", partial, nil))
	if resp.StatusCode != 403 {
		t.Fatalf("partial token: got %d, want 403; body: %s", resp.StatusCode, body)
	}

	full := issueAPIToken(t, e, "full", []string{auth.ScopeWildcard})
	resp, body = runRequest(t, e.app, tokenReq(http.MethodGet, "/api/v1/sync/snapshot", full, nil))
	if resp.StatusCode != 200 {
		t.Fatalf("full token: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// applyChanges is the applier a client runs: an upsert stores the row under its
// entity and id, a delete drops it.
func applyChanges(t *testing.T, replica map[string]map[int64]json.RawMessage, changes []syncChange) {
	t.Helper()
	for _, ch := range changes {
		bucket, ok := replica[ch.Entity]
		if !ok {
			bucket = map[int64]json.RawMessage{}
			replica[ch.Entity] = bucket
		}
		if ch.Op == repo.SyncOpDelete {
			delete(bucket, ch.ID)
			continue
		}
		bucket[ch.ID] = ch.Data
	}
}

// assertSameReplica diffs a reconstructed replica against the server's own view
// of the same data, entity by entity and row by row.
func assertSameReplica(t *testing.T, got, want map[string]map[int64]json.RawMessage) {
	t.Helper()
	for entity, wantRows := range want {
		gotRows := got[entity]
		for id, wantRow := range wantRows {
			gotRow, ok := gotRows[id]
			if !ok {
				t.Errorf("%s %d is on the server but missing from the replica", entity, id)
				continue
			}
			assertSameJSON(t, gotRow, wantRow)
		}
		for id := range gotRows {
			if _, ok := wantRows[id]; !ok {
				t.Errorf("%s %d is in the replica but gone from the server", entity, id)
			}
		}
	}
}
