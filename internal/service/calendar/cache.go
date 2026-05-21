package calendar

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// CalendarEvent is the domain representation of a single calendar event.
type CalendarEvent struct {
	ID          string
	SourceID    int64
	SourceName  string
	SourceColor string
	Provider    string
	ExternalID  string
	Title       string
	Location    string
	Start       time.Time
	End         time.Time
	StartDate   string
	EndDate     string
	AllDay      bool
	HTMLLink    string
}

// EventsCacheKey builds a deterministic cache key for the given query parameters.
func EventsCacheKey(userID int64, start, end time.Time, sources []model.CalendarSource) string {
	parts := make([]string, 0, 3+len(sources))
	parts = append(parts,
		fmt.Sprintf("%d", userID),
		start.UTC().Format(time.RFC3339Nano),
		end.UTC().Format(time.RFC3339Nano),
	)
	for _, source := range sources {
		parts = append(parts, fmt.Sprintf("%d:%s", source.ID, source.ExternalID))
	}
	return strings.Join(parts, "|")
}

type cachedEntry struct {
	expiresAt time.Time
	items     []CalendarEvent
}

// EventCache is a thread-safe in-memory cache for calendar events.
type EventCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]cachedEntry
}

// NewEventCache creates an EventCache with the given TTL.
func NewEventCache(ttl time.Duration) *EventCache {
	return &EventCache{
		ttl:   ttl,
		items: make(map[string]cachedEntry),
	}
}

// Get returns cached events for key. Returns nil, false on miss or expiry.
func (c *EventCache) Get(key string) ([]CalendarEvent, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.items, key)
		return nil, false
	}
	return cloneEvents(entry.items), true
}

// Set stores events in the cache under key.
func (c *EventCache) Set(key string, items []CalendarEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = cachedEntry{
		expiresAt: time.Now().Add(c.ttl),
		items:     cloneEvents(items),
	}
}

// DeleteUser removes all cached entries for the given user.
func (c *EventCache) DeleteUser(userID int64) {
	prefix := fmt.Sprintf("%d|", userID)
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.items {
		if strings.HasPrefix(key, prefix) {
			delete(c.items, key)
		}
	}
}

func cloneEvents(items []CalendarEvent) []CalendarEvent {
	out := make([]CalendarEvent, len(items))
	copy(out, items)
	return out
}
