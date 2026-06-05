package handlers_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/handshake"
	"github.com/lebe-dev/turboist/internal/federation/protocol"
	"github.com/lebe-dev/turboist/internal/federation/ratelimit"
	"github.com/lebe-dev/turboist/internal/federation/transport"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// Federation v1 F7.7 — NFR-3 security verification (HTTP-boundary half).
//
// This file asserts the per-peer handshake rate limit (NFR-3): a peer hammering
// the signed /federation/handshake endpoint is throttled with 429 + Retry-After
// before any invite work, blunting invite brute-force / handshake-flood DoS. The
// request is driven through the REAL HTTPSignatureMiddleware (the peer's Ed25519
// signature is genuinely verified), so the limiter sits behind real authentication
// and the assertion is end-to-end, not a handler-unit shortcut. The companion
// service-layer NFR-3 checks (invite entropy, constant-time grep-guard) live in
// internal/service/federation/nfr3_security_test.go; the no-secret-in-logs leg is
// asserted below at the join handler boundary.

// signedHandshake signs a handshake body as the joiner (using the joiner's loaded
// Ed25519 private key) over the PINNED 7-line transport canonical string and POSTs
// it to the owner via the in-process registry. nonceSeed forces a fresh nonce per
// call so a normal repeat is never itself a transport replay. It returns the
// owner's raw response so the test can read the authoritative status + headers.
func signedHandshake(t *testing.T, reg *fedRegistry, joiner *fedInstance, ownerURL string, inv fedsvc.ParsedInvite, nonceSeed string) *http.Response {
	t.Helper()
	ctx := context.Background()
	keys, err := joiner.keys.Ensure(ctx, crypto.NewTokenCipher(fedHandlerKey), "joiner")
	if err != nil {
		t.Fatalf("ensure joiner keys: %v", err)
	}
	priv, _, err := crypto.LoadInstanceKeypair(crypto.NewTokenCipher(fedHandlerKey), keys.PublicKey, keys.PrivateSeedEnc)
	if err != nil {
		t.Fatalf("load joiner keypair: %v", err)
	}

	body := handshake.Request{
		InviteID:          inv.InviteID,
		Secret:            inv.Secret,
		JoinerInstanceURL: joiner.url,
		JoinerPublicKey:   keys.PublicKey,
		JoinerDisplayName: "joiner",
		ProtocolVersions:  protocol.SupportedProtocolVersions,
	}
	bodyBytes, err := crypto.CanonicalJSON(body)
	if err != nil {
		t.Fatalf("canonical handshake body: %v", err)
	}

	ts := model.FormatUTC(time.Now().UTC())
	ver := protocol.FormatVersion(protocol.SupportedProtocolVersions[0])
	digest := transport.BodyDigest(bodyBytes)
	params := transport.SignatureParams{
		Method:          http.MethodPost,
		Path:            handshake.Path,
		InstanceURL:     joiner.url,
		Timestamp:       ts,
		Nonce:           "hs-nonce-" + nonceSeed,
		ProtocolVersion: ver,
		BodyDigest:      digest,
	}

	owner := reg.apps[ownerURL]
	if owner == nil {
		t.Fatalf("owner %q not registered", ownerURL)
	}
	req := httptest.NewRequest(http.MethodPost, ownerURL+handshake.Path, strings.NewReader(string(bodyBytes)))
	req.ContentLength = int64(len(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(transport.HeaderInstance, params.InstanceURL)
	req.Header.Set(transport.HeaderTimestamp, params.Timestamp)
	req.Header.Set(transport.HeaderNonce, params.Nonce)
	req.Header.Set(transport.HeaderProtocolVer, params.ProtocolVersion)
	req.Header.Set(transport.HeaderDigest, params.BodyDigest)
	req.Header.Set(transport.HeaderSignature, transport.SignB64(priv, params))
	resp, err := owner.Test(req, fiber.TestConfig{Timeout: -1})
	if err != nil {
		t.Fatalf("handshake test: %v", err)
	}
	return resp
}

// TestF77_HandshakeRateLimit429 asserts a peer exceeding its handshake burst is
// rejected 429 federation_rate_limited with a positive Retry-After, end-to-end
// through the real transport-signature middleware (NFR-3). With burst=2, the third
// signed handshake to the owner is throttled; the limit is keyed on the verified
// peer, runs BEFORE the invite is consulted, and carries Retry-After so the peer
// backs off. A high max_uses invite keeps every attempt invite-valid so the ONLY
// thing that changes between attempt 2 (accepted) and attempt 3 (429) is the rate
// limit.
func TestF77_HandshakeRateLimit429(t *testing.T) {
	// 60/min = 1/s, burst 2: the first two handshakes pass, the third is throttled.
	limiter := ratelimit.NewPeerLimiter(60, 2, time.Minute)
	t.Cleanup(limiter.Stop)

	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL, withHandshakeLimiter(limiter))
	joiner := newFedInstance(t, reg, joinerURL)

	// A multi-use invite so the rate limit — not invite consumption — is the only
	// thing that can reject a later attempt.
	ctx := context.Background()
	cx, err := owner.contexts.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := owner.projects.Create(ctx, repo.CreateProject{ContextID: cx.ID, Title: "Roadmap", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := owner.svc.EnableForProject(ctx, p.ID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	res, err := owner.svc.CreateInvite(ctx, p.ID, fedsvc.CreateInviteParams{Permissions: model.FederationPermissionWrite, MaxUses: 100})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	inv := fedsvc.ParsedInvite{InviteID: res.InviteID, Secret: res.Secret}

	// First two within the burst: accepted (2xx).
	for i := 0; i < 2; i++ {
		resp := signedHandshake(t, reg, joiner, ownerURL, inv, "seed-"+strconv.Itoa(i))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			t.Fatalf("handshake %d within burst: got %d (%s), want 2xx", i, resp.StatusCode, errCode(t, resp))
		}
		_ = resp.Body.Close()
	}

	// Third over the burst: 429 federation_rate_limited + Retry-After.
	resp := signedHandshake(t, reg, joiner, ownerURL, inv, "seed-over")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("handshake over burst: got %d (%s), want 429", resp.StatusCode, errCode(t, resp))
	}
	if code := errCode(t, resp); code != httpapi.CodeFederationRateLimited {
		t.Errorf("over-burst code: got %q, want %q (NFR-3)", code, httpapi.CodeFederationRateLimited)
	}
	ra := resp.Header.Get("Retry-After")
	secs, err := strconv.Atoi(ra)
	if err != nil || secs < 1 {
		t.Errorf("throttled handshake must carry a positive integer Retry-After, got %q", ra)
	}
}

// signedGet signs a GET to a CONCRETE path as the security-suite peer and runs it
// through the real transport-signature middleware. pathToSign is what the signature
// covers; concretePath is what the request line carries. When they match, the
// verifier (which rebuilds over Request().URI().Path()) accepts. When pathToSign is
// the Fiber route TEMPLATE while the request carries the concrete path, the
// signature must FAIL — proving the verifier binds the concrete path (R4 / NFR-4.3).
func (e *fedSecEnv) signedGet(t *testing.T, concretePath, pathToSign string) *http.Response {
	t.Helper()
	ts := model.FormatUTC(e.now)
	params := transport.SignatureParams{
		Method:          http.MethodGet,
		Path:            pathToSign,
		InstanceURL:     e.peerURL,
		Timestamp:       ts,
		Nonce:           "proxy-" + pathToSign,
		ProtocolVersion: "1",
		BodyDigest:      transport.BodyDigest(nil),
	}
	req := httptest.NewRequest(http.MethodGet, concretePath, http.NoBody)
	req.Header.Set(transport.HeaderInstance, params.InstanceURL)
	req.Header.Set(transport.HeaderTimestamp, params.Timestamp)
	req.Header.Set(transport.HeaderNonce, params.Nonce)
	req.Header.Set(transport.HeaderProtocolVer, params.ProtocolVersion)
	req.Header.Set(transport.HeaderDigest, params.BodyDigest)
	req.Header.Set(transport.HeaderSignature, transport.SignB64(e.peerPriv, params))
	resp, err := e.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("signed get: %v", err)
	}
	return resp
}

// TestF77_SignatureBindsConcretePathBehindProxy asserts the transport signature is
// verified over the CONCRETE request path (Request().URI().Path()), NOT the Fiber
// route template — the property that keeps signature verification correct behind a
// reverse proxy that rewrites the path (NFR-4.3 / R4). The pull route
// /federation/projects/:id/events is parameterized, so the concrete path
// (/federation/projects/<id>/events) differs from the template
// (/federation/projects/:id/events). Signing over the concrete path → accepted;
// signing over the template → 401, proving the verifier never used the template.
func TestF77_SignatureBindsConcretePathBehindProxy(t *testing.T) {
	env := newFedSecEnv(t)
	concrete := "/federation/projects/" + strconv.FormatInt(env.localProject, 10) + "/events"
	const template = "/federation/projects/:id/events"

	// Signed over the CONCRETE path → the verifier (using Request().URI().Path())
	// agrees, so the request passes the signature plane (a 2xx pull, not a 401).
	ok := env.signedGet(t, concrete, concrete)
	defer func() { _ = ok.Body.Close() }()
	if ok.StatusCode == http.StatusUnauthorized {
		t.Fatalf("signing over the concrete path must verify behind a proxy: got 401 (%s)", errCode(t, ok))
	}

	// Signed over the route TEMPLATE while the request carries the concrete path →
	// the verifier rebuilds over the concrete path, the signatures differ, 401. If
	// the middleware ever switched to c.Path() (the template), THIS would wrongly
	// pass and concrete-path signing would fail — the regression this guards.
	bad := env.signedGet(t, concrete, template)
	defer func() { _ = bad.Body.Close() }()
	if bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signing over the route TEMPLATE must be rejected (verifier binds the concrete path): got %d (%s)", bad.StatusCode, errCode(t, bad))
	}
	if code := errCode(t, bad); code != httpapi.CodeFederationSignatureInvalid {
		t.Errorf("template-signed request code: got %q, want %q", code, httpapi.CodeFederationSignatureInvalid)
	}
}

// recordText renders a slog.Record (message + every attribute key/value) to a
// single string so a substring search finds a leaked secret regardless of which
// field it landed in.
func recordText(r slog.Record) string {
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		b.WriteByte(' ')
		b.WriteString(a.Key)
		b.WriteByte('=')
		b.WriteString(a.Value.String())
		return true
	})
	return b.String()
}

// TestF77_NoInviteSecretInLogs asserts the invite secret never reaches any log
// record on either the owner or the joiner side of the handshake, across BOTH a
// successful join and a wrong-secret rejection (NFR-3 / US-1.2 AC6). The secret is
// minted on the owner; both instances run with a capture logger; every record they
// emit (message + attributes) is scanned for the plaintext secret. The secret
// leaves the process only in the join request body / response, never the logs.
func TestF77_NoInviteSecretInLogs(t *testing.T) {
	ownerCap := newCaptureHandler()
	joinerCap := newCaptureHandler()

	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL, withLogger(slog.New(ownerCap)))
	joiner := newFedInstance(t, reg, joinerURL, withLogger(slog.New(joinerCap)))

	inv, _ := owner.enableAndInvite(t, model.FederationPermissionWrite)

	// A successful join (logs an accepted handshake on the owner + a join mutation
	// on the joiner).
	resp, body := joiner.join(t, ownerURL, inv)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("join: got %d, want 200; %s", resp.StatusCode, body)
	}

	// A wrong-secret join (logs a validation rejection on the owner). The owner must
	// reject WITHOUT echoing the presented secret.
	bad := fedsvc.ParsedInvite{InviteID: inv.InviteID, Secret: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"}
	respBad, _ := joiner.join(t, ownerURL, bad)
	if respBad.StatusCode == http.StatusOK {
		t.Fatalf("wrong-secret join unexpectedly succeeded")
	}

	for name, cap := range map[string]*captureHandler{"owner": ownerCap, "joiner": joinerCap} {
		for _, r := range cap.snapshot() {
			line := recordText(r)
			if strings.Contains(line, inv.Secret) {
				t.Errorf("%s: real invite secret leaked into a log record: %q", name, line)
			}
			if strings.Contains(line, bad.Secret) {
				t.Errorf("%s: presented (wrong) secret leaked into a log record: %q", name, line)
			}
		}
	}
}

// TestF77_HandshakeRateLimitIsolatedPerPeer asserts one peer being throttled does
// NOT throttle a different peer's handshake — the bucket is keyed on the verified
// instance_url, so a noisy joiner cannot deny a healthy one (NFR-3 isolation).
func TestF77_HandshakeRateLimitIsolatedPerPeer(t *testing.T) {
	// rate=1/min (NOT 60/min): with burst=1 the single token refills only once per
	// 60s, so Bob's second handshake cannot be re-allowed by an in-test token refill
	// even if the goroutine is descheduled for several seconds under full-suite
	// parallel load. (At 60/min = 1 token/sec a >1s deschedule between Bob's two
	// handshakes refilled his bucket and flaked the 429 assertion.) The test asserts
	// burst exhaustion + per-peer isolation; the exact steady rate is irrelevant to
	// both, so the slower refill only makes the assertions deterministic.
	limiter := ratelimit.NewPeerLimiter(1, 1, time.Minute)
	t.Cleanup(limiter.Stop)

	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL, withHandshakeLimiter(limiter))
	bob := newFedInstance(t, reg, joinerURL)
	const carolURL = "https://carol.example"
	carol := newFedInstance(t, reg, carolURL)

	ctx := context.Background()
	cx, _ := owner.contexts.Create(ctx, "Work", "blue", false)
	p, _ := owner.projects.Create(ctx, repo.CreateProject{ContextID: cx.ID, Title: "Roadmap", Color: "blue"})
	if _, err := owner.svc.EnableForProject(ctx, p.ID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	res, _ := owner.svc.CreateInvite(ctx, p.ID, fedsvc.CreateInviteParams{Permissions: model.FederationPermissionWrite, MaxUses: 100})
	inv := fedsvc.ParsedInvite{InviteID: res.InviteID, Secret: res.Secret}

	// Bob exhausts his burst (1) then is throttled on his second handshake.
	first := signedHandshake(t, reg, bob, ownerURL, inv, "bob-1")
	if first.StatusCode < 200 || first.StatusCode >= 300 {
		t.Fatalf("bob first handshake: got %d (%s), want 2xx", first.StatusCode, errCode(t, first))
	}
	_ = first.Body.Close()
	throttled := signedHandshake(t, reg, bob, ownerURL, inv, "bob-2")
	if throttled.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("bob over burst: got %d, want 429", throttled.StatusCode)
	}
	_ = throttled.Body.Close()

	// Carol has her own untouched bucket: her first handshake is accepted.
	carolResp := signedHandshake(t, reg, carol, ownerURL, inv, "carol-1")
	defer func() { _ = carolResp.Body.Close() }()
	if carolResp.StatusCode < 200 || carolResp.StatusCode >= 300 {
		t.Errorf("carol should be allowed despite bob being throttled: got %d (%s)", carolResp.StatusCode, errCode(t, carolResp))
	}
}
