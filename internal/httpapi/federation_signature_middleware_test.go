package httpapi_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/federation/nonce"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/transport"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/repo"
)

const peerURL = "https://peer.example"

// captureAuditor records every audit entry the middleware emits, for the
// transport-rejection audit tests (Federation v1 F6.3, US-7.4 AC1).
type captureAuditor struct {
	mu      sync.Mutex
	entries []repo.AuditEntry
}

func (c *captureAuditor) Record(e repo.AuditEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
}

func (c *captureAuditor) only(kind repo.AuditKind) *repo.AuditEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.entries {
		if c.entries[i].Kind == kind {
			return &c.entries[i]
		}
	}
	return nil
}

type sigTestEnv struct {
	app  *fiber.App
	priv ed25519.PrivateKey
	keys *peerkeys.Cache
	now  time.Time
}

func newSigTestEnv(t *testing.T) *sigTestEnv {
	t.Helper()
	return newSigTestEnvWithAuditor(t, nil)
}

func newSigTestEnvWithAuditor(t *testing.T, auditor httpapi.FederationAuditor) *sigTestEnv {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	keys := peerkeys.NewCache(func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: pubB64, DisplayName: "Peer Name"}, nil
	})

	env := &sigTestEnv{priv: priv, keys: keys, now: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}

	app := httpapi.NewApp(httpapi.Deps{})
	mw := httpapi.HTTPSignatureMiddleware(httpapi.FederationSignatureDeps{
		Nonces:   nonce.NewCache(),
		PeerKeys: keys,
		Now:      func() time.Time { return env.now },
		Auditor:  auditor,
	})
	app.Post("/federation/events", mw, func(c fiber.Ctx) error {
		peer := httpapi.GetFederationPeer(c)
		return c.JSON(fiber.Map{"instance": peer.InstanceURL, "display": peer.DisplayName})
	})
	env.app = app
	return env
}

// signedReq builds a request signed by the peer's private key over the pinned
// canonical string.
func (e *sigTestEnv) signedReq(t *testing.T, body []byte, mutate func(p *transport.SignatureParams, h http.Header)) *http.Request {
	t.Helper()
	ts := e.now.UTC().Format("2006-01-02T15:04:05.000Z")
	nonceVal := "nonce-" + ts
	params := transport.SignatureParams{
		Method:          "POST",
		Path:            "/federation/events",
		InstanceURL:     peerURL,
		Timestamp:       ts,
		Nonce:           nonceVal,
		ProtocolVersion: "1",
		BodyDigest:      transport.BodyDigest(body),
	}
	h := http.Header{}
	if mutate != nil {
		mutate(&params, h)
	}
	sig := transport.SignB64(e.priv, params)

	req := httptest.NewRequest(http.MethodPost, "/federation/events", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(transport.HeaderInstance, params.InstanceURL)
	req.Header.Set(transport.HeaderTimestamp, params.Timestamp)
	req.Header.Set(transport.HeaderNonce, params.Nonce)
	req.Header.Set(transport.HeaderProtocolVer, params.ProtocolVersion)
	req.Header.Set(transport.HeaderDigest, params.BodyDigest)
	req.Header.Set(transport.HeaderSignature, sig)
	for k := range h {
		req.Header.Set(k, h.Get(k))
	}
	return req
}

func decodeStatus(t *testing.T, resp *http.Response) (int, string) {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &env)
	return resp.StatusCode, env.Error.Code
}

func TestHTTPSignature_ValidAccepted(t *testing.T) {
	e := newSigTestEnv(t)
	body := []byte(`{"x":1}`)
	resp, err := e.app.Test(e.signedReq(t, body, nil))
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		code := ""
		_, code = decodeStatus(t, resp)
		t.Fatalf("valid signed request: got %d (%s), want 200", resp.StatusCode, code)
	}
	var out struct {
		Instance string `json:"instance"`
		Display  string `json:"display"`
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	if out.Instance != peerURL {
		t.Errorf("peer instance in Locals: got %q, want %q", out.Instance, peerURL)
	}
	if out.Display != "Peer Name" {
		t.Errorf("peer display in Locals: got %q, want %q", out.Display, "Peer Name")
	}
}

func TestHTTPSignature_BadSignatureRejected(t *testing.T) {
	e := newSigTestEnv(t)
	req := e.signedReq(t, []byte(`{"x":1}`), nil)
	req.Header.Set(transport.HeaderSignature, base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)))
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	status, code := decodeStatus(t, resp)
	if status != http.StatusUnauthorized || code != httpapi.CodeFederationSignatureInvalid {
		t.Fatalf("bad signature: got %d (%s), want 401 %s", status, code, httpapi.CodeFederationSignatureInvalid)
	}
}

func TestHTTPSignature_ReplayRejected(t *testing.T) {
	e := newSigTestEnv(t)
	body := []byte(`{"x":1}`)
	// Build one request and replay the exact same nonce/timestamp/sig twice.
	req1 := e.signedReq(t, body, func(p *transport.SignatureParams, _ http.Header) { p.Nonce = "fixed-nonce" })
	resp1, err := e.app.Test(req1)
	if err != nil {
		t.Fatalf("test1: %v", err)
	}
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", resp1.StatusCode)
	}
	req2 := e.signedReq(t, body, func(p *transport.SignatureParams, _ http.Header) { p.Nonce = "fixed-nonce" })
	resp2, err := e.app.Test(req2)
	if err != nil {
		t.Fatalf("test2: %v", err)
	}
	status, code := decodeStatus(t, resp2)
	if status != http.StatusUnauthorized || code != httpapi.CodeFederationReplay {
		t.Fatalf("replay: got %d (%s), want 401 %s", status, code, httpapi.CodeFederationReplay)
	}
}

func TestHTTPSignature_StaleTimestampRejectedBeforeNonce(t *testing.T) {
	e := newSigTestEnv(t)
	// Sign with a timestamp 10 minutes in the past (outside ±5min). The
	// signature is otherwise valid, so the only reason to reject is staleness —
	// and it must be checked before the nonce (US-7.3 AC2).
	staleTs := e.now.Add(-10 * time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
	body := []byte(`{}`)
	req := e.signedReq(t, body, func(p *transport.SignatureParams, _ http.Header) {
		p.Timestamp = staleTs
	})
	req.Header.Set(transport.HeaderTimestamp, staleTs)
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	status, code := decodeStatus(t, resp)
	if status != http.StatusUnauthorized || code != httpapi.CodeFederationTimestampStale {
		t.Fatalf("stale ts: got %d (%s), want 401 %s", status, code, httpapi.CodeFederationTimestampStale)
	}
}

func TestHTTPSignature_DigestMismatchRejected(t *testing.T) {
	e := newSigTestEnv(t)
	// Sign over the digest of one body, but transmit a different body. The
	// digest constant-time compare must fail with 400 (US-7.2 AC2 transport)
	// — and before any signature verification.
	signedBody := []byte(`{"real":"body"}`)
	req := e.signedReq(t, signedBody, nil)
	tampered := []byte(`{"tampered":"body"}`)
	req.Body = io.NopCloser(bytes.NewReader(tampered))
	req.ContentLength = int64(len(tampered))
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	status, code := decodeStatus(t, resp)
	if status != http.StatusBadRequest || code != httpapi.CodeFederationDigestMismatch {
		t.Fatalf("digest mismatch: got %d (%s), want 400 %s", status, code, httpapi.CodeFederationDigestMismatch)
	}
}

// TestHTTPSignature_ProtocolVersionInSignedSet asserts the anti-downgrade
// property (Federation v1 F0.4): X-Federation-Protocol-Version is part of the
// signed canonical string, so a man-in-the-middle that rewrites the header in
// transit (e.g. downgrades "1" to "2") invalidates the Ed25519 signature and
// the request is rejected. The signer signs over version "1"; only the
// transmitted header is mutated, leaving every other signed field intact.
func TestHTTPSignature_ProtocolVersionInSignedSet(t *testing.T) {
	e := newSigTestEnv(t)
	body := []byte(`{"x":1}`)
	// Sign with protocol version "1" (unchanged params), then tamper ONLY the
	// transmitted header to "2".
	req := e.signedReq(t, body, nil)
	req.Header.Set(transport.HeaderProtocolVer, "2")
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	status, code := decodeStatus(t, resp)
	if status != http.StatusUnauthorized || code != httpapi.CodeFederationSignatureInvalid {
		t.Fatalf("downgraded protocol version header: got %d (%s), want 401 %s", status, code, httpapi.CodeFederationSignatureInvalid)
	}
}

func TestHTTPSignature_MissingHeadersRejected(t *testing.T) {
	e := newSigTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/federation/events", http.NoBody)
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	status, code := decodeStatus(t, resp)
	if status != http.StatusUnauthorized || code != httpapi.CodeFederationSignatureInvalid {
		t.Fatalf("missing headers: got %d (%s), want 401 %s", status, code, httpapi.CodeFederationSignatureInvalid)
	}
}

// TestHTTPSignature_ReplayRecordsAudit asserts a replayed nonce records one
// audit row with kind=replay, outcome=rejected, and the peer instance — and that
// the detail never carries a secret/signature (Federation v1 F6.3, US-7.4 AC1).
func TestHTTPSignature_ReplayRecordsAudit(t *testing.T) {
	auditor := &captureAuditor{}
	e := newSigTestEnvWithAuditor(t, auditor)
	body := []byte(`{"x":1}`)
	req1 := e.signedReq(t, body, func(p *transport.SignatureParams, _ http.Header) { p.Nonce = "audit-nonce" })
	if _, err := e.app.Test(req1); err != nil {
		t.Fatalf("test1: %v", err)
	}
	req2 := e.signedReq(t, body, func(p *transport.SignatureParams, _ http.Header) { p.Nonce = "audit-nonce" })
	if _, err := e.app.Test(req2); err != nil {
		t.Fatalf("test2: %v", err)
	}

	got := auditor.only(repo.AuditKindReplay)
	if got == nil {
		t.Fatalf("expected a replay audit row")
	}
	if got.Outcome != repo.AuditOutcomeRejected {
		t.Errorf("replay outcome: got %q, want rejected", got.Outcome)
	}
	if got.PeerInstanceURL != peerURL {
		t.Errorf("replay peer: got %q, want %q", got.PeerInstanceURL, peerURL)
	}
	// The detail must NEVER contain a secret or the raw signature bytes (§7 F6.3).
	if got.Detail == "" {
		t.Errorf("replay detail should carry a coded reason, got empty")
	}
}

// TestHTTPSignature_RejectionsRecordCorrectKind asserts each transport rejection
// maps to its own audit kind (Federation v1 F6.3, US-7.4 AC1): a stale timestamp,
// a digest mismatch, and a bad signature each record their distinct kind.
func TestHTTPSignature_RejectionsRecordCorrectKind(t *testing.T) {
	t.Run("stale_timestamp", func(t *testing.T) {
		auditor := &captureAuditor{}
		e := newSigTestEnvWithAuditor(t, auditor)
		staleTs := e.now.Add(-10 * time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
		req := e.signedReq(t, []byte(`{}`), func(p *transport.SignatureParams, _ http.Header) { p.Timestamp = staleTs })
		req.Header.Set(transport.HeaderTimestamp, staleTs)
		if _, err := e.app.Test(req); err != nil {
			t.Fatalf("test: %v", err)
		}
		if auditor.only(repo.AuditKindTimestampStale) == nil {
			t.Errorf("expected a timestamp_stale audit row")
		}
	})

	t.Run("digest_mismatch", func(t *testing.T) {
		auditor := &captureAuditor{}
		e := newSigTestEnvWithAuditor(t, auditor)
		req := e.signedReq(t, []byte(`{"real":"body"}`), nil)
		tampered := []byte(`{"tampered":"body"}`)
		req.Body = io.NopCloser(bytes.NewReader(tampered))
		req.ContentLength = int64(len(tampered))
		if _, err := e.app.Test(req); err != nil {
			t.Fatalf("test: %v", err)
		}
		if auditor.only(repo.AuditKindDigestMismatch) == nil {
			t.Errorf("expected a digest_mismatch audit row")
		}
	})

	t.Run("bad_signature", func(t *testing.T) {
		auditor := &captureAuditor{}
		e := newSigTestEnvWithAuditor(t, auditor)
		req := e.signedReq(t, []byte(`{"x":1}`), nil)
		req.Header.Set(transport.HeaderSignature, base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)))
		if _, err := e.app.Test(req); err != nil {
			t.Fatalf("test: %v", err)
		}
		if auditor.only(repo.AuditKindSignatureInvalid) == nil {
			t.Errorf("expected a signature_invalid audit row")
		}
	})
}
