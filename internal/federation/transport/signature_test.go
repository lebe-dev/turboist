package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/protocol"
)

// TestHeaderProtocolVer_SingleSource asserts the transport version-header
// constant is exactly the protocol package's single definition (R23), guarding
// against a future re-hardcoded literal silently drifting from it.
func TestHeaderProtocolVer_SingleSource(t *testing.T) {
	if HeaderProtocolVer != protocol.HeaderProtocolVersion {
		t.Errorf("HeaderProtocolVer: got %q, want protocol.HeaderProtocolVersion %q",
			HeaderProtocolVer, protocol.HeaderProtocolVersion)
	}
}

func TestBodyDigest_EmptyBodyIsSHA256OfEmpty(t *testing.T) {
	got := BodyDigest(nil)
	sum := sha256.Sum256([]byte{})
	want := base64.StdEncoding.EncodeToString(sum[:])
	if got != want {
		t.Errorf("empty body digest: got %q, want %q", got, want)
	}
	if got != BodyDigest([]byte("")) {
		t.Error("nil and empty body must digest identically")
	}
}

func TestBuildSigningString_PinnedShape(t *testing.T) {
	req := SignatureParams{
		Method:          "POST",
		Path:            "/federation/events",
		InstanceURL:     "https://alice.example",
		Timestamp:       "2026-06-01T00:00:00.000Z",
		Nonce:           "n-1",
		ProtocolVersion: "1",
		BodyDigest:      "DIGEST",
	}
	got := BuildSigningString(req)
	want := strings.Join([]string{
		"POST",
		"/federation/events",
		"https://alice.example",
		"2026-06-01T00:00:00.000Z",
		"n-1",
		"1",
		"DIGEST",
	}, "\n")
	if got != want {
		t.Errorf("signing string:\n got %q\nwant %q", got, want)
	}
}

func TestSignVerify_RoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	body := []byte(`{"hello":"world"}`)
	params := SignatureParams{
		Method:          "POST",
		Path:            "/federation/events",
		InstanceURL:     "https://alice.example",
		Timestamp:       "2026-06-01T00:00:00.000Z",
		Nonce:           "n-1",
		ProtocolVersion: "1",
		BodyDigest:      BodyDigest(body),
	}
	sig := Sign(priv, params)

	if !Verify(pub, params, sig) {
		t.Fatal("valid signature failed to verify")
	}

	// Tampering with any signed field breaks verification.
	bad := params
	bad.Nonce = "n-2"
	if Verify(pub, bad, sig) {
		t.Error("verify accepted a tampered nonce")
	}
	bad = params
	bad.ProtocolVersion = "2"
	if Verify(pub, bad, sig) {
		t.Error("verify accepted a tampered protocol version (downgrade)")
	}
	bad = params
	bad.BodyDigest = BodyDigest([]byte("other"))
	if Verify(pub, bad, sig) {
		t.Error("verify accepted a tampered body digest")
	}
}

func TestVerify_BadSignatureBytesRejected(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	params := SignatureParams{Method: "GET", Path: "/x", InstanceURL: "https://a", Timestamp: "t", Nonce: "n", ProtocolVersion: "1", BodyDigest: BodyDigest(nil)}
	if Verify(pub, params, []byte("not-a-signature")) {
		t.Error("verify accepted garbage signature bytes")
	}
}
