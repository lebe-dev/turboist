package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

func relationsURL(taskID int64) string {
	return fmt.Sprintf("/api/v1/tasks/%d/relations", taskID)
}

// addRelation posts a relation and returns the updated task the endpoint answers
// with. Every mutation returns the task so the SPA never needs a follow-up read.
func addRelation(t *testing.T, e *apiEnv, taskID int64, body map[string]any) dto.TaskDTO {
	t.Helper()
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, relationsURL(taskID), body))
	if resp.StatusCode != 200 {
		t.Fatalf("add relation: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var out dto.TaskDTO
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parse task: %v", err)
	}
	return out
}

func TestTaskRelationAdd_BlockedBy(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	blocker := createTestTask(t, e, c.ID, "Blocker")
	blocked := createTestTask(t, e, c.ID, "Blocked")

	got := addRelation(t, e, blocked.ID, map[string]any{
		"targetTaskId": blocker.ID,
		"type":         "blocks",
		"direction":    "incoming",
	})
	if got.BlockedByCount != 1 {
		t.Errorf("blockedByCount: got %d, want 1", got.BlockedByCount)
	}
	if got.RelationCount != 1 {
		t.Errorf("relationCount: got %d, want 1", got.RelationCount)
	}
	if len(got.Relations) != 1 {
		t.Fatalf("relations: got %d, want 1", len(got.Relations))
	}
	rel := got.Relations[0]
	if rel.Direction != "incoming" {
		t.Errorf("direction: got %q, want incoming", rel.Direction)
	}
	if rel.Task.ID != blocker.ID {
		t.Errorf("peer id: got %d, want %d", rel.Task.ID, blocker.ID)
	}
	if rel.Task.Title != "Blocker" {
		t.Errorf("peer title: got %q, want %q", rel.Task.Title, "Blocker")
	}
}

func TestTaskRelationAdd_Validation(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	a := createTestTask(t, e, c.ID, "A")
	b := createTestTask(t, e, c.ID, "B")

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"unknown type", map[string]any{"targetTaskId": b.ID, "type": "mentions"}, 400},
		{"bad direction", map[string]any{"targetTaskId": b.ID, "type": "blocks", "direction": "sideways"}, 400},
		{"missing target", map[string]any{"type": "blocks"}, 400},
		{"self relation", map[string]any{"targetTaskId": a.ID, "type": "related"}, 400},
		{"unknown target", map[string]any{"targetTaskId": 999999, "type": "blocks"}, 404},
	}
	for _, tc := range cases {
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, relationsURL(a.ID), tc.body))
		if resp.StatusCode != tc.want {
			t.Errorf("%s: got %d, want %d; body: %s", tc.name, resp.StatusCode, tc.want, body)
		}
	}
}

func TestTaskRelationAdd_Duplicate(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	a := createTestTask(t, e, c.ID, "A")
	b := createTestTask(t, e, c.ID, "B")
	body := map[string]any{"targetTaskId": b.ID, "type": "related"}

	addRelation(t, e, a.ID, body)
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, relationsURL(a.ID), body))
	if resp.StatusCode != 409 {
		t.Errorf("duplicate: got %d, want 409; body: %s", resp.StatusCode, raw)
	}
}

func TestTaskRelationAdd_Cycle(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	a := createTestTask(t, e, c.ID, "A")
	b := createTestTask(t, e, c.ID, "B")

	addRelation(t, e, a.ID, map[string]any{"targetTaskId": b.ID, "type": "blocks", "direction": "outgoing"})
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, relationsURL(b.ID),
		map[string]any{"targetTaskId": a.ID, "type": "blocks", "direction": "outgoing"}))
	if resp.StatusCode != 400 {
		t.Errorf("cycle: got %d, want 400; body: %s", resp.StatusCode, raw)
	}
}

func TestTaskRelationRemove(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	a := createTestTask(t, e, c.ID, "A")
	b := createTestTask(t, e, c.ID, "B")
	added := addRelation(t, e, a.ID, map[string]any{
		"targetTaskId": b.ID, "type": "blocks", "direction": "incoming",
	})
	relationID := added.Relations[0].ID

	url := fmt.Sprintf("%s/%d", relationsURL(a.ID), relationID)
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodDelete, url, nil))
	if resp.StatusCode != 200 {
		t.Fatalf("remove: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var out dto.TaskDTO
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parse task: %v", err)
	}
	if out.BlockedByCount != 0 || out.RelationCount != 0 {
		t.Errorf("counts after remove: got blocked=%d total=%d, want 0/0", out.BlockedByCount, out.RelationCount)
	}
	if len(out.Relations) != 0 {
		t.Errorf("relations after remove: got %d, want 0", len(out.Relations))
	}

	resp, _ = doReq(t, e.app, e.authedReq(t, http.MethodDelete, url, nil))
	if resp.StatusCode != 404 {
		t.Errorf("second remove: got %d, want 404", resp.StatusCode)
	}
}

// A relation must not be removable through a task it does not touch.
func TestTaskRelationRemove_WrongTask(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	a := createTestTask(t, e, c.ID, "A")
	b := createTestTask(t, e, c.ID, "B")
	unrelated := createTestTask(t, e, c.ID, "C")
	added := addRelation(t, e, a.ID, map[string]any{"targetTaskId": b.ID, "type": "related"})

	url := fmt.Sprintf("%s/%d", relationsURL(unrelated.ID), added.Relations[0].ID)
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodDelete, url, nil))
	if resp.StatusCode != 404 {
		t.Errorf("got %d, want 404; body: %s", resp.StatusCode, raw)
	}
}

func TestTaskComplete_Blocked(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	blocker := createTestTask(t, e, c.ID, "Blocker")
	blocked := createTestTask(t, e, c.ID, "Blocked")
	addRelation(t, e, blocked.ID, map[string]any{
		"targetTaskId": blocker.ID, "type": "blocks", "direction": "incoming",
	})

	url := fmt.Sprintf("/api/v1/tasks/%d/complete", blocked.ID)
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, url, nil))
	if resp.StatusCode != 409 {
		t.Fatalf("complete blocked: got %d, want 409; body: %s", resp.StatusCode, raw)
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				BlockerIDs []int64 `json:"blockerIds"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if envelope.Error.Code != httpapi.CodeTaskBlocked {
		t.Errorf("code: got %q, want %q", envelope.Error.Code, httpapi.CodeTaskBlocked)
	}
	if len(envelope.Error.Details.BlockerIDs) != 1 || envelope.Error.Details.BlockerIDs[0] != blocker.ID {
		t.Errorf("blockerIds: got %v, want [%d]", envelope.Error.Details.BlockerIDs, blocker.ID)
	}

	// Completing the blocker releases the dependent.
	blockerURL := fmt.Sprintf("/api/v1/tasks/%d/complete", blocker.ID)
	if resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, blockerURL, nil)); resp.StatusCode != 200 {
		t.Fatalf("complete blocker: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	if resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, url, nil)); resp.StatusCode != 200 {
		t.Fatalf("complete after unblock: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
}

// A subtask of a blocked parent inherits the block: finishing it would start work
// the parent is still waiting on.
func TestTaskComplete_BlockedThroughParent(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	blocker := createTestTask(t, e, c.ID, "Blocker")
	parent := createTestTask(t, e, c.ID, "Parent")
	child := createTestSubtask(t, e, parent.ID, "Child")
	addRelation(t, e, parent.ID, map[string]any{
		"targetTaskId": blocker.ID, "type": "blocks", "direction": "incoming",
	})

	// The badge has to agree with the guard, so the child reads as blocked too.
	childURL := fmt.Sprintf("/api/v1/tasks/%d", child.ID)
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodGet, childURL, nil))
	if resp.StatusCode != 200 {
		t.Fatalf("get child: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var childDTO dto.TaskDTO
	if err := json.Unmarshal(raw, &childDTO); err != nil {
		t.Fatalf("parse child: %v", err)
	}
	if childDTO.BlockedByCount != 1 {
		t.Errorf("child blockedByCount: got %d, want 1", childDTO.BlockedByCount)
	}
	if childDTO.RelationCount != 0 {
		t.Errorf("child relationCount: got %d, want 0 — the relation belongs to the parent", childDTO.RelationCount)
	}

	completeURL := fmt.Sprintf("/api/v1/tasks/%d/complete", child.ID)
	resp, raw = doReq(t, e.app, e.authedReq(t, http.MethodPost, completeURL, nil))
	if resp.StatusCode != 409 {
		t.Fatalf("complete child: got %d, want 409; body: %s", resp.StatusCode, raw)
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				BlockerIDs []int64 `json:"blockerIds"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if envelope.Error.Code != httpapi.CodeTaskBlocked {
		t.Errorf("code: got %q, want %q", envelope.Error.Code, httpapi.CodeTaskBlocked)
	}
	if len(envelope.Error.Details.BlockerIDs) != 1 || envelope.Error.Details.BlockerIDs[0] != blocker.ID {
		t.Errorf("blockerIds: got %v, want [%d]", envelope.Error.Details.BlockerIDs, blocker.ID)
	}

	// Releasing the parent releases the subtree with it.
	blockerURL := fmt.Sprintf("/api/v1/tasks/%d/complete", blocker.ID)
	if resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, blockerURL, nil)); resp.StatusCode != 200 {
		t.Fatalf("complete blocker: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	if resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, completeURL, nil)); resp.StatusCode != 200 {
		t.Fatalf("complete child after unblock: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
}

// Bulk complete reports a blocked task per item rather than failing the batch, so
// the user can see which of a selection was held back and why.
func TestTaskBulkComplete_ReportsBlockedPerItem(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	blocker := createTestTask(t, e, c.ID, "Blocker")
	blocked := createTestTask(t, e, c.ID, "Blocked")
	free := createTestTask(t, e, c.ID, "Free")
	addRelation(t, e, blocked.ID, map[string]any{
		"targetTaskId": blocker.ID, "type": "blocks", "direction": "incoming",
	})

	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/tasks/bulk/complete",
		map[string]any{"ids": []int64{free.ID, blocked.ID}}))
	if resp.StatusCode != 200 {
		t.Fatalf("bulk complete: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var out struct {
		Succeeded []int64 `json:"succeeded"`
		Failed    []struct {
			ID    int64 `json:"id"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"failed"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(out.Succeeded) != 1 || out.Succeeded[0] != free.ID {
		t.Errorf("succeeded: got %v, want [%d]", out.Succeeded, free.ID)
	}
	if len(out.Failed) != 1 || out.Failed[0].ID != blocked.ID {
		t.Fatalf("failed: got %+v, want one entry for %d", out.Failed, blocked.ID)
	}
	if out.Failed[0].Error.Code != httpapi.CodeTaskBlocked {
		t.Errorf("failed code: got %q, want %q", out.Failed[0].Error.Code, httpapi.CodeTaskBlocked)
	}
}

// Relations are opt-in on the detail read so list views do not pay for the join;
// the summary counters, by contrast, must be present everywhere.
func TestTaskGet_RelationsOptIn(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	blocker := createTestTask(t, e, c.ID, "Blocker")
	blocked := createTestTask(t, e, c.ID, "Blocked")
	addRelation(t, e, blocked.ID, map[string]any{
		"targetTaskId": blocker.ID, "type": "blocks", "direction": "incoming",
	})
	base := fmt.Sprintf("/api/v1/tasks/%d", blocked.ID)

	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodGet, base, nil))
	if resp.StatusCode != 200 {
		t.Fatalf("get: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var plain dto.TaskDTO
	if err := json.Unmarshal(raw, &plain); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(plain.Relations) != 0 {
		t.Errorf("relations without the flag: got %d, want 0", len(plain.Relations))
	}
	if plain.BlockedByCount != 1 {
		t.Errorf("blockedByCount without the flag: got %d, want 1", plain.BlockedByCount)
	}

	resp, raw = doReq(t, e.app, e.authedReq(t, http.MethodGet, base+"?relations=true", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("get with relations: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var withRels dto.TaskDTO
	if err := json.Unmarshal(raw, &withRels); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(withRels.Relations) != 1 {
		t.Errorf("relations with the flag: got %d, want 1", len(withRels.Relations))
	}
}

// The counters must reach the list views too — that is what lets a list render a
// blocked task with a disabled checkbox instead of a completable one.
func TestTaskViews_CarryRelationCounters(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	blocker := createTestTask(t, e, c.ID, "Blocker")
	blocked := createTestTask(t, e, c.ID, "Blocked")
	addRelation(t, e, blocked.ID, map[string]any{
		"targetTaskId": blocker.ID, "type": "blocks", "direction": "incoming",
	})

	url := fmt.Sprintf("/api/v1/contexts/%d/tasks", c.ID)
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodGet, url, nil))
	if resp.StatusCode != 200 {
		t.Fatalf("list: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var page dto.PagedResponse[dto.TaskDTO]
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found bool
	for _, item := range page.Items {
		if item.ID != blocked.ID {
			continue
		}
		found = true
		if item.BlockedByCount != 1 {
			t.Errorf("blockedByCount in list: got %d, want 1", item.BlockedByCount)
		}
		if item.RelationCount != 1 {
			t.Errorf("relationCount in list: got %d, want 1", item.RelationCount)
		}
	}
	if !found {
		t.Fatalf("blocked task missing from the context listing")
	}
}
