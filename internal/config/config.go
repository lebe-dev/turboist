package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lebe-dev/turboist/internal/auth"
)

type Config struct {
	Timezone   string             `yaml:"timezone"`
	MaxPinned  int                `yaml:"max-pinned"`
	Weekly     WeeklyConfig       `yaml:"weekly"`
	Backlog    BacklogConfig      `yaml:"backlog"`
	Inbox      InboxConfig        `yaml:"inbox"`
	DayParts   map[string]DayPart `yaml:"day-parts"`
	Federation FederationConfig   `yaml:"federation"`

	Location *time.Location `yaml:"-"`
}

// FederationConfig holds the federation sync-worker knobs (Federation v1 F3.2).
// All fields are optional and default to safe values, so a config file without a
// federation block runs unchanged. Later milestones (F4.1 pull interval, F4.4
// backpressure caps, F6.5 retention) extend this block additively.
type FederationConfig struct {
	// PublishIntervalSeconds is the publisher worker's safety-net tick (the
	// commit-ping is what makes push immediate; this is the catch-up backstop).
	// 0 → the default (60s).
	PublishIntervalSeconds int `yaml:"publish-interval-seconds"`

	// PullIntervalSeconds is the recovery loop's catch-up tick (Federation v1
	// F4.1, US-4.1): how often a joined peer is re-pulled from its
	// last_received_hlc cursor so an instance back from a short outage auto-
	// catches-up. Push is the fast path; this is the pull backstop. 0 → 60s.
	PullIntervalSeconds int `yaml:"pull-interval-seconds"`
	// PullBatchLimit caps how many events one recovery pull pass requests from a
	// peer (Federation v1 F4.1). A larger backlog drains over successive passes as
	// the cursor advances. 0 → the default (500), matching the pull handler cap.
	PullBatchLimit int `yaml:"pull-batch-limit"`

	// TombstoneRetentionDays is how long a soft-deleted (tombstoned) federated
	// entity is kept before the retention GC hard-deletes it (Federation v1 F3.3,
	// US-3.7 AC5). The window is the resurrection-safety horizon: a peer returning
	// from longer than this re-snapshots rather than resurrecting a dead entity
	// (§8.2). 0 → the default (90 days, the documented minimum).
	TombstoneRetentionDays int `yaml:"tombstone-retention-days"`
	// OutboxRetentionDays bounds how long delivered/aged federation_outbox rows are
	// kept as the recovery + pull-replay buffer (§16.3 hardcap 30d). 0 → 30 days.
	OutboxRetentionDays int `yaml:"outbox-retention-days"`
	// InboxRetentionDays bounds how long APPLIED federation_inbox rows are kept as
	// the dedup window (un-applied rows are never purged). 0 → 30 days.
	InboxRetentionDays int `yaml:"inbox-retention-days"`

	// InboundRatePerMinute is the per-peer inbound event rate the signed event
	// endpoint accepts before answering 429 + Retry-After (Federation v1 F4.4,
	// US-8.3 AC1). 0 → the default (600/min); a negative value disables limiting.
	InboundRatePerMinute int `yaml:"inbound-rate-per-minute"`
	// InboundBurst is the per-peer token-bucket burst (a short spike a peer may
	// send before throttling). 0 → equal to one minute's steady rate.
	InboundBurst int `yaml:"inbound-burst"`
	// MaxBatchEvents caps how many events one inbound batch may carry before the
	// endpoint answers 413 WHOLE (Federation v1 F4.4, US-8.3 AC3). 0 → 500.
	MaxBatchEvents int `yaml:"max-batch-events"`

	// HandshakeRatePerMinute is the per-peer rate the signed /federation/handshake
	// endpoint accepts before answering 429 + Retry-After (Federation v1 F7.7,
	// NFR-3) — a defence against invite brute-force / handshake-flood DoS. A
	// handshake is rare and trust-establishing, so its budget is tighter than the
	// event endpoint's. 0 → the default (30/min); a negative value disables it.
	HandshakeRatePerMinute int `yaml:"handshake-rate-per-minute"`
	// HandshakeBurst is the per-peer handshake token-bucket burst (a short spike a
	// peer may send before throttling). 0 → the default (5).
	HandshakeBurst int `yaml:"handshake-burst"`

	// OwnerTimeoutDays is how long a JOINED project's owner may go without contact
	// before the joiner flags the project "owner offline" (Federation v1 F5.6a,
	// US-6.5 AC1). Past this window the joiner keeps editing — its edits queue in
	// federation_outbox and flush + LWW-resolve when the owner returns (US-6.5
	// AC2/AC3) — but the UI surfaces a "pending — owner offline" badge so the user
	// knows changes are not yet synced. 0 (or negative) → the default (30 days).
	OwnerTimeoutDays int `yaml:"owner-timeout-days"`

	// AuditRetentionDays bounds how long federation audit-log rows are kept before
	// the nightly GC hard-deletes them (Federation v1 F6.3, US-7.4 AC2). 0 (or
	// negative) → the default (365 days, the documented 1-year retention).
	AuditRetentionDays int `yaml:"audit-retention-days"`
	// AuditAlertThreshold is how many signature failures from ONE peer within the
	// alert window trip the "possible attack on peer X" alert (Federation v1 F6.3,
	// US-7.4 AC3). 0 (or negative) → the default (10).
	AuditAlertThreshold int `yaml:"audit-alert-threshold"`
	// AuditAlertWindowMinutes is the recent window the signature-failure count is
	// taken over for the alert (US-7.4 AC3). 0 (or negative) → the default (60min).
	AuditAlertWindowMinutes int `yaml:"audit-alert-window-minutes"`
}

// FederationTombstoneRetention is the tombstone GC retention window, defaulting to
// 90 days (the §8.2 minimum) when unset. A peer offline longer than this re-
// snapshots instead of resurrecting (US-3.7 AC4/AC5).
func (c *Config) FederationTombstoneRetention() time.Duration {
	if c.Federation.TombstoneRetentionDays <= 0 {
		return 90 * 24 * time.Hour
	}
	return time.Duration(c.Federation.TombstoneRetentionDays) * 24 * time.Hour
}

// FederationOutboxRetention is the outbox recovery/replay buffer window,
// defaulting to and hard-capped at 30 days (§16.3): a larger configured value is
// clamped so the outbox can never grow unbounded.
func (c *Config) FederationOutboxRetention() time.Duration {
	const hardcap = 30 * 24 * time.Hour
	if c.Federation.OutboxRetentionDays <= 0 {
		return hardcap
	}
	d := time.Duration(c.Federation.OutboxRetentionDays) * 24 * time.Hour
	if d > hardcap {
		return hardcap
	}
	return d
}

// FederationInboxRetention is the inbox dedup-window retention, defaulting to 30
// days when unset.
func (c *Config) FederationInboxRetention() time.Duration {
	if c.Federation.InboxRetentionDays <= 0 {
		return 30 * 24 * time.Hour
	}
	return time.Duration(c.Federation.InboxRetentionDays) * 24 * time.Hour
}

// FederationPublishInterval is the publisher worker's ticker interval, defaulting
// to 60s when unset. The commit-ping drives the NFR-1.1 <5s push; this tick only
// catches anything left undelivered (e.g. after a peer outage).
func (c *Config) FederationPublishInterval() time.Duration {
	if c.Federation.PublishIntervalSeconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(c.Federation.PublishIntervalSeconds) * time.Second
}

// FederationPullInterval is the recovery loop's catch-up ticker interval,
// defaulting to 60s when unset (Federation v1 F4.1, US-4.1). It is the pull
// backstop that auto-catches-up a peer after a short offline gap.
func (c *Config) FederationPullInterval() time.Duration {
	if c.Federation.PullIntervalSeconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(c.Federation.PullIntervalSeconds) * time.Second
}

// FederationPullBatchLimit is the per-peer recovery pull batch cap, defaulting to
// 500 when unset (Federation v1 F4.1) — matching the pull handler's own limit
// clamp so a request never asks for more than the handler will serve.
func (c *Config) FederationPullBatchLimit() int {
	if c.Federation.PullBatchLimit <= 0 {
		return 500
	}
	return c.Federation.PullBatchLimit
}

// FederationInboundRatePerMinute is the per-peer inbound event rate the signed
// endpoint accepts before answering 429 (Federation v1 F4.4, US-8.3 AC1),
// defaulting to 600/min when unset. A negative value disables limiting (returns
// 0, which the limiter treats as "always allow").
func (c *Config) FederationInboundRatePerMinute() int {
	if c.Federation.InboundRatePerMinute == 0 {
		return 600
	}
	if c.Federation.InboundRatePerMinute < 0 {
		return 0
	}
	return c.Federation.InboundRatePerMinute
}

// FederationInboundBurst is the per-peer token-bucket burst, defaulting to one
// minute's steady rate when unset so a peer may send up to a minute's worth in a
// spike before throttling (Federation v1 F4.4, US-8.3).
func (c *Config) FederationInboundBurst() int {
	if c.Federation.InboundBurst <= 0 {
		return c.FederationInboundRatePerMinute()
	}
	return c.Federation.InboundBurst
}

// FederationMaxBatchEvents is the inbound events-per-batch ceiling before the
// endpoint answers 413 (Federation v1 F4.4, US-8.3 AC3), defaulting to 500.
func (c *Config) FederationMaxBatchEvents() int {
	if c.Federation.MaxBatchEvents <= 0 {
		return 500
	}
	return c.Federation.MaxBatchEvents
}

// FederationHandshakeRatePerMinute is the per-peer rate the signed handshake
// endpoint accepts before answering 429 (Federation v1 F7.7, NFR-3), defaulting
// to 30/min. A negative value disables handshake rate limiting (Allow always
// permits) so a misconfiguration never locks out legitimate joins.
func (c *Config) FederationHandshakeRatePerMinute() int {
	if c.Federation.HandshakeRatePerMinute == 0 {
		return 30
	}
	if c.Federation.HandshakeRatePerMinute < 0 {
		return -1
	}
	return c.Federation.HandshakeRatePerMinute
}

// FederationHandshakeBurst is the per-peer handshake token-bucket burst (a short
// spike before throttling), defaulting to 5 when unset (Federation v1 F7.7).
func (c *Config) FederationHandshakeBurst() int {
	if c.Federation.HandshakeBurst <= 0 {
		return 5
	}
	return c.Federation.HandshakeBurst
}

// FederationOwnerTimeout is how long a joined project's owner may go without
// contact before the joiner flags it "owner offline" (Federation v1 F5.6a,
// US-6.5 AC1), defaulting to 30 days when unset. The window is generous — far
// longer than the PeerStaleAfter (24h) the owner UI uses for peers — because an
// owner being briefly unreachable is normal; "owner DEAD" (read-only fallback /
// queue-and-resolve) is only declared after a sustained, multi-week silence.
func (c *Config) FederationOwnerTimeout() time.Duration {
	if c.Federation.OwnerTimeoutDays <= 0 {
		return 30 * 24 * time.Hour
	}
	return time.Duration(c.Federation.OwnerTimeoutDays) * 24 * time.Hour
}

// FederationAuditRetention is the federation audit-log retention window,
// defaulting to 365 days (the §US-7.4 AC2 1-year minimum) when unset. The nightly
// GC hard-deletes rows older than this.
func (c *Config) FederationAuditRetention() time.Duration {
	if c.Federation.AuditRetentionDays <= 0 {
		return 365 * 24 * time.Hour
	}
	return time.Duration(c.Federation.AuditRetentionDays) * 24 * time.Hour
}

// FederationAuditAlertThreshold is how many signature failures from one peer in
// the alert window trip the "possible attack on peer X" alert (US-7.4 AC3),
// defaulting to 10 when unset.
func (c *Config) FederationAuditAlertThreshold() int {
	if c.Federation.AuditAlertThreshold <= 0 {
		return 10
	}
	return c.Federation.AuditAlertThreshold
}

// FederationAuditAlertWindow is the recent window the signature-failure count is
// taken over for the alert (US-7.4 AC3), defaulting to 60 minutes when unset.
func (c *Config) FederationAuditAlertWindow() time.Duration {
	if c.Federation.AuditAlertWindowMinutes <= 0 {
		return 60 * time.Minute
	}
	return time.Duration(c.Federation.AuditAlertWindowMinutes) * time.Minute
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
	Bind          string
	LogLevel      string
	BaseURL       string
	JWTSecret     string
	APITokenSalt  string
	TOTPSecretKey string
	// FederationKey is the secret used to derive the TokenCipher that encrypts
	// the federation Ed25519 private seed at rest (Federation v1 F0.3). Optional
	// like TOTPSecretKey; when unset, federation key generation is disabled.
	// Must be ≥32 bytes when set (mirrors JWT_SECRET / TOTP_SECRET_KEY).
	FederationKey string
	DataPath      string
	Argon2Params  auth.Argon2Params

	// Sentry error reporting. All optional; a blank DSN disables that side.
	// SentryDSN drives backend reporting; SentryFrontendDSN is served to the
	// browser via GET /api/config (so it is never baked into the static bundle);
	// SentryEnvironment is shared by both planes.
	SentryDSN         string
	SentryFrontendDSN string
	SentryEnvironment string
}

func LoadEnv() (*Env, error) {
	e := &Env{
		Bind:          os.Getenv("BIND"),
		LogLevel:      os.Getenv("LOG_LEVEL"),
		BaseURL:       os.Getenv("BASE_URL"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		APITokenSalt:  os.Getenv("API_TOKEN_SALT"),
		TOTPSecretKey: os.Getenv("TOTP_SECRET_KEY"),
		FederationKey: os.Getenv("FEDERATION_KEY"),
		DataPath:      os.Getenv("DATA_PATH"),

		SentryDSN:         os.Getenv("SENTRY_DSN"),
		SentryFrontendDSN: os.Getenv("SENTRY_FRONTEND_DSN"),
		SentryEnvironment: os.Getenv("SENTRY_ENVIRONMENT"),
	}
	if e.LogLevel == "" {
		e.LogLevel = "info"
	}
	if e.DataPath == "" {
		e.DataPath = "data/turboist.db"
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
	if len(e.JWTSecret) < 32 {
		return nil, fmt.Errorf("env: JWT_SECRET must be at least 32 bytes")
	}
	if e.APITokenSalt == "" {
		return nil, fmt.Errorf("env: API_TOKEN_SALT is required")
	}
	if len(e.APITokenSalt) < 32 {
		return nil, fmt.Errorf("env: API_TOKEN_SALT must be at least 32 bytes")
	}
	if e.TOTPSecretKey != "" && len(e.TOTPSecretKey) < 32 {
		return nil, fmt.Errorf("env: TOTP_SECRET_KEY must be at least 32 bytes")
	}
	if e.FederationKey != "" && len(e.FederationKey) < 32 {
		return nil, fmt.Errorf("env: FEDERATION_KEY must be at least 32 bytes")
	}
	argon2Params, err := loadArgon2Params()
	if err != nil {
		return nil, err
	}
	e.Argon2Params = argon2Params

	return e, nil
}

func loadArgon2Params() (auth.Argon2Params, error) {
	p := auth.DefaultArgon2Params()

	if v := os.Getenv("ARGON2_MEMORY_KIB"); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil || n == 0 {
			return p, fmt.Errorf("env: ARGON2_MEMORY_KIB must be a positive integer")
		}
		p.Memory = uint32(n)
	}
	if v := os.Getenv("ARGON2_TIME"); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil || n == 0 {
			return p, fmt.Errorf("env: ARGON2_TIME must be a positive integer")
		}
		p.Time = uint32(n)
	}
	if v := os.Getenv("ARGON2_THREADS"); v != "" {
		n, err := strconv.ParseUint(v, 10, 8)
		if err != nil || n == 0 {
			return p, fmt.Errorf("env: ARGON2_THREADS must be a positive integer (1-255)")
		}
		p.Threads = uint8(n)
	}

	return p, nil
}
