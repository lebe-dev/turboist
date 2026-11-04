// Package openapi turns a table of described HTTP operations into an OpenAPI
// 3.1 document. Request and response schemas are reflected from the very Go
// types the handlers bind and send, so a change to a DTO changes the document
// without anybody touching a description.
package openapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/swaggest/jsonschema-go"
	oapi "github.com/swaggest/openapi-go"
	"github.com/swaggest/openapi-go/openapi31"
)

// Auth says which credentials an operation accepts.
type Auth int

const (
	// AuthNone marks a public endpoint.
	AuthNone Auth = iota
	// AuthAPI accepts a session access token or a personal API token.
	AuthAPI
	// AuthJWT accepts only a session access token; API tokens are refused.
	AuthJWT
)

// SecurityScheme is the name of the bearer scheme in components.securitySchemes.
const SecurityScheme = "bearerAuth"

// Operation describes one route. Path uses the router's own syntax
// (`/api/v1/tasks/:id`), which is what lets tests match the table against the
// routes that are actually mounted.
type Operation struct {
	Method      string
	Path        string
	Summary     string
	Description string
	Tag         string
	Auth        Auth
	// Scopes an API token must carry; a session token always passes.
	Scopes []string
	// StringParams names path parameters that are strings; every other path
	// parameter is an integer id.
	StringParams []string
	// Query is a struct whose `query:"name"` fields document query parameters.
	Query any
	// Request is the JSON body the handler binds.
	Request any
	// RequestContentType overrides application/json for a non-JSON body.
	RequestContentType string
	// Response is the value the handler sends on success; nil means no body.
	Response any
	// Status is the success status; zero means 200, or 204 without a Response.
	Status int
	// ResponseContentType overrides application/json for a non-JSON body.
	ResponseContentType string
}

// Document holds what the generated document says about the API as a whole.
type Document struct {
	Title       string
	Version     string
	Description string
	// Error is the body every failed request answers with.
	Error any
}

// Build reflects ops into an OpenAPI 3.1 document.
func Build(doc Document, ops []Operation) (*openapi31.Spec, error) {
	r := openapi31.NewReflector()
	r.Spec.Info.WithTitle(doc.Title).WithVersion(doc.Version)
	if doc.Description != "" {
		r.Spec.Info.WithDescription(doc.Description)
	}
	r.Spec.SetHTTPBearerTokenSecurity(SecurityScheme, "JWT or API token",
		"A session access token, or a personal API token limited to the scopes it was issued with.")

	mapOptionalTypes(&r.Reflector, ops)
	r.AddTypeMapping(json.RawMessage{}, new(any))
	r.DefaultOptions = append(r.DefaultOptions,
		jsonschema.InterceptDefName(genericDefName),
		jsonschema.InterceptNullability(markOptionalNullable),
	)

	sorted := append([]Operation(nil), ops...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		return sorted[i].Method < sorted[j].Method
	})

	for _, op := range sorted {
		if err := addOperation(r, doc, op); err != nil {
			return nil, err
		}
	}
	return r.Spec, nil
}

// YAML builds the document and renders it as YAML.
func YAML(doc Document, ops []Operation) ([]byte, error) {
	spec, err := Build(doc, ops)
	if err != nil {
		return nil, err
	}
	return spec.MarshalYAML()
}

// JSON builds the document and renders it as indented JSON.
func JSON(doc Document, ops []Operation) ([]byte, error) {
	spec, err := Build(doc, ops)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(spec, "", "  ")
}

// SpecPath converts router syntax to OpenAPI syntax: `/tasks/:id` → `/tasks/{id}`.
func SpecPath(routerPath string) string {
	segs := strings.Split(routerPath, "/")
	for i, s := range segs {
		if strings.HasPrefix(s, ":") {
			segs[i] = "{" + strings.TrimSuffix(s[1:], "?") + "}"
		}
	}
	return strings.Join(segs, "/")
}

func addOperation(r *openapi31.Reflector, doc Document, op Operation) error {
	oc, err := r.NewOperationContext(op.Method, SpecPath(op.Path))
	if err != nil {
		return fmt.Errorf("operation %s %s: %w", op.Method, op.Path, err)
	}
	oc.SetID(operationID(op.Method, op.Path))
	if op.Summary != "" {
		oc.SetSummary(op.Summary)
	}
	if op.Tag != "" {
		oc.SetTags(op.Tag)
	}
	if desc := describe(op); desc != "" {
		oc.SetDescription(desc)
	}
	if op.Auth != AuthNone {
		oc.AddSecurity(SecurityScheme, op.Scopes...)
	}

	if params := pathParams(op); params != nil {
		oc.AddReqStructure(params)
	}
	if op.Query != nil {
		oc.AddReqStructure(op.Query)
	}
	if op.Request != nil {
		ct := op.RequestContentType
		if ct == "" {
			ct = "application/json"
		}
		oc.AddReqStructure(op.Request, oapi.WithContentType(ct))
	}

	status := op.Status
	switch {
	case status != 0:
	case op.Response == nil:
		status = http.StatusNoContent
	default:
		status = http.StatusOK
	}
	if op.Response == nil {
		oc.AddRespStructure(nil, oapi.WithHTTPStatus(status))
	} else {
		opts := []oapi.ContentOption{oapi.WithHTTPStatus(status)}
		if op.ResponseContentType != "" {
			opts = append(opts, oapi.WithContentType(op.ResponseContentType))
		}
		oc.AddRespStructure(op.Response, opts...)
	}
	if doc.Error != nil {
		oc.AddRespStructure(doc.Error, func(cu *oapi.ContentUnit) {
			cu.IsDefault = true
			cu.Description = "Error"
		})
	}

	if err := r.AddOperation(oc); err != nil {
		return fmt.Errorf("operation %s %s: %w", op.Method, op.Path, err)
	}
	return nil
}

func describe(op Operation) string {
	parts := make([]string, 0, 2)
	if op.Description != "" {
		parts = append(parts, op.Description)
	}
	if op.Auth == AuthJWT {
		parts = append(parts, "Requires a session access token; API tokens are refused.")
	}
	return strings.Join(parts, "\n\n")
}

// pathParams builds a struct with one `path` field per placeholder in the route,
// because the reflector learns parameters only from struct tags.
func pathParams(op Operation) any {
	var fields []reflect.StructField
	for _, seg := range strings.Split(op.Path, "/") {
		if !strings.HasPrefix(seg, ":") {
			continue
		}
		name := strings.TrimSuffix(seg[1:], "?")
		typ := reflect.TypeOf(int64(0))
		for _, s := range op.StringParams {
			if s == name {
				typ = reflect.TypeOf("")
			}
		}
		fields = append(fields, reflect.StructField{
			Name: fmt.Sprintf("P%d", len(fields)),
			Type: typ,
			Tag:  reflect.StructTag(fmt.Sprintf(`path:%q required:"true"`, name)),
		})
	}
	if fields == nil {
		return nil
	}
	return reflect.New(reflect.StructOf(fields)).Interface()
}

// operationID derives a stable id from the method and path:
// GET /api/v1/tasks/:id/subtasks → getTasksIdSubtasks.
func operationID(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	for _, seg := range strings.Split(strings.TrimPrefix(path, "/api/v1"), "/") {
		seg = strings.TrimSuffix(strings.TrimPrefix(seg, ":"), "?")
		for _, word := range strings.FieldsFunc(seg, func(r rune) bool {
			return r == '-' || r == '_' || r == '.'
		}) {
			b.WriteString(strings.ToUpper(word[:1]) + word[1:])
		}
	}
	return b.String()
}

// genericDefName shortens the name of a generic instantiation: the reflector
// spells out every type argument's full import path, so
// `dto.PagedResponse[github.com/.../dto.TaskDTO]` becomes `DtoPagedResponseTaskDTO`.
func genericDefName(t reflect.Type, name string) string {
	base, args, ok := strings.Cut(t.Name(), "[")
	if !ok {
		return name
	}
	var b strings.Builder
	if pkg := t.PkgPath(); pkg != "" {
		pkg = pkg[strings.LastIndex(pkg, "/")+1:]
		b.WriteString(strings.ToUpper(pkg[:1]))
		b.WriteString(pkg[1:])
	}
	b.WriteString(base)
	for arg := range strings.SplitSeq(strings.TrimSuffix(args, "]"), ",") {
		arg = strings.TrimSpace(arg)
		arg = arg[strings.LastIndexAny(arg, "./")+1:]
		b.WriteString(strings.ToUpper(arg[:1]))
		b.WriteString(arg[1:])
	}
	return b.String()
}

// optionalMarker is satisfied by dto.Optional[T]: a PATCH field that tells an
// absent key from an explicit null. On the wire it is just a nullable T.
type optionalMarker interface {
	IsAbsent() bool
	IsNull() bool
	IsSet() bool
}

var optionalMarkerType = reflect.TypeOf((*optionalMarker)(nil)).Elem()

// markOptionalNullable adds null to an Optional[T] property: the reflector
// decides nullability from the Go field type, which is a struct, not a pointer.
func markOptionalNullable(p jsonschema.InterceptNullabilityParams) {
	if p.NullAdded || !p.Type.Implements(optionalMarkerType) {
		return
	}
	if p.Schema.Ref != nil {
		ref := *p.Schema
		*p.Schema = jsonschema.Schema{AnyOf: []jsonschema.SchemaOrBool{
			jsonschema.Null.ToSchemaOrBool(),
			ref.ToSchemaOrBool(),
		}}
		return
	}
	p.Schema.AddType(jsonschema.Null)
}

// mapOptionalTypes finds every Optional[T] reachable from the operations and
// maps it to *T, which the reflector renders as a nullable T.
func mapOptionalTypes(r *jsonschema.Reflector, ops []Operation) {
	seen := map[reflect.Type]bool{}
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || seen[t] {
			return
		}
		seen[t] = true
		if t.Implements(optionalMarkerType) {
			if m, ok := t.MethodByName("Value"); ok && m.Type.NumOut() == 2 {
				inner := m.Type.Out(0)
				r.AddTypeMapping(reflect.New(t).Elem().Interface(), reflect.New(inner).Interface())
				walk(inner)
			}
			return
		}
		for i := 0; i < t.NumField(); i++ {
			walk(t.Field(i).Type)
		}
	}
	for _, op := range ops {
		for _, v := range []any{op.Query, op.Request, op.Response} {
			if v != nil {
				walk(reflect.TypeOf(v))
			}
		}
	}
}

// OneOf documents a body that is exactly one of the given shapes.
func OneOf(variants ...any) any {
	return jsonschema.OneOf(variants...)
}
