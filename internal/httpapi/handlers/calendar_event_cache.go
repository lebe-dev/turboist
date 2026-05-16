package handlers

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

type cachedCalendarEvents struct {
	expiresAt time.Time
	items     []calendarEventResp
}

type calendarEventCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]cachedCalendarEvents
}

func newCalendarEventCache(ttl time.Duration) *calendarEventCache {
	return &calendarEventCache{
		ttl:   ttl,
		items: make(map[string]cachedCalendarEvents),
	}
}

func (c *calendarEventCache) get(key string) ([]calendarEventResp, bool) {
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
	return cloneCalendarEvents(entry.items), true
}

func (c *calendarEventCache) set(key string, items []calendarEventResp) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = cachedCalendarEvents{
		expiresAt: time.Now().Add(c.ttl),
		items:     cloneCalendarEvents(items),
	}
}

func (c *calendarEventCache) deleteUser(userID int64) {
	prefix := fmt.Sprintf("%d|", userID)
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.items {
		if strings.HasPrefix(key, prefix) {
			delete(c.items, key)
		}
	}
}

func calendarEventsCacheKey(userID int64, start, end time.Time, sources []model.CalendarSource) string {
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

func cloneCalendarEvents(items []calendarEventResp) []calendarEventResp {
	out := make([]calendarEventResp, len(items))
	copy(out, items)
	return out
}
