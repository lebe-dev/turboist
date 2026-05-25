package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
)

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
