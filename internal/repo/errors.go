package repo

import "errors"

var (
	ErrNotFound = errors.New("repo: not found")
	ErrConflict = errors.New("repo: conflict")
	// ErrGone signals that a row exists but has been soft-deleted (its
	// deleted_at tombstone is set). A tombstone is final, so any attempt to
	// re-edit it is rejected; handlers map this to HTTP 410 Gone (Federation
	// v1 F0.1, US-3.7 AC2 foundation).
	ErrGone = errors.New("repo: gone")
)

type Page struct {
	Limit  int
	Offset int
}

func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}
