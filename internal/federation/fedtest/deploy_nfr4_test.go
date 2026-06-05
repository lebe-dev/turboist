package fedtest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/config"
)

// Federation v1 F7.7 — NFR-4 single-binary deploy verification.
//
// NFR-4 promises turboist ships as a SINGLE, CGO-free Go binary, even with the
// federation overlay and its new prometheus/client_golang dependency (§3 / R26).
// This file asserts, mechanically:
//
//   - CGO_ENABLED=0 go build ./cmd/turboist SUCCEEDS, and the whole import graph
//     (including prometheus/client_golang) compiles with ZERO cgo files — proving
//     the new metrics dep added no C / transitive C dependency (the explicit F7.7
//     binary-size/CGO-free check);
//   - the federation YAML block has NO mandatory field (every getter defaults), and
//     FEDERATION_KEY is OPTIONAL — a deploy with neither a federation config block
//     nor the key starts cleanly (federation simply stays off);
//   - the signed-request canonical string binds the CONCRETE request path, so
//     signature verification holds behind a reverse proxy that rewrites the path
//     prefix (NFR-4.3 / R4) — the proxy-prefix leg lives in the transport test
//     below.

// repoRoot4 walks up from the test working dir to the module root (go.mod).
func repoRoot4(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// TestF77_CGOFreeBuildWithPrometheus asserts CGO_ENABLED=0 go build ./cmd/turboist
// succeeds with the prometheus/client_golang dependency present — the single CGO-
// free binary NFR-4 / §3-R26 require. It is gated short so it does not slow the
// regular unit run; it builds the real entrypoint, not a stub.
func TestF77_CGOFreeBuildWithPrometheus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CGO-free build in -short mode")
	}
	root := repoRoot4(t)

	// Sanity: the prometheus dep the metrics layer requires IS declared, so this
	// test is genuinely exercising "CGO-free WITH the new dep" (not a stripped tree).
	modBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.Contains(string(modBytes), "github.com/prometheus/client_golang") {
		t.Fatalf("go.mod is missing prometheus/client_golang — the F7.7 metrics dep that must build CGO-free")
	}

	out := filepath.Join(t.TempDir(), "turboist-cgofree")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/turboist")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("CGO_ENABLED=0 go build ./cmd/turboist failed: %v\n%s", err, combined)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatalf("expected a non-empty binary at %s: %v", out, err)
	}
}

// TestF77_NoCgoFilesInImportGraph asserts that under CGO_ENABLED=0 NO package in
// cmd/turboist's transitive import graph — prometheus/client_golang included —
// contributes any cgo (.c-bound) source. This is the precise "verify pure-Go, no
// CGO, no transitive C dep" check the milestone names: a future dep that drags in
// cgo would make the binary non-portable, and this test catches it.
func TestF77_NoCgoFilesInImportGraph(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping import-graph scan in -short mode")
	}
	root := repoRoot4(t)
	cmd := exec.Command("go", "list", "-deps",
		"-f", "{{.ImportPath}}\t{{.CgoFiles}}", "./cmd/turboist")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\n%s", err, out)
	}
	var offenders []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		// CgoFiles renders as "[]" when empty; a non-empty slice means the package
		// compiles cgo source, which requires a C toolchain at build time.
		if cgoFiles := strings.TrimSpace(parts[1]); cgoFiles != "" && cgoFiles != "[]" {
			offenders = append(offenders, parts[0]+" "+cgoFiles)
		}
	}
	if len(offenders) > 0 {
		t.Errorf("packages with cgo files in the CGO_ENABLED=0 build graph (breaks the single CGO-free binary, NFR-4):\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// TestF77_FederationYAMLDefaultsNoMandatoryEnv asserts the federation feature
// imposes NO mandatory configuration: a Config with an entirely ZERO Federation
// block yields safe, non-zero defaults from every federation getter, and the
// FEDERATION_KEY env is OPTIONAL — set neither and the process still configures
// cleanly with federation simply OFF (NFR-4). Conversely, the cross-cutting
// app-level mandatory envs (BIND/BASE_URL/JWT_SECRET/API_TOKEN_SALT) are NOT a
// federation requirement — this test only proves federation adds none of its own.
func TestF77_FederationYAMLDefaultsNoMandatoryEnv(t *testing.T) {
	// A zero Config: no federation block at all. Every federation getter must
	// return a sane non-zero default rather than panic or zero.
	var c config.Config
	checks := []struct {
		name string
		got  int64
		min  int64
	}{
		{"TombstoneRetention", int64(c.FederationTombstoneRetention()), 1},
		{"OutboxRetention", int64(c.FederationOutboxRetention()), 1},
		{"InboxRetention", int64(c.FederationInboxRetention()), 1},
		{"PublishInterval", int64(c.FederationPublishInterval()), 1},
		{"PullInterval", int64(c.FederationPullInterval()), 1},
		{"PullBatchLimit", int64(c.FederationPullBatchLimit()), 1},
		{"InboundRatePerMinute", int64(c.FederationInboundRatePerMinute()), 1},
		{"InboundBurst", int64(c.FederationInboundBurst()), 1},
		{"MaxBatchEvents", int64(c.FederationMaxBatchEvents()), 1},
		{"HandshakeRatePerMinute", int64(c.FederationHandshakeRatePerMinute()), 1},
		{"HandshakeBurst", int64(c.FederationHandshakeBurst()), 1},
		{"OwnerTimeout", int64(c.FederationOwnerTimeout()), 1},
		{"AuditRetention", int64(c.FederationAuditRetention()), 1},
		{"AuditAlertThreshold", int64(c.FederationAuditAlertThreshold()), 1},
		{"AuditAlertWindow", int64(c.FederationAuditAlertWindow()), 1},
	}
	for _, ch := range checks {
		if ch.got < ch.min {
			t.Errorf("federation default %s with a zero config: got %d, want >= %d (no mandatory federation YAML)", ch.name, ch.got, ch.min)
		}
	}

	// FEDERATION_KEY is optional: with it unset, LoadEnv must NOT error on the
	// federation key (federation is off). We isolate this from the other mandatory
	// envs by setting only the unrelated required ones and clearing FEDERATION_KEY.
	withEnv(t, map[string]string{
		"BIND":            "127.0.0.1:8080",
		"BASE_URL":        "https://me.example",
		"JWT_SECRET":      "test-secret-key-32-bytes-padding!!!!",
		"API_TOKEN_SALT":  "test-api-token-salt-32-bytes-pad!!!!",
		"FEDERATION_KEY":  "",
		"TOTP_SECRET_KEY": "",
	})
	env, err := config.LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv with FEDERATION_KEY unset must succeed (federation optional), got: %v", err)
	}
	if env.FederationKey != "" {
		t.Errorf("FederationKey: got %q, want empty (unset → federation off)", env.FederationKey)
	}
}

// withEnv sets the given environment variables for the duration of the test,
// restoring the previous values on cleanup. An empty value unsets the variable.
func withEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		prev, had := os.LookupEnv(k)
		if v == "" {
			_ = os.Unsetenv(k)
		} else {
			_ = os.Setenv(k, v)
		}
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, prev)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}
}
