package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestNew_LevelParsing(t *testing.T) {
	cases := []struct {
		name     string
		level    string
		enabled  []slog.Level
		disabled []slog.Level
	}{
		{
			name:     "debug",
			level:    "debug",
			enabled:  []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError},
			disabled: nil,
		},
		{
			name:     "uppercase_debug",
			level:    "DEBUG",
			enabled:  []slog.Level{slog.LevelDebug, slog.LevelInfo},
			disabled: nil,
		},
		{
			name:     "warn_with_whitespace",
			level:    " warn ",
			enabled:  []slog.Level{slog.LevelWarn, slog.LevelError},
			disabled: []slog.Level{slog.LevelDebug, slog.LevelInfo},
		},
		{
			name:     "warning_alias",
			level:    "warning",
			enabled:  []slog.Level{slog.LevelWarn, slog.LevelError},
			disabled: []slog.Level{slog.LevelDebug, slog.LevelInfo},
		},
		{
			name:     "error",
			level:    "error",
			enabled:  []slog.Level{slog.LevelError},
			disabled: []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn},
		},
		{
			name:     "empty_defaults_to_info",
			level:    "",
			enabled:  []slog.Level{slog.LevelInfo, slog.LevelWarn, slog.LevelError},
			disabled: []slog.Level{slog.LevelDebug},
		},
		{
			name:     "unknown_defaults_to_info",
			level:    "trace",
			enabled:  []slog.Level{slog.LevelInfo, slog.LevelWarn, slog.LevelError},
			disabled: []slog.Level{slog.LevelDebug},
		},
	}

	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := New(tc.level)
			if logger == nil {
				t.Fatal("got nil logger")
			}
			for _, lvl := range tc.enabled {
				if !logger.Enabled(ctx, lvl) {
					t.Errorf("level %v: got disabled, want enabled", lvl)
				}
			}
			for _, lvl := range tc.disabled {
				if logger.Enabled(ctx, lvl) {
					t.Errorf("level %v: got enabled, want disabled", lvl)
				}
			}
		})
	}
}
