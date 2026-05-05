package repo

import "errors"

var (
	ErrNotFound = errors.New("repo: not found")
	ErrConflict = errors.New("repo: conflict")
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
