package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Timezone   string             `yaml:"timezone"`
	MaxPinned  int                `yaml:"max-pinned"`
	Weekly     WeeklyConfig       `yaml:"weekly"`
	Backlog    BacklogConfig      `yaml:"backlog"`
	Inbox      InboxConfig        `yaml:"inbox"`
	DayParts   map[string]DayPart `yaml:"day-parts"`
	AutoLabels []AutoLabel        `yaml:"auto-labels"`

	Location *time.Location `yaml:"-"`
}

type WeeklyConfig struct {
	Limit int `yaml:"limit"`
}

type BacklogConfig struct {
	Limit int `yaml:"limit"`
}

type InboxConfig struct {
	WarnThreshold int          `yaml:"warn-threshold"`
	OverflowTask  OverflowTask `yaml:"overflow-task"`
}

type OverflowTask struct {
	Title    string `yaml:"title"`
	Priority string `yaml:"priority"`
}

type DayPart struct {
	Start int `yaml:"start"`
	End   int `yaml:"end"`
}

type AutoLabel struct {
	Mask       string `yaml:"mask"`
	Label      string `yaml:"label"`
	IgnoreCase *bool  `yaml:"ignore-case,omitempty"`
}

func (a AutoLabel) IgnoreCaseValue() bool {
	if a.IgnoreCase == nil {
		return true
	}
	return *a.IgnoreCase
}

var validPriorities = map[string]struct{}{
	"high": {}, "medium": {}, "low": {}, "no-priority": {},
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Timezone == "" {
		return fmt.Errorf("config: timezone is required")
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return fmt.Errorf("config: invalid timezone %q: %w", c.Timezone, err)
	}
	c.Location = loc

	if c.MaxPinned <= 0 {
		return fmt.Errorf("config: max-pinned must be > 0")
	}
	if c.Weekly.Limit <= 0 {
		return fmt.Errorf("config: weekly.limit must be > 0")
	}
	if c.Backlog.Limit <= 0 {
		return fmt.Errorf("config: backlog.limit must be > 0")
	}
	if c.Inbox.WarnThreshold <= 0 {
		return fmt.Errorf("config: inbox.warn-threshold must be > 0")
	}
	if c.Inbox.OverflowTask.Title == "" {
		return fmt.Errorf("config: inbox.overflow-task.title is required")
	}
	if _, ok := validPriorities[c.Inbox.OverflowTask.Priority]; !ok {
		return fmt.Errorf("config: inbox.overflow-task.priority %q is not a valid priority", c.Inbox.OverflowTask.Priority)
	}

	if err := validateDayParts(c.DayParts); err != nil {
		return err
	}

	for i, al := range c.AutoLabels {
		if al.Label == "" {
			return fmt.Errorf("config: auto-labels[%d].label must not be empty", i)
		}
		if al.Mask == "" {
			return fmt.Errorf("config: auto-labels[%d].mask must not be empty", i)
		}
	}

	return nil
}

func validateDayParts(parts map[string]DayPart) error {
	if len(parts) == 0 {
		return fmt.Errorf("config: day-parts must not be empty")
	}
	type interval struct {
		name       string
		start, end int
	}
	intervals := make([]interval, 0, len(parts))
	for name, p := range parts {
		if p.Start < 0 || p.Start > 24 || p.End < 0 || p.End > 24 {
			return fmt.Errorf("config: day-parts.%s out of [0,24]", name)
		}
		if p.Start >= p.End {
			return fmt.Errorf("config: day-parts.%s start must be < end", name)
		}
		intervals = append(intervals, interval{name, p.Start, p.End})
	}
	for i := 0; i < len(intervals); i++ {
		for j := i + 1; j < len(intervals); j++ {
			a, b := intervals[i], intervals[j]
			if a.start < b.end && b.start < a.end {
				return fmt.Errorf("config: day-parts %s and %s overlap", a.name, b.name)
			}
		}
	}
	return nil
}

type Env struct {
	Bind      string
	LogLevel  string
	BaseURL   string
	JWTSecret string
}

func LoadEnv() (*Env, error) {
	e := &Env{
		Bind:      os.Getenv("BIND"),
		LogLevel:  os.Getenv("LOG_LEVEL"),
		BaseURL:   os.Getenv("BASE_URL"),
		JWTSecret: os.Getenv("JWT_SECRET"),
	}
	if e.LogLevel == "" {
		e.LogLevel = "info"
	}
	if e.Bind == "" {
		return nil, fmt.Errorf("env: BIND is required")
	}
	if e.BaseURL == "" {
		return nil, fmt.Errorf("env: BASE_URL is required")
	}
	if e.JWTSecret == "" {
		return nil, fmt.Errorf("env: JWT_SECRET is required")
	}
	return e, nil
}
