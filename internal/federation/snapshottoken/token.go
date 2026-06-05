// Package snapshottoken mints and verifies the short-lived (15-minute) bearer
// token an owner hands a freshly-joined peer so it can pull the project snapshot
// (Federation v1 F2.2, consumed in F2.3, US-2.3 AC4).
//
// The token is Ed25519-signed by the owner's federation key over a canonical
// claims object {project_id, exp} — the same separate trust plane as every other
// federation signature. The owner verifies it on the F2.3 snapshot endpoint; an
// expired token is rejected (US-2.3 AC4). Format is `<base64url(claims)>.<base64url(sig)>`.
package snapshottoken

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TTL is the lifetime of a snapshot token (US-2.3 AC4 — expired → 401).
const TTL = 15 * time.Minute

// ErrExpired is returned by Verify when the token's exp is in the past.
var ErrExpired = errors.New("snapshot token expired")

// ErrInvalid is returned by Verify when the token is malformed or its signature
// does not verify under the owner's public key.
var ErrInvalid = errors.New("snapshot token invalid")

// claims is the signed payload. Times are unix-millis so the token is compact
// and clock comparison is unambiguous.
type claims struct {
	ProjectID int64 `json:"project_id"`
	ExpMS     int64 `json:"exp_ms"`
}

// Mint returns a snapshot token for projectID, signed by priv and expiring TTL
// after now. now is injected (not time.Now) so the minting and verifying clocks
// can be controlled in tests and stay coherent with the rest of a request.
func Mint(priv ed25519.PrivateKey, projectID int64, now time.Time) (string, error) {
	payload, err := json.Marshal(claims{ProjectID: projectID, ExpMS: now.Add(TTL).UnixMilli()})
	if err != nil {
		return "", fmt.Errorf("marshal snapshot claims: %w", err)
	}
	sig := ed25519.Sign(priv, payload)
	return encode(payload) + "." + encode(sig), nil
}

// Verify checks the token's signature under pub and that it has not expired at
// now, returning the embedded project id. A bad signature or malformed token
// returns ErrInvalid; a well-formed but expired token returns ErrExpired so the
// snapshot endpoint can map it to a 401 (US-2.3 AC4).
func Verify(pub ed25519.PublicKey, token string, now time.Time) (int64, error) {
	payload, sig, ok := split(token)
	if !ok {
		return 0, ErrInvalid
	}
	if !ed25519.Verify(pub, payload, sig) {
		return 0, ErrInvalid
	}
	var c claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return 0, ErrInvalid
	}
	if now.UnixMilli() > c.ExpMS {
		return c.ProjectID, ErrExpired
	}
	return c.ProjectID, nil
}

func split(token string) (payload, sig []byte, ok bool) {
	dot := strings.IndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 {
		return nil, nil, false
	}
	p, err := decode(token[:dot])
	if err != nil {
		return nil, nil, false
	}
	s, err := decode(token[dot+1:])
	if err != nil || len(s) != ed25519.SignatureSize {
		return nil, nil, false
	}
	return p, s, true
}

func encode(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func decode(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }
