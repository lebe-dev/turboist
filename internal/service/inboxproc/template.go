package inboxproc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// ContextView is a context as the prompt template sees it.
type ContextView struct {
	ID   int64
	Name string
}

// ProjectView is an open project as the prompt template sees it.
type ProjectView struct {
	ID             int64
	Title          string
	Description    string
	Type           string
	Context        string
	Labels         []string
	TroikiCategory string
}

// LabelView is a label as the prompt template sees it.
type LabelView struct {
	ID   int64
	Name string
}

// TaskView is the task being classified as the prompt template sees it.
type TaskView struct {
	ID          int64
	Title       string
	Description string
	CreatedAt   string
	Labels      []string
}

// PromptData is everything a prompt template can reference.
type PromptData struct {
	Now        string
	Timezone   string
	Locale     string
	Contexts   []ContextView
	Projects   []ProjectView
	Labels     []LabelView
	Priorities []string
	Task       TaskView
}

// Catalogue is the workspace snapshot one run classifies against. It is both
// what the model is shown and the source of truth its answer is validated with,
// so an id the model was never offered can never be applied.
type Catalogue struct {
	Contexts []model.Context
	Projects []model.Project
	Labels   []model.Label

	contextNames map[int64]string
	projects     map[int64]*model.Project
	labels       map[int64]*model.Label
}

// NewCatalogue keeps only open projects and sorts everything for a stable
// prompt: contexts and labels by name, projects by context name then title.
func NewCatalogue(contexts []model.Context, projects []model.Project, labels []model.Label) *Catalogue {
	c := &Catalogue{
		Contexts:     append([]model.Context(nil), contexts...),
		Labels:       append([]model.Label(nil), labels...),
		contextNames: make(map[int64]string, len(contexts)),
		projects:     make(map[int64]*model.Project),
		labels:       make(map[int64]*model.Label, len(labels)),
	}
	sort.SliceStable(c.Contexts, func(i, j int) bool { return c.Contexts[i].Name < c.Contexts[j].Name })
	for _, ctx := range c.Contexts {
		c.contextNames[ctx.ID] = ctx.Name
	}
	for _, p := range projects {
		if p.Status != model.ProjectStatusOpen {
			continue
		}
		c.Projects = append(c.Projects, p)
	}
	sort.SliceStable(c.Projects, func(i, j int) bool {
		ci, cj := c.contextNames[c.Projects[i].ContextID], c.contextNames[c.Projects[j].ContextID]
		if ci != cj {
			return ci < cj
		}
		return c.Projects[i].Title < c.Projects[j].Title
	})
	for i := range c.Projects {
		c.projects[c.Projects[i].ID] = &c.Projects[i]
	}
	sort.SliceStable(c.Labels, func(i, j int) bool { return c.Labels[i].Name < c.Labels[j].Name })
	for i := range c.Labels {
		c.labels[c.Labels[i].ID] = &c.Labels[i]
	}
	return c
}

// Project looks up an open project of the catalogue.
func (c *Catalogue) Project(id int64) (*model.Project, bool) {
	p, ok := c.projects[id]
	return p, ok
}

// Label looks up a label of the catalogue.
func (c *Catalogue) Label(id int64) (*model.Label, bool) {
	l, ok := c.labels[id]
	return l, ok
}

// PromptData renders the catalogue and one task into template data. Times are
// shown in the server's timezone, which is also the one due dates resolve in.
func (c *Catalogue) PromptData(now time.Time, loc *time.Location, locale string, task model.Task) PromptData {
	if loc == nil {
		loc = time.UTC
	}
	data := PromptData{
		Now:        now.In(loc).Format(time.RFC3339),
		Timezone:   loc.String(),
		Locale:     locale,
		Contexts:   make([]ContextView, 0, len(c.Contexts)),
		Projects:   make([]ProjectView, 0, len(c.Projects)),
		Labels:     make([]LabelView, 0, len(c.Labels)),
		Priorities: []string{string(model.PriorityHigh), string(model.PriorityMedium), string(model.PriorityLow), string(model.PriorityNone)},
		Task:       taskView(task, loc),
	}
	for _, ctx := range c.Contexts {
		data.Contexts = append(data.Contexts, ContextView{ID: ctx.ID, Name: ctx.Name})
	}
	for _, p := range c.Projects {
		view := ProjectView{
			ID:          p.ID,
			Title:       p.Title,
			Description: strings.TrimSpace(p.Description),
			Type:        string(p.Type),
			Context:     c.contextNames[p.ContextID],
			Labels:      labelNames(p.Labels),
		}
		if p.TroikiCategory != nil {
			view.TroikiCategory = string(*p.TroikiCategory)
		}
		data.Projects = append(data.Projects, view)
	}
	for _, l := range c.Labels {
		data.Labels = append(data.Labels, LabelView{ID: l.ID, Name: l.Name})
	}
	return data
}

func taskView(t model.Task, loc *time.Location) TaskView {
	return TaskView{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		CreatedAt:   t.CreatedAt.In(loc).Format(time.RFC3339),
		Labels:      labelNames(t.Labels),
	}
}

func labelNames(labels []model.Label) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, l.Name)
	}
	return out
}

// ExampleTask stands in for a real task when the prompt is previewed against an
// empty Inbox.
func ExampleTask(now time.Time) model.Task {
	return model.Task{
		ID:        0,
		Title:     "Fix login redirect on Safari",
		CreatedAt: now,
	}
}

// ParsePrompt parses a prompt template. An empty text means DefaultPrompt.
func ParsePrompt(text string) (*template.Template, error) {
	if strings.TrimSpace(text) == "" {
		text = DefaultPrompt
	}
	if len(text) > MaxPromptLength {
		return nil, fmt.Errorf("prompt is longer than %d characters", MaxPromptLength)
	}
	return template.New("prompt").
		Option("missingkey=error").
		Funcs(template.FuncMap{"join": strings.Join}).
		Parse(text)
}

// RenderSystem renders the template and appends the fixed output contract.
func RenderSystem(tpl *template.Template, data PromptData) (string, error) {
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n") + "\n\n" + outputContract, nil
}

// ValidatePrompt checks that a prompt both parses and renders against real data.
// Field typos only surface at execution, so parsing alone is not enough.
func ValidatePrompt(text string, data PromptData) error {
	tpl, err := ParsePrompt(text)
	if err != nil {
		return err
	}
	_, err = RenderSystem(tpl, data)
	return err
}

type userMessage struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	CreatedAt   string   `json:"createdAt"`
	Labels      []string `json:"labels"`
}

// UserMessage is the fixed JSON the task travels in as the user message.
func UserMessage(t model.Task, loc *time.Location) (string, error) {
	if loc == nil {
		loc = time.UTC
	}
	v := taskView(t, loc)
	raw, err := json.Marshal(userMessage{
		ID:          v.ID,
		Title:       v.Title,
		Description: v.Description,
		CreatedAt:   v.CreatedAt,
		Labels:      v.Labels,
	})
	if err != nil {
		return "", fmt.Errorf("encode task: %w", err)
	}
	return string(raw), nil
}
