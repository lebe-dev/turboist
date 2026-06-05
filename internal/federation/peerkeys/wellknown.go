package peerkeys

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WellKnownPath is the path of an instance's federation discovery document,
// mounted publicly before the SPA (Federation v1 F0.3).
const WellKnownPath = "/federation/.well-known/instance"

// wellKnownMaxBytes caps the discovery document size to defend against a peer
// streaming an unbounded body.
const wellKnownMaxBytes = 64 * 1024

// wellKnownDocument is the wire shape of GET /federation/.well-known/instance.
type wellKnownDocument struct {
	InstanceURL      string `json:"instance_url"`
	PublicKey        string `json:"public_key"`
	DisplayName      string `json:"display_name"`
	ProtocolVersions []int  `json:"protocol_versions"`
}

// HTTPFetcher returns a Fetcher that retrieves a peer's .well-known/instance
// document over HTTP using client. The instanceURL is the peer's base URL; the
// well-known path is appended. A nil client uses a default with a 10s timeout.
//
// The fetch performs no DB access and is safe to call off the request DB
// connection (R1 — peer-key fetch must not hold the connection).
func HTTPFetcher(client *http.Client) Fetcher {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return func(ctx context.Context, instanceURL string) (*Instance, error) {
		url := strings.TrimRight(instanceURL, "/") + WellKnownPath
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("build well-known request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("well-known request: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("well-known %q: status %d", url, resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, wellKnownMaxBytes))
		if err != nil {
			return nil, fmt.Errorf("read well-known body: %w", err)
		}
		var doc wellKnownDocument
		if err := json.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("decode well-known body: %w", err)
		}
		if doc.PublicKey == "" {
			return nil, fmt.Errorf("well-known %q: missing public_key", url)
		}
		return &Instance{
			InstanceURL: instanceURL,
			PublicKey:   doc.PublicKey,
			DisplayName: doc.DisplayName,
		}, nil
	}
}
