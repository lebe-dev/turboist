package config

import (
	"testing"
	"time"
)

const archMdExample = `
poll_interval: "30s"

contexts:
  dev:
    display_name: "Разработка"
    filters:
      projects: ["Проекты"]
      sections: ["Категория - Разработка"]
  personal:
    display_name: "Личное"
    filters:
      projects: ["Личное"]

weekly:
  label: "на неделе"
  max_tasks: 15

next_week:
  label: "на след неделе"

auto_expire:
  - label: "срочное"
    ttl: "24h"
  - label: "горит"
    ttl: "4h"
`

func TestParseAppConfig(t *testing.T) {
	app, err := ParseAppConfig([]byte(archMdExample))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if app.PollInterval != 30*time.Second {
		t.Errorf("poll_interval: got %v, want 30s", app.PollInterval)
	}

	if len(app.Contexts) != 2 {
		t.Errorf("contexts: got %d, want 2", len(app.Contexts))
	}

	dev, ok := app.Contexts["dev"]
	if !ok {
		t.Fatal("context 'dev' not found")
	}
	if dev.DisplayName != "Разработка" {
		t.Errorf("dev.display_name: got %q, want 'Разработка'", dev.DisplayName)
	}
	if len(dev.Filters.Projects) != 1 || dev.Filters.Projects[0] != "Проекты" {
		t.Errorf("dev.filters.projects: got %v", dev.Filters.Projects)
	}
	if len(dev.Filters.Sections) != 1 || dev.Filters.Sections[0] != "Категория - Разработка" {
		t.Errorf("dev.filters.sections: got %v", dev.Filters.Sections)
	}

	if app.Weekly.Label != "на неделе" {
		t.Errorf("weekly.label: got %q", app.Weekly.Label)
	}
	if app.Weekly.MaxTasks != 15 {
		t.Errorf("weekly.max_tasks: got %d, want 15", app.Weekly.MaxTasks)
	}

	if app.NextWeek.Label != "на след неделе" {
		t.Errorf("next_week.label: got %q", app.NextWeek.Label)
	}

	if len(app.AutoExpire) != 2 {
		t.Fatalf("auto_expire: got %d, want 2", len(app.AutoExpire))
	}
	if app.AutoExpire[0].Label != "срочное" || app.AutoExpire[0].TTL != 24*time.Hour {
		t.Errorf("auto_expire[0]: got %+v", app.AutoExpire[0])
	}
	if app.AutoExpire[1].Label != "горит" || app.AutoExpire[1].TTL != 4*time.Hour {
		t.Errorf("auto_expire[1]: got %+v", app.AutoExpire[1])
	}
}

func TestParseAppConfig_DefaultPollInterval(t *testing.T) {
	app, err := ParseAppConfig([]byte(`weekly: {label: "x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.PollInterval != 30*time.Second {
		t.Errorf("default poll_interval: got %v, want 30s", app.PollInterval)
	}
}
