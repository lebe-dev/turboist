package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
)

// createTestSection creates a context, project, and section for section handler tests.
func createTestSection(t *testing.T, e *apiEnv) (contextID, projectID, sectionID int64) {
	t.Helper()
	ctx, err := e.ctxs.Create(context.Background(), "SectionCtx", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	proj, err := e.projects.Create(context.Background(), repo.CreateProject{
		ContextID:   ctx.ID,
		Title:       "SectionProj",
		Description: "",
		Color:       "red",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sec, err := e.sections.Create(context.Background(), proj.ID, "My Section")
	if err != nil {
		t.Fatalf("create section: %v", err)
	}
	return ctx.ID, proj.ID, sec.ID
}

func TestSectionGet_Found(t *testing.T) {
	e := setupAPIEnv(t)
	_, _, secID := createTestSection(t, e)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/sections/"+itoa(secID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.SectionDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.ID != secID {
		t.Errorf("id: got %d, want %d", result.ID, secID)
	}
	if result.Title != "My Section" {
		t.Errorf("title: got %q, want %q", result.Title, "My Section")
	}
}

func TestSectionGet_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/sections/9999", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeNotFound {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeNotFound)
	}
}

func TestSectionPatch_Success(t *testing.T) {
	e := setupAPIEnv(t)
	_, _, secID := createTestSection(t, e)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/sections/"+itoa(secID), map[string]any{
		"title": "Updated Section",
	}))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.SectionDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "Updated Section" {
		t.Errorf("title: got %q, want %q", result.Title, "Updated Section")
	}
}

func TestSectionDelete_Success(t *testing.T) {
	e := setupAPIEnv(t)
	_, _, secID := createTestSection(t, e)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/sections/"+itoa(secID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("delete: got %d, want 204", resp.StatusCode)
	}
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/sections/"+itoa(secID), nil))
	if resp2.StatusCode != 404 {
		t.Fatalf("after delete: got %d, want 404; body: %s", resp2.StatusCode, body2)
	}
}

func TestSectionDelete_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/sections/9999", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestSectionListTasks_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	_, _, secID := createTestSection(t, e)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/sections/"+itoa(secID)+"/tasks", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.TaskDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("items: got %d, want 0", len(result.Items))
	}
}

func TestSectionReorder_Success(t *testing.T) {
	e := setupAPIEnv(t)
	_, projID, firstID := createTestSection(t, e)
	second, err := e.sections.Create(context.Background(), projID, "Second")
	if err != nil {
		t.Fatalf("seed second: %v", err)
	}
	third, err := e.sections.Create(context.Background(), projID, "Third")
	if err != nil {
		t.Fatalf("seed third: %v", err)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/sections/"+itoa(third.ID)+"/reorder", map[string]any{
		"position": 0,
	}))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.SectionDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Position != 0 {
		t.Errorf("position: got %d, want 0", result.Position)
	}

	items, _, err := e.sections.ListByProject(context.Background(), projID, repo.Page{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	wantOrder := []int64{third.ID, firstID, second.ID}
	for i, want := range wantOrder {
		if items[i].ID != want {
			t.Errorf("order[%d]: got %d, want %d", i, items[i].ID, want)
		}
	}
}

func TestSectionReorder_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/sections/9999/reorder", map[string]any{
		"position": 0,
	}))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeNotFound {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeNotFound)
	}
}

func TestSectionReorder_NegativePosition(t *testing.T) {
	e := setupAPIEnv(t)
	_, _, secID := createTestSection(t, e)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/sections/"+itoa(secID)+"/reorder", map[string]any{
		"position": -1,
	}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestSectionCreateTask_Success(t *testing.T) {
	e := setupAPIEnv(t)
	_, _, secID := createTestSection(t, e)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/sections/"+itoa(secID)+"/tasks", map[string]any{
		"title": "Section task",
	}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "Section task" {
		t.Errorf("title: got %q, want %q", result.Title, "Section task")
	}
	if result.SectionID == nil || *result.SectionID != secID {
		t.Errorf("sectionId: got %v, want %d", result.SectionID, secID)
	}
}
