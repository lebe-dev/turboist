package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type EnvConfig struct {
	Bind          string
	LogLevel      string
	BaseURL       string
	TodoistAPIKey string
	AdminPassword string
}

type Config struct {
	Env          EnvConfig
	PollInterval time.Duration
	Contexts     map[string]ContextConfig
	Weekly       WeeklyConfig
	NextWeek     NextWeekConfig
	AutoExpire   []AutoExpireConfig
}

type ContextConfig struct {
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
	Label string        `yaml:"label"`
	TTL   time.Duration `yaml:"ttl"`
}

type yamlFile struct {
	PollInterval string                    `yaml:"poll_interval"`
	Contexts     map[string]ContextConfig  `yaml:"contexts"`
	Weekly       WeeklyConfig              `yaml:"weekly"`
	NextWeek     NextWeekConfig            `yaml:"next_week"`
	AutoExpire   []yamlAutoExpire          `yaml:"auto_expire"`
}

type yamlAutoExpire struct {
	Label string `yaml:"label"`
	TTL   string `yaml:"ttl"`
}

func loadEnv() (EnvConfig, error) {
	env := EnvConfig{
		Bind:          getEnv("BIND", ":8080"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
		BaseURL:       getEnv("BASE_URL", "http://localhost:8080"),
		TodoistAPIKey: os.Getenv("TODOIST_API_KEY"),
		AdminPassword: os.Getenv("TURBOIST_ADMIN_PASSWORD"),
	}
	if env.TodoistAPIKey == "" {
		return env, fmt.Errorf("TODOIST_API_KEY is required")
	}
	if env.AdminPassword == "" {
		return env, fmt.Errorf("TURBOIST_ADMIN_PASSWORD is required")
	}
	return env, nil
}

func Load() (*Config, error) {
	env, err := loadEnv()
	if err != nil {
		return nil, err
	}

	cfg := &Config{Env: env}

	yf, err := loadYAML("config.yml")
	if err != nil {
		return nil, fmt.Errorf("config.yml: %w", err)
	}

	cfg.PollInterval, err = time.ParseDuration(yf.PollInterval)
	if err != nil {
		return nil, fmt.Errorf("poll_interval: %w", err)
	}

	cfg.Contexts = yf.Contexts
	cfg.Weekly = yf.Weekly
	cfg.NextWeek = yf.NextWeek


	for _, ae := range yf.AutoExpire {
		ttl, err := time.ParseDuration(ae.TTL)
		if err != nil {
			return nil, fmt.Errorf("auto_expire ttl for %q: %w", ae.Label, err)
		}
		cfg.AutoExpire = append(cfg.AutoExpire, AutoExpireConfig{
			Label: ae.Label,
			TTL:   ttl,
		})
	}

	return cfg, nil
}

func loadYAML(path string) (*yamlFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var yf yamlFile
	if err := yaml.Unmarshal(data, &yf); err != nil {
		return nil, err
	}
	return &yf, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
