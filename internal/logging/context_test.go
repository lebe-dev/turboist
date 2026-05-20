package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

type stubCloser struct {
	err    error
	called int
}

func (s *stubCloser) Close() error {
	s.called++
	return s.err
}

func newBufferLogger(t *testing.T, lvl slog.Level) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: lvl})
	return slog.New(h), buf
}

func TestFromContext_NilContextReturnsDefault(t *testing.T) {
	got := FromContext(context.TODO())
	if got == nil {
		t.Fatal("got nil, want default logger")
	}
	if got != slog.Default() {
		t.Errorf("got %p, want default %p", got, slog.Default())
	}
}

func TestFromContext_NoLoggerReturnsDefault(t *testing.T) {
	got := FromContext(context.Background())
	if got != slog.Default() {
		t.Errorf("got %p, want default %p", got, slog.Default())
	}
}

func TestWithLogger_RoundTrip(t *testing.T) {
	log, _ := newBufferLogger(t, slog.LevelDebug)
	ctx := WithLogger(context.Background(), log)
	got := FromContext(ctx)
	if got != log {
		t.Errorf("got %p, want %p", got, log)
	}
}

func TestWithLogger_NilLoggerNoOp(t *testing.T) {
	ctx := WithLogger(context.Background(), nil)
	got := FromContext(ctx)
	if got != slog.Default() {
		t.Errorf("got %p, want default %p", got, slog.Default())
	}
}

func TestLogClose_NilCloserNoOp(t *testing.T) {
	log, buf := newBufferLogger(t, slog.LevelDebug)
	ctx := WithLogger(context.Background(), log)
	LogClose(ctx, "test.op", nil)
	if buf.Len() != 0 {
		t.Errorf("got log output %q, want empty", buf.String())
	}
}

func TestLogClose_SuccessfulCloseNoLog(t *testing.T) {
	log, buf := newBufferLogger(t, slog.LevelDebug)
	ctx := WithLogger(context.Background(), log)
	c := &stubCloser{err: nil}
	LogClose(ctx, "test.op", c)
	if c.called != 1 {
		t.Errorf("close called %d times, want 1", c.called)
	}
	if buf.Len() != 0 {
		t.Errorf("got log output %q, want empty", buf.String())
	}
}

func TestLogClose_ErrorEmitsWarn(t *testing.T) {
	log, buf := newBufferLogger(t, slog.LevelDebug)
	ctx := WithLogger(context.Background(), log)
	c := &stubCloser{err: errors.New("boom")}
	LogClose(ctx, "test.op", c)
	if c.called != 1 {
		t.Errorf("close called %d times, want 1", c.called)
	}
	if buf.Len() == 0 {
		t.Fatal("got no log output, want WARN record")
	}
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("parse log: %v", err)
	}
	if lvl, _ := rec["level"].(string); !strings.EqualFold(lvl, "WARN") {
		t.Errorf("level: got %v, want WARN", rec["level"])
	}
	if op, _ := rec["op"].(string); op != "test.op" {
		t.Errorf("op: got %v, want test.op", rec["op"])
	}
	if errMsg, _ := rec["err"].(string); errMsg != "boom" {
		t.Errorf("err: got %v, want boom", rec["err"])
	}
}

func TestLogClose_UsesDefaultLoggerWhenContextHasNone(t *testing.T) {
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	c := &stubCloser{err: errors.New("boom")}
	LogClose(context.Background(), "default.op", c)
	if buf.Len() == 0 {
		t.Fatal("got no log output, want WARN record on default logger")
	}
	if !strings.Contains(buf.String(), "default.op") {
		t.Errorf("output %q missing op", buf.String())
	}
}
