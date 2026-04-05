package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type EnvConfig struct {
	Bind          string
	LogLevel      string
	BaseURL       string
	TodoistAPIKey string
	AdminPassword string
	Dev           bool
}

// TaskSort defines how tasks are sorted in API responses.
type TaskSort string

const (
	TaskSortPriority TaskSort = "priority"
	TaskSortDueDate  TaskSort = "due_date"
	TaskSortContent  TaskSort = "content"
	TaskSortAddedAt  TaskSort = "added_at"
)

type CompletedConfig struct {
	Days int `yaml:"days"`
}

type QuickCaptureConfig struct {
	Title string `yaml:"title"`
}

type ProjectConfig struct {
	Label string `yaml:"label"`
}

type LabelConfig struct {
	Name              string `yaml:"name"`
	InheritToSubtasks *bool  `yaml:"inherit_to_subtasks"`
}

// ShouldInheritToSubtasks returns whether this label should be inherited by subtasks.
// Defaults to true when InheritToSubtasks is nil.
func (l *LabelConfig) ShouldInheritToSubtasks() bool {
	if l.InheritToSubtasks == nil {
		return true
	}
	return *l.InheritToSubtasks
}

type InboxConfig struct {
	MaxLimit            int    `yaml:"max_limit"`
	OverflowTaskContent string `yaml:"overflow_task_content"`
}

type AppConfig struct {
	PollInterval       time.Duration
	SyncInterval       time.Duration
	Timezone           string
	TaskSort           TaskSort
	MaxPinned          int
	Contexts           []ContextConfig
	Labels             []LabelConfig
	Weekly             WeeklyConfig
	Backlog            BacklogConfig
	Inbox              InboxConfig
	Project            ProjectConfig
	ProjectsLabel      string
	Today              TodayConfig
	Tomorrow           TomorrowConfig
	Completed          CompletedConfig
	AutoRemove         AutoRemoveConfig
	QuickCapture       *QuickCaptureConfig
	AutoLabels         []AutoLabelConfig
	CompiledAutoLabels []CompiledAutoLabel
	LabelProjectMap    LabelProjectMapConfig
}

// FindContext returns the context with the given ID, or nil if not found.
func (c *AppConfig) FindContext(id string) *ContextConfig {
	for i := range c.Contexts {
		if c.Contexts[i].ID == id {
			return &c.Contexts[i]
		}
	}
	return nil
}

// FindLabel returns the label config with the given name, or nil if not configured.
func (c *AppConfig) FindLabel(name string) *LabelConfig {
	for i := range c.Labels {
		if c.Labels[i].Name == name {
			return &c.Labels[i]
		}
	}
	return nil
}

type Config struct {
	Env EnvConfig
	App AppConfig
}

type ContextConfig struct {
	ID            string         `yaml:"id"`
	DisplayName   string         `yaml:"display_name"`
	Color         string         `yaml:"color"`
	InheritLabels *bool          `yaml:"inherit_labels"`
	Filters       ContextFilters `yaml:"filters"`
}

// ShouldInheritLabels returns whether labels should be inherited on task creation.
// Defaults to true when InheritLabels is nil.
func (c *ContextConfig) ShouldInheritLabels() bool {
	if c.InheritLabels == nil {
		return true
	}
	return *c.InheritLabels
}

type ContextFilters struct {
	Projects []string `yaml:"projects"`
	Sections []string `yaml:"sections"`
	Labels   []string `yaml:"labels"`
}

type WeeklyConfig struct {
	Label    string `yaml:"label"`
	MaxTasks int    `yaml:"max_tasks"`
}

type BacklogConfig struct {
	Label    string   `yaml:"label"`
	TaskSort TaskSort `yaml:"task_sort"`
	MaxLimit int      `yaml:"max_limit"`
}

type DayPartConfig struct {
	Label string `yaml:"label"`
	Start int    `yaml:"start"` // hour 0-23
	End   int    `yaml:"end"`   // hour 0-23
}

type TodayConfig struct {
	IncludeOverdue       bool            `yaml:"include_overdue"`
	DayParts             []DayPartConfig `yaml:"day_parts"`
	MaxDayPartNoteLength int             `yaml:"max_day_part_note_length"`
}

type TomorrowConfig struct {
}

// AutoRemoveConfig holds safety-guarded settings for automatic task deletion.
type AutoRemoveConfig struct {
	Enabled    bool
	MinTTL     time.Duration
	MaxPerTick int
	MaxPercent int
	Rules      []AutoRemoveRuleConfig
}

type AutoRemoveRuleConfig struct {
	Label string
	TTL   time.Duration
}

type AutoLabelConfig struct {
	Mask       string `yaml:"mask"`
	Label      string `yaml:"label"`
	IgnoreCase *bool  `yaml:"ignore_case"`
}

type LabelProjectMapConfig struct {
	Enabled  bool
	Mappings []LabelProjectMapping
}

type LabelProjectMapping struct {
	Label   string `yaml:"label"`
	Project string `yaml:"project"`
	Section string `yaml:"section"`
}

func (a *AutoLabelConfig) ShouldIgnoreCase() bool {
	if a.IgnoreCase == nil {
		return true
	}
	return *a.IgnoreCase
}

type CompiledAutoLabel struct {
	Label      string
	Mask       string // normalized: lowercased when IgnoreCase=true
	IgnoreCase bool
}

type yamlFile struct {
	Timezone        string                `yaml:"timezone"`
	PollInterval    string                `yaml:"poll_interval"`
	TaskSort        string                `yaml:"task_sort"`
	MaxPinned       int                   `yaml:"max_pinned"`
	Contexts        []ContextConfig       `yaml:"contexts"`
	Labels          []LabelConfig         `yaml:"labels"`
	Weekly          WeeklyConfig          `yaml:"weekly"`
	Backlog         BacklogConfig         `yaml:"backlog"`
	Inbox           InboxConfig           `yaml:"inbox"`
	Project         ProjectConfig         `yaml:"project"`
	ProjectsLabel   string                `yaml:"projects_label"`
	Today           TodayConfig           `yaml:"today"`
	Tomorrow        TomorrowConfig        `yaml:"tomorrow"`
	Completed       CompletedConfig       `yaml:"completed"`
	AutoRemove      *yamlAutoRemove       `yaml:"auto_remove"`
	QuickCapture    *QuickCaptureConfig   `yaml:"quick_capture"`
	AutoLabels      []AutoLabelConfig     `yaml:"auto_labels"`
	LabelProjectMap *yamlLabelProjectMap `yaml:"label_project_map"`
}

type yamlAutoRemove struct {
	Enabled    bool                 `yaml:"enabled"`
	MinTTL     string               `yaml:"min_ttl"`
	MaxPerTick int                  `yaml:"max_per_tick"`
	MaxPercent int                  `yaml:"max_percent"`
	Rules      []yamlAutoRemoveRule `yaml:"rules"`
}

type yamlLabelProjectMap struct {
	Enabled  bool                  `yaml:"enabled"`
	Mappings []LabelProjectMapping `yaml:"mappings"`
}

type yamlAutoRemoveRule struct {
	Label string `yaml:"label"`
	TTL   string `yaml:"ttl"`
}

func LoadAppConfig(path string) (AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AppConfig{}, err
	}
	return ParseAppConfig(data)
}

func ParseAppConfig(data []byte) (AppConfig, error) {
	var yf yamlFile
	if err := yaml.Unmarshal(data, &yf); err != nil {
		return AppConfig{}, err
	}

	if yf.PollInterval == "" {
		yf.PollInterval = "30s"
	}

	pollInterval, err := time.ParseDuration(yf.PollInterval)
	if err != nil {
		return AppConfig{}, fmt.Errorf("poll_interval: %w", err)
	}

	tz := yf.Timezone
	if tz == "" {
		tz = "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return AppConfig{}, fmt.Errorf("timezone: %w", err)
	}

	taskSort := TaskSort(yf.TaskSort)
	switch taskSort {
	case TaskSortPriority, TaskSortDueDate, TaskSortContent, TaskSortAddedAt:
	case "":
		taskSort = TaskSortPriority
	default:
		return AppConfig{}, fmt.Errorf("task_sort: unknown value %q", yf.TaskSort)
	}

	completed := yf.Completed
	if completed.Days <= 0 {
		completed.Days = 3
	}

	maxPinned := yf.MaxPinned
	if maxPinned <= 0 {
		maxPinned = 5
	}

	backlog := yf.Backlog
	switch backlog.TaskSort {
	case TaskSortPriority, TaskSortDueDate, TaskSortContent, TaskSortAddedAt:
	case "":
		backlog.TaskSort = TaskSortAddedAt
	default:
		return AppConfig{}, fmt.Errorf("backlog.task_sort: unknown value %q", backlog.TaskSort)
	}
	if backlog.MaxLimit <= 0 {
		backlog.MaxLimit = 20
	}

	maxDayPartNoteLength := yf.Today.MaxDayPartNoteLength
	if maxDayPartNoteLength <= 0 {
		maxDayPartNoteLength = 200
	}

	inbox := yf.Inbox
	if inbox.MaxLimit <= 0 {
		inbox.MaxLimit = 10
	}
	if inbox.OverflowTaskContent == "" {
		inbox.OverflowTaskContent = "Разобрать Входящие"
	}

	app := AppConfig{
		PollInterval:  pollInterval,
		Timezone:      tz,
		TaskSort:      taskSort,
		MaxPinned:     maxPinned,
		Contexts:      yf.Contexts,
		Labels:        yf.Labels,
		Weekly:        yf.Weekly,
		Backlog:       backlog,
		Inbox:         inbox,
		Project:       yf.Project,
		ProjectsLabel: yf.ProjectsLabel,
		Today: TodayConfig{
			IncludeOverdue:       yf.Today.IncludeOverdue,
			DayParts:             yf.Today.DayParts,
			MaxDayPartNoteLength: maxDayPartNoteLength,
		},
		Tomorrow:        yf.Tomorrow,
		Completed:       completed,
		QuickCapture:    yf.QuickCapture,
		AutoLabels:      yf.AutoLabels,
		LabelProjectMap: parseLabelProjectMap(yf.LabelProjectMap),
	}

	if err := validateDayParts(yf.Today.DayParts); err != nil {
		return AppConfig{}, err
	}

	if err := validateLabels(yf.Labels); err != nil {
		return AppConfig{}, err
	}

	if err := validateLabelProjectMap(app.LabelProjectMap.Mappings); err != nil {
		return AppConfig{}, err
	}

	if yf.AutoRemove != nil {
		ar, err := parseAutoRemove(yf.AutoRemove)
		if err != nil {
			return AppConfig{}, err
		}
		app.AutoRemove = ar
	}

	compiled, err := compileAutoLabels(yf.AutoLabels)
	if err != nil {
		return AppConfig{}, err
	}
	app.CompiledAutoLabels = compiled

	return app, nil
}

func parseLabelProjectMap(ylp *yamlLabelProjectMap) LabelProjectMapConfig {
	if ylp == nil {
		return LabelProjectMapConfig{}
	}
	return LabelProjectMapConfig{
		Enabled:  ylp.Enabled,
		Mappings: ylp.Mappings,
	}
}

func parseAutoRemove(yar *yamlAutoRemove) (AutoRemoveConfig, error) {
	minTTL := time.Hour // default
	if yar.MinTTL != "" {
		d, err := time.ParseDuration(yar.MinTTL)
		if err != nil {
			return AutoRemoveConfig{}, fmt.Errorf("auto_remove.min_ttl: %w", err)
		}
		minTTL = d
	}

	maxPerTick := yar.MaxPerTick
	if maxPerTick <= 0 {
		maxPerTick = 1
	}
	maxPercent := yar.MaxPercent
	if maxPercent <= 0 {
		maxPercent = 10
	}

	var rules []AutoRemoveRuleConfig
	for i, r := range yar.Rules {
		if r.Label == "" {
			return AutoRemoveConfig{}, fmt.Errorf("auto_remove.rules[%d]: label is required", i)
		}
		if r.TTL == "" {
			return AutoRemoveConfig{}, fmt.Errorf("auto_remove.rules[%d]: ttl is required", i)
		}
		ttl, err := time.ParseDuration(r.TTL)
		if err != nil {
			return AutoRemoveConfig{}, fmt.Errorf("auto_remove.rules[%d] ttl: %w", i, err)
		}
		if ttl < minTTL {
			return AutoRemoveConfig{}, fmt.Errorf("auto_remove.rules[%d]: ttl %v is below minimum %v", i, ttl, minTTL)
		}
		rules = append(rules, AutoRemoveRuleConfig{Label: r.Label, TTL: ttl})
	}

	return AutoRemoveConfig{
		Enabled:    yar.Enabled,
		MinTTL:     minTTL,
		MaxPerTick: maxPerTick,
		MaxPercent: maxPercent,
		Rules:      rules,
	}, nil
}

func compileAutoLabels(tags []AutoLabelConfig) ([]CompiledAutoLabel, error) {
	seen := make(map[string]struct{}, len(tags))
	result := make([]CompiledAutoLabel, 0, len(tags))
	for i, at := range tags {
		if at.Mask == "" {
			return nil, fmt.Errorf("auto_tags[%d]: mask is required", i)
		}
		if at.Label == "" {
			return nil, fmt.Errorf("auto_tags[%d]: label is required", i)
		}
		key := at.Mask + "\x00" + at.Label
		if _, ok := seen[key]; ok {
			log.Printf("warning: auto_tags[%d]: duplicate mask+label %q+%q", i, at.Mask, at.Label)
			continue
		}
		seen[key] = struct{}{}
		mask := at.Mask
		ignoreCase := at.ShouldIgnoreCase()
		if ignoreCase {
			mask = strings.ToLower(mask)
		}
		result = append(result, CompiledAutoLabel{Label: at.Label, Mask: mask, IgnoreCase: ignoreCase})
	}
	return result, nil
}

func loadEnv() (EnvConfig, error) {
	env := EnvConfig{
		Bind:          getEnv("BIND", ":8080"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
		BaseURL:       getEnv("BASE_URL", "http://localhost:8080"),
		TodoistAPIKey: os.Getenv("TODOIST_API_KEY"),
		AdminPassword: os.Getenv("TURBOIST_ADMIN_PASSWORD"),
		Dev:           os.Getenv("DEV") == "true",
	}
	if env.TodoistAPIKey == "" {
		return env, fmt.Errorf("TODOIST_API_KEY is required")
	}
	if env.AdminPassword == "" {
		return env, fmt.Errorf("TURBOIST_ADMIN_PASSWORD is required")
	}
	return env, nil
}

// loadDotEnv reads KEY=VALUE pairs from path and sets them as env vars,
// skipping keys that are already set. Missing file is silently ignored.
func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		if os.Getenv(k) == "" {
			os.Setenv(k, v) //nolint:errcheck
		}
	}
	return nil
}

func Load() (*Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return nil, fmt.Errorf(".env: %w", err)
	}

	env, err := loadEnv()
	if err != nil {
		return nil, err
	}

	app, err := LoadAppConfig("config.yml")
	if err != nil {
		return nil, fmt.Errorf("config.yml: %w", err)
	}

	// SyncInterval comes from env var, not YAML
	syncStr := getEnv("TODOIST_API_SYNC_INTERVAL", "60s")
	syncInterval, err := time.ParseDuration(syncStr)
	if err != nil {
		return nil, fmt.Errorf("TODOIST_API_SYNC_INTERVAL: %w", err)
	}
	if syncInterval < 5*time.Second {
		syncInterval = 5 * time.Second
	}
	app.SyncInterval = syncInterval

	return &Config{Env: env, App: app}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func validateDayParts(parts []DayPartConfig) error {
	for i, p := range parts {
		if p.Label == "" {
			return fmt.Errorf("today.day_parts[%d]: label is required", i)
		}
		if p.Start < 0 || p.Start > 23 {
			return fmt.Errorf("today.day_parts[%d]: start must be 0-23, got %d", i, p.Start)
		}
		if p.End < 0 || p.End > 23 {
			return fmt.Errorf("today.day_parts[%d]: end must be 0-23, got %d", i, p.End)
		}
		if p.Start >= p.End {
			return fmt.Errorf("today.day_parts[%d]: start (%d) must be less than end (%d)", i, p.Start, p.End)
		}
	}
	// Check for overlapping ranges
	for i := 0; i < len(parts); i++ {
		for j := i + 1; j < len(parts); j++ {
			if parts[i].Start < parts[j].End && parts[j].Start < parts[i].End {
				return fmt.Errorf("today.day_parts: ranges [%d,%d) and [%d,%d) overlap",
					parts[i].Start, parts[i].End, parts[j].Start, parts[j].End)
			}
		}
	}
	return nil
}

func validateLabels(labels []LabelConfig) error {
	seen := make(map[string]bool, len(labels))
	for i, l := range labels {
		if l.Name == "" {
			return fmt.Errorf("labels[%d]: name is required", i)
		}
		if seen[l.Name] {
			return fmt.Errorf("labels[%d]: duplicate name %q", i, l.Name)
		}
		seen[l.Name] = true
	}
	return nil
}

func validateLabelProjectMap(mappings []LabelProjectMapping) error {
	for i, m := range mappings {
		if m.Label == "" {
			return fmt.Errorf("label_project_map[%d]: label is required", i)
		}
		if m.Project == "" {
			return fmt.Errorf("label_project_map[%d]: project is required", i)
		}
	}
	return nil
}
