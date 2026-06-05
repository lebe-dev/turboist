package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/outbox"
	"github.com/lebe-dev/turboist/internal/federation/protocol"
	"github.com/lebe-dev/turboist/internal/federation/transport"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// codeFederationStalePull mirrors httpapi.CodeFederationStalePull — the error
// code the owner's pull endpoint returns in a 410 body when the caller's cursor
// predates retained history (F3.3). It is duplicated here (rather than imported)
// to keep the service layer free of an httpapi dependency; a drift guard test
// asserts the two stay equal.
const codeFederationStalePull = "federation_stale_pull"

// Publisher is the outbound side of the F3.2 sync core (US-3.1/US-3.2). It
// resolves a project's delivery targets (PeersForProject — satisfies
// outbox.PeerLister) and signs+POSTs a batch of canonical events to a peer's
// /federation/events endpoint (Push — satisfies outbox.Pusher), and pulls a
// peer's catch-up events (Pull, used by the recovery loop in F4.1).
//
// Every outbound request is signed with the SINGLE pinned transport signature
// (the same construction as the handshake/snapshot) — there is no second signing
// scheme. The signing key is loaded once per call and no DB connection is held
// across the network I/O (R1): peer resolution reads the connection, then the
// HTTP POST runs with nothing held, then the worker takes a short tx to stamp
// delivery.
type Publisher struct {
	fedProjects *repo.FederatedProjectRepo
	keys        *repo.FederationKeysRepo
	cipher      *crypto.TokenCipher
	sender      HandshakeSender
	instanceURL string
	now         func() time.Time
}

// NewPublisher constructs the outbound publisher. sender is the signed-request
// HTTP client (shared with the handshake/snapshot); now may be nil.
func NewPublisher(fedProjects *repo.FederatedProjectRepo, keys *repo.FederationKeysRepo, cipher *crypto.TokenCipher, sender HandshakeSender, instanceURL string, now func() time.Time) *Publisher {
	if now == nil {
		now = time.Now
	}
	return &Publisher{
		fedProjects: fedProjects,
		keys:        keys,
		cipher:      cipher,
		sender:      sender,
		instanceURL: instanceURL,
		now:         now,
	}
}

// PeersForProject returns the non-revoked, non-left, non-paused, non-self delivery
// targets for a project (outbox.PeerLister). The owner self-row (is_owner=1, peer
// == this instance) is excluded so we never deliver to ourselves; a revoked peer is
// excluded permanently from fan-out (US-5.2 AC1); a peer that voluntarily LEFT is
// excluded permanently too (Federation v1 F5.5, US-6.3 AC2 — the owner stops
// sending to it, and any already-queued events for it never go out); a PAUSED peer
// is excluded temporarily (Federation v1 F5.3, US-6.1 AC1) — its row stays,
// paused=1, so its events accumulate in federation_outbox (delivered_to unstamped)
// and flush on resume. Read peers still receive fan-out (US-5.1 AC3); the read grant
// only constrains their own local edits, so permission is NOT filtered here.
func (p *Publisher) PeersForProject(ctx context.Context, localProjectID int64) ([]outbox.Peer, error) {
	rows, err := p.fedProjects.ListByProject(ctx, localProjectID)
	if err != nil {
		return nil, fmt.Errorf("publisher list peers: %w", err)
	}
	out := make([]outbox.Peer, 0, len(rows))
	for _, fp := range rows {
		if fp.IsOwner || fp.Revoked || fp.Lost || fp.Paused {
			continue
		}
		if fp.PeerInstanceURL == p.instanceURL {
			continue
		}
		out = append(out, outbox.Peer{InstanceURL: fp.PeerInstanceURL})
	}
	return out, nil
}

// Push signs and POSTs a batch of canonical event payloads to peerURL's
// /federation/events (outbox.Pusher). The payloads are the verbatim outbox bytes
// (the per-event signature is over them); they are wrapped in the Batch envelope.
// A non-2xx response is an error so the worker leaves the batch pending for
// retry (per-peer isolation, US-3.2 AC3).
func (p *Publisher) Push(ctx context.Context, peerURL string, payloads []string) error {
	if len(payloads) == 0 {
		return nil
	}
	body, err := encodeBatch(payloads)
	if err != nil {
		return err
	}
	target := trimSlash(peerURL) + events.PushPath
	resp, err := p.signedSend(ctx, "POST", target, events.PushPath, body)
	if err != nil {
		return fmt.Errorf("push to %s: %w", peerURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return p.remoteError(resp)
	}
	return nil
}

// remoteError builds the classified push/pull rejection from a non-2xx response.
// A 429 carries its Retry-After window so the outbox worker honors the peer's
// backpressure verbatim (Federation v1 F4.4, US-4.4 AC1); other statuses carry
// only the federation error code (read by the dead-letter / permanent seams).
func (p *Publisher) remoteError(resp *SignedResponse) *RemoteHandshakeError {
	e := &RemoteHandshakeError{StatusCode: resp.StatusCode, Code: errorCodeOf(resp.Body)}
	if resp.StatusCode == 429 && resp.Headers != nil {
		if d, ok := parseRetryAfter(resp.Headers["Retry-After"], p.now()); ok {
			e.retryAfter = d
			e.hasRetryAfter = true
		}
	}
	return e
}

// Pull GETs a peer's catch-up events from sinceHLC (US-4.1 recovery / US-3.2 AC3
// pull replay). The since_hlc rides as a query param; the signed path is the
// concrete request path WITHOUT the query (R4). It returns the decoded events
// (caller feeds them to the inbox-apply path) and the cursor to advance to.
func (p *Publisher) Pull(ctx context.Context, peerURL, remoteProjectID, sinceHLC string, limit int) (*events.PullResponse, error) {
	full, path, err := pullRequestURL(peerURL, remoteProjectID, sinceHLC, limit)
	if err != nil {
		return nil, err
	}
	resp, err := p.signedSend(ctx, "GET", full, path, nil)
	if err != nil {
		return nil, fmt.Errorf("pull from %s: %w", peerURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// A 410 federation_stale_pull means the cursor predates the owner's retained
		// history: surface the typed StalePullError carrying {snapshot_url, as_of_hlc}
		// so the recovery loop drives the F4.2 re-bootstrap consumer instead of just
		// re-pulling the same stale range forever (US-3.7 AC4 consume half).
		code := errorCodeOf(resp.Body)
		if resp.StatusCode == 410 && code == codeFederationStalePull {
			snapshotURL, asOfHLC := stalePullDetailsOf(resp.Body)
			if snapshotURL != "" {
				return nil, &events.StalePullError{SnapshotURL: snapshotURL, AsOfHLC: asOfHLC}
			}
		}
		return nil, &RemoteHandshakeError{StatusCode: resp.StatusCode, Code: code}
	}
	var out events.PullResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode pull response: %w", err)
	}
	return &out, nil
}

// signedSend signs (the pinned transport string) and sends an outbound request.
// body may be nil for a GET (empty-body digest = SHA256("")).
func (p *Publisher) signedSend(ctx context.Context, method, fullURL, path string, body []byte) (*SignedResponse, error) {
	if p.sender == nil {
		return nil, fmt.Errorf("federation: no sender configured")
	}
	keys, err := p.keys.Ensure(ctx, p.cipher, defaultInstanceDisplayName(p.instanceURL))
	if err != nil {
		return nil, fmt.Errorf("ensure federation keys: %w", err)
	}
	priv, _, err := crypto.LoadInstanceKeypair(p.cipher, keys.PublicKey, keys.PrivateSeedEnc)
	if err != nil {
		return nil, fmt.Errorf("load instance keypair: %w", err)
	}

	nonce, err := newNonce()
	if err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	ts := model.FormatUTC(p.now())
	ver := protocol.FormatVersion(defaultProtocolVersion())
	digest := transport.BodyDigest(body)
	sigParams := transport.SignatureParams{
		Method:          method,
		Path:            path,
		InstanceURL:     p.instanceURL,
		Timestamp:       ts,
		Nonce:           nonce,
		ProtocolVersion: ver,
		BodyDigest:      digest,
	}
	headers := map[string]string{
		transport.HeaderInstance:    p.instanceURL,
		transport.HeaderTimestamp:   ts,
		transport.HeaderNonce:       nonce,
		transport.HeaderProtocolVer: ver,
		transport.HeaderDigest:      digest,
		transport.HeaderSignature:   transport.SignB64(priv, sigParams),
	}
	if body != nil {
		headers["Content-Type"] = "application/json"
	}
	return p.sender.Send(ctx, SignedRequest{
		Method:  method,
		URL:     fullURL,
		Path:    path,
		Headers: headers,
		Body:    body,
	})
}

// encodeBatch wraps the verbatim event payloads in the Batch envelope. The
// payloads are raw JSON of each canonical signed event; they are embedded as-is
// so the receiver verifies each event's signature over the same bytes the origin
// signed (F3.2a — sign/verify over the received canonical bytes).
func encodeBatch(payloads []string) ([]byte, error) {
	raws := make([]json.RawMessage, len(payloads))
	for i, pl := range payloads {
		raws[i] = json.RawMessage(pl)
	}
	wire := struct {
		Events []json.RawMessage `json:"events"`
	}{Events: raws}
	b, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("encode event batch: %w", err)
	}
	return b, nil
}

// pullRequestURL builds the full pull URL (with since_hlc + limit query params)
// and the concrete signed path (without the query, R4). remoteProjectID is the
// owner's project id segment in the route.
func pullRequestURL(peerURL, remoteProjectID, sinceHLC string, limit int) (full, path string, err error) {
	base := trimSlash(peerURL)
	path = fmt.Sprintf("/federation/projects/%s/events", remoteProjectID)
	u, err := url.Parse(base + path)
	if err != nil {
		return "", "", fmt.Errorf("parse pull url: %w", err)
	}
	q := u.Query()
	if sinceHLC != "" {
		q.Set("since_hlc", sinceHLC)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	u.RawQuery = q.Encode()
	return u.String(), u.Path, nil
}
