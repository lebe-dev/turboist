package config

import (
	"fmt"
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

type AppConfig struct {
	PollInterval time.Duration
	Contexts     []ContextConfig
	Weekly       WeeklyConfig
	NextWeek     NextWeekConfig
	AutoExpire   []AutoExpireConfig
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

type Config struct {
	Env EnvConfig
	App AppConfig
}

type ContextConfig struct {
	ID          string         `yaml:"id"`
	DisplayName string         `yaml:"display_name"`
	Filters     ContextFilters `yaml:"filters"`
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

type NextWeekConfig struct {
	Label string `yaml:"label"`
}

type AutoExpireConfig struct {
	Label string
	TTL   time.Duration
}

type yamlFile struct {
	PollInterval string           `yaml:"poll_interval"`
	Contexts     []ContextConfig  `yaml:"contexts"`
	Weekly       WeeklyConfig     `yaml:"weekly"`
	NextWeek     NextWeekConfig   `yaml:"next_week"`
	AutoExpire   []yamlAutoExpire `yaml:"auto_expire"`
}

type yamlAutoExpire struct {
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

	app := AppConfig{
		PollInterval: pollInterval,
		Contexts:     yf.Contexts,
		Weekly:       yf.Weekly,
		NextWeek:     yf.NextWeek,
	}

	for _, ae := range yf.AutoExpire {
		ttl, err := time.ParseDuration(ae.TTL)
		if err != nil {
			return AppConfig{}, fmt.Errorf("auto_expire ttl for %q: %w", ae.Label, err)
		}
		app.AutoExpire = append(app.AutoExpire, AutoExpireConfig{
			Label: ae.Label,
			TTL:   ttl,
		})
	}

	return app, nil
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

	return &Config{Env: env, App: app}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
