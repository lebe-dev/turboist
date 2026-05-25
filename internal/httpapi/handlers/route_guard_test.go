package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/auth"
)

// TestRouteGuard_AllAPIRoutesProtected is a fail-open guard: it enumerates every
// route registered under /api/v1 and sends a request authenticated with an API
// token that holds a single, narrowly-scoped permission (search:read). Routes
// outside that scope must be rejected by either RequireScope (403) or
// RequireJWTAuth (401). If a route slips through without either middleware, the
// handler runs and produces some other status — the test catches that as a
// fail-open and reports the offending route, so accidentally-unprotected
// endpoints can never reach production.
func TestRouteGuard_AllAPIRoutesProtected(t *testing.T) {
	env := setupAPIEnv(t)

	plain, err := auth.GenerateAPIToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	hash := auth.HashAPIToken(plain, env.apiTokenSalt)
	if _, err := env.apiTokens.Create(context.Background(), 1, "guard", hash, []string{"search:read"}); err != nil {
		t.Fatalf("create api token: %v", err)
	}

	// Routes mounted outside the /api/v1 group's middleware chain and therefore
	// not subject to this guard. /events streams via ticket auth; the Google
	// OAuth callback is intentionally public.
	skipExact := map[string]bool{
		"/api/v1/events":                    true,
		"/api/v1/calendars/google/callback": true,
	}
	// Paths the guard token can legitimately access — excluded so the test does
	// not flag a 200 that reflects real entitlement rather than fail-open.
	allowedPrefixes := []string{"/api/v1/search"}

	routes := env.app.GetRoutes(true)
	if len(routes) == 0 {
		t.Fatal("no routes registered")
	}

	checked := 0
	for _, r := range routes {
		if !strings.HasPrefix(r.Path, "/api/v1/") {
			continue
		}
		// Fiber auto-registers HEAD for every GET — same middleware chain,
		// would double the noise on failure.
		if r.Method == http.MethodHead {
			continue
		}
		if skipExact[r.Path] {
			continue
		}
		allowed := false
		for _, p := range allowedPrefixes {
			if strings.HasPrefix(r.Path, p) {
				allowed = true
				break
			}
		}
		if allowed {
			continue
		}

		url := substitutePathParams(r.Path)
		req := httptest.NewRequest(r.Method, url, nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		req.Header.Set("Content-Type", "application/json")
		resp, err := env.app.Test(req)
		if err != nil {
			t.Errorf("route %s %s: request failed: %v", r.Method, r.Path, err)
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
			t.Errorf("route %s %s is not protected: got status %d, want 401 or 403", r.Method, r.Path, resp.StatusCode)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no /api/v1 routes were checked — guard scope is too wide")
	}
}

// substitutePathParams replaces ":param" segments with "1" so the path is
// a valid URL that the router can match.
func substitutePathParams(p string) string {
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		if strings.HasPrefix(seg, ":") {
			parts[i] = "1"
		}
	}
	return strings.Join(parts, "/")
}
