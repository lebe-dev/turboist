// Package protocol owns federation protocol version negotiation (Federation v1).
//
// F0.3 lands the stable constants the trust plane depends on: the single source
// of supported versions and the signed X-Federation-Protocol-Version header
// name (so the signature middleware can include it in the signed header set —
// anti-downgrade). The Negotiate() max-intersection logic and header read/write
// helpers are added by F0.4 (additively), which is why those live in this same
// package rather than being inlined into the middleware.
package protocol

import (
	"errors"
	"strconv"
)

// ErrNoVersionOverlap is returned by Negotiate when the local and peer
// supported-version sets share no common version. The handshake (F2.2) maps it
// to a 400 federation_version_unsupported and consumes NO invite — the reject
// happens BEFORE the invite-consume transaction (US-9.1 AC2 / R23 atomicity).
var ErrNoVersionOverlap = errors.New("federation: no common protocol version")

// HeaderProtocolVersion is the SINGLE definition of the request header carrying
// the sender's chosen federation protocol version. The signature middleware
// reads it via transport.HeaderProtocolVer, which aliases this constant; it is
// part of the signed transport header set (Federation v1 F0.3) so a
// man-in-the-middle cannot strip or downgrade it without invalidating the
// Ed25519 signature.
const HeaderProtocolVersion = "X-Federation-Protocol-Version"

// SupportedProtocolVersions is the SINGLE source of truth for the federation
// protocol versions this build speaks (R23 — one source, drift-guarded). v1 is
// the only version in the v1 release. F0.4's Negotiate() intersects this with a
// peer's advertised set.
var SupportedProtocolVersions = []int{1}

// Negotiate returns the highest version present in BOTH the local and peer
// supported-version sets (US-9.1 AC1 — pick the max common version). When the
// intersection is empty it returns 0 and ErrNoVersionOverlap; the handshake
// must reject on that error before consuming the invite (R23 atomicity).
//
// Order of the inputs is irrelevant; the result is the numeric maximum of the
// intersection.
func Negotiate(local, peer []int) (int, error) {
	peerSet := make(map[int]struct{}, len(peer))
	for _, v := range peer {
		peerSet[v] = struct{}{}
	}

	best := 0
	found := false
	for _, v := range local {
		if _, ok := peerSet[v]; !ok {
			continue
		}
		if !found || v > best {
			best = v
			found = true
		}
	}

	if !found {
		return 0, ErrNoVersionOverlap
	}
	return best, nil
}

// FormatVersion renders a protocol version as its on-the-wire string form for
// the X-Federation-Protocol-Version header.
func FormatVersion(v int) string {
	return strconv.Itoa(v)
}

// ParseVersion parses the X-Federation-Protocol-Version header value into an
// int. It rejects empty, non-numeric, signed, zero, and surrounding-whitespace
// values so the signature middleware never feeds a malformed version into the
// signed canonical string (protocol versions are positive integers).
func ParseVersion(s string) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if v < 1 {
		return 0, errors.New("federation: protocol version must be a positive integer")
	}
	return v, nil
}
