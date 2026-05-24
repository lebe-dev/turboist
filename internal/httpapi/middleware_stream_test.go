package httpapi

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

// TestAccessLogMiddleware_DoesNotDrainStreamingBody verifies the access log
// middleware does not call Response.Body() on a streaming response, which
// would synchronously drain the stream and block until it ends — turning a
// long-lived SSE handler into a deadlock that prevents fasthttp from writing
// headers to the client.
func TestAccessLogMiddleware_DoesNotDrainStreamingBody(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := fiber.New()
	app.Use(AccessLogMiddleware(logger))

	streamEntered := make(chan struct{})
	streamCancel := make(chan struct{})

	app.Get("/stream", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		return c.SendStreamWriter(func(w *bufio.Writer) {
			close(streamEntered)
			if _, err := w.WriteString(": hello\n\n"); err != nil {
				return
			}
			if err := w.Flush(); err != nil {
				return
			}
			<-streamCancel
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 2 * time.Second})
	if err != nil {
		close(streamCancel)
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		close(streamCancel)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != 200 {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type: want text/event-stream, got %q", ct)
	}

	select {
	case <-streamEntered:
	case <-time.After(1 * time.Second):
		t.Fatal("stream writer was never invoked — middleware likely deadlocked draining the body")
	}

	// Verify the first chunk is on the wire even though the writer hasn't
	// returned. ReadBytes guards against the alternative regression where the
	// middleware silently buffers the entire stream into memory before reply.
	buf := make([]byte, 32)
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		readDone := make(chan struct{})
		var n int
		var rerr error
		go func() {
			n, rerr = resp.Body.Read(buf)
			close(readDone)
		}()
		select {
		case <-readDone:
		case <-ctx.Done():
		}
		cancel()
		if n > 0 {
			if !strings.Contains(string(buf[:n]), "hello") {
				t.Fatalf("unexpected first chunk: %q", buf[:n])
			}
			return
		}
		if rerr != nil {
			t.Fatalf("read: %v", rerr)
		}
	}
	t.Fatal("did not receive streamed chunk within deadline")
}
