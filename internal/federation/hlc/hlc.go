// Package hlc implements the federation Hybrid Logical Clock (Federation v1
// F2.3, §5.3). An HLC is the causal timestamp federated events are ordered by;
// per-field Last-Writer-Wins compares HLCs to decide which write wins.
//
// The wire/string form is `{physical_ms}-{logical}-{node_id}` with physical_ms
// and logical ZERO-PADDED to a fixed width so a plain lexical (string) compare
// equals the numeric total order — that is what the per-field LWW SQL relies on.
//
//	physical_ms — Unix milliseconds of the local wall clock; on the federation
//	              path this is sourced from the SAME time.Now() that writes
//	              updated_at, so the two never drift (§3 DEVIATE / R11).
//	logical     — collision counter, bumped when physical does not advance.
//	node_id     — a stable generated install UUID (R10), used ONLY to break ties
//	              when physical and logical are equal; never derived from BASE_URL.
package hlc

import (
	"fmt"
	"strconv"
	"strings"
)

// physicalWidth / logicalWidth are the fixed zero-padded field widths. 14 digits
// of milliseconds reaches the year ~5138, comfortably beyond any real clock; 4
// digits of logical matches the design's "4-значный uint" collision counter.
const (
	physicalWidth = 14
	logicalWidth  = 4
	maxPhysical   = 99999999999999 // 14 nines
	maxLogical    = 9999           // 4 nines
)

// HLC is a parsed Hybrid Logical Clock value.
type HLC struct {
	PhysicalMS int64
	Logical    int64
	NodeID     string
}

// String renders the canonical zero-padded form `{physical}-{logical}-{node}`.
func (h HLC) String() string {
	return h.physicalField() + "-" + h.logicalField() + "-" + h.NodeID
}

func (h HLC) physicalField() string {
	return fmt.Sprintf("%0*d", physicalWidth, h.PhysicalMS)
}

func (h HLC) logicalField() string {
	return fmt.Sprintf("%0*d", logicalWidth, h.Logical)
}

// Parse decodes a canonical HLC string. The node_id is the remainder after the
// first two dash-separated fields, so a node_id that itself contains dashes (a
// UUID) is preserved. A malformed or out-of-range string returns an error.
func Parse(s string) (HLC, error) {
	// Split into at most 3 fields: physical, logical, node (node may contain '-').
	first := strings.IndexByte(s, '-')
	if first <= 0 {
		return HLC{}, fmt.Errorf("hlc: malformed %q", s)
	}
	rest := s[first+1:]
	second := strings.IndexByte(rest, '-')
	if second <= 0 {
		return HLC{}, fmt.Errorf("hlc: malformed %q", s)
	}
	physStr := s[:first]
	logStr := rest[:second]
	node := rest[second+1:]
	if node == "" {
		return HLC{}, fmt.Errorf("hlc: missing node_id in %q", s)
	}
	if len(physStr) != physicalWidth || len(logStr) != logicalWidth {
		return HLC{}, fmt.Errorf("hlc: field width in %q", s)
	}
	phys, err := strconv.ParseInt(physStr, 10, 64)
	if err != nil {
		return HLC{}, fmt.Errorf("hlc: parse physical in %q: %w", s, err)
	}
	logical, err := strconv.ParseInt(logStr, 10, 64)
	if err != nil {
		return HLC{}, fmt.Errorf("hlc: parse logical in %q: %w", s, err)
	}
	return HLC{PhysicalMS: phys, Logical: logical, NodeID: node}, nil
}

// Compare returns -1, 0, or 1 ordering a before, equal to, or after b. The total
// order is physical, then logical, then node_id (the tie-break, §5.3).
func Compare(a, b HLC) int {
	if a.PhysicalMS != b.PhysicalMS {
		return signI64(a.PhysicalMS - b.PhysicalMS)
	}
	if a.Logical != b.Logical {
		return signI64(a.Logical - b.Logical)
	}
	return strings.Compare(a.NodeID, b.NodeID)
}

// CompareString compares two canonical HLC strings lexically — equivalent to
// Compare on the parsed values because the numeric fields are zero-padded. It is
// the cheap path the per-field LWW uses on stored strings without re-parsing.
func CompareString(a, b string) int {
	return strings.Compare(a, b)
}

func signI64(d int64) int {
	if d < 0 {
		return -1
	}
	if d > 0 {
		return 1
	}
	return 0
}

// LocalTick advances a local clock for a local event given the current wall
// clock in milliseconds (§5.3 local rule):
//
//	now = max(local.physical, wall)
//	if now == local.physical: logical++  else: physical=now, logical=0
//
// A wall clock that ran backward never moves physical backward — it stays put
// and bumps logical. NodeID is preserved.
func LocalTick(local HLC, wallMS int64) HLC {
	now := maxI64(local.PhysicalMS, wallMS)
	out := HLC{PhysicalMS: now, NodeID: local.NodeID}
	if now == local.PhysicalMS {
		out.Logical = clampLogical(local.Logical + 1)
	} else {
		out.Logical = 0
	}
	return out
}

// Recv merges an incoming HLC into the local clock on receipt of a remote event
// (§5.3 receive rule). NodeID stays the LOCAL node (the merged clock is ours).
func Recv(local, incoming HLC, wallMS int64) HLC {
	newPhysical := maxI64(maxI64(local.PhysicalMS, incoming.PhysicalMS), wallMS)
	var newLogical int64
	switch {
	case newPhysical == local.PhysicalMS && newPhysical == incoming.PhysicalMS:
		newLogical = maxI64(local.Logical, incoming.Logical) + 1
	case newPhysical == local.PhysicalMS:
		newLogical = local.Logical + 1
	case newPhysical == incoming.PhysicalMS:
		newLogical = incoming.Logical + 1
	default:
		newLogical = 0
	}
	return HLC{PhysicalMS: newPhysical, Logical: clampLogical(newLogical), NodeID: local.NodeID}
}

func maxI64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// clampLogical keeps the logical counter inside its fixed width. In practice the
// counter only climbs within a single millisecond, so the bound is effectively
// unreachable; clamping is a defensive guard against a malformed/overflowing
// value breaking the fixed-width string invariant.
func clampLogical(l int64) int64 {
	if l > maxLogical {
		return maxLogical
	}
	if l < 0 {
		return 0
	}
	return l
}
