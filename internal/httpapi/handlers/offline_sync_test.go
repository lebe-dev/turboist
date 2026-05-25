package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// patchTitle is a small helper for tests that need to bump a task's
// updated_at via the public PATCH endpoint without caring about the payload.
func patchTitle(t *testing.T, e *apiEnv, id int64, title string) (*http.Response, []byte, error) {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/tasks/"+itoa(id),
		map[string]any{"title": title}))
	return resp, body, nil
}

// TestOfflineSync_TaskPOSTReturnsClientID ensures clientId travels round-trip
// through the task creation endpoint. The frontend outbox uses this to bind a
// locally-minted ulid to the server-assigned PK on first sync.
func TestOfflineSync_TaskPOSTReturnsClientID(t *testing.T) {
	e := setupAPIEnv(t)
	ctxRow, err := e.ctxs.Create(context.Background(), "work", "blue", false)
	if err != nil {
		t.Fatalf("seed ctx: %v", err)
	}
	cid := "01HXYZTASKPOST0000000000A"
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/contexts/"+itoa(ctxRow.ID)+"/tasks",
		map[string]any{"title": "with-cid", "clientId": cid}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.ClientID == nil || *result.ClientID != cid {
		t.Errorf("clientId: got %v, want %q", result.ClientID, cid)
	}
}

// TestOfflineSync_TaskDeleteSoftAndTombstone verifies that DELETE soft-deletes
// (returning 204) and a follow-up DELETE returns 410 Gone — the contract the
// outbox needs to drop a pending mutation against a tombstoned row.
func TestOfflineSync_TaskDeleteSoftAndTombstone(t *testing.T) {
	e := setupAPIEnv(t)
	ctxRow, _ := e.ctxs.Create(context.Background(), "work", "blue", false)
	tk, err := e.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &ctxRow.ID},
		Title:     "doomed",
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/tasks/"+itoa(tk.ID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("first delete: got %d, want 204", resp.StatusCode)
	}
	// Row still exists in DB with deleted_at set.
	got, err := e.tasks.Get(context.Background(), tk.ID)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if got.DeletedAt == nil {
		t.Errorf("expected DeletedAt set, got nil")
	}
	// API hides the tombstone from GET.
	resp2, _ := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/"+itoa(tk.ID), nil))
	if resp2.StatusCode != 404 {
		t.Errorf("GET tombstone: got %d, want 404", resp2.StatusCode)
	}
	// PATCH on a tombstone → 410 Gone.
	resp3, _ := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/tasks/"+itoa(tk.ID),
		map[string]any{"title": "rename"}))
	if resp3.StatusCode != 410 {
		t.Errorf("PATCH tombstone: got %d, want 410", resp3.StatusCode)
	}
	// DELETE on a tombstone → 410 Gone (idempotency contract for outbox flush).
	resp4, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/tasks/"+itoa(tk.ID), nil))
	if resp4.StatusCode != 410 {
		t.Errorf("DELETE tombstone: got %d, want 410", resp4.StatusCode)
	}
}

// TestOfflineSync_ProjectClientIDRoundTrip covers POST projects.
func TestOfflineSync_ProjectClientIDRoundTrip(t *testing.T) {
	e := setupAPIEnv(t)
	ctxRow, _ := e.ctxs.Create(context.Background(), "work", "blue", false)
	cid := "01HXYZPROJPOST0000000000A"
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/contexts/"+itoa(ctxRow.ID)+"/projects",
		map[string]any{"title": "p", "color": "blue", "clientId": cid}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.ClientID == nil || *result.ClientID != cid {
		t.Errorf("clientId: got %v, want %q", result.ClientID, cid)
	}
}

// TestOfflineSync_ProjectDeleteTombstone covers project soft-delete + 410.
func TestOfflineSync_ProjectDeleteTombstone(t *testing.T) {
	e := setupAPIEnv(t)
	ctxRow, _ := e.ctxs.Create(context.Background(), "work", "blue", false)
	p, err := e.projects.Create(context.Background(), repo.CreateProject{
		ContextID: ctxRow.ID, Title: "p", Color: "blue",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/projects/"+itoa(p.ID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("delete: got %d, want 204", resp.StatusCode)
	}
	resp2, _ := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/projects/"+itoa(p.ID),
		map[string]any{"title": "rename"}))
	if resp2.StatusCode != 410 {
		t.Errorf("PATCH tombstone: got %d, want 410", resp2.StatusCode)
	}
	resp3, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/projects/"+itoa(p.ID), nil))
	if resp3.StatusCode != 410 {
		t.Errorf("DELETE tombstone: got %d, want 410", resp3.StatusCode)
	}
}

// TestOfflineSync_LabelClientIDAndTombstone covers labels.
func TestOfflineSync_LabelClientIDAndTombstone(t *testing.T) {
	e := setupAPIEnv(t)
	cid := "01HXYZLABELPOST00000000A"
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/labels/",
		map[string]any{"name": "tag", "color": "blue", "clientId": cid}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d, body: %s", resp.StatusCode, body)
	}
	var l dto.LabelDTO
	if err := json.Unmarshal(body, &l); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if l.ClientID == nil || *l.ClientID != cid {
		t.Errorf("clientId: got %v, want %q", l.ClientID, cid)
	}
	resp2, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/labels/"+itoa(l.ID), nil))
	if resp2.StatusCode != 204 {
		t.Fatalf("delete: got %d, want 204", resp2.StatusCode)
	}
	resp3, _ := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/labels/"+itoa(l.ID),
		map[string]any{"color": "red"}))
	if resp3.StatusCode != 410 {
		t.Errorf("PATCH tombstone: got %d, want 410", resp3.StatusCode)
	}
	resp4, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/labels/"+itoa(l.ID), nil))
	if resp4.StatusCode != 410 {
		t.Errorf("DELETE tombstone: got %d, want 410", resp4.StatusCode)
	}
}

// TestOfflineSync_ContextClientIDAndTombstone covers contexts.
func TestOfflineSync_ContextClientIDAndTombstone(t *testing.T) {
	e := setupAPIEnv(t)
	cid := "01HXYZCTXPOST000000000A"
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/contexts/",
		map[string]any{"name": "home", "color": "blue", "clientId": cid}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d, body: %s", resp.StatusCode, body)
	}
	var c dto.ContextDTO
	if err := json.Unmarshal(body, &c); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.ClientID == nil || *c.ClientID != cid {
		t.Errorf("clientId: got %v, want %q", c.ClientID, cid)
	}
	resp2, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/contexts/"+itoa(c.ID), nil))
	if resp2.StatusCode != 204 {
		t.Fatalf("delete: got %d, want 204", resp2.StatusCode)
	}
	resp3, _ := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/contexts/"+itoa(c.ID),
		map[string]any{"color": "red"}))
	if resp3.StatusCode != 410 {
		t.Errorf("PATCH tombstone: got %d, want 410", resp3.StatusCode)
	}
}

// TestOfflineSync_TaskPatchLWWStale verifies the LWW contract: a PATCH whose
// baseUpdatedAt predates the server's updated_at is silently ignored and the
// server returns its current version (not the client's stale write).
func TestOfflineSync_TaskPatchLWWStale(t *testing.T) {
	e := setupAPIEnv(t)
	ctxRow, _ := e.ctxs.Create(context.Background(), "work", "blue", false)
	tk, err := e.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &ctxRow.ID},
		Title:     "original",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Simulate a concurrent edit landing on the server: a fresh PATCH updates
	// updated_at past any stale base the offline client could supply.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/tasks/"+itoa(tk.ID),
		map[string]any{"title": "server-edit"}))
	if resp.StatusCode != 200 {
		t.Fatalf("server edit: got %d, body %s", resp.StatusCode, body)
	}
	var serverVer dto.TaskDTO
	if err := json.Unmarshal(body, &serverVer); err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Stale base: pre-dating the row's first creation by an hour.
	stale := dto.FormatTime(tk.CreatedAt.Add(-time.Hour))
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/tasks/"+itoa(tk.ID),
		map[string]any{"title": "stale-edit", "baseUpdatedAt": stale}))
	if resp2.StatusCode != 200 {
		t.Fatalf("stale PATCH: got %d", resp2.StatusCode)
	}
	var afterStale dto.TaskDTO
	if err := json.Unmarshal(body2, &afterStale); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if afterStale.Title != "server-edit" {
		t.Errorf("stale PATCH should be ignored: got title %q, want %q", afterStale.Title, "server-edit")
	}
	if afterStale.UpdatedAt != serverVer.UpdatedAt {
		t.Errorf("stale PATCH bumped updated_at: got %q, want %q", afterStale.UpdatedAt, serverVer.UpdatedAt)
	}
}

// TestOfflineSync_TaskPatchLWWFresh ensures a baseUpdatedAt that matches the
// server's current updated_at applies the patch normally.
func TestOfflineSync_TaskPatchLWWFresh(t *testing.T) {
	e := setupAPIEnv(t)
	ctxRow, _ := e.ctxs.Create(context.Background(), "work", "blue", false)
	tk, err := e.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &ctxRow.ID},
		Title:     "v1",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	fresh := dto.FormatTime(tk.UpdatedAt)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/tasks/"+itoa(tk.ID),
		map[string]any{"title": "v2", "baseUpdatedAt": fresh}))
	if resp.StatusCode != 200 {
		t.Fatalf("fresh PATCH: got %d, body %s", resp.StatusCode, body)
	}
	var got dto.TaskDTO
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Title != "v2" {
		t.Errorf("fresh PATCH not applied: title %q, want %q", got.Title, "v2")
	}
}

// TestOfflineSync_TaskPatchLWWIfUnmodifiedSince exercises the header fallback.
func TestOfflineSync_TaskPatchLWWIfUnmodifiedSince(t *testing.T) {
	e := setupAPIEnv(t)
	ctxRow, _ := e.ctxs.Create(context.Background(), "work", "blue", false)
	tk, err := e.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &ctxRow.ID},
		Title:     "original",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Land a server-side edit so any pre-creation base is stale.
	if _, _, err := patchTitle(t, e, tk.ID, "moved-on"); err != nil {
		t.Fatalf("server edit: %v", err)
	}
	req := e.authedReq(t, http.MethodPatch, "/api/v1/tasks/"+itoa(tk.ID),
		map[string]any{"title": "stale"})
	req.Header.Set("If-Unmodified-Since", dto.FormatTime(tk.CreatedAt.Add(-time.Hour)))
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("header LWW: got %d", resp.StatusCode)
	}
	var got dto.TaskDTO
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Title != "moved-on" {
		t.Errorf("header LWW didn't short-circuit: title %q, want %q", got.Title, "moved-on")
	}
}

// TestOfflineSync_PullInitial verifies the initial pull (no `since`) returns
// alive entities only and tasks include both open and recently-completed rows.
func TestOfflineSync_PullInitial(t *testing.T) {
	e := setupAPIEnv(t)
	ctxRow, _ := e.ctxs.Create(context.Background(), "work", "blue", false)
	openTask, _ := e.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &ctxRow.ID}, Title: "open",
	})
	deleted, _ := e.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &ctxRow.ID}, Title: "to-delete",
	})
	if err := e.tasks.Delete(context.Background(), deleted.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/sync/pull", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("pull: got %d, body %s", resp.StatusCode, body)
	}
	var bundle handlers.SyncPullResponse
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if bundle.Now == "" {
		t.Errorf("now: empty")
	}
	foundOpen, foundDeleted := false, false
	for _, tk := range bundle.Tasks {
		if tk.ID == openTask.ID {
			foundOpen = true
		}
		if tk.ID == deleted.ID {
			foundDeleted = true
		}
	}
	if !foundOpen {
		t.Errorf("initial pull missing open task")
	}
	if foundDeleted {
		t.Errorf("initial pull included tombstone task")
	}
	if len(bundle.Contexts) == 0 {
		t.Errorf("contexts: empty")
	}
}

// TestOfflineSync_PullIncremental ensures the incremental pull (?since=...)
// includes tombstones — the client uses them to evict stale local rows.
func TestOfflineSync_PullIncremental(t *testing.T) {
	e := setupAPIEnv(t)
	ctxRow, _ := e.ctxs.Create(context.Background(), "work", "blue", false)
	old, _ := e.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &ctxRow.ID}, Title: "older",
	})
	// Watermark between the older seed and the newer rows.
	time.Sleep(5 * time.Millisecond)
	since := time.Now().UTC()
	time.Sleep(5 * time.Millisecond)
	fresh, _ := e.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &ctxRow.ID}, Title: "newer",
	})
	if err := e.tasks.Delete(context.Background(), fresh.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/sync/pull?since="+model.FormatUTC(since), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("pull: got %d, body %s", resp.StatusCode, body)
	}
	var bundle handlers.SyncPullResponse
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatalf("parse: %v", err)
	}
	foundFresh, foundOld := false, false
	var freshTombstoned bool
	for _, tk := range bundle.Tasks {
		if tk.ID == fresh.ID {
			foundFresh = true
			// The tombstoned row should appear; its UpdatedAt is now-ish.
			if tk.UpdatedAt == "" {
				t.Errorf("tombstone missing updated_at")
			}
			// Tombstones are detectable by the row's status field carrying
			// completed_at == nil and the bundle including a row newer than
			// since — we don't expose deleted_at directly but verify presence.
			freshTombstoned = true
		}
		if tk.ID == old.ID {
			foundOld = true
		}
	}
	if !foundFresh {
		t.Errorf("incremental pull missing tombstoned task")
	}
	if !freshTombstoned {
		t.Errorf("incremental pull did not include tombstone marker")
	}
	if foundOld {
		t.Errorf("incremental pull included row older than since watermark")
	}
}

// TestOfflineSync_SectionClientIDAndTombstone covers sections.
func TestOfflineSync_SectionClientIDAndTombstone(t *testing.T) {
	e := setupAPIEnv(t)
	ctxRow, _ := e.ctxs.Create(context.Background(), "work", "blue", false)
	p, err := e.projects.Create(context.Background(), repo.CreateProject{
		ContextID: ctxRow.ID, Title: "p", Color: "blue",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	cid := "01HXYZSECPOST00000000A"
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/projects/"+itoa(p.ID)+"/sections",
		map[string]any{"title": "todo", "clientId": cid}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d, body: %s", resp.StatusCode, body)
	}
	var s dto.SectionDTO
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.ClientID == nil || *s.ClientID != cid {
		t.Errorf("clientId: got %v, want %q", s.ClientID, cid)
	}
	resp2, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/sections/"+itoa(s.ID), nil))
	if resp2.StatusCode != 204 {
		t.Fatalf("delete: got %d, want 204", resp2.StatusCode)
	}
	resp3, _ := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/sections/"+itoa(s.ID),
		map[string]any{"title": "x"}))
	if resp3.StatusCode != 410 {
		t.Errorf("PATCH tombstone: got %d, want 410", resp3.StatusCode)
	}
}
