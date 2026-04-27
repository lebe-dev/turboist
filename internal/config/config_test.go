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
auto-labels:
  - mask: "купить"
    label: "покупки"
  - mask: "Проект -"
    label: "проект"
    ignore-case: false
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
	if len(cfg.AutoLabels) != 2 {
		t.Fatalf("auto-labels count: %d", len(cfg.AutoLabels))
	}
	if cfg.AutoLabels[0].IgnoreCaseValue() != true {
		t.Fatalf("default ignore-case must be true")
	}
	if cfg.AutoLabels[1].IgnoreCaseValue() != false {
		t.Fatalf("explicit ignore-case must be false")
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

func TestLoad_EmptyAutoLabel(t *testing.T) {
	body := validYAML + "\n  - mask: \"foo\"\n    label: \"\"\n"
	p := writeConfig(t, body)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "label") {
		t.Fatalf("expected empty label error, got %v", err)
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
	t.Setenv("JWT_SECRET", "supersecret")
	t.Setenv("LOG_LEVEL", "")
	e, err := LoadEnv()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if e.LogLevel != "info" {
		t.Fatalf("default LOG_LEVEL must be info, got %q", e.LogLevel)
	}
}
