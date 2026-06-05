package hlc

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// F7.2 — HLC correctness table-tests (§15.5 "heart of correctness").
//
// This file owns the F7.2 milestone scenarios. The HLC type's pure round-trip /
// Recv / Compare table-tests already live in hlc_test.go (the F2.3 foundation);
// F7.2 adds the scenarios the milestone explicitly enumerates that the
// foundation did not assert end-to-end:
//
//   - Now() same-ms logical++ AND clock-backward driven through the REAL
//     SQLite-backed hlc_state Store under SetMaxOpenConns(1) (risk note: "one
//     test through real SQLite store under single connection").
//   - total order incl. node_id tie-break asserted on the zero-padded STORED
//     strings via CompareString — the cheap lexical path the per-field LWW SQL
//     actually uses, where the node_id is the final tie-break.
//   - parse/format round-trip + malformed reject as an exhaustive table.
//   - physical_ms == updated_at ms: the same time.Now() that writes updated_at
//     mints the HLC, so the two never drift (§3 DEVIATE / R11).
//   - logical overflow at the millisecond rollover boundary (risk note:
//     "logical overflow at ms rollover") — the fixed-width string invariant must
//     survive a clamped logical counter.

// fixedClock returns a now() func pinned to t.
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// TestStore_NowSameMillisecondLogicalIncrements drives the same-ms local-tick
// rule through the real SQLite-backed Store: three Now() calls pinned to one
// wall millisecond must yield logical 0,1,2 at a single physical_ms, each
// strictly greater than the last (F7.2: "Now() same-ms logical++").
func TestStore_NowSameMillisecondLogicalIncrements(t *testing.T) {
	d := openMigrated(t)
	store := NewStore(d, "install-node-same-ms").WithClock(
		fixedClock(time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)))
	ctx := context.Background()

	wantPhysical := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC).UnixMilli()
	var prev HLC
	for i := int64(0); i < 3; i++ {
		got, err := store.Now(ctx)
		if err != nil {
			t.Fatalf("Now() #%d: %v", i, err)
		}
		if got.PhysicalMS != wantPhysical {
			t.Errorf("physical_ms #%d: got %d, want %d", i, got.PhysicalMS, wantPhysical)
		}
		if got.Logical != i {
			t.Errorf("logical #%d: got %d, want %d", i, got.Logical, i)
		}
		if i > 0 && Compare(got, prev) != 1 {
			t.Errorf("Now() not strictly increasing at #%d: %+v after %+v", i, got, prev)
		}
		prev = got
	}
}

// TestStore_NowClockBackward drives the clock-backward rule through the real
// SQLite-backed Store under SetMaxOpenConns(1): once the clock has advanced,
// a later wall clock that runs BACKWARD must never move physical_ms backward —
// it pins to the last physical and bumps logical instead (F7.2 clock-backward;
// risk note "one test through real SQLite store under single connection").
func TestStore_NowClockBackward(t *testing.T) {
	d := openMigrated(t)
	store := NewStore(d, "install-node-backward")
	ctx := context.Background()

	ahead := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	store.WithClock(fixedClock(ahead))
	first, err := store.Now(ctx)
	if err != nil {
		t.Fatalf("Now() ahead: %v", err)
	}

	// Wall clock jumps back 1h: physical_ms must stay at `ahead`, logical bumps.
	behind := ahead.Add(-time.Hour)
	store.WithClock(fixedClock(behind))
	second, err := store.Now(ctx)
	if err != nil {
		t.Fatalf("Now() behind: %v", err)
	}

	if second.PhysicalMS != ahead.UnixMilli() {
		t.Errorf("physical_ms after backward jump: got %d, want %d (must not regress)",
			second.PhysicalMS, ahead.UnixMilli())
	}
	if second.Logical != first.Logical+1 {
		t.Errorf("logical after backward jump: got %d, want %d", second.Logical, first.Logical+1)
	}
	if Compare(second, first) != 1 {
		t.Errorf("clock-backward must still be monotonic: %+v not after %+v", second, first)
	}
}

// TestStore_PhysicalMSEqualsUpdatedAt asserts the binding §3/R11 invariant: the
// SAME time.Now() that writes a row's updated_at also mints the HLC, so the
// HLC physical_ms equals the updated_at millisecond exactly. The test pins one
// instant, writes updated_at = model.FormatUTC(now) to a real TEXT column AND
// mints the HLC via the same clock, then asserts they agree to the millisecond.
func TestStore_PhysicalMSEqualsUpdatedAt(t *testing.T) {
	d := openMigrated(t)
	ctx := context.Background()

	// A scratch table standing in for any synced entity's updated_at column.
	if _, err := d.ExecContext(ctx,
		`CREATE TABLE f72_scratch (id INTEGER PRIMARY KEY, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create scratch: %v", err)
	}

	instant := time.Date(2026, 6, 5, 13, 14, 15, 678_000_000, time.UTC)
	store := NewStore(d, "install-node-updated-at").WithClock(fixedClock(instant))

	// Same instant feeds both writes (production sources updated_at and the HLC
	// physical_ms from one time.Now()).
	updatedAt := model.FormatUTC(instant)
	if _, err := d.ExecContext(ctx,
		`INSERT INTO f72_scratch (id, updated_at) VALUES (1, ?)`, updatedAt); err != nil {
		t.Fatalf("insert scratch: %v", err)
	}
	got, err := store.Now(ctx)
	if err != nil {
		t.Fatalf("Now(): %v", err)
	}

	var stored string
	if err := d.QueryRowContext(ctx,
		`SELECT updated_at FROM f72_scratch WHERE id = 1`).Scan(&stored); err != nil {
		t.Fatalf("read scratch: %v", err)
	}
	parsed, err := model.ParseUTC(stored)
	if err != nil {
		t.Fatalf("parse updated_at %q: %v", stored, err)
	}
	if got.PhysicalMS != parsed.UnixMilli() {
		t.Errorf("physical_ms != updated_at ms: hlc=%d updated_at=%d (%q) — they must not drift (R11)",
			got.PhysicalMS, parsed.UnixMilli(), stored)
	}
}

// TestCompareString_TotalOrderWithNodeTieBreak asserts the total order —
// physical, then logical, then node_id — holds on the zero-padded STORED
// strings compared lexically (CompareString), which is the path the per-field
// LWW SQL uses. The node_id is the final tie-break when physical and logical
// are equal (F7.2: "total order + node_id tie-break").
func TestCompareString_TotalOrderWithNodeTieBreak(t *testing.T) {
	cases := []struct {
		name string
		a, b HLC
		want int
	}{
		{
			name: "physical dominates a larger logical+node",
			a:    HLC{PhysicalMS: 1, Logical: 9999, NodeID: "zzz"},
			b:    HLC{PhysicalMS: 2, Logical: 0, NodeID: "aaa"},
			want: -1,
		},
		{
			name: "logical breaks an equal-physical tie",
			a:    HLC{PhysicalMS: 7, Logical: 1, NodeID: "zzz"},
			b:    HLC{PhysicalMS: 7, Logical: 2, NodeID: "aaa"},
			want: -1,
		},
		{
			name: "node_id breaks an equal physical+logical tie",
			a:    HLC{PhysicalMS: 7, Logical: 3, NodeID: "alice-node"},
			b:    HLC{PhysicalMS: 7, Logical: 3, NodeID: "bob-node"},
			want: -1,
		},
		{
			name: "fully equal triples compare equal",
			a:    HLC{PhysicalMS: 7, Logical: 3, NodeID: "same-node"},
			b:    HLC{PhysicalMS: 7, Logical: 3, NodeID: "same-node"},
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			as, bs := tc.a.String(), tc.b.String()
			if got := CompareString(as, bs); got != tc.want {
				t.Errorf("CompareString(%q,%q): got %d, want %d", as, bs, got, tc.want)
			}
			// CompareString on the padded strings must equal the numeric Compare.
			if got := CompareString(as, bs); got != Compare(tc.a, tc.b) {
				t.Errorf("lexical/numeric mismatch: CompareString=%d Compare=%d", got, Compare(tc.a, tc.b))
			}
			// Reversed argument order flips the sign.
			if got := CompareString(bs, as); got != -tc.want {
				t.Errorf("CompareString reversed: got %d, want %d", got, -tc.want)
			}
		})
	}
}

// TestParseFormat_RoundTripTable round-trips a table of HLCs through
// String()→Parse() and asserts the parsed value equals the input, including
// boundary widths (zero physical, max logical, UUID node with dashes).
func TestParseFormat_RoundTripTable(t *testing.T) {
	cases := []struct {
		name string
		in   HLC
	}{
		{"zero physical", HLC{PhysicalMS: 0, Logical: 0, NodeID: "n"}},
		{"typical", HLC{PhysicalMS: 1718000000123, Logical: 7, NodeID: "node-a"}},
		{"max logical", HLC{PhysicalMS: 1718000000123, Logical: maxLogical, NodeID: "n"}},
		{"max physical", HLC{PhysicalMS: maxPhysical, Logical: 1, NodeID: "n"}},
		{"uuid node with dashes", HLC{PhysicalMS: 42, Logical: 1, NodeID: "01234567-89ab-cdef-0123-456789abcdef"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.in.String()
			got, err := Parse(s)
			if err != nil {
				t.Fatalf("Parse(%q): %v", s, err)
			}
			if got != tc.in {
				t.Errorf("round-trip: got %+v, want %+v", got, tc.in)
			}
		})
	}
}

// TestParse_MalformedTable rejects malformed and out-of-range HLC strings as a
// labeled table (F7.2: "malformed reject"). Each case documents why it is
// invalid so a regression names the broken rule.
func TestParse_MalformedTable(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"no dashes", "no-dashes-but-wrong-widths"}, // dashes present but widths wrong
		{"only two parts", "00000000000123-0456"},
		{"non-numeric physical", "abcdefghijklmn-0000-node"},
		{"physical wrong width", "123-0001-node"},
		{"logical wrong width", "00000000000001-1-node"},
		{"missing node", "00000000000001-0001"},
		{"empty node", "00000000000001-0001-"},
		{"leading dash", "-00000000000001-0001-node"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.in); err == nil {
				t.Errorf("Parse(%q): expected error, got nil", tc.in)
			}
		})
	}
}

// TestLocalTick_LogicalOverflowAtMsRollover documents the logical-overflow
// boundary (risk note "logical overflow at ms rollover"): when many local
// events land within a single millisecond the logical counter climbs to its
// fixed width (maxLogical) and CLAMPS there — it never exceeds the width that
// keeps the zero-padded string lexically comparable. The clamped value still
// fits logicalWidth digits, and once the wall clock finally advances a
// millisecond the counter resets to 0. This guards the fixed-width string
// invariant the per-field LWW SQL relies on.
func TestLocalTick_LogicalOverflowAtMsRollover(t *testing.T) {
	// Climb logical within one frozen millisecond past the fixed width.
	cur := HLC{PhysicalMS: 1000, Logical: 0, NodeID: "n"}
	for i := 0; i < maxLogical+50; i++ {
		cur = LocalTick(cur, 1000) // wall clock frozen at the same ms
	}
	if cur.Logical != maxLogical {
		t.Fatalf("logical did not clamp at the width boundary: got %d, want %d", cur.Logical, maxLogical)
	}
	if cur.PhysicalMS != 1000 {
		t.Errorf("physical_ms drifted during same-ms climb: got %d, want 1000", cur.PhysicalMS)
	}
	// The clamped value must still serialise within logicalWidth digits so the
	// string stays fixed-width and lexically comparable.
	field := cur.logicalField()
	if len(field) != logicalWidth {
		t.Errorf("clamped logical field width: got %d (%q), want %d", len(field), field, logicalWidth)
	}
	if n, err := strconv.ParseInt(field, 10, 64); err != nil || n != maxLogical {
		t.Errorf("clamped logical field: got %q (n=%d, err=%v), want %d", field, n, err, maxLogical)
	}

	// Once the wall clock rolls to the next millisecond, physical advances and
	// logical resets — the overflow does not leak across the ms boundary.
	rolled := LocalTick(cur, 1001)
	if rolled.PhysicalMS != 1001 || rolled.Logical != 0 {
		t.Errorf("ms rollover after overflow: got %+v, want physical=1001 logical=0", rolled)
	}
	// And the rolled value strictly follows the clamped one.
	if Compare(rolled, cur) != 1 {
		t.Errorf("post-rollover not after clamped: %+v not after %+v", rolled, cur)
	}
}

// TestRecv_LogicalOverflowClamps asserts the receive merge also clamps an
// overflowing logical counter at the same fixed-width boundary, so a remote
// HLC carrying a near-max logical cannot push the merged value past the width.
func TestRecv_LogicalOverflowClamps(t *testing.T) {
	local := HLC{PhysicalMS: 2000, Logical: maxLogical, NodeID: "local"}
	incoming := HLC{PhysicalMS: 2000, Logical: maxLogical, NodeID: "peer"}
	got := Recv(local, incoming, 2000) // all equal physical → max(logical)+1, clamped
	if got.Logical != maxLogical {
		t.Errorf("Recv logical clamp: got %d, want %d", got.Logical, maxLogical)
	}
	if len(got.logicalField()) != logicalWidth {
		t.Errorf("Recv logical field width: got %d, want %d", len(got.logicalField()), logicalWidth)
	}
	if got.NodeID != "local" {
		t.Errorf("Recv keeps local node: got %q, want local", got.NodeID)
	}
}
