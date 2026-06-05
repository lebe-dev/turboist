// Package transport defines the federation HTTP-signature transport layer:
// the pinned canonical signing string, the request header names, and the
// Ed25519 sign/verify helpers shared by the inbound signature middleware
// (verify) and the outbound peer client (sign) (Federation v1 F0.3).
//
// There is exactly ONE transport signing scheme; the handshake reuses it
// (US-2.2 AC1). A separate per-event payload signature (distinct from this
// request signature) is added in F3.2a — the two are intentionally kept apart.
package transport

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"github.com/lebe-dev/turboist/internal/federation/protocol"
)

// Federation transport request header names (Federation v1 F0.3). The set
// {Instance, Timestamp, Nonce, ProtocolVersion, Digest} plus the request body
// is what the Ed25519 signature covers; Signature carries the signature itself.
//
// HeaderProtocolVer aliases protocol.HeaderProtocolVersion so the version-header
// name has exactly one definition (R23 single-source): protocol owns the name,
// transport keeps it alongside the rest of the signed header set the signature
// middleware reads.
const (
	HeaderInstance    = "X-Federation-Instance"
	HeaderTimestamp   = "X-Federation-Timestamp"
	HeaderNonce       = "X-Federation-Nonce"
	HeaderDigest      = "X-Federation-Digest"
	HeaderSignature   = "X-Federation-Signature"
	HeaderProtocolVer = protocol.HeaderProtocolVersion
)

// SignatureParams are the components of the pinned canonical signing string.
//
// The pinned string is, line-separated (concrete path, NOT the Fiber route
// template — R4):
//
//	METHOD
//	Request().URI().Path()
//	instance_url
//	timestamp
//	nonce
//	protocol_version   (binds X-Federation-Protocol-Version into the signature — anti-downgrade)
//	SHA256(body)       (base64-std; empty body = SHA256(""))
type SignatureParams struct {
	Method          string
	Path            string
	InstanceURL     string
	Timestamp       string
	Nonce           string
	ProtocolVersion string
	BodyDigest      string
}

// BuildSigningString assembles the pinned canonical signing string from params.
func BuildSigningString(p SignatureParams) string {
	return strings.Join([]string{
		p.Method,
		p.Path,
		p.InstanceURL,
		p.Timestamp,
		p.Nonce,
		p.ProtocolVersion,
		p.BodyDigest,
	}, "\n")
}

// BodyDigest returns the base64-std SHA-256 of the request body. An empty or nil
// body yields SHA256("") (the well-defined empty-body digest, NFR-4.3 / R4).
func BodyDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Sign returns the Ed25519 signature over the canonical signing string.
func Sign(priv ed25519.PrivateKey, p SignatureParams) []byte {
	return ed25519.Sign(priv, []byte(BuildSigningString(p)))
}

// SignB64 is Sign with the signature base64-std encoded for the header value.
func SignB64(priv ed25519.PrivateKey, p SignatureParams) string {
	return base64.StdEncoding.EncodeToString(Sign(priv, p))
}

// Verify reports whether sig is a valid Ed25519 signature over the canonical
// signing string under pub. A malformed signature simply returns false.
func Verify(pub ed25519.PublicKey, p SignatureParams, sig []byte) bool {
	if len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, []byte(BuildSigningString(p)), sig)
}
