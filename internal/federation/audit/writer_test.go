package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/repo"
)

// captureSink is a hand-written test double for the audit insert sink. It records
// every entry it receives and can be made to block or fail.
type captureSink struct {
	mu      sync.Mutex
	entries []repo.AuditEntry
	gate    chan struct{} // when non-nil, Insert blocks until a value is read from it
	fail    bool
}

func (c *captureSink) Insert(ctx context.Context, e repo.AuditEntry) error {
	if c.gate != nil {
		<-c.gate
	}
	if c.fail {
		return errors.New("boom")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
	return nil
}

func (c *captureSink) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *captureSink) snapshot() []repo.AuditEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]repo.AuditEntry, len(c.entries))
	copy(out, c.entries)
	return out
}

// waitFor polls cond until it is true or the deadline elapses.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

// TestWriter_RecordPersistsAsync asserts a Recorded entry reaches the sink via
// the background goroutine (Federation v1 F6.3 async writer). Record itself never
// blocks on the DB.
func TestWriter_RecordPersistsAsync(t *testing.T) {
	sink := &captureSink{}
	w := NewWriter(sink, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	t.Cleanup(w.Stop)

	w.Record(repo.AuditEntry{Kind: repo.AuditKindReplay, Outcome: repo.AuditOutcomeRejected, PeerInstanceURL: "https://peer.example"})

	waitFor(t, func() bool { return sink.count() == 1 })
	got := sink.snapshot()[0]
	if got.Kind != repo.AuditKindReplay {
		t.Errorf("entry kind: got %q, want %q", got.Kind, repo.AuditKindReplay)
	}
	if got.PeerInstanceURL != "https://peer.example" {
		t.Errorf("entry peer: got %q, want https://peer.example", got.PeerInstanceURL)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("writer must stamp CreatedAt when the entry carries none")
	}
}

// TestWriter_RecordNeverBlocksWhenBufferFull asserts that when the writer's buffer
// is saturated (the sink is stalled), Record returns immediately and DROPS rather
// than blocking the caller — logging never blocks a rejection (§7 F6.3 "async
// writer, failure-spam is worst-case write load").
func TestWriter_RecordNeverBlocksWhenBufferFull(t *testing.T) {
	gate := make(chan struct{})
	sink := &captureSink{gate: gate}
	// A tiny buffer so it fills immediately while the sink is stalled on the gate.
	w := NewWriter(sink, nil).WithBuffer(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	t.Cleanup(func() {
		close(gate) // release the stalled sink so Stop can drain
		w.Stop()
	})

	// Fire many more records than the buffer can hold while the sink is stalled.
	// If Record blocked, this loop would deadlock and the test would time out.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			w.Record(repo.AuditEntry{Kind: repo.AuditKindSignatureInvalid, Outcome: repo.AuditOutcomeRejected})
		}
		close(done)
	}()

	select {
	case <-done:
		// Record returned for all 1000 calls without blocking — the dropped overflow
		// is the intended behaviour.
	case <-time.After(2 * time.Second):
		t.Fatal("Record blocked when the buffer was full — it must drop, not block")
	}

	if dropped := w.Dropped(); dropped == 0 {
		t.Errorf("expected some records to be dropped under saturation, got %d", dropped)
	}
}

// TestWriter_SinkFailureDoesNotCrash asserts a sink error is swallowed (logged)
// and the writer keeps draining subsequent entries — a transient DB failure must
// not kill the audit goroutine.
func TestWriter_SinkFailureDoesNotCrash(t *testing.T) {
	sink := &captureSink{fail: true}
	w := NewWriter(sink, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	t.Cleanup(w.Stop)

	// Record under the failing sink, then flip it to succeed and record again.
	w.Record(repo.AuditEntry{Kind: repo.AuditKindReplay, Outcome: repo.AuditOutcomeRejected})
	waitFor(t, func() bool {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		return true // give the goroutine a chance to process the failing insert
	})

	sink.mu.Lock()
	sink.fail = false
	sink.mu.Unlock()

	w.Record(repo.AuditEntry{Kind: repo.AuditKindHandshake, Outcome: repo.AuditOutcomeAccepted})
	waitFor(t, func() bool { return sink.count() >= 1 })
}

// TestWriter_StopDrains asserts Stop flushes buffered entries before returning so
// entries recorded just before shutdown are not lost.
func TestWriter_StopDrains(t *testing.T) {
	sink := &captureSink{}
	w := NewWriter(sink, nil).WithBuffer(64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	for i := 0; i < 10; i++ {
		w.Record(repo.AuditEntry{Kind: repo.AuditKindReplay, Outcome: repo.AuditOutcomeRejected})
	}
	w.Stop()

	if got := sink.count(); got != 10 {
		t.Errorf("entries after Stop: got %d, want 10 (Stop must drain the buffer)", got)
	}
}
