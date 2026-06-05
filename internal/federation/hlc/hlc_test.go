package hlc

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
)

// TestFormatParse_RoundTrip asserts a formatted HLC parses back to the same
// components, with physical_ms + logical zero-padded to a fixed width so the
// string compares lexically (US-3.x foundation; §5.3).
func TestFormatParse_RoundTrip(t *testing.T) {
	in := HLC{PhysicalMS: 1718000000123, Logical: 7, NodeID: "node-a"}
	s := in.String()
	got, err := Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	if got != in {
		t.Errorf("round-trip: got %+v, want %+v", got, in)
	}
}

// TestFormat_ZeroPadded asserts the physical and logical fields are zero-padded
// to a fixed width so lexical (string) comparison equals numeric comparison.
func TestFormat_ZeroPadded(t *testing.T) {
	small := HLC{PhysicalMS: 1, Logical: 2, NodeID: "n"}
	large := HLC{PhysicalMS: 1718000000123, Logical: 9999, NodeID: "n"}
	if len(small.physicalField()) != len(large.physicalField()) {
		t.Errorf("physical field width not fixed: %q vs %q", small.physicalField(), large.physicalField())
	}
	if len(small.logicalField()) != len(large.logicalField()) {
		t.Errorf("logical field width not fixed: %q vs %q", small.logicalField(), large.logicalField())
	}
	// And lexical order matches numeric order for the padded fields.
	if small.String() >= large.String() {
		t.Errorf("lexical order broken: %q !< %q", small.String(), large.String())
	}
}

// TestParse_Malformed rejects garbage and out-of-range HLC strings.
func TestParse_Malformed(t *testing.T) {
	for _, bad := range []string{
		"",
		"no-dashes",
		"123-456", // only two parts
		"abc-0000-node",
		"00000000000001-xxxx-node",
		"00000000000001-0001", // missing node
	} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q): expected error, got nil", bad)
		}
	}
}

// TestParse_NodeWithDashes keeps a node_id that itself contains dashes intact
// (it is the remainder after the first two fields).
func TestParse_NodeWithDashes(t *testing.T) {
	in := HLC{PhysicalMS: 42, Logical: 1, NodeID: "01234567-89ab-cdef-0123-456789abcdef"}
	got, err := Parse(in.String())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.NodeID != in.NodeID {
		t.Errorf("node_id with dashes: got %q, want %q", got.NodeID, in.NodeID)
	}
}

// TestCompare_Ordering asserts the total order: physical, then logical, then
// node_id (tie-break, §5.3). Equal triples compare equal.
func TestCompare_Ordering(t *testing.T) {
	cases := []struct {
		name string
		a, b HLC
		want int
	}{
		{"physical wins", HLC{PhysicalMS: 1, Logical: 9, NodeID: "z"}, HLC{PhysicalMS: 2, Logical: 0, NodeID: "a"}, -1},
		{"logical breaks tie", HLC{PhysicalMS: 5, Logical: 1, NodeID: "z"}, HLC{PhysicalMS: 5, Logical: 2, NodeID: "a"}, -1},
		{"node breaks tie", HLC{PhysicalMS: 5, Logical: 5, NodeID: "alice"}, HLC{PhysicalMS: 5, Logical: 5, NodeID: "bob"}, -1},
		{"equal", HLC{PhysicalMS: 5, Logical: 5, NodeID: "x"}, HLC{PhysicalMS: 5, Logical: 5, NodeID: "x"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Compare(tc.a, tc.b); got != tc.want {
				t.Errorf("Compare(%+v,%+v): got %d, want %d", tc.a, tc.b, got, tc.want)
			}
			if got := Compare(tc.b, tc.a); got != -tc.want {
				t.Errorf("Compare reversed: got %d, want %d", got, -tc.want)
			}
		})
	}
}

// TestCompareString matches the lexical comparison of the padded strings to the
// numeric Compare (this is what the per-field LWW SQL uses).
func TestCompareString(t *testing.T) {
	a := HLC{PhysicalMS: 5, Logical: 1, NodeID: "a"}
	b := HLC{PhysicalMS: 5, Logical: 2, NodeID: "a"}
	if CompareString(a.String(), b.String()) != -1 {
		t.Errorf("CompareString: got %d, want -1", CompareString(a.String(), b.String()))
	}
}

// TestLocalTick_SameMillisecond asserts a local tick at the same wall ms bumps
// the logical counter (§5.3 local event rule).
func TestLocalTick_SameMillisecond(t *testing.T) {
	prev := HLC{PhysicalMS: 1000, Logical: 0, NodeID: "n"}
	next := LocalTick(prev, 1000)
	if next.PhysicalMS != 1000 || next.Logical != 1 {
		t.Errorf("same-ms tick: got %+v, want physical=1000 logical=1", next)
	}
}

// TestLocalTick_Advances asserts a local tick at a newer wall ms advances
// physical and resets logical to 0.
func TestLocalTick_Advances(t *testing.T) {
	prev := HLC{PhysicalMS: 1000, Logical: 5, NodeID: "n"}
	next := LocalTick(prev, 2000)
	if next.PhysicalMS != 2000 || next.Logical != 0 {
		t.Errorf("advance tick: got %+v, want physical=2000 logical=0", next)
	}
}

// TestLocalTick_ClockBackward asserts a wall clock that ran backward does not
// move physical backward; it stays put and bumps logical (§5.3 max rule).
func TestLocalTick_ClockBackward(t *testing.T) {
	prev := HLC{PhysicalMS: 5000, Logical: 0, NodeID: "n"}
	next := LocalTick(prev, 3000) // wall clock < last physical
	if next.PhysicalMS != 5000 || next.Logical != 1 {
		t.Errorf("backward tick: got %+v, want physical=5000 logical=1", next)
	}
}

// TestRecv_Boundaries asserts the three Recv merge branches (§5.3 receive rule).
func TestRecv_Boundaries(t *testing.T) {
	node := "local"
	cases := []struct {
		name           string
		local          HLC
		incoming       HLC
		wallMS         int64
		wantPhysical   int64
		wantLogicalMin int64 // logical we expect exactly
	}{
		{
			name:         "all equal physical → max(logical)+1",
			local:        HLC{PhysicalMS: 100, Logical: 3, NodeID: node},
			incoming:     HLC{PhysicalMS: 100, Logical: 5, NodeID: "peer"},
			wallMS:       100,
			wantPhysical: 100, wantLogicalMin: 6,
		},
		{
			name:         "local physical wins → local logical+1",
			local:        HLC{PhysicalMS: 200, Logical: 4, NodeID: node},
			incoming:     HLC{PhysicalMS: 100, Logical: 9, NodeID: "peer"},
			wallMS:       150,
			wantPhysical: 200, wantLogicalMin: 5,
		},
		{
			name:         "incoming physical wins → incoming logical+1",
			local:        HLC{PhysicalMS: 100, Logical: 4, NodeID: node},
			incoming:     HLC{PhysicalMS: 300, Logical: 7, NodeID: "peer"},
			wallMS:       150,
			wantPhysical: 300, wantLogicalMin: 8,
		},
		{
			name:         "wall clock newest → logical 0",
			local:        HLC{PhysicalMS: 100, Logical: 4, NodeID: node},
			incoming:     HLC{PhysicalMS: 200, Logical: 7, NodeID: "peer"},
			wallMS:       500,
			wantPhysical: 500, wantLogicalMin: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Recv(tc.local, tc.incoming, tc.wallMS)
			if got.PhysicalMS != tc.wantPhysical {
				t.Errorf("physical: got %d, want %d", got.PhysicalMS, tc.wantPhysical)
			}
			if got.Logical != tc.wantLogicalMin {
				t.Errorf("logical: got %d, want %d", got.Logical, tc.wantLogicalMin)
			}
			if got.NodeID != node {
				t.Errorf("node_id: got %q, want %q (Recv keeps local node)", got.NodeID, node)
			}
		})
	}
}

// TestStore_NowMonotonic drives Now() through the real SQLite-backed hlc_state
// under SetMaxOpenConns(1): successive calls are strictly increasing and the
// physical_ms equals the wall-clock ms of the injected clock (physical_ms ==
// updated_at ms — §3 DEVIATE / R11).
func TestStore_NowMonotonic(t *testing.T) {
	d := openMigrated(t)
	store := NewStore(d, "install-node-1")

	// Pin the clock to a single millisecond: three Now() calls within the same ms
	// must produce logical 0,1,2 — all at the same physical_ms.
	fixed := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixed }

	ctx := context.Background()
	var prev HLC
	for i := 0; i < 3; i++ {
		got, err := store.Now(ctx)
		if err != nil {
			t.Fatalf("Now() #%d: %v", i, err)
		}
		if got.PhysicalMS != fixed.UnixMilli() {
			t.Errorf("physical_ms: got %d, want %d (== wall ms)", got.PhysicalMS, fixed.UnixMilli())
		}
		if got.NodeID != "install-node-1" {
			t.Errorf("node_id: got %q, want install-node-1", got.NodeID)
		}
		if i > 0 && Compare(got, prev) != 1 {
			t.Errorf("Now() not strictly increasing: %+v after %+v", got, prev)
		}
		if int64(i) != got.Logical {
			t.Errorf("logical at same ms: got %d, want %d", got.Logical, i)
		}
		prev = got
	}
}

// TestStore_NowAdvancesPhysical asserts that when the wall clock advances, Now()
// advances physical and resets logical to 0.
func TestStore_NowAdvancesPhysical(t *testing.T) {
	d := openMigrated(t)
	store := NewStore(d, "node-x")
	ctx := context.Background()

	t0 := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return t0 }
	first, err := store.Now(ctx)
	if err != nil {
		t.Fatalf("Now() #1: %v", err)
	}

	t1 := t0.Add(time.Second)
	store.now = func() time.Time { return t1 }
	second, err := store.Now(ctx)
	if err != nil {
		t.Fatalf("Now() #2: %v", err)
	}
	if second.PhysicalMS != t1.UnixMilli() {
		t.Errorf("physical after advance: got %d, want %d", second.PhysicalMS, t1.UnixMilli())
	}
	if second.Logical != 0 {
		t.Errorf("logical after advance: got %d, want 0", second.Logical)
	}
	if Compare(second, first) != 1 {
		t.Errorf("second %+v not after first %+v", second, first)
	}
}

func openMigrated(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "hlc.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}
