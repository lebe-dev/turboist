package protocol

import (
	"errors"
	"testing"
)

// TestNegotiate_MaxIntersect asserts Negotiate picks the highest version present
// in BOTH the local and peer sets (US-9.1 AC1 — negotiate the max common version).
func TestNegotiate_MaxIntersect(t *testing.T) {
	cases := []struct {
		name  string
		local []int
		peer  []int
		want  int
	}{
		{name: "single common v1", local: []int{1}, peer: []int{1}, want: 1},
		{name: "peer ahead, common is 1", local: []int{1}, peer: []int{1, 2, 3}, want: 1},
		{name: "local ahead, common is 1", local: []int{1, 2}, peer: []int{1}, want: 1},
		{name: "max common is 2", local: []int{1, 2}, peer: []int{2, 3}, want: 2},
		{name: "max common is 3 not 4", local: []int{1, 3, 4}, peer: []int{2, 3}, want: 3},
		{name: "unordered input still picks max", local: []int{3, 1, 2}, peer: []int{2, 1, 3}, want: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Negotiate(tc.local, tc.peer)
			if err != nil {
				t.Fatalf("Negotiate(%v, %v): unexpected error: %v", tc.local, tc.peer, err)
			}
			if got != tc.want {
				t.Errorf("Negotiate(%v, %v): got %d, want %d", tc.local, tc.peer, got, tc.want)
			}
		})
	}
}

// TestNegotiate_NoOverlap asserts that an empty intersection returns the
// sentinel ErrNoVersionOverlap (the handshake maps it to a 400
// federation_version_unsupported and consumes NO invite — US-9.1 AC2).
func TestNegotiate_NoOverlap(t *testing.T) {
	cases := []struct {
		name  string
		local []int
		peer  []int
	}{
		{name: "disjoint", local: []int{1}, peer: []int{2, 3}},
		{name: "empty peer", local: []int{1}, peer: nil},
		{name: "empty local", local: nil, peer: []int{1}},
		{name: "both empty", local: nil, peer: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Negotiate(tc.local, tc.peer)
			if !errors.Is(err, ErrNoVersionOverlap) {
				t.Fatalf("Negotiate(%v, %v): got err %v, want ErrNoVersionOverlap", tc.local, tc.peer, err)
			}
			if got != 0 {
				t.Errorf("Negotiate(%v, %v): got version %d, want 0 on no-overlap", tc.local, tc.peer, got)
			}
		})
	}
}

// TestNegotiate_UsesSupportedAsLocal asserts the convenience that negotiating
// against this build's advertised set resolves to v1 (the only v1 version), so
// the handshake can call Negotiate(SupportedProtocolVersions, peer) directly.
func TestNegotiate_UsesSupportedAsLocal(t *testing.T) {
	got, err := Negotiate(SupportedProtocolVersions, []int{1, 99})
	if err != nil {
		t.Fatalf("Negotiate against SupportedProtocolVersions: %v", err)
	}
	if got != 1 {
		t.Errorf("negotiated version: got %d, want 1", got)
	}
}

// TestSupportedProtocolVersions_SingleSource is the drift regression guard: the
// supported-versions constant has exactly one source of truth and is [1] for
// the v1 release. If a later version is added, this test is the canary that the
// const moved as expected (R23 — one source).
func TestSupportedProtocolVersions_SingleSource(t *testing.T) {
	if len(SupportedProtocolVersions) != 1 || SupportedProtocolVersions[0] != 1 {
		t.Fatalf("SupportedProtocolVersions: got %v, want [1] (single source, v1)", SupportedProtocolVersions)
	}
	// The header name is likewise the single source the signature middleware and
	// .well-known both read; it must be the X-Federation-Protocol-Version header.
	if HeaderProtocolVersion != "X-Federation-Protocol-Version" {
		t.Errorf("HeaderProtocolVersion: got %q, want X-Federation-Protocol-Version", HeaderProtocolVersion)
	}
}

// TestProtocolVersionHeader_RoundTrip asserts the header value helpers
// round-trip an int version through its on-the-wire string form, and that the
// reader rejects malformed / unknown values.
func TestProtocolVersionHeader_RoundTrip(t *testing.T) {
	for _, v := range []int{1, 2, 42} {
		s := FormatVersion(v)
		got, err := ParseVersion(s)
		if err != nil {
			t.Fatalf("ParseVersion(FormatVersion(%d)) = err %v", v, err)
		}
		if got != v {
			t.Errorf("round-trip version: got %d, want %d", got, v)
		}
	}

	for _, bad := range []string{"", "abc", "1.0", " 1", "1 ", "-1", "0", "0x1"} {
		if _, err := ParseVersion(bad); err == nil {
			t.Errorf("ParseVersion(%q): want error, got nil", bad)
		}
	}
}
