// Federation v1 F7.6 — NFR-1 performance benchmarks (advisory, non-blocking).
//
// These are the four NFR-1 targets the milestone pins, asserted on the REAL
// production paths through the F7.1 harness:
//
//   - inbox apply p95 < 50ms @ 100k entities (NFR-1.3) — explicit percentile
//     sampling over the production Applier.Apply merge (per-field LWW CAS + the
//     domain UPDATE) against a project already holding 100k federated tasks, not
//     the testing-framework's ns/op mean.
//   - snapshot 10k < 30s (NFR-1.2) — the production buffer-first build
//     (BuildSnapshotForMember: consistent read → release the lone writer
//     connection) + NDJSON serialisation of a 10k-task project.
//   - bootstrap 1k < 60s (NFR-1.4) — a real signed handshake + snapshot consume
//     of a 1k-task project into a brand-new joiner.
//   - push < 5s with commit-ping (NFR-1.1) — an edit emitted through the
//     production transactional Emitter, the publisher worker drained on its
//     commit-ping, and converged on the peer inside the 5s budget.
//
// PLUS the §3 / R1 buffer-first availability assertion the milestone calls out:
// a concurrent app write must NOT stall while a large snapshot is being built —
// the buffer-first read releases the SetMaxOpenConns(1) writer connection before
// streaming, so the write completes well inside a tight budget. If buffer-first
// still contended, this is the test that would surface the need for a dedicated
// read connection (R1).
//
// These are GATED behind FEDERATION_BENCH=1 so they never run under `just test`
// or CI — they are advisory, run on demand via `just bench-federation`. The
// gate, not testing.Short(), because `just test` runs `go test -run "" ./...`
// (no -short flag), so a Short() skip would not exclude them. They are written
// as ordinary tests (not Benchmark funcs) because the milestone mandates EXPLICIT
// p95 percentile sampling, which `testing.B`'s ns/op mean cannot express.
package fedtest

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// benchEnv is the FEDERATION_BENCH gate every F7.6 perf test guards on. The perf
// targets seed 100k/10k/1k rows and are wall-clock-threshold assertions, so they
// are advisory only — run on demand via `just bench-federation`, never under the
// `just test` / CI default (which runs `go test -run "" ./...` with no -short).
const benchEnv = "FEDERATION_BENCH"

// requireBench skips a perf test unless FEDERATION_BENCH=1 is set, keeping the
// 100k-scale benchmarks out of the default test + CI run (advisory, non-blocking).
func requireBench(t *testing.T) {
	t.Helper()
	if os.Getenv(benchEnv) != "1" {
		t.Skipf("federation perf benchmark skipped; set %s=1 (advisory, run via `just bench-federation`)", benchEnv)
	}
}

// nfrInboxApplyP95 is the NFR-1.3 per-apply p95 ceiling at 100k entities.
const nfrInboxApplyP95 = 50 * time.Millisecond

// nfrInboxApplyEntities is the corpus size NFR-1.3 measures apply latency at.
const nfrInboxApplyEntities = 100_000

// nfrSnapshotBudget is the NFR-1.2 build-and-serialise ceiling for a 10k project.
const nfrSnapshotBudget = 30 * time.Second

// nfrSnapshotEntities is the project size NFR-1.2 measures snapshot build at.
const nfrSnapshotEntities = 10_000

// nfrBootstrapBudget is the NFR-1.4 full-bootstrap ceiling for a 1k project.
const nfrBootstrapBudget = 60 * time.Second

// nfrBootstrapEntities is the project size NFR-1.4 measures bootstrap at.
const nfrBootstrapEntities = 1_000

// nfrConcurrentWriteBudget bounds how long a single app write may take while a
// large snapshot is being built — the buffer-first availability assertion (§3 /
// R1). Buffer-first releases the lone writer connection after the consistent
// read, so a concurrent write must not be stalled for the whole bootstrap.
const nfrConcurrentWriteBudget = 2 * time.Second

// inbox apply p95 < 50ms @ 100k (NFR-1.3). A federated project is loaded with
// 100k tasks (each with a title field_hlc baseline), then UPDATE events are
// applied one at a time through the PRODUCTION Applier.Apply — the exact merge
// the single inbox-apply goroutine runs (per-field CAS over entity_field_hlc +
// the domain UPDATE). Per-apply latency is sampled and the 95th percentile is
// asserted under the 50ms ceiling. This is EXPLICIT percentile sampling, not the
// framework's ns/op mean (the risk the milestone names).
func TestF76_InboxApplyP95_100k(t *testing.T) {
	requireBench(t)
	ctx := context.Background()
	h := NewHarness(t)
	owner := h.AddInstance(t, "https://owner.example")

	projID := owner.CreateFederatedProject(t, ctx, "Big")
	projClient := owner.ProjectClientID(t, projID)
	clientIDs := owner.SeedFederatedTasks(t, ctx, projID, nfrInboxApplyEntities)

	const samples = 2000
	latencies := make([]time.Duration, 0, samples)
	// Walk distinct entities so each apply hits a fresh row's indexed lookups
	// (the realistic steady-state cost), and advance the HLC each time so every
	// event WINS the per-field CAS (a stale no-op would understate apply cost).
	for i := 0; i < samples; i++ {
		taskClient := clientIDs[i%len(clientIDs)]
		incoming := hlc.HLC{PhysicalMS: int64(2_000_000_000_000 + i), Logical: 0, NodeID: "ownernode"}.String()
		e := events.Event{
			EventID:         model.NewClientID(),
			Op:              events.OpUpdate,
			EntityType:      events.EntityTask,
			EntityID:        taskClient,
			ProjectClientID: projClient,
			Author:          owner.URL(),
			OriginInstance:  owner.URL(),
			CreatedAt:       model.FormatUTC(time.Now()),
			Fields: map[string]events.Field{
				"title": {Value: "edit", HLC: incoming},
			},
		}
		start := time.Now()
		res, err := owner.ApplyEvent(ctx, e, owner.URL())
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("apply sample %d: %v", i, err)
		}
		if !res.AppliedFields["title"] {
			t.Fatalf("apply sample %d did not win the CAS (stale HLC?)", i)
		}
		latencies = append(latencies, elapsed)
	}

	p95 := percentile(latencies, 95)
	t.Logf("NFR-1.3 inbox apply @%d entities (%d samples): p50=%s p95=%s p99=%s max=%s",
		nfrInboxApplyEntities, len(latencies),
		percentile(latencies, 50), p95, percentile(latencies, 99), maxDuration(latencies))
	if p95 > nfrInboxApplyP95 {
		t.Errorf("inbox apply p95: got %s, want <= %s (NFR-1.3 @%d entities)", p95, nfrInboxApplyP95, nfrInboxApplyEntities)
	}
}

// snapshot 10k < 30s (NFR-1.2). A 10k-task federated project is snapshotted
// through the PRODUCTION buffer-first build (BuildSnapshotForMember takes the
// consistent read, releases the lone writer connection, returns the buffer) and
// the result is serialised to NDJSON exactly as the snapshot handler streams it.
// Both legs together must finish under 30s.
func TestF76_Snapshot10k(t *testing.T) {
	requireBench(t)
	ctx := context.Background()
	h := NewHarness(t)
	owner := h.AddInstance(t, "https://owner.example")

	projID := owner.CreateFederatedProject(t, ctx, "Big")
	owner.SeedFederatedTasks(t, ctx, projID, nfrSnapshotEntities)

	start := time.Now()
	n := owner.BuildAndSerializeMemberSnapshot(t, ctx, projID)
	elapsed := time.Since(start)

	t.Logf("NFR-1.2 snapshot build+serialise @%d tasks: %s (%d NDJSON bytes)", nfrSnapshotEntities, elapsed, n)
	if elapsed > nfrSnapshotBudget {
		t.Errorf("snapshot 10k: got %s, want <= %s (NFR-1.2)", elapsed, nfrSnapshotBudget)
	}
}

// bootstrap 1k < 60s (NFR-1.4). The owner federates a 1k-task project; a
// brand-new joiner performs the REAL signed handshake + snapshot consume (the
// production Join path) and the whole bootstrap must complete under 60s, with
// every task converged onto the joiner.
func TestF76_Bootstrap1k(t *testing.T) {
	requireBench(t)
	ctx := context.Background()
	h := NewHarness(t)
	// The in-process app.Test() transport can re-serve the same signed handshake
	// and trip a spurious transport replay — a pure harness artifact (see
	// WithPermissiveNonces). The DOMAIN assertion here is bootstrap LATENCY +
	// convergence, not transport anti-replay (owned by F0.3), so both ends opt
	// into the no-op nonce cache.
	owner := h.AddInstance(t, "https://owner.example", WithPermissiveNonces())
	joiner := h.AddInstance(t, "https://joiner.example", WithPermissiveNonces())

	projID := owner.CreateFederatedProject(t, ctx, "Big")
	clientIDs := owner.SeedFederatedTasks(t, ctx, projID, nfrBootstrapEntities)
	invite := owner.CreateInvite(t, ctx, projID, model.FederationPermissionWrite)

	start := time.Now()
	joined := joiner.Join(t, ctx, owner.URL(), invite)
	elapsed := time.Since(start)

	if joined.ProjectID == 0 {
		t.Fatalf("bootstrap returned no local project id")
	}
	t.Logf("NFR-1.4 bootstrap @%d tasks: %s", nfrBootstrapEntities, elapsed)
	if elapsed > nfrBootstrapBudget {
		t.Errorf("bootstrap 1k: got %s, want <= %s (NFR-1.4)", elapsed, nfrBootstrapBudget)
	}
	// The snapshot must have landed every task on the joiner.
	got := joiner.TaskCount(joined.ProjectID)
	if got != nfrBootstrapEntities {
		t.Errorf("bootstrap converged %d tasks, want %d", got, nfrBootstrapEntities)
	}
	// And a representative task converged with its value.
	last := clientIDs[len(clientIDs)-1]
	AssertConverged(t, func() bool { return joiner.TaskTitle(last) != "" },
		"bootstrap did not converge the last task onto the joiner")
}

// push < 5s with commit-ping (NFR-1.1). The owner edits a task; the edit is
// emitted through the production transactional Emitter (which pings the
// publisher worker on commit), the worker is running, and the change must
// converge on the peer inside the 5s budget. This asserts the commit-ping push
// latency, not a manual drain.
func TestF76_PushUnder5s_CommitPing(t *testing.T) {
	requireBench(t)
	ctx := context.Background()
	h := NewHarness(t)
	owner := h.AddInstance(t, "https://owner.example", WithPermissiveNonces())
	joiner := h.AddInstance(t, "https://joiner.example", WithPermissiveNonces())

	projID := owner.CreateFederatedProject(t, ctx, "Shared")
	taskClient := model.NewClientID()
	cx := int64(1)
	if _, err := owner.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &projID},
		Title:     "Original",
	}, taskClient); err != nil {
		t.Fatalf("owner create task: %v", err)
	}
	invite := owner.CreateInvite(t, ctx, projID, model.FederationPermissionWrite)
	joiner.Join(t, ctx, owner.URL(), invite)
	AssertConverged(t, func() bool { return joiner.TaskTitle(taskClient) == "Original" },
		"snapshot did not converge before the push test")

	joiner.StartApply(t, ctx)
	// Wire the production worker + commit-ping so the emit pushes immediately on
	// commit (the NFR-1.1 mechanism) rather than waiting on a manual PumpOutbox.
	owner.StartCommitPingWorker(t, ctx)

	ownerTask := owner.TaskByClientID(t, ctx, taskClient)
	newTitle := "Pushed by commit-ping"
	start := time.Now()
	if err := owner.Mutator().Update(ctx, ownerTask, repo.TaskUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("owner update task: %v", err)
	}
	AssertConverged(t, func() bool { return joiner.TaskTitle(taskClient) == newTitle },
		"edit did not converge on the joiner within the NFR-1.1 5s budget")
	t.Logf("NFR-1.1 commit-ping push converged in %s (budget %s)", time.Since(start), convergeBudget)
}

// buffer-first availability under bootstrap (§3 / R1). While a large
// (10k-task) member snapshot is being built+serialised on one goroutine, a
// concurrent app write on another must NOT stall for the whole build: the
// buffer-first read releases the lone SetMaxOpenConns(1) writer connection after
// the consistent read, so the write completes well inside a tight budget. If
// buffer-first still contended, this write would block for the whole bootstrap
// and surface the need for a dedicated read connection (R1).
func TestF76_BufferFirstDoesNotStallWrites(t *testing.T) {
	requireBench(t)
	ctx := context.Background()
	h := NewHarness(t)
	owner := h.AddInstance(t, "https://owner.example")

	projID := owner.CreateFederatedProject(t, ctx, "Big")
	owner.SeedFederatedTasks(t, ctx, projID, nfrSnapshotEntities)

	// Build+serialise the big snapshot repeatedly on a background goroutine for the
	// duration of the test so a concurrent write races a build in progress. The
	// goroutine uses the error-returning build form (t.Fatalf is unsafe off the
	// test goroutine); any build error is captured and surfaced on the main
	// goroutine after the loop stops.
	buildCtx, stopBuilds := context.WithCancel(ctx)
	buildsDone := make(chan struct{})
	var buildErr error
	go func() {
		defer close(buildsDone)
		for buildCtx.Err() == nil {
			if err := owner.BuildAndSerializeMemberSnapshotBackground(ctx, projID); err != nil {
				buildErr = err
				return
			}
		}
	}()
	t.Cleanup(func() {
		stopBuilds()
		<-buildsDone
		if buildErr != nil {
			t.Errorf("background snapshot build failed: %v", buildErr)
		}
	})

	// A concurrent app write (a federated task create through the production
	// Mutator) must complete inside the tight budget despite snapshots being
	// built — proving buffer-first does not hold the writer connection across the
	// whole bootstrap.
	const writes = 20
	var worst time.Duration
	cx := int64(1)
	for i := 0; i < writes; i++ {
		start := time.Now()
		if _, err := owner.Mutator().Create(ctx, repo.CreateTask{
			Placement: repo.Placement{ContextID: &cx, ProjectID: &projID},
			Title:     "concurrent",
		}, model.NewClientID()); err != nil {
			t.Fatalf("concurrent write %d: %v", i, err)
		}
		if d := time.Since(start); d > worst {
			worst = d
		}
	}
	t.Logf("buffer-first availability: worst concurrent write under snapshot build = %s (budget %s)", worst, nfrConcurrentWriteBudget)
	if worst > nfrConcurrentWriteBudget {
		t.Errorf("concurrent write stalled %s under snapshot build, want <= %s (buffer-first regressed — R1)", worst, nfrConcurrentWriteBudget)
	}
}

// percentile returns the p-th percentile (0..100) of d using nearest-rank on a
// sorted copy — the explicit percentile sampling NFR-1.3 mandates (not ns/op).
func percentile(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(d))
	copy(sorted, d)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	// Nearest-rank: rank = ceil(p/100 * N), 1-indexed.
	rank := (p*len(sorted) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// maxDuration returns the largest sample (the tail the p95 ceiling guards).
func maxDuration(d []time.Duration) time.Duration {
	var m time.Duration
	for _, v := range d {
		if v > m {
			m = v
		}
	}
	return m
}
