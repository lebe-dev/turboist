package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// --- Argon2 params ---

func loadEnvWithRequiredVars(t *testing.T) {
	t.Helper()
	t.Setenv("BIND", "0.0.0.0:8080")
	t.Setenv("BASE_URL", "https://x.test")
	t.Setenv("JWT_SECRET", "supersecret-supersecret-supersecret")
	t.Setenv("API_TOKEN_SALT", "supersalt-supersalt-supersalt-supersalt")
}

func TestLoadEnv_Argon2_DefaultsWhenUnset(t *testing.T) {
	loadEnvWithRequiredVars(t)
	t.Setenv("ARGON2_MEMORY_KIB", "")
	t.Setenv("ARGON2_TIME", "")
	t.Setenv("ARGON2_THREADS", "")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if e.Argon2Params.Memory != 19*1024 || e.Argon2Params.Time != 2 || e.Argon2Params.Threads != 1 {
		t.Errorf("defaults: got %+v, want m=19MiB t=2 p=1", e.Argon2Params)
	}
}

func TestLoadEnv_Argon2_CustomValues(t *testing.T) {
	loadEnvWithRequiredVars(t)
	t.Setenv("ARGON2_MEMORY_KIB", "65536")
	t.Setenv("ARGON2_TIME", "3")
	t.Setenv("ARGON2_THREADS", "4")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if e.Argon2Params.Memory != 65536 || e.Argon2Params.Time != 3 || e.Argon2Params.Threads != 4 {
		t.Errorf("custom: got %+v", e.Argon2Params)
	}
}

func TestLoadEnv_Argon2_ZeroMemoryRejected(t *testing.T) {
	loadEnvWithRequiredVars(t)
	t.Setenv("ARGON2_MEMORY_KIB", "0")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "ARGON2_MEMORY_KIB") {
		t.Errorf("expected ARGON2_MEMORY_KIB error, got %v", err)
	}
}

func TestLoadEnv_Argon2_NegativeMemoryRejected(t *testing.T) {
	loadEnvWithRequiredVars(t)
	t.Setenv("ARGON2_MEMORY_KIB", "-1")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "ARGON2_MEMORY_KIB") {
		t.Errorf("expected ARGON2_MEMORY_KIB error, got %v", err)
	}
}

func TestLoadEnv_Argon2_NonNumericMemoryRejected(t *testing.T) {
	loadEnvWithRequiredVars(t)
	t.Setenv("ARGON2_MEMORY_KIB", "abc")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "ARGON2_MEMORY_KIB") {
		t.Errorf("expected ARGON2_MEMORY_KIB error, got %v", err)
	}
}

func TestLoadEnv_Argon2_ZeroTimeRejected(t *testing.T) {
	loadEnvWithRequiredVars(t)
	t.Setenv("ARGON2_TIME", "0")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "ARGON2_TIME") {
		t.Errorf("expected ARGON2_TIME error, got %v", err)
	}
}

func TestLoadEnv_Argon2_ZeroThreadsRejected(t *testing.T) {
	loadEnvWithRequiredVars(t)
	t.Setenv("ARGON2_THREADS", "0")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "ARGON2_THREADS") {
		t.Errorf("expected ARGON2_THREADS error, got %v", err)
	}
}

func TestLoadEnv_Argon2_ThreadsOverflowRejected(t *testing.T) {
	loadEnvWithRequiredVars(t)
	t.Setenv("ARGON2_THREADS", "256")
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "ARGON2_THREADS") {
		t.Errorf("expected ARGON2_THREADS error (overflow uint8), got %v", err)
	}
}
