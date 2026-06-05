package federation_test

import (
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// TestRemoteHandshakeError_PausedIsTransientNotPermanent asserts a 403
// federation_paused is classified TRANSIENT (like a 429 backoff), NOT a permanent
// peer-scoped link death (Federation v1 F5.3 review fix). A paused peer is a
// reversible state: its outbound backlog must flush after the owner resumes it, so
// the outbox worker must keep retrying — not gate the link forever. A genuine
// non-paused 403 (revoked / read-only) stays permanent.
func TestRemoteHandshakeError_PausedIsTransientNotPermanent(t *testing.T) {
	paused := &fedsvc.RemoteHandshakeError{StatusCode: 403, Code: httpapi.CodeFederationPaused}
	if paused.FederationPermanent() {
		t.Errorf("federation_paused (403) must be TRANSIENT — the paused peer's outbound backlog must flush after resume, not be permanently gated")
	}

	revoked := &fedsvc.RemoteHandshakeError{StatusCode: 403, Code: httpapi.CodeFederationRevoked}
	if !revoked.FederationPermanent() {
		t.Errorf("a non-paused 403 (revoked) must remain PERMANENT (peer-scoped link death)")
	}
	if !revoked.FederationPeerScoped() {
		t.Errorf("a revoked 403 must be peer-scoped")
	}
}
