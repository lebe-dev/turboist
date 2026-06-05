// Package client implements the outbound peer HTTP client for federation
// (Federation v1). F2.2 lands the minimal signed-request sender the joiner uses
// to deliver a handshake to the owner; the full push/pull/snapshot client lands
// in F3.2 on the same package.
package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// maxResponseBytes caps the handshake response body to defend against an owner
// streaming an unbounded reply.
const maxResponseBytes = 1 << 20 // 1 MiB

// HTTPSender delivers a signed federation request over HTTP. It performs no DB
// access and is safe to call off the request DB connection (R1). It satisfies
// fedsvc.HandshakeSender.
type HTTPSender struct {
	client *http.Client
}

// NewHTTPSender returns an HTTPSender. A nil client uses a default with a 15s
// timeout (the handshake involves a synchronous owner-side .well-known fetch).
func NewHTTPSender(client *http.Client) *HTTPSender {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &HTTPSender{client: client}
}

// Send POSTs the signed request to the owner and returns the status + body. A
// non-2xx status is NOT an error here — the caller maps the status/code to a
// typed error so the owner's rejection reason is preserved.
func (s *HTTPSender) Send(ctx context.Context, sr fedsvc.SignedRequest) (*fedsvc.SignedResponse, error) {
	req, err := http.NewRequestWithContext(ctx, sr.Method, sr.URL, bytes.NewReader(sr.Body))
	if err != nil {
		return nil, fmt.Errorf("build handshake request: %w", err)
	}
	for k, v := range sr.Headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("handshake request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read handshake response: %w", err)
	}
	// Surface Retry-After so the outbox worker can honor a peer's 429 backpressure
	// window verbatim (Federation v1 F4.4, US-4.4 AC1). Other headers are ignored.
	var headers map[string]string
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		headers = map[string]string{"Retry-After": ra}
	}
	return &fedsvc.SignedResponse{StatusCode: resp.StatusCode, Body: body, Headers: headers}, nil
}
