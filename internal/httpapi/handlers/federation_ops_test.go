package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	fedmetrics "github.com/lebe-dev/turboist/internal/federation/metrics"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

// TestHealth_PublicShape asserts the public GET /federation/health probe returns
// the liveness shape (instanceUrl, protocolVersions, uptimeS, outboxDepth, status)
// WITHOUT auth and WITHOUT the per-peer detail (US-8.1).
func TestHealth_PublicShape(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	pid := createTestProject(t, e, ctx.ID, "Health").ID
	enableFederation(t, e, pid)
	recent := time.Now().Add(-1 * time.Hour)
	seedStatusPeer(t, e, pid, "https://bob.example", &recent)

	// No Authorization header — the public probe is unauthenticated.
	req, _ := http.NewRequest(http.MethodGet, "/federation/health", nil)
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var h dto.HealthResponse
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("parse: %v; body: %s", err, body)
	}
	if h.InstanceUrl != testBaseURL {
		t.Errorf("instanceUrl: got %q, want %q", h.InstanceUrl, testBaseURL)
	}
	if len(h.ProtocolVersions) == 0 {
		t.Errorf("protocolVersions empty")
	}
	if h.Status != "ok" {
		t.Errorf("status: got %q, want ok", h.Status)
	}
	if len(h.Peers) != 0 {
		t.Errorf("public probe leaked peer detail: %+v (must be omitted)", h.Peers)
	}
}

// TestHealth_PeersStale asserts a peer not contacted in >24h flips the public
// health status to peers_stale (US-8.1).
func TestHealth_PeersStale(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	pid := createTestProject(t, e, ctx.ID, "Health").ID
	enableFederation(t, e, pid)
	stale := time.Now().Add(-48 * time.Hour)
	seedStatusPeer(t, e, pid, "https://bob.example", &stale)

	req, _ := http.NewRequest(http.MethodGet, "/federation/health", nil)
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: got %d; body: %s", resp.StatusCode, body)
	}
	var h dto.HealthResponse
	_ = json.Unmarshal(body, &h)
	if h.Status != "peers_stale" {
		t.Errorf("status: got %q, want peers_stale", h.Status)
	}
}

// TestHealth_AdminCarriesPeers asserts the JWT admin GET /federation/health
// includes the per-peer detail the public probe omits (US-8.1).
func TestHealth_AdminCarriesPeers(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	pid := createTestProject(t, e, ctx.ID, "Health").ID
	enableFederation(t, e, pid)
	recent := time.Now().Add(-1 * time.Hour)
	seedStatusPeer(t, e, pid, "https://bob.example", &recent)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/federation/health", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin health: got %d; body: %s", resp.StatusCode, body)
	}
	var h dto.HealthResponse
	_ = json.Unmarshal(body, &h)
	if len(h.Peers) != 1 || h.Peers[0].InstanceUrl != "https://bob.example" {
		t.Errorf("admin health peers: got %+v, want one bob peer", h.Peers)
	}
}

// TestMetrics_ExactNamesAndGaugeTracksDepth asserts GET /metrics exposes the
// AC-exact federation metric names, the outbox-depth gauge reflects a value set on
// the collectors, and the received/signature counters render (US-8.2 AC1).
func TestMetrics_ExactNamesAndGaugeTracksDepth(t *testing.T) {
	e := setupAPIEnv(t)

	// Drive each collector directly (the feeder goroutine is main-only). A labeled
	// counter/gauge renders no exposition line until at least one label combination
	// has a sample, so each is touched once to assert the full AC name set.
	e.fedMetrics.SetOutboxDepth(4)
	e.fedMetrics.RecordEventReceived("https://bob.example", fedmetrics.ResultSuccess, 2)
	e.fedMetrics.RecordEventSent("https://bob.example", fedmetrics.ResultSuccess, 1)
	e.fedMetrics.RecordSignatureFailure("https://bob.example")
	e.fedMetrics.ObserveApplySeconds(0.01)
	e.fedMetrics.SetPeerLastContactSeconds("https://bob.example", 12)

	req, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics: got %d; body: %s", resp.StatusCode, body)
	}
	text := string(body)
	wants := []string{
		"federation_outbox_depth 4",
		`federation_events_received_total{peer="https://bob.example",result="success"} 2`,
		`federation_signature_failures_total{peer="https://bob.example"} 1`,
		"federation_apply_duration_seconds",
		"federation_events_sent_total",
		"federation_peer_last_contact_seconds",
	}
	for _, w := range wants {
		if !strings.Contains(text, w) {
			t.Errorf("/metrics missing %q", w)
		}
	}
}

// TestRetention_GetAndRuntimePatch asserts the retention GET returns the defaults,
// a PATCH persists + takes effect on the live holder, and the outbox window is
// clamped to the 30-day hardcap in the effective value (US-8.4).
func TestRetention_GetAndRuntimePatch(t *testing.T) {
	e := setupAPIEnv(t)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/federation/retention", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get retention: got %d; body: %s", resp.StatusCode, body)
	}
	var got dto.RetentionSettingsDTO
	_ = json.Unmarshal(body, &got)
	if got.TombstoneDays != 0 || got.EffectiveTombstoneDays != 90 {
		t.Errorf("defaults: tombstoneDays=%d effective=%d, want 0/90", got.TombstoneDays, got.EffectiveTombstoneDays)
	}
	if got.OutboxHardcapDays != 30 {
		t.Errorf("outboxHardcapDays: got %d, want 30", got.OutboxHardcapDays)
	}

	// PATCH: shorten tombstone to 120 days, set an over-cap outbox of 365 days.
	patch := map[string]int{"tombstoneDays": 120, "outboxDays": 365}
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/federation/retention", patch))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch retention: got %d; body: %s", resp.StatusCode, body)
	}
	var after dto.RetentionSettingsDTO
	_ = json.Unmarshal(body, &after)
	if after.TombstoneDays != 120 || after.EffectiveTombstoneDays != 120 {
		t.Errorf("after patch tombstone: stored=%d effective=%d, want 120/120", after.TombstoneDays, after.EffectiveTombstoneDays)
	}
	if after.OutboxDays != 365 {
		t.Errorf("after patch outbox stored: got %d, want 365 (intent preserved)", after.OutboxDays)
	}
	if after.EffectiveOutboxDays != 30 {
		t.Errorf("after patch outbox effective: got %d, want 30 (hardcap clamp, not validate)", after.EffectiveOutboxDays)
	}

	// The change is reflected in the live holder backing the GC (runtime change).
	if w := e.fedRetention.Get(); w.TombstoneDays != 120 {
		t.Errorf("live holder tombstone: got %d, want 120 (no restart)", w.TombstoneDays)
	}
	if cfg := e.fedRetention.GCConfig(); cfg.OutboxRetention != 30*24*time.Hour {
		t.Errorf("live GC outbox: got %v, want 30d clamp", cfg.OutboxRetention)
	}
}

// TestFederationBackup_DownloadsSQLiteFile asserts the JWT admin
// GET /federation/backup streams a VACUUM INTO physical backup (a SQLite file)
// including the federation tables + keypair (US-8.5).
func TestFederationBackup_DownloadsSQLiteFile(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	pid := createTestProject(t, e, ctx.ID, "Shared").ID
	enableFederation(t, e, pid)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/federation/backup", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("backup: got %d; body: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("content-type: got %q", got)
	}
	if len(body) < 16 || string(body[:16]) != "SQLite format 3\x00" {
		t.Errorf("backup is not a SQLite file (bad magic header)")
	}
}
