package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

func TestMetaConfig_Success(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/config", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v — body: %s", err, body)
	}
	if result["timezone"] != "UTC" {
		t.Errorf("timezone: got %v, want UTC", result["timezone"])
	}
	if result["maxPinned"] == nil {
		t.Error("maxPinned missing from config")
	}
	if result["weekly"] == nil {
		t.Error("weekly missing from config")
	}
	if result["inbox"] == nil {
		t.Error("inbox missing from config")
	}
	for _, key := range []string{
		"contexts", "projects", "labels",
		"settings", "appSettings", "userState", "troiki",
		"planStats", "inboxStats", "pinnedTasks",
		"harpoon", "taskTemplates",
	} {
		if _, ok := result[key]; !ok {
			t.Errorf("%s missing from config", key)
		}
	}
	planStats, ok := result["planStats"].(map[string]any)
	if !ok {
		t.Fatalf("planStats: got %T, want map[string]any", result["planStats"])
	}
	if _, ok := planStats["week"]; !ok {
		t.Error("planStats.week missing")
	}
	if _, ok := planStats["backlog"]; !ok {
		t.Error("planStats.backlog missing")
	}
	inboxStats, ok := result["inboxStats"].(map[string]any)
	if !ok {
		t.Fatalf("inboxStats: got %T, want map[string]any", result["inboxStats"])
	}
	if _, ok := inboxStats["count"]; !ok {
		t.Error("inboxStats.count missing")
	}
	if _, ok := result["pinnedTasks"].([]any); !ok {
		t.Errorf("pinnedTasks: got %T, want []any", result["pinnedTasks"])
	}
	if _, ok := result["contexts"].([]any); !ok {
		t.Errorf("contexts: got %T, want []any", result["contexts"])
	}
	if _, ok := result["projects"].([]any); !ok {
		t.Errorf("projects: got %T, want []any", result["projects"])
	}
	if _, ok := result["labels"].([]any); !ok {
		t.Errorf("labels: got %T, want []any", result["labels"])
	}
	settings, ok := result["settings"].(map[string]any)
	if !ok {
		t.Fatalf("settings: got %T, want map[string]any", result["settings"])
	}
	if _, ok := settings["locale"]; !ok {
		t.Error("settings.locale missing")
	}
	appSettings, ok := result["appSettings"].(map[string]any)
	if !ok {
		t.Fatalf("appSettings: got %T, want map[string]any", result["appSettings"])
	}
	if _, ok := appSettings["autoLabels"]; !ok {
		t.Error("appSettings.autoLabels missing")
	}
	if _, ok := result["userState"].(map[string]any); !ok {
		t.Errorf("userState: got %T, want map[string]any", result["userState"])
	}
	troiki, ok := result["troiki"].(map[string]any)
	if !ok {
		t.Fatalf("troiki: got %T, want map[string]any", result["troiki"])
	}
	for _, slot := range []string{"important", "medium", "rest"} {
		if _, ok := troiki[slot]; !ok {
			t.Errorf("troiki.%s missing", slot)
		}
	}
	harpoon, ok := result["harpoon"].(map[string]any)
	if !ok {
		t.Fatalf("harpoon: got %T, want map[string]any", result["harpoon"])
	}
	// Byte-identical to GET /api/v1/harpoon: the envelope, not a bare array.
	if _, ok := harpoon["slots"].([]any); !ok {
		t.Errorf("harpoon.slots: got %T, want []any", harpoon["slots"])
	}
	// A BARE array, unlike GET /api/v1/task-templates which returns a paged
	// envelope. The frontend type mirrors this — see configResp in meta.go.
	if _, ok := result["taskTemplates"].([]any); !ok {
		t.Errorf("taskTemplates: got %T, want []any (bare array, not a paged envelope)", result["taskTemplates"])
	}
}

// The SPA no longer calls GET /api/v1/harpoon or GET /api/v1/task-templates at
// all, so the aggregate must carry the same data those endpoints do. A shape
// check alone would pass on a permanently empty pair and template menu.
func TestMetaConfig_HarpoonAndTemplatesCarryData(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Do thing")
	attachHarpoon(t, e, "task", task.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/task-templates/", map[string]any{
		"name":     "Onboard",
		"subtasks": []map[string]any{{"title": "Call"}},
	}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create template: got %d; body: %s", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/config", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("config: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var cfg struct {
		Harpoon       harpoonRespJSON `json:"harpoon"`
		TaskTemplates []struct {
			Name     string `json:"name"`
			Subtasks []struct {
				Title string `json:"title"`
			} `json:"subtasks"`
		} `json:"taskTemplates"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatalf("parse: %v — body: %s", err, body)
	}

	if len(cfg.Harpoon.Slots) != 1 {
		t.Fatalf("harpoon.slots: got %d, want 1", len(cfg.Harpoon.Slots))
	}
	if got := cfg.Harpoon.Slots[0]; got.Kind != "task" || got.ID != task.ID || got.Title != "Do thing" {
		t.Errorf("harpoon slot: got %+v", got)
	}

	if len(cfg.TaskTemplates) != 1 {
		t.Fatalf("taskTemplates: got %d, want 1", len(cfg.TaskTemplates))
	}
	if cfg.TaskTemplates[0].Name != "Onboard" {
		t.Errorf("template name: got %q, want %q", cfg.TaskTemplates[0].Name, "Onboard")
	}
	// The quick-add menu instantiates the whole tree, so subtasks must ride along.
	if len(cfg.TaskTemplates[0].Subtasks) != 1 || cfg.TaskTemplates[0].Subtasks[0].Title != "Call" {
		t.Errorf("template subtasks: got %+v", cfg.TaskTemplates[0].Subtasks)
	}
}

// The SPA refetches /api/v1/config on every SSE reconnect — i.e. every phone
// unlock — and most of those find nothing changed.
func TestMetaConfig_ETag(t *testing.T) {
	e := setupAPIEnv(t)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/config", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("first GET: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the config response")
	}

	// Unchanged data must hash identically across requests — otherwise the ETag
	// churns every call and buys nothing.
	resp2, _ := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/config", nil))
	if got := resp2.Header.Get("ETag"); got != etag {
		t.Fatalf("ETag is not stable for unchanged data: %q then %q", etag, got)
	}

	req := e.authedReq(t, http.MethodGet, "/api/v1/config", nil)
	req.Header.Set("If-None-Match", etag)
	resp3, body3 := doReq(t, e.app, req)
	if resp3.StatusCode != http.StatusNotModified {
		t.Fatalf("If-None-Match match: got %d, want 304; body: %s", resp3.StatusCode, body3)
	}
	if len(body3) != 0 {
		t.Errorf("304 must have an empty body, got %q", body3)
	}

	stale := e.authedReq(t, http.MethodGet, "/api/v1/config", nil)
	stale.Header.Set("If-None-Match", `"stale"`)
	resp4, body4 := doReq(t, e.app, stale)
	if resp4.StatusCode != 200 {
		t.Fatalf("stale If-None-Match: got %d, want 200", resp4.StatusCode)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body4, &parsed); err != nil {
		t.Fatalf("200 body must still be the full config: %v", err)
	}
	if _, ok := parsed["contexts"]; !ok {
		t.Error("contexts missing from the non-304 response")
	}
}

// A change to the workspace must produce a different ETag, or a client holding
// the old one would never see the update.
func TestMetaConfig_ETagChangesWithData(t *testing.T) {
	e := setupAPIEnv(t)

	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/config", nil))
	before := resp.Header.Get("ETag")

	created, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/labels", map[string]any{
		"name": "etag-probe", "color": "red",
	}))
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create label: got %d; body: %s", created.StatusCode, body)
	}

	resp2, _ := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/config", nil))
	if after := resp2.Header.Get("ETag"); after == before {
		t.Fatalf("ETag unchanged (%q) after adding a label", after)
	}
}

func TestMetaConfig_RequiresAuth(t *testing.T) {
	e := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

func TestHealthz_Public(t *testing.T) {
	e := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp, _ := doReq(t, e.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestVersion_Public(t *testing.T) {
	e := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result["version"] == nil {
		t.Error("version field missing")
	}
}

// The config aggregate is the SPA's boot payload, and its pinnedTasks feed the
// sidebar directly. Those tasks must carry the relation counters or a pinned
// blocked task would render with a completable checkbox.
func TestMetaConfig_PinnedTasksCarryRelationCounters(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	blocker := createTestTask(t, e, c.ID, "Blocker")
	blocked := createTestTask(t, e, c.ID, "Blocked")
	addRelation(t, e, blocked.ID, map[string]any{
		"targetTaskId": blocker.ID, "type": "blocks", "direction": "incoming",
	})
	pinURL := fmt.Sprintf("/api/v1/tasks/%d/pin", blocked.ID)
	if resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, pinURL, nil)); resp.StatusCode != 200 {
		t.Fatalf("pin: got %d, want 200; body: %s", resp.StatusCode, raw)
	}

	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/config", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("config: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var cfg struct {
		PinnedTasks []dto.TaskDTO `json:"pinnedTasks"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found bool
	for _, task := range cfg.PinnedTasks {
		if task.ID != blocked.ID {
			continue
		}
		found = true
		if task.BlockedByCount != 1 {
			t.Errorf("blockedByCount: got %d, want 1", task.BlockedByCount)
		}
		if task.RelationCount != 1 {
			t.Errorf("relationCount: got %d, want 1", task.RelationCount)
		}
	}
	if !found {
		t.Fatalf("pinned blocked task missing from config.pinnedTasks")
	}
}
