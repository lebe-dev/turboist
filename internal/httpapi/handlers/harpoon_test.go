package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type harpoonSlotJSON struct {
	Kind  string `json:"kind"`
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type harpoonRespJSON struct {
	Slots []harpoonSlotJSON `json:"slots"`
}

func attachHarpoon(t *testing.T, e *apiEnv, kind string, id int64) harpoonRespJSON {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/harpoon/attach",
		map[string]any{"kind": kind, "id": id}))
	if resp.StatusCode != 200 {
		t.Fatalf("attach: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var out harpoonRespJSON
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return out
}

func TestHarpoon_AttachAndGet(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Do thing")
	proj := createTestProject(t, e, ctx.ID, "My project")

	attachHarpoon(t, e, "task", task.ID)
	attachHarpoon(t, e, "project", proj.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/harpoon", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("get: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var out harpoonRespJSON
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(out.Slots) != 2 {
		t.Fatalf("slots: got %d, want 2", len(out.Slots))
	}
	if out.Slots[0].Kind != "task" || out.Slots[0].ID != task.ID || out.Slots[0].Title != "Do thing" {
		t.Errorf("slot0: got %+v", out.Slots[0])
	}
	if out.Slots[1].Kind != "project" || out.Slots[1].ID != proj.ID || out.Slots[1].Title != "My project" {
		t.Errorf("slot1: got %+v", out.Slots[1])
	}
}

func TestHarpoon_Detach(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	t1 := createTestTask(t, e, ctx.ID, "t1")
	t2 := createTestTask(t, e, ctx.ID, "t2")

	attachHarpoon(t, e, "task", t1.ID)
	attachHarpoon(t, e, "task", t2.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/harpoon/detach",
		map[string]any{"kind": "task", "id": t1.ID}))
	if resp.StatusCode != 200 {
		t.Fatalf("detach: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var out harpoonRespJSON
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(out.Slots) != 1 || out.Slots[0].ID != t2.ID {
		t.Errorf("slots after detach: got %+v, want only t2 (%d)", out.Slots, t2.ID)
	}
}

func TestHarpoon_AttachUnknownTarget(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/harpoon/attach",
		map[string]any{"kind": "task", "id": 999}))
	if resp.StatusCode != 404 {
		t.Errorf("attach unknown: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

func TestHarpoon_AttachInvalidKind(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/harpoon/attach",
		map[string]any{"kind": "label", "id": 1}))
	if resp.StatusCode != 400 {
		t.Errorf("attach invalid kind: got %d, want 400; body: %s", resp.StatusCode, body)
	}
}
