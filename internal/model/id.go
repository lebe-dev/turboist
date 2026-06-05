package model

import "github.com/google/uuid"

// NewClientID returns a stable, instance-portable identifier for a synchronized
// entity (Federation v1 F0.1). It is a UUIDv7: a 128-bit, lexicographically
// sortable, time-ordered id (R6 — decided once, used consistently for all
// federation entity ids). It plays the role the federation design calls a
// "ULID" — a sortable, cross-instance entity id — reusing the already-present
// google/uuid dependency rather than adding a new one.
//
// On the (vanishingly unlikely) failure of the system RNG, NewClientID falls
// back to a v4 UUID via uuid.New so callers never have to handle an error when
// minting an id.
func NewClientID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New().String()
	}
	return id.String()
}
