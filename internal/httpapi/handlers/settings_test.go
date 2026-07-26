package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestSettingsHandler_GetDefault(t *testing.T) {
	env := setupAPIEnv(t)
	req := env.authedReq(t, http.MethodGet, "/api/v1/settings", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	ids, ok := body["weeklyUnplannedExcludedLabelIds"]
	if !ok {
		t.Fatal("missing weeklyUnplannedExcludedLabelIds")
	}
	slice, ok := ids.([]any)
	if !ok {
		t.Fatalf("weeklyUnplannedExcludedLabelIds: got %T, want []any", ids)
	}
	if len(slice) != 0 {
		t.Errorf("weeklyUnplannedExcludedLabelIds: got %v, want []", slice)
	}
	loc, ok := body["locale"].(string)
	if !ok {
		t.Fatalf("locale: missing or wrong type: %T", body["locale"])
	}
	if loc != "" {
		t.Errorf("locale: got %q, want \"\"", loc)
	}
	hidePast, ok := body["calendarHidePastEvents"].(bool)
	if !ok {
		t.Fatalf("calendarHidePastEvents: missing or wrong type: %T", body["calendarHidePastEvents"])
	}
	if !hidePast {
		t.Errorf("calendarHidePastEvents: got %v, want true", hidePast)
	}
}

func TestSettingsHandler_PatchCalendarHidePastEvents(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"calendarHidePastEvents": false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp2, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	hidePast, _ := body["calendarHidePastEvents"].(bool)
	if hidePast {
		t.Errorf("calendarHidePastEvents: got %v, want false", hidePast)
	}
}

func TestSettingsHandler_PatchLocale(t *testing.T) {
	env := setupAPIEnv(t)

	req := env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"locale": "ru",
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	req2 := env.authedReq(t, http.MethodGet, "/api/v1/settings", nil)
	resp2, err := env.app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	loc, _ := body["locale"].(string)
	if loc != "ru" {
		t.Errorf("locale: got %q, want %q", loc, "ru")
	}
}

func TestSettingsHandler_PatchLocaleEmpty(t *testing.T) {
	env := setupAPIEnv(t)

	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"locale": "en",
	})); err != nil {
		t.Fatal(err)
	}
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"locale": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	req := env.authedReq(t, http.MethodGet, "/api/v1/settings", nil)
	resp2, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	loc, _ := body["locale"].(string)
	if loc != "" {
		t.Errorf("locale: got %q, want \"\"", loc)
	}
}

func TestSettingsHandler_PatchLocaleInvalid(t *testing.T) {
	env := setupAPIEnv(t)

	req := env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"locale": "de",
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestSettingsHandler_PatchLocalePreservesOtherFields(t *testing.T) {
	env := setupAPIEnv(t)

	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"weeklyUnplannedExcludedLabelIds": []int{7, 8},
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"locale": "ru",
	})); err != nil {
		t.Fatal(err)
	}

	req := env.authedReq(t, http.MethodGet, "/api/v1/settings", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	ids, _ := body["weeklyUnplannedExcludedLabelIds"].([]any)
	if len(ids) != 2 {
		t.Errorf("weeklyUnplannedExcludedLabelIds: got %v, want [7 8]", ids)
	}
	loc, _ := body["locale"].(string)
	if loc != "ru" {
		t.Errorf("locale: got %q, want %q", loc, "ru")
	}
}

func TestSettingsHandler_Patch(t *testing.T) {
	env := setupAPIEnv(t)

	req := env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"weeklyUnplannedExcludedLabelIds": []int{10, 20},
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// verify persisted via GET
	req2 := env.authedReq(t, http.MethodGet, "/api/v1/settings", nil)
	resp2, err := env.app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	ids, _ := body["weeklyUnplannedExcludedLabelIds"].([]any)
	if len(ids) != 2 {
		t.Fatalf("weeklyUnplannedExcludedLabelIds: got %v, want [10 20]", ids)
	}
}

func TestSettingsHandler_PatchPublicView(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"publicView": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp2, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	pv, _ := body["publicView"].(bool)
	if !pv {
		t.Errorf("publicView: got %v, want true", body["publicView"])
	}
}

func TestSettingsHandler_PatchPublicViewPreservesOtherFields(t *testing.T) {
	env := setupAPIEnv(t)

	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"locale": "ru",
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"publicView": true,
	})); err != nil {
		t.Fatal(err)
	}

	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if loc, _ := body["locale"].(string); loc != "ru" {
		t.Errorf("locale: got %q, want %q", loc, "ru")
	}
	if pv, _ := body["publicView"].(bool); !pv {
		t.Errorf("publicView: got %v, want true", body["publicView"])
	}
}

func TestSettingsHandler_PatchBanner(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"bannerText":      "Heads up: maintenance on Friday.",
		"bannerPublished": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp2, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if txt, _ := body["bannerText"].(string); txt != "Heads up: maintenance on Friday." {
		t.Errorf("bannerText: got %q, want %q", txt, "Heads up: maintenance on Friday.")
	}
	if pub, _ := body["bannerPublished"].(bool); !pub {
		t.Errorf("bannerPublished: got %v, want true", body["bannerPublished"])
	}
}

func TestSettingsHandler_PatchBannerPublishedPreservesText(t *testing.T) {
	env := setupAPIEnv(t)

	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"bannerText":      "Keep me around",
		"bannerPublished": true,
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"bannerPublished": false,
	})); err != nil {
		t.Fatal(err)
	}

	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if txt, _ := body["bannerText"].(string); txt != "Keep me around" {
		t.Errorf("bannerText: got %q, want %q", txt, "Keep me around")
	}
	if pub, _ := body["bannerPublished"].(bool); pub {
		t.Errorf("bannerPublished: got %v, want false", body["bannerPublished"])
	}
}

func TestSettingsHandler_GetDefaultBugLabelIds(t *testing.T) {
	env := setupAPIEnv(t)
	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	ids, ok := body["bugLabelIds"]
	if !ok {
		t.Fatal("missing bugLabelIds")
	}
	slice, ok := ids.([]any)
	if !ok {
		t.Fatalf("bugLabelIds: got %T, want []any", ids)
	}
	if len(slice) != 0 {
		t.Errorf("bugLabelIds: got %v, want []", slice)
	}
}

func TestSettingsHandler_PatchBugLabelIds(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"bugLabelIds": []int{42, 7},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp2, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	ids, _ := body["bugLabelIds"].([]any)
	if len(ids) != 2 {
		t.Fatalf("bugLabelIds: got %v, want [42 7]", ids)
	}
}

func TestSettingsHandler_PatchBugLabelIdsPreservesOtherFields(t *testing.T) {
	env := setupAPIEnv(t)

	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"locale": "ru",
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"bugLabelIds": []int{1, 2, 3},
	})); err != nil {
		t.Fatal(err)
	}

	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if loc, _ := body["locale"].(string); loc != "ru" {
		t.Errorf("locale: got %q, want %q", loc, "ru")
	}
	ids, _ := body["bugLabelIds"].([]any)
	if len(ids) != 3 {
		t.Errorf("bugLabelIds: got %v, want 3 entries", ids)
	}
}

func TestSettingsHandler_PatchBugLabelIdsClear(t *testing.T) {
	env := setupAPIEnv(t)

	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"bugLabelIds": []int{5, 6},
	})); err != nil {
		t.Fatal(err)
	}
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"bugLabelIds": []int{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp2, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	ids, _ := body["bugLabelIds"].([]any)
	if len(ids) != 0 {
		t.Fatalf("bugLabelIds: got %v, want []", ids)
	}
}

func TestSettingsHandler_PatchClear(t *testing.T) {
	env := setupAPIEnv(t)

	env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"weeklyUnplannedExcludedLabelIds": []int{5},
	})
	req := env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"weeklyUnplannedExcludedLabelIds": []int{},
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	req2 := env.authedReq(t, http.MethodGet, "/api/v1/settings", nil)
	resp2, err := env.app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	ids, _ := body["weeklyUnplannedExcludedLabelIds"].([]any)
	if len(ids) != 0 {
		t.Fatalf("weeklyUnplannedExcludedLabelIds: got %v, want []", ids)
	}
}

func TestSettingsHandler_PatchBannerDayPart(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"bannerDayPart": "afternoon",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp2, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	part, ok := body["bannerDayPart"].(string)
	if !ok {
		t.Fatalf("bannerDayPart: missing or wrong type: %T", body["bannerDayPart"])
	}
	if part != "afternoon" {
		t.Errorf("bannerDayPart: got %q, want %q", part, "afternoon")
	}
}

func TestSettingsHandler_PatchBannerDayPartAllDay(t *testing.T) {
	env := setupAPIEnv(t)

	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"bannerDayPart": "evening",
	})); err != nil {
		t.Fatal(err)
	}
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"bannerDayPart": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp2, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	part, _ := body["bannerDayPart"].(string)
	if part != "" {
		t.Errorf("bannerDayPart: got %q, want \"\"", part)
	}
}

func TestSettingsHandler_PatchBannerDayPartInvalid(t *testing.T) {
	env := setupAPIEnv(t)

	for _, part := range []string{"none", "night", "Morning"} {
		resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
			"bannerDayPart": part,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%q: status: got %d, want %d", part, resp.StatusCode, http.StatusBadRequest)
		}
	}
}

func TestSettingsHandler_GetDefaultBannerDayPart(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	part, ok := body["bannerDayPart"].(string)
	if !ok {
		t.Fatalf("bannerDayPart: missing or wrong type: %T", body["bannerDayPart"])
	}
	if part != "" {
		t.Errorf("bannerDayPart: got %q, want \"\"", part)
	}
}

func TestSettingsHandler_GetDefaultMaxPinned(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"maxPinnedTasks", "maxPinnedProjects"} {
		v, ok := body[field].(float64)
		if !ok {
			t.Fatalf("%s: missing or wrong type: %T", field, body[field])
		}
		if v != model.DefaultMaxPinned {
			t.Errorf("%s: got %v, want %d", field, v, model.DefaultMaxPinned)
		}
	}
}

func TestSettingsHandler_PatchMaxPinned(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"maxPinnedTasks":    3,
		"maxPinnedProjects": 25,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if got := body["maxPinnedTasks"]; got != float64(3) {
		t.Errorf("maxPinnedTasks: got %v, want 3", got)
	}
	if got := body["maxPinnedProjects"]; got != float64(25) {
		t.Errorf("maxPinnedProjects: got %v, want 25", got)
	}

	// The value must survive a round-trip through users.settings.
	resp, err = env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var reread map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&reread); err != nil {
		t.Fatal(err)
	}
	if got := reread["maxPinnedTasks"]; got != float64(3) {
		t.Errorf("reread maxPinnedTasks: got %v, want 3", got)
	}
	if got := reread["maxPinnedProjects"]; got != float64(25) {
		t.Errorf("reread maxPinnedProjects: got %v, want 25", got)
	}
}

func TestSettingsHandler_PatchMaxPinnedOutOfRange(t *testing.T) {
	env := setupAPIEnv(t)

	for _, field := range []string{"maxPinnedTasks", "maxPinnedProjects"} {
		for _, v := range []int{0, -1, model.MaxMaxPinned + 1} {
			resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
				field: v,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s=%d: status: got %d, want %d", field, v, resp.StatusCode, http.StatusBadRequest)
			}
		}
	}
}

func TestSettingsHandler_PatchMaxPinnedPreservesOtherFields(t *testing.T) {
	env := setupAPIEnv(t)

	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"locale":     "ru",
		"publicView": true,
	})); err != nil {
		t.Fatal(err)
	}
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"maxPinnedTasks": 7,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if got := body["locale"]; got != "ru" {
		t.Errorf("locale: got %v, want ru", got)
	}
	if got := body["publicView"]; got != true {
		t.Errorf("publicView: got %v, want true", got)
	}
	if got := body["maxPinnedTasks"]; got != float64(7) {
		t.Errorf("maxPinnedTasks: got %v, want 7", got)
	}
	if got := body["maxPinnedProjects"]; got != float64(model.DefaultMaxPinned) {
		t.Errorf("maxPinnedProjects: got %v, want %d", got, model.DefaultMaxPinned)
	}
}

// TestSettingsHandler_PatchMaxPinnedBoundaries asserts the inclusive ends of the
// accepted range go through — an off-by-one in validateMaxPinned would make the
// UI's own min/max unreachable.
func TestSettingsHandler_PatchMaxPinnedBoundaries(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"maxPinnedTasks":    model.MinMaxPinned,
		"maxPinnedProjects": model.MaxMaxPinned,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["maxPinnedTasks"] != float64(model.MinMaxPinned) {
		t.Errorf("maxPinnedTasks: got %v, want %d", body["maxPinnedTasks"], model.MinMaxPinned)
	}
	if body["maxPinnedProjects"] != float64(model.MaxMaxPinned) {
		t.Errorf("maxPinnedProjects: got %v, want %d", body["maxPinnedProjects"], model.MaxMaxPinned)
	}
}

// TestSettingsHandler_PatchMaxPinnedNonInteger asserts fractional and
// non-numeric values are refused rather than silently truncated.
func TestSettingsHandler_PatchMaxPinnedNonInteger(t *testing.T) {
	env := setupAPIEnv(t)

	for _, v := range []any{3.5, "5", true} {
		resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
			"maxPinnedTasks": v,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("maxPinnedTasks=%v: status: got %d, want %d", v, resp.StatusCode, http.StatusBadRequest)
		}
	}
	// The rejected patches must not have moved the stored value.
	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["maxPinnedTasks"] != float64(model.DefaultMaxPinned) {
		t.Errorf("maxPinnedTasks: got %v, want %d", body["maxPinnedTasks"], model.DefaultMaxPinned)
	}
}
