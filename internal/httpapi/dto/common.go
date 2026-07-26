package dto

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// PageParams holds validated pagination parameters.
type PageParams struct {
	Limit  int
	Offset int
}

const (
	DefaultLimit = 50
	MaxLimit     = 200
	// CompletedMaxLimit is a higher ceiling for the completed-history view,
	// which is fetched in one shot (no pagination) and grouped by day on the
	// client — it can legitimately span hundreds of rows over its window.
	CompletedMaxLimit = 1000
)

// ParsePageParams reads limit/offset strings, applies defaults and clamps to
// MaxLimit.
func ParsePageParams(limitStr, offsetStr string) PageParams {
	return ParsePageParamsMax(limitStr, offsetStr, MaxLimit)
}

// ParsePageParamsMax is ParsePageParams with a caller-supplied upper bound, for
// views (e.g. completed history) that fetch a full window in a single request.
func ParsePageParamsMax(limitStr, offsetStr string, maxLimit int) PageParams {
	p := PageParams{Limit: DefaultLimit}
	if n, err := strconv.Atoi(limitStr); err == nil {
		p.Limit = n
	}
	if n, err := strconv.Atoi(offsetStr); err == nil {
		p.Offset = n
	}
	if p.Limit <= 0 {
		p.Limit = DefaultLimit
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}

// PagedResponse is the standard list envelope.
type PagedResponse[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// NewPagedResponse constructs a paged envelope, guaranteeing a non-nil items slice.
func NewPagedResponse[T any](items []T, total, limit, offset int) PagedResponse[T] {
	if items == nil {
		items = []T{}
	}
	return PagedResponse[T]{Items: items, Total: total, Limit: limit, Offset: offset}
}

// Optional[T] distinguishes absent (key not in JSON), null (explicit JSON null),
// and a concrete value. Used for PATCH requests where missing != null != value.
type Optional[T any] struct {
	value T
	state optState
}

type optState uint8

const (
	optAbsent optState = iota
	optNull
	optSet
)

func (o Optional[T]) IsAbsent() bool { return o.state == optAbsent }
func (o Optional[T]) IsNull() bool   { return o.state == optNull }
func (o Optional[T]) IsSet() bool    { return o.state == optSet }

// Value returns the contained value and true when the optional is set.
func (o Optional[T]) Value() (T, bool) {
	if o.state == optSet {
		return o.value, true
	}
	var zero T
	return zero, false
}

func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		o.state = optNull
		return nil
	}
	if err := json.Unmarshal(data, &o.value); err != nil {
		return fmt.Errorf("optional: %w", err)
	}
	o.state = optSet
	return nil
}

func (o Optional[T]) MarshalJSON() ([]byte, error) {
	switch o.state {
	case optNull:
		return []byte("null"), nil
	case optSet:
		return json.Marshal(o.value)
	default:
		return []byte("null"), nil
	}
}

// FormatTime formats a time.Time per API convention: 2006-01-02T15:04:05.000Z
func FormatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// FormatTimePtr returns the formatted string pointer, or nil for a nil time.
func FormatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := FormatTime(*t)
	return &s
}
