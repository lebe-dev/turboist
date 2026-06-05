package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/expfmt"
)

// TestCollectors_ExactNamesAndLabels asserts the federation collectors expose
// the exact Prometheus metric names + labels US-8.2 AC1 enumerates, and that the
// outbox-depth gauge tracks the set value (Federation v1 F6.5).
func TestCollectors_ExactNamesAndLabels(t *testing.T) {
	c := New()

	// Drive each instrument so it appears in the exposition with its labels.
	c.SetOutboxDepth(7)
	c.RecordEventSent("https://peer.example", ResultSuccess, 3)
	c.RecordEventReceived("https://peer.example", ResultError, 2)
	c.RecordSignatureFailure("https://peer.example")
	c.ObserveApplySeconds(0.012)
	c.SetPeerLastContactSeconds("https://peer.example", 42)

	text := gather(t, c)
	wants := []string{
		"federation_outbox_depth 7",
		`federation_events_sent_total{peer="https://peer.example",result="success"} 3`,
		`federation_events_received_total{peer="https://peer.example",result="error"} 2`,
		`federation_signature_failures_total{peer="https://peer.example"} 1`,
		"federation_apply_duration_seconds_bucket",
		`federation_peer_last_contact_seconds{peer="https://peer.example"} 42`,
	}
	for _, w := range wants {
		if !strings.Contains(text, w) {
			t.Errorf("exposition missing %q\n---\n%s", w, text)
		}
	}
}

// TestCollectors_OutboxDepthGaugeTracks asserts SetOutboxDepth overwrites (gauge
// semantics) rather than accumulating, so the metric reflects the live depth.
func TestCollectors_OutboxDepthGaugeTracks(t *testing.T) {
	c := New()
	c.SetOutboxDepth(5)
	c.SetOutboxDepth(2)
	if got := testutil.ToFloat64(c.OutboxDepth); got != 2 {
		t.Errorf("outbox depth gauge: got %v, want 2 (gauge must track, not accumulate)", got)
	}
}

// gather renders the registry's exposition as the Prometheus text format.
func gather(t *testing.T, c *Collectors) string {
	t.Helper()
	mfs, err := c.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var sb strings.Builder
	enc := expfmt.NewEncoder(&sb, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		if err := enc.Encode(mf); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	return sb.String()
}
