package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/repo"
)

// newIdemApp builds a minimal Fiber app whose /api/v1 group runs
// APIAuthMiddleware → IdempotencyMiddleware → extraMW... → handler, mirroring the
// real chain position (Idempotency wraps everything after auth). It seeds user
// id=1 (FK target for idempotency_keys) and returns a valid bearer token plus
// the shared DB so tests can assert on stored reservations.
func newIdemApp(t *testing.T, extraMW []fiber.Handler, h fiber.Handler) (*fiber.App, string, *sql.DB, *repo.IdempotencyRepo) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "idem.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.NewUserRepo(d).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	issuer := auth.NewJWTIssuer(testSecret)
	apiTokens := repo.NewAPITokenRepo(d)
	salt := []byte("test-api-token-salt-32-bytes-pad!")
	idem := repo.NewIdempotencyRepo(d)

	app := httpapi.NewApp(httpapi.Deps{})
	mws := []any{
		httpapi.APIAuthMiddleware(issuer, apiTokens, salt),
		httpapi.IdempotencyMiddleware(idem),
	}
	for _, m := range extraMW {
		mws = append(mws, m)
	}
	grp := app.Group("/api/v1", mws...)
	grp.Post("/thing", h)
	grp.Get("/thing", h)

	tok, _, err := issuer.Issue(1, 1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return app, tok, d, idem
}

func idemReq(t *testing.T, method, path, tok, key string, body any) *http.Request {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	return req
}

func countingJSONHandler(count *int, status int) fiber.Handler {
	return func(c fiber.Ctx) error {
		*count++
		return c.Status(status).JSON(fiber.Map{"n": *count})
	}
}

func idemRowCount(t *testing.T, d *sql.DB) int {
	t.Helper()
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM idempotency_keys").Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func idemErrCode(t *testing.T, b []byte) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("decode error body %q: %v", b, err)
	}
	return env.Error.Code
}

// A request without the header must be a pure pass-through: the handler runs
// once and nothing is reserved.
func TestIdempotencyMiddleware_NoHeaderRunsHandlerOnce(t *testing.T) {
	var n int
	app, tok, d, _ := newIdemApp(t, nil, countingJSONHandler(&n, fiber.StatusCreated))

	resp := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, "", map[string]any{"x": 1}))
	readBody(t, resp)
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status: got %d, want 201", resp.StatusCode)
	}
	if n != 1 {
		t.Fatalf("handler runs: got %d, want 1", n)
	}
	if got := idemRowCount(t, d); got != 0 {
		t.Fatalf("no-header request must not reserve a key: rows=%d", got)
	}
}

// A GET carrying the header is ignored — only mutating methods are covered.
func TestIdempotencyMiddleware_GetWithKeyIgnored(t *testing.T) {
	var n int
	app, tok, d, _ := newIdemApp(t, nil, countingJSONHandler(&n, fiber.StatusOK))

	resp := doRequest(t, app, idemReq(t, http.MethodGet, "/api/v1/thing", tok, "get-key-abcdef", nil))
	readBody(t, resp)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if n != 1 {
		t.Fatalf("handler runs: got %d, want 1", n)
	}
	if got := idemRowCount(t, d); got != 0 {
		t.Fatalf("GET must not reserve a key: rows=%d", got)
	}
}

// A second call with the same key replays the stored response without running
// the handler again.
func TestIdempotencyMiddleware_ReplayReturnsStoredResponse(t *testing.T) {
	var n int
	app, tok, _, _ := newIdemApp(t, nil, countingJSONHandler(&n, fiber.StatusCreated))
	const key = "replay-key-abcdef"

	resp1 := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, key, map[string]any{"x": 1}))
	body1 := readBody(t, resp1)
	if resp1.StatusCode != fiber.StatusCreated {
		t.Fatalf("first status: got %d, want 201", resp1.StatusCode)
	}
	if resp1.Header.Get("X-Idempotent-Replay") != "" {
		t.Fatalf("first call must not be flagged as a replay")
	}

	resp2 := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, key, map[string]any{"x": 1}))
	body2 := readBody(t, resp2)
	if resp2.StatusCode != fiber.StatusCreated {
		t.Fatalf("replay status: got %d, want 201", resp2.StatusCode)
	}
	if resp2.Header.Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("replay must set X-Idempotent-Replay: true, got %q", resp2.Header.Get("X-Idempotent-Replay"))
	}
	if string(body1) != string(body2) {
		t.Fatalf("replay body mismatch:\n first:  %s\n replay: %s", body1, body2)
	}
	if n != 1 {
		t.Fatalf("handler must run exactly once, ran %d times", n)
	}
}

// A key still in flight (a bare reservation with status 0) returns 409.
func TestIdempotencyMiddleware_InFlightReturns409(t *testing.T) {
	var n int
	app, tok, _, idem := newIdemApp(t, nil, countingJSONHandler(&n, fiber.StatusCreated))
	const key = "inflight-key-1234"

	if _, existed, err := idem.Reserve(context.Background(), key, 1, http.MethodPost, "/api/v1/thing"); err != nil {
		t.Fatalf("pre-reserve: %v", err)
	} else if existed {
		t.Fatal("precondition: key should be freshly reserved")
	}

	resp := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, key, map[string]any{}))
	body := readBody(t, resp)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status: got %d, want 409; body %s", resp.StatusCode, body)
	}
	if code := idemErrCode(t, body); code != "idempotency_in_flight" {
		t.Fatalf("error code: got %q, want idempotency_in_flight", code)
	}
	if n != 0 {
		t.Fatalf("handler must not run for an in-flight duplicate, ran %d", n)
	}
}

// A non-2xx response is not cached: the reservation is released and a retry with
// the same key re-runs the handler.
func TestIdempotencyMiddleware_NonSuccessReleasesAndReruns(t *testing.T) {
	var n int
	app, tok, d, _ := newIdemApp(t, nil, countingJSONHandler(&n, fiber.StatusBadRequest))
	const key = "nonok-key-abcdef"

	resp1 := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, key, map[string]any{}))
	readBody(t, resp1)
	if resp1.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("first status: got %d, want 400", resp1.StatusCode)
	}
	if n != 1 {
		t.Fatalf("handler runs: got %d, want 1", n)
	}
	if got := idemRowCount(t, d); got != 0 {
		t.Fatalf("non-2xx must release the reservation, rows=%d", got)
	}

	resp2 := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, key, map[string]any{}))
	readBody(t, resp2)
	if resp2.Header.Get("X-Idempotent-Replay") != "" {
		t.Fatalf("retry after non-2xx must re-run the handler, not replay")
	}
	if n != 2 {
		t.Fatalf("handler must re-run after a non-2xx, ran %d", n)
	}
}

// If the handler returns an error, the reservation is released so an honest
// retry re-runs the handler.
func TestIdempotencyMiddleware_HandlerErrorReleases(t *testing.T) {
	var n int
	h := func(c fiber.Ctx) error {
		n++
		return fiber.NewError(fiber.StatusInternalServerError, "boom")
	}
	app, tok, d, _ := newIdemApp(t, nil, h)
	const key = "err-key-abcdef1"

	resp1 := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, key, map[string]any{}))
	readBody(t, resp1)
	if n != 1 {
		t.Fatalf("handler runs: got %d, want 1", n)
	}
	if got := idemRowCount(t, d); got != 0 {
		t.Fatalf("handler error must release the reservation, rows=%d", got)
	}

	resp2 := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, key, map[string]any{}))
	readBody(t, resp2)
	if n != 2 {
		t.Fatalf("handler must re-run after an error, ran %d", n)
	}
}

// A malformed key is rejected with 400 before the handler runs.
func TestIdempotencyMiddleware_InvalidKeyReturns400(t *testing.T) {
	var n int
	app, tok, d, _ := newIdemApp(t, nil, countingJSONHandler(&n, fiber.StatusCreated))

	// Too short (< 8 chars).
	resp := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, "short", map[string]any{}))
	body := readBody(t, resp)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("short key status: got %d, want 400; body %s", resp.StatusCode, body)
	}
	if code := idemErrCode(t, body); code != httpapi.CodeValidationFailed {
		t.Fatalf("short key code: got %q, want %q", code, httpapi.CodeValidationFailed)
	}

	// Illegal characters.
	resp2 := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, "bad key with spaces!!", map[string]any{}))
	readBody(t, resp2)
	if resp2.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("illegal-charset key status: got %d, want 400", resp2.StatusCode)
	}

	if n != 0 {
		t.Fatalf("handler must not run for an invalid key, ran %d", n)
	}
	if got := idemRowCount(t, d); got != 0 {
		t.Fatalf("invalid key must not reserve, rows=%d", got)
	}
}

// nil repo disables the middleware entirely (pass-through).
func TestIdempotencyMiddleware_NilRepoPassthrough(t *testing.T) {
	app := httpapi.NewApp(httpapi.Deps{})
	var n int
	grp := app.Group("/api/v1", httpapi.IdempotencyMiddleware(nil))
	grp.Post("/thing", func(c fiber.Ctx) error {
		n++
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"n": n})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/thing", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "nilrepo-key-abcdef")
	resp := doRequest(t, app, req)
	readBody(t, resp)
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("nil repo should pass through, got %d", resp.StatusCode)
	}
	if n != 1 {
		t.Fatalf("handler should run once, ran %d", n)
	}
}

// On replay the middleware returns without calling c.Next(), so the next
// middleware in the chain (the position PublishMiddleware occupies in
// RegisterRoutes) never runs — this is what keeps replays from re-emitting SSE.
func TestIdempotencyMiddleware_ReplayShortCircuitsNextMiddleware(t *testing.T) {
	var handlerN, spyN int
	spy := func(c fiber.Ctx) error { spyN++; return c.Next() }
	app, tok, _, _ := newIdemApp(t, []fiber.Handler{spy}, countingJSONHandler(&handlerN, fiber.StatusCreated))
	const key = "order-key-abcdef"

	r1 := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, key, map[string]any{}))
	readBody(t, r1)
	if handlerN != 1 || spyN != 1 {
		t.Fatalf("first call: handlerN=%d spyN=%d, want 1/1", handlerN, spyN)
	}

	r2 := doRequest(t, app, idemReq(t, http.MethodPost, "/api/v1/thing", tok, key, map[string]any{}))
	readBody(t, r2)
	if r2.Header.Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("second call must be a replay")
	}
	if handlerN != 1 {
		t.Fatalf("handler must not run on replay, ran %d", handlerN)
	}
	if spyN != 1 {
		t.Fatalf("downstream middleware (Publish position) must not run on replay, ran %d", spyN)
	}
}
