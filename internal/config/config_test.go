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
	if cfg.Weekly.Limit != 30 || cfg.Backlog.Limit != 30 {
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

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
}

func TestLoadEnv_CalendarCacheTTLDefault(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CALENDAR_CACHE_TTL", "")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if e.CalendarCacheTTL != defaultCalendarCacheTTL {
		t.Fatalf("CalendarCacheTTL: got %v, want %v", e.CalendarCacheTTL, defaultCalendarCacheTTL)
	}
}

func TestLoadEnv_CalendarCacheTTLCustom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CALENDAR_CACHE_TTL", "30s")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if e.CalendarCacheTTL != 30*time.Second {
		t.Fatalf("CalendarCacheTTL: got %v, want %v", e.CalendarCacheTTL, 30*time.Second)
	}
}

func TestLoadEnv_CalendarCacheTTLInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CALENDAR_CACHE_TTL", "nonsense")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "CALENDAR_CACHE_TTL") {
		t.Fatalf("expected CALENDAR_CACHE_TTL parse error, got %v", err)
	}
}

func TestLoadEnv_CalendarCacheTTLNonPositive(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CALENDAR_CACHE_TTL", "0s")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "CALENDAR_CACHE_TTL") {
		t.Fatalf("expected CALENDAR_CACHE_TTL positive error, got %v", err)
	}
}

// setupEnvBase sets the required variables so a test can focus on one knob.
func setupEnvBase(t *testing.T) {
	t.Helper()
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://todo.example.com")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
	t.Setenv("WEBAUTHN_RP_ID", "")
	t.Setenv("WEBAUTHN_ORIGINS", "")
}

func TestLoadEnv_WebAuthnDefaultsFromBaseURL(t *testing.T) {
	setupEnvBase(t)
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("load env: %v", err)
	}
	if e.WebAuthnRPID != "todo.example.com" {
		t.Errorf("rp id: got %q, want todo.example.com", e.WebAuthnRPID)
	}
	if len(e.WebAuthnOrigins) != 1 || e.WebAuthnOrigins[0] != "https://todo.example.com" {
		t.Errorf("origins: got %v, want [https://todo.example.com]", e.WebAuthnOrigins)
	}
}

func TestLoadEnv_WebAuthnOverrides(t *testing.T) {
	setupEnvBase(t)
	t.Setenv("WEBAUTHN_RP_ID", "example.com")
	// Duplicates and blanks are tolerated; the native app's origin is appended.
	t.Setenv("WEBAUTHN_ORIGINS", "https://todo.example.com, android:apk-key-hash:abc , ")

	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("load env: %v", err)
	}
	if e.WebAuthnRPID != "example.com" {
		t.Errorf("rp id: got %q, want example.com", e.WebAuthnRPID)
	}
	want := []string{"https://todo.example.com", "android:apk-key-hash:abc"}
	if len(e.WebAuthnOrigins) != len(want) {
		t.Fatalf("origins: got %v, want %v", e.WebAuthnOrigins, want)
	}
	for i := range want {
		if e.WebAuthnOrigins[i] != want[i] {
			t.Errorf("origins[%d]: got %q, want %q", i, e.WebAuthnOrigins[i], want[i])
		}
	}
}

func TestLoadEnv_WebAuthnRejectsHostlessBaseURL(t *testing.T) {
	setupEnvBase(t)
	t.Setenv("BASE_URL", "not-a-url")
	if _, err := LoadEnv(); err == nil {
		t.Fatal("load env: got nil error, want a rejection of a BASE_URL with no host")
	}
}

func TestLoadEnv_InboxProcessing_Defaults(t *testing.T) {
	setupEnvBase(t)
	for _, k := range []string{"INBOX_PROCESSING_ENABLED", "INBOX_PROCESSING_INTERVAL", "INBOX_PROCESSING_API_URL",
		"INBOX_PROCESSING_API_KEY", "INBOX_PROCESSING_MODEL", "INBOX_PROCESSING_BATCH_LIMIT", "INBOX_PROCESSING_TIMEOUT"} {
		t.Setenv(k, "")
	}
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("load env: %v", err)
	}
	ip := e.InboxProcessing
	if ip.Enabled {
		t.Errorf("enabled: got true, want false")
	}
	if ip.Interval != 3*time.Minute {
		t.Errorf("interval: got %v, want 3m", ip.Interval)
	}
	if ip.APIURL != "https://openrouter.ai/api/v1" {
		t.Errorf("api url: got %q, want the OpenRouter default", ip.APIURL)
	}
	if ip.BatchLimit != 10 {
		t.Errorf("batch limit: got %d, want 10", ip.BatchLimit)
	}
	if ip.Timeout != 60*time.Second {
		t.Errorf("timeout: got %v, want 60s", ip.Timeout)
	}
}

func TestLoadEnv_InboxProcessing_EnabledWithoutKeyFails(t *testing.T) {
	setupEnvBase(t)
	t.Setenv("INBOX_PROCESSING_ENABLED", "true")
	t.Setenv("INBOX_PROCESSING_API_KEY", "")
	t.Setenv("INBOX_PROCESSING_MODEL", "openai/gpt-4.1-mini")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "INBOX_PROCESSING_API_KEY") {
		t.Fatalf("got %v, want an INBOX_PROCESSING_API_KEY error", err)
	}
}

func TestLoadEnv_InboxProcessing_EnabledWithoutModelFails(t *testing.T) {
	setupEnvBase(t)
	t.Setenv("INBOX_PROCESSING_ENABLED", "true")
	t.Setenv("INBOX_PROCESSING_API_KEY", "sk-test")
	t.Setenv("INBOX_PROCESSING_MODEL", "")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "INBOX_PROCESSING_MODEL") {
		t.Fatalf("got %v, want an INBOX_PROCESSING_MODEL error", err)
	}
}

func TestLoadEnv_InboxProcessing_Enabled(t *testing.T) {
	setupEnvBase(t)
	t.Setenv("INBOX_PROCESSING_ENABLED", "true")
	t.Setenv("INBOX_PROCESSING_API_KEY", "sk-test")
	t.Setenv("INBOX_PROCESSING_MODEL", "openai/gpt-4.1-mini")
	t.Setenv("INBOX_PROCESSING_INTERVAL", "45s")
	t.Setenv("INBOX_PROCESSING_API_URL", "https://api.example.com/v1/")
	t.Setenv("INBOX_PROCESSING_BATCH_LIMIT", "25")
	t.Setenv("INBOX_PROCESSING_TIMEOUT", "20s")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("load env: %v", err)
	}
	ip := e.InboxProcessing
	if !ip.Enabled || ip.APIKey != "sk-test" || ip.Model != "openai/gpt-4.1-mini" {
		t.Errorf("got %+v, want enabled with key and model", ip)
	}
	if ip.Interval != 45*time.Second || ip.BatchLimit != 25 || ip.Timeout != 20*time.Second {
		t.Errorf("got %+v, want interval 45s, batch 25, timeout 20s", ip)
	}
	if ip.APIURL != "https://api.example.com/v1" {
		t.Errorf("api url: got %q, want the trailing slash trimmed", ip.APIURL)
	}
}

func TestLoadEnv_InboxProcessing_IntervalTooShort(t *testing.T) {
	setupEnvBase(t)
	t.Setenv("INBOX_PROCESSING_INTERVAL", "10s")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "INBOX_PROCESSING_INTERVAL") {
		t.Fatalf("got %v, want an INBOX_PROCESSING_INTERVAL error", err)
	}
}

func TestLoadEnv_InboxProcessing_BadURL(t *testing.T) {
	setupEnvBase(t)
	t.Setenv("INBOX_PROCESSING_API_URL", "openrouter.ai/api")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "INBOX_PROCESSING_API_URL") {
		t.Fatalf("got %v, want an INBOX_PROCESSING_API_URL error", err)
	}
}

func TestLoadEnv_InboxProcessing_BadBatchLimit(t *testing.T) {
	setupEnvBase(t)
	t.Setenv("INBOX_PROCESSING_BATCH_LIMIT", "101")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "INBOX_PROCESSING_BATCH_LIMIT") {
		t.Fatalf("got %v, want an INBOX_PROCESSING_BATCH_LIMIT error", err)
	}
}

func TestLoadEnv_InboxProcessing_BadTimeout(t *testing.T) {
	setupEnvBase(t)
	t.Setenv("INBOX_PROCESSING_TIMEOUT", "-1s")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "INBOX_PROCESSING_TIMEOUT") {
		t.Fatalf("got %v, want an INBOX_PROCESSING_TIMEOUT error", err)
	}
}
