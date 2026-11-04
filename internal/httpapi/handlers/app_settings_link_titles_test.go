package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type linkTitlesBody struct {
	LinkTitles struct {
		Enabled  bool   `json:"enabled"`
		Template string `json:"template"`
		Rules    []struct {
			Domain   string `json:"domain"`
			Template string `json:"template"`
		} `json:"rules"`
	} `json:"linkTitles"`
}

func TestAppSettings_LinkTitles(t *testing.T) {
	env := setupAPIEnv(t)

	get := func() linkTitlesBody {
		t.Helper()
		resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/app-settings", nil))
		if err != nil {
			t.Fatal(err)
		}
		var b linkTitlesBody
		if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	put := func(body map[string]any) *http.Response {
		t.Helper()
		resp, err := env.app.Test(env.authedReq(t, http.MethodPut, "/api/v1/app-settings/link-titles", body))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	if b := get(); b.LinkTitles.Enabled || b.LinkTitles.Rules == nil || len(b.LinkTitles.Rules) != 0 {
		t.Fatalf("default: %+v", b.LinkTitles)
	}

	resp := put(map[string]any{
		"enabled":  true,
		"template": " Статья «{title}» ",
		"rules":    []map[string]any{{"domain": "https://www.YouTube.com/", "template": "Посмотреть «{title}»"}},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put: status %d", resp.StatusCode)
	}
	b := get()
	if !b.LinkTitles.Enabled || b.LinkTitles.Template != "Статья «{title}»" ||
		len(b.LinkTitles.Rules) != 1 || b.LinkTitles.Rules[0].Domain != "youtube.com" {
		t.Fatalf("after put: %+v", b.LinkTitles)
	}

	// A partial update keeps the rest.
	if resp := put(map[string]any{"enabled": false}); resp.StatusCode != http.StatusOK {
		t.Fatalf("toggle: status %d", resp.StatusCode)
	}
	if b := get(); b.LinkTitles.Enabled || len(b.LinkTitles.Rules) != 1 {
		t.Fatalf("after toggle: %+v", b.LinkTitles)
	}

	for name, body := range map[string]map[string]any{
		"empty":          {},
		"no placeholder": {"template": "Article"},
		"private domain": {"rules": []map[string]any{{"domain": "nas.local", "template": "{title}"}}},
		"rule w/o title": {"rules": []map[string]any{{"domain": "a.com", "template": "x"}}},
	} {
		if resp := put(body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
	}
}
