package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validYAML = `
timezone: "Europe/Moscow"
max-pinned: 5
weekly:
  limit: 30
backlog:
  limit: 30
inbox:
  warn-threshold: 10
  overflow-task:
    title: "Разобрать Входящие"
    priority: "medium"
day-parts:
  morning:
    start: 9
    end: 13
  afternoon:
    start: 13
    end: 17
  evening:
    start: 17
    end: 22
`

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestLoad_Valid(t *testing.T) {
	p := writeConfig(t, validYAML)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Location == nil || cfg.Location.String() != "Europe/Moscow" {
		t.Fatalf("location not loaded: %+v", cfg.Location)
	}
	if cfg.Weekly.Limit != 30 || cfg.Backlog.Limit != 30 || cfg.MaxPinned != 5 {
		t.Fatalf("limits not parsed: %+v", cfg)
	}
}

func TestLoad_OverlappingDayParts(t *testing.T) {
	body := strings.Replace(validYAML,
		"  afternoon:\n    start: 13\n    end: 17",
		"  afternoon:\n    start: 12\n    end: 17", 1)
	p := writeConfig(t, body)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("expected overlap error, got %v", err)
	}
}

func TestLoad_BadTimezone(t *testing.T) {
	body := strings.Replace(validYAML, `timezone: "Europe/Moscow"`, `timezone: "Mars/Phobos"`, 1)
	p := writeConfig(t, body)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "timezone") {
		t.Fatalf("expected timezone error, got %v", err)
	}
}

func TestLoad_BadPriority(t *testing.T) {
	body := strings.Replace(validYAML, `priority: "medium"`, `priority: "urgent"`, 1)
	p := writeConfig(t, body)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "priority") {
		t.Fatalf("expected priority error, got %v", err)
	}
}

func TestLoad_BadDayPartRange(t *testing.T) {
	body := strings.Replace(validYAML,
		"  evening:\n    start: 17\n    end: 22",
		"  evening:\n    start: 17\n    end: 25", 1)
	p := writeConfig(t, body)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "[0,24]") {
		t.Fatalf("expected range error, got %v", err)
	}
}

func TestLoad_NonPositiveLimit(t *testing.T) {
	body := strings.Replace(validYAML, "  limit: 30\nbacklog:", "  limit: 0\nbacklog:", 1)
	p := writeConfig(t, body)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "weekly.limit") {
		t.Fatalf("expected weekly.limit error, got %v", err)
	}
}

func TestLoadEnv_MissingRequired(t *testing.T) {
	t.Setenv("BIND", "")
	t.Setenv("BASE_URL", "")
	t.Setenv("JWT_SECRET", "")
	if _, err := LoadEnv(); err == nil {
		t.Fatalf("expected error for missing BIND")
	}
}

func TestLoadEnv_OK(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
	t.Setenv("LOG_LEVEL", "")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if e.LogLevel != "info" {
		t.Fatalf("default LOG_LEVEL must be info, got %q", e.LogLevel)
	}
}

func TestLoadEnv_JWTSecretTooShort(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "short")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("expected JWT_SECRET length error, got %v", err)
	}
}

func TestLoadEnv_APITokenSaltMissing(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "API_TOKEN_SALT is required") {
		t.Fatalf("expected API_TOKEN_SALT required error, got %v", err)
	}
}

func TestLoadEnv_TOTPSecretKeyEmptyIsOK(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
	t.Setenv("TOTP_SECRET_KEY", "")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if e.TOTPSecretKey != "" {
		t.Fatalf("TOTPSecretKey: got %q, want empty", e.TOTPSecretKey)
	}
}

func TestLoadEnv_TOTPSecretKeyTooShort(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
	t.Setenv("TOTP_SECRET_KEY", "tooshort")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "TOTP_SECRET_KEY") {
		t.Fatalf("expected TOTP_SECRET_KEY length error, got %v", err)
	}
}

func TestLoadEnv_TOTPSecretKeyOK(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
	t.Setenv("TOTP_SECRET_KEY", "totp-totp-totp-totp-totp-totp-totp")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got := e.TOTPSecretKey; len(got) < 32 {
		t.Fatalf("TOTPSecretKey: got %q (len %d), want ≥32 bytes", got, len(got))
	}
}

func TestLoadEnv_APITokenSaltTooShort(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "short")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("expected API_TOKEN_SALT length error, got %v", err)
	}
}

func TestLoadEnv_FederationKeyEmptyIsOK(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
	t.Setenv("FEDERATION_KEY", "")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if e.FederationKey != "" {
		t.Fatalf("FederationKey: got %q, want empty", e.FederationKey)
	}
}

func TestLoadEnv_FederationKeyTooShort(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
	t.Setenv("FEDERATION_KEY", "tooshort")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "FEDERATION_KEY") {
		t.Fatalf("expected FEDERATION_KEY length error, got %v", err)
	}
}

func TestLoadEnv_FederationKeyOK(t *testing.T) {
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
	t.Setenv("FEDERATION_KEY", "federation-federation-federation-key")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got := e.FederationKey; len(got) < 32 {
		t.Fatalf("FederationKey: got %q (len %d), want ≥32 bytes", got, len(got))
	}
}

func TestFederationPullInterval_DefaultAndOverride(t *testing.T) {
	var c Config
	if got := c.FederationPullInterval(); got.Seconds() != 60 {
		t.Errorf("default pull interval: got %s, want 60s", got)
	}
	c.Federation.PullIntervalSeconds = 15
	if got := c.FederationPullInterval(); got.Seconds() != 15 {
		t.Errorf("override pull interval: got %s, want 15s", got)
	}
}

func TestFederationPullBatchLimit_DefaultAndOverride(t *testing.T) {
	var c Config
	if got := c.FederationPullBatchLimit(); got != 500 {
		t.Errorf("default pull batch limit: got %d, want 500", got)
	}
	c.Federation.PullBatchLimit = 100
	if got := c.FederationPullBatchLimit(); got != 100 {
		t.Errorf("override pull batch limit: got %d, want 100", got)
	}
}

func TestFederationInboundRatePerMinute_DefaultOverrideAndDisable(t *testing.T) {
	var c Config
	if got := c.FederationInboundRatePerMinute(); got != 600 {
		t.Errorf("default inbound rate: got %d, want 600 (US-8.3 AC1)", got)
	}
	c.Federation.InboundRatePerMinute = 1200
	if got := c.FederationInboundRatePerMinute(); got != 1200 {
		t.Errorf("override inbound rate: got %d, want 1200", got)
	}
	c.Federation.InboundRatePerMinute = -1
	if got := c.FederationInboundRatePerMinute(); got != 0 {
		t.Errorf("negative inbound rate disables limiting: got %d, want 0", got)
	}
}

func TestFederationInboundBurst_DefaultsToRate(t *testing.T) {
	var c Config
	if got := c.FederationInboundBurst(); got != 600 {
		t.Errorf("default burst tracks the rate: got %d, want 600", got)
	}
	c.Federation.InboundBurst = 50
	if got := c.FederationInboundBurst(); got != 50 {
		t.Errorf("override burst: got %d, want 50", got)
	}
}

func TestFederationMaxBatchEvents_DefaultAndOverride(t *testing.T) {
	var c Config
	if got := c.FederationMaxBatchEvents(); got != 500 {
		t.Errorf("default max batch events: got %d, want 500 (US-8.3 AC3)", got)
	}
	c.Federation.MaxBatchEvents = 200
	if got := c.FederationMaxBatchEvents(); got != 200 {
		t.Errorf("override max batch events: got %d, want 200", got)
	}
}

// TestFederationOwnerTimeout_DefaultAndOverride covers the owner-death timeout
// (Federation v1 F5.6a, US-6.5 AC1): a joiner whose owner has not been contacted
// within this window flags the project "owner offline" so local edits queue
// instead of silently failing. The default is 30 days; a positive override is
// honoured; a non-positive value falls back to the default.
func TestFederationOwnerTimeout_DefaultAndOverride(t *testing.T) {
	var c Config
	if got := c.FederationOwnerTimeout(); got != 30*24*time.Hour {
		t.Errorf("default owner timeout: got %s, want 720h (30d, US-6.5 AC1)", got)
	}
	c.Federation.OwnerTimeoutDays = 7
	if got := c.FederationOwnerTimeout(); got != 7*24*time.Hour {
		t.Errorf("override owner timeout: got %s, want 168h (7d)", got)
	}
	c.Federation.OwnerTimeoutDays = -1
	if got := c.FederationOwnerTimeout(); got != 30*24*time.Hour {
		t.Errorf("non-positive owner timeout falls back to default: got %s, want 720h", got)
	}
}
