package repo

import "errors"

var (
	ErrNotFound = errors.New("repo: not found")
	ErrConflict = errors.New("repo: conflict")
	// ErrGone signals the row exists but is tombstoned (deleted_at != NULL).
	// Surfaces as 410 Gone at the API boundary so a sync client can drop the
	// pending mutation rather than retrying.
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
