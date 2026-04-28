package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

type inboxResp struct {
	Count                 int           `json:"count"`
	WarnThresholdExceeded bool          `json:"warnThresholdExceeded"`
	Tasks                 []dto.TaskDTO `json:"tasks"`
}

func TestInboxGet_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/inbox/", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result inboxResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v — body: %s", err, body)
	}
	if result.Count != 0 {
		t.Errorf("count: got %d, want 0", result.Count)
	}
	if result.WarnThresholdExceeded {
		t.Error("warnThresholdExceeded: got true, want false")
	}
	if len(result.Tasks) != 0 {
		t.Errorf("tasks: got %d, want 0", len(result.Tasks))
	}
}

func TestInboxCreateTask_Success(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/tasks", map[string]any{
		"title":       "Inbox task",
		"description": "do it",
	}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "Inbox task" {
		t.Errorf("title: got %q, want %q", result.Title, "Inbox task")
	}
	if result.InboxID == nil {
		t.Error("inboxId: got nil, want non-nil")
	}
}

func TestInboxCreateTask_Validation(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/tasks", map[string]any{
		"title": "",
	}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestInboxGet_WarnThreshold(t *testing.T) {
	e := setupAPIEnv(t)
	// Create tasks up to warn threshold (5 in test config).
	for i := range 5 {
		_ = i
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/tasks", map[string]any{
			"title": "Inbox task",
		}))
		if resp.StatusCode != 201 {
			t.Fatalf("create task: got %d; body: %s", resp.StatusCode, body)
		}
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/inbox/", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result inboxResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !result.WarnThresholdExceeded {
		t.Error("warnThresholdExceeded: got false, want true")
	}
	if result.Count != 5 {
		t.Errorf("count: got %d, want 5", result.Count)
	}
}
