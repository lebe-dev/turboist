// Package metrics holds the federation Prometheus collectors (Federation v1
// F6.5, US-8.2). It uses prometheus/client_golang (a committed new go.mod
// dependency, §3 — pure-Go, CGO-free) so the /metrics exposition carries the
// exact labeled metric names the AC enumerates:
//
//	federation_outbox_depth                          (gauge)
//	federation_events_sent_total{peer,result}        (counter)
//	federation_events_received_total{peer,result}    (counter)
//	federation_signature_failures_total{peer}        (counter)
//	federation_apply_duration_seconds                (histogram)
//	federation_peer_last_contact_seconds{peer}       (gauge)
//
// The `result` label set is FIXED to {success, error} so cardinality stays
// bounded; the `peer` label is the instance_url, which is bounded by the
// (small) set of federated peers (R26 cardinality note).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Result is the fixed outcome label on the sent/received counters. Keeping it a
// closed type prevents an open-ended label that would blow up cardinality.
type Result string

const (
	// ResultSuccess marks an event that was delivered / accepted.
	ResultSuccess Result = "success"
	// ResultError marks an event that failed delivery / was rejected.
	ResultError Result = "error"
)

// Collectors bundles the federation metric instruments. Construct one with New
// (registering into a fresh registry for tests) or NewWith to register into an
// existing registry (the process default in main.go). All instruments are safe
// for concurrent use.
type Collectors struct {
	// OutboxDepth is the current number of undelivered federation_outbox rows.
	// A gauge so it tracks the live depth (set, not incremented) — US-8.2 AC1.
	OutboxDepth prometheus.Gauge

	// EventsSent counts outbound events by peer + result.
	EventsSent *prometheus.CounterVec
	// EventsReceived counts inbound events by peer + result.
	EventsReceived *prometheus.CounterVec
	// SignatureFailures counts per-peer per-event signature failures.
	SignatureFailures *prometheus.CounterVec

	// ApplyDuration observes the inbox-apply latency in seconds.
	ApplyDuration prometheus.Histogram

	// PeerLastContact is the seconds-since-last-contact per peer (a gauge so it
	// can be refreshed to the freshest value on each scrape-feeding update).
	PeerLastContact *prometheus.GaugeVec

	registry *prometheus.Registry
}

// New builds the federation collectors registered into a fresh, dedicated
// registry. This is the test-friendly constructor (no global state); the
// returned Collectors carries its registry so a /metrics handler can be built
// against exactly these instruments.
func New() *Collectors {
	reg := prometheus.NewRegistry()
	return NewWith(reg)
}

// NewWith builds the federation collectors and registers them into reg. The
// instrument definitions live here so the AC-exact names/labels are pinned in
// one place.
func NewWith(reg *prometheus.Registry) *Collectors {
	c := &Collectors{
		OutboxDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "federation_outbox_depth",
			Help: "Number of undelivered federation outbox events.",
		}),
		EventsSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "federation_events_sent_total",
			Help: "Total federation events sent to peers, by peer and result.",
		}, []string{"peer", "result"}),
		EventsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "federation_events_received_total",
			Help: "Total federation events received from peers, by peer and result.",
		}, []string{"peer", "result"}),
		SignatureFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "federation_signature_failures_total",
			Help: "Total per-event signature failures, by peer.",
		}, []string{"peer"}),
		ApplyDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "federation_apply_duration_seconds",
			Help:    "Inbox apply duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		PeerLastContact: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "federation_peer_last_contact_seconds",
			Help: "Seconds since the last successful contact with a peer.",
		}, []string{"peer"}),
		registry: reg,
	}
	reg.MustRegister(
		c.OutboxDepth,
		c.EventsSent,
		c.EventsReceived,
		c.SignatureFailures,
		c.ApplyDuration,
		c.PeerLastContact,
	)
	return c
}

// Registry returns the registry the collectors are registered into so the
// caller can build a promhttp handler against it.
func (c *Collectors) Registry() *prometheus.Registry { return c.registry }

// SetOutboxDepth sets the live outbox depth gauge (US-8.2 AC1).
func (c *Collectors) SetOutboxDepth(n int) {
	if c == nil {
		return
	}
	c.OutboxDepth.Set(float64(n))
}

// RecordEventSent increments the sent counter for a peer + result.
func (c *Collectors) RecordEventSent(peer string, result Result, n int) {
	if c == nil || n <= 0 {
		return
	}
	c.EventsSent.WithLabelValues(peer, string(result)).Add(float64(n))
}

// RecordEventReceived increments the received counter for a peer + result.
func (c *Collectors) RecordEventReceived(peer string, result Result, n int) {
	if c == nil || n <= 0 {
		return
	}
	c.EventsReceived.WithLabelValues(peer, string(result)).Add(float64(n))
}

// RecordSignatureFailure increments the per-peer signature-failure counter.
func (c *Collectors) RecordSignatureFailure(peer string) {
	if c == nil {
		return
	}
	c.SignatureFailures.WithLabelValues(peer).Inc()
}

// ObserveApplySeconds records one inbox-apply latency sample.
func (c *Collectors) ObserveApplySeconds(seconds float64) {
	if c == nil {
		return
	}
	c.ApplyDuration.Observe(seconds)
}

// SetPeerLastContactSeconds sets the seconds-since-last-contact gauge for a peer.
func (c *Collectors) SetPeerLastContactSeconds(peer string, seconds float64) {
	if c == nil {
		return
	}
	c.PeerLastContact.WithLabelValues(peer).Set(seconds)
}

// RecordSent is the string-result adapter the outbox worker's SentObserver
// expects (Federation v1 F6.5, US-8.2). It maps the worker's "success"/"error"
// string to the typed Result so the worker holds no dependency on this package's
// Result type. An unknown result string is treated as an error.
func (c *Collectors) RecordSent(peer, result string, n int) {
	if c == nil {
		return
	}
	r := ResultError
	if result == string(ResultSuccess) {
		r = ResultSuccess
	}
	c.RecordEventSent(peer, r, n)
}
