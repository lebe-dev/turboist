package inboxproc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func sampleCatalogue() *Catalogue {
	important := model.TroikiCategoryImportant
	return NewCatalogue(
		[]model.Context{{ID: 2, Name: "work"}, {ID: 1, Name: "home"}},
		[]model.Project{
			{ID: 10, ContextID: 2, Title: "Turboist", Description: "Task manager app", Type: model.ProjectTypeSoftware,
				Status: model.ProjectStatusOpen, TroikiCategory: &important, Labels: []model.Label{{ID: 5, Name: "dev"}}},
			{ID: 11, ContextID: 1, Title: "Garden", Type: model.ProjectTypeGeneric, Status: model.ProjectStatusOpen},
			{ID: 12, ContextID: 1, Title: "Old", Type: model.ProjectTypeGeneric, Status: model.ProjectStatusArchived},
		},
		[]model.Label{{ID: 6, Name: "urgent"}, {ID: 5, Name: "dev"}},
	)
}

func sampleTask() model.Task {
	return model.Task{
		ID: 42, Title: "Fix login redirect on Safari",
		CreatedAt: time.Date(2026, 9, 13, 6, 12, 0, 0, time.UTC),
		Labels:    []model.Label{{ID: 6, Name: "urgent"}},
	}
}

func sampleData(c *Catalogue) PromptData {
	loc, _ := time.LoadLocation("Europe/Moscow")
	return c.PromptData(time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC), loc, "ru", sampleTask())
}

func TestCatalogue_DropsClosedProjectsAndSorts(t *testing.T) {
	c := sampleCatalogue()
	if len(c.Projects) != 2 {
		t.Fatalf("projects: got %d, want 2 open ones", len(c.Projects))
	}
	// home < work, so Garden comes first.
	if c.Projects[0].ID != 11 || c.Projects[1].ID != 10 {
		t.Errorf("project order: got %d,%d, want 11,10", c.Projects[0].ID, c.Projects[1].ID)
	}
	if c.Labels[0].Name != "dev" {
		t.Errorf("label order: got %q first, want dev", c.Labels[0].Name)
	}
	if _, ok := c.Project(12); ok {
		t.Error("archived project must not be in the catalogue")
	}
	if _, ok := c.Label(6); !ok {
		t.Error("label 6 must be in the catalogue")
	}
}

func TestRenderSystem_DefaultPrompt(t *testing.T) {
	c := sampleCatalogue()
	tpl, err := ParsePrompt("")
	if err != nil {
		t.Fatalf("parse default: %v", err)
	}
	got, err := RenderSystem(tpl, sampleData(c))
	if err != nil {
		t.Fatalf("render default: %v", err)
	}
	for _, want := range []string{
		"2026-09-13T13:00:00+03:00 (Europe/Moscow)",
		`"ru"`,
		`- id=10 | "Turboist" | context: work | type: software | daily-plan bucket: important | project labels: dev`,
		"  Task manager app",
		`- id=11 | "Garden" | context: home | type: generic`,
		`- id=6 | "urgent"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered prompt lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Old") {
		t.Errorf("rendered prompt lists an archived project:\n%s", got)
	}
	if !strings.HasSuffix(got, outputContract) {
		t.Errorf("rendered prompt must end with the output contract:\n%s", got)
	}
}

func TestRenderSystem_DefaultPromptOnEmptyCatalogue(t *testing.T) {
	c := NewCatalogue(nil, nil, nil)
	tpl, err := ParsePrompt("")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := RenderSystem(tpl, sampleData(c))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasSuffix(got, outputContract) {
		t.Errorf("empty-catalogue prompt must still end with the contract:\n%s", got)
	}
}

func TestRenderSystem_ContractAppendedToCustomPrompt(t *testing.T) {
	tpl, err := ParsePrompt("Sort {{.Task.Title}} into one of {{len .Projects}} projects.")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := RenderSystem(tpl, sampleData(sampleCatalogue()))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "Sort Fix login redirect on Safari into one of 2 projects.\n\n" + outputContract
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestValidatePrompt_TypoIsAnError(t *testing.T) {
	err := ValidatePrompt("{{range .Projcets}}x{{end}}", sampleData(sampleCatalogue()))
	if err == nil || !strings.Contains(err.Error(), "Projcets") {
		t.Fatalf("got %v, want an error naming the misspelled field", err)
	}
}

func TestValidatePrompt_SyntaxError(t *testing.T) {
	if err := ValidatePrompt("{{range .Projects}}", sampleData(sampleCatalogue())); err == nil {
		t.Fatal("got nil, want a parse error for an unclosed range")
	}
}

func TestValidatePrompt_TooLong(t *testing.T) {
	if err := ValidatePrompt(strings.Repeat("a", MaxPromptLength+1), sampleData(sampleCatalogue())); err == nil {
		t.Fatal("got nil, want a length error")
	}
}

func TestJoinFunc(t *testing.T) {
	tpl, err := ParsePrompt(`{{join .Task.Labels "+"}}|{{join .Priorities ","}}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := RenderSystem(tpl, sampleData(sampleCatalogue()))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(got, "urgent|high,medium,low,no-priority\n\n") {
		t.Errorf("got %q", got)
	}
}

func TestUserMessage(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	raw, err := UserMessage(sampleTask(), loc)
	if err != nil {
		t.Fatalf("user message: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	if got["id"] != float64(42) || got["title"] != "Fix login redirect on Safari" || got["description"] != "" {
		t.Errorf("fields: got %v", got)
	}
	if got["createdAt"] != "2026-09-13T09:12:00+03:00" {
		t.Errorf("createdAt: got %v, want local RFC3339", got["createdAt"])
	}
	labels, ok := got["labels"].([]any)
	if !ok || len(labels) != 1 || labels[0] != "urgent" {
		t.Errorf("labels: got %v, want [urgent]", got["labels"])
	}
}
