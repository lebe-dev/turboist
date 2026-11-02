package openapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/httpapi/openapi"
)

type itemResp struct {
	ID    int64   `json:"id"`
	Title string  `json:"title"`
	Note  *string `json:"note"`
}

type patchReq struct {
	Title dto.Optional[string] `json:"title"`
}

type listQuery struct {
	Limit int `query:"limit"`
}

type errBody struct {
	Code string `json:"code"`
}

func build(t *testing.T, ops ...openapi.Operation) map[string]any {
	t.Helper()
	raw, err := openapi.JSON(openapi.Document{Title: "T", Version: "1", Error: errBody{}}, ops)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func dig(t *testing.T, v any, keys ...string) any {
	t.Helper()
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("at %q: not an object: %v", k, v)
		}
		v, ok = m[k]
		if !ok {
			t.Fatalf("missing key %q in %v", k, m)
		}
	}
	return v
}

func TestSpecPath(t *testing.T) {
	got := openapi.SpecPath("/api/v1/tasks/:id/relations/:relationId")
	if got != "/api/v1/tasks/{id}/relations/{relationId}" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildDescribesPathQueryBodyAndResponses(t *testing.T) {
	spec := build(t, openapi.Operation{
		Method:   http.MethodPatch,
		Path:     "/api/v1/items/:id",
		Summary:  "Update item",
		Tag:      "items",
		Auth:     openapi.AuthAPI,
		Scopes:   []string{"items:write"},
		Query:    listQuery{},
		Request:  patchReq{},
		Response: itemResp{},
	})

	op := dig(t, spec, "paths", "/api/v1/items/{id}", "patch").(map[string]any)
	if op["operationId"] != "patchItemsId" {
		t.Errorf("operationId = %v", op["operationId"])
	}
	params := op["parameters"].([]any)
	if len(params) != 2 {
		t.Fatalf("want path + query parameter, got %v", params)
	}
	path := params[0].(map[string]any)
	if path["in"] != "path" || path["name"] != "id" || dig(t, path, "schema", "type") != "integer" {
		t.Errorf("path parameter = %v", path)
	}
	sec := op["security"].([]any)[0].(map[string]any)
	if scopes := sec[openapi.SecurityScheme].([]any); len(scopes) != 1 || scopes[0] != "items:write" {
		t.Errorf("security = %v", sec)
	}
	dig(t, op, "responses", "200", "content", "application/json")
	dig(t, op, "responses", "default", "content", "application/json")
	dig(t, op, "requestBody", "content", "application/json")
}

func TestBuildRendersOptionalAsNullableValue(t *testing.T) {
	spec := build(t, openapi.Operation{
		Method: http.MethodPost, Path: "/x", Request: patchReq{}, Response: itemResp{},
	})
	schemas := dig(t, spec, "components", "schemas").(map[string]any)
	var title any
	for name, s := range schemas {
		if strings.HasSuffix(name, "PatchReq") {
			title = dig(t, s, "properties", "title")
		}
	}
	raw, _ := json.Marshal(title)
	if !strings.Contains(string(raw), `"string"`) || !strings.Contains(string(raw), `"null"`) {
		t.Fatalf("Optional[string] should render as nullable string, got %s", raw)
	}
}

func TestBuildWithoutResponseIsNoContent(t *testing.T) {
	spec := build(t, openapi.Operation{Method: http.MethodDelete, Path: "/api/v1/items/:id"})
	dig(t, spec, "paths", "/api/v1/items/{id}", "delete", "responses", "204")
}

func TestBuildStringPathParam(t *testing.T) {
	spec := build(t, openapi.Operation{
		Method: http.MethodGet, Path: "/files/:name", StringParams: []string{"name"}, Response: itemResp{},
	})
	p := dig(t, spec, "paths", "/files/{name}", "get", "parameters").([]any)[0]
	if dig(t, p, "schema", "type") != "string" {
		t.Fatalf("got %v", p)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	ops := []openapi.Operation{
		{Method: http.MethodGet, Path: "/b", Response: itemResp{}},
		{Method: http.MethodGet, Path: "/a", Response: itemResp{}},
	}
	doc := openapi.Document{Title: "T", Version: "1"}
	a, err := openapi.YAML(doc, ops)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := openapi.YAML(doc, []openapi.Operation{ops[1], ops[0]})
	if string(a) != string(b) {
		t.Fatal("output depends on operation order")
	}
}

func TestBuildNamesGenericInstantiationsShortly(t *testing.T) {
	spec := build(t, openapi.Operation{
		Method: http.MethodGet, Path: "/items", Response: dto.PagedResponse[itemResp]{},
	})
	schemas := dig(t, spec, "components", "schemas").(map[string]any)
	if _, ok := schemas["DtoPagedResponseItemResp"]; !ok {
		names := make([]string, 0, len(schemas))
		for k := range schemas {
			names = append(names, k)
		}
		t.Fatalf("got schemas %v, want DtoPagedResponseItemResp", names)
	}
}
