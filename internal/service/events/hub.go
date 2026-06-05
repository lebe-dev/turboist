// Package events provides an in-memory publish/subscribe hub used to deliver
// data-change notifications to long-lived clients (SSE). Events carry only a
// coarse scope; the client uses that scope as an invalidation hint and refetches
// via the regular REST endpoints. There is no persistence and no replay —
// missed events are recovered by the client's regular refetch after reconnect.
package events

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/lebe-dev/turboist/internal/logging"
)

// Scope identifies a coarse-grained data domain that has changed.
type Scope string

const (
	ScopeTasks    Scope = "tasks"
	ScopeCalendar Scope = "calendar"
	ScopeInbox    Scope = "inbox"
	ScopeProjects Scope = "projects"
	ScopeLabels   Scope = "labels"
	ScopeContexts Scope = "contexts"
	ScopeSections Scope = "sections"
	ScopePlan     Scope = "plan"
	// ScopeComments and ScopeChecklist are emitted when a task's comments or
	// checklist items change (Federation v1 F0.2). The frontend uses them as
	// invalidation hints for the open task card.
	ScopeComments  Scope = "comments"
	ScopeChecklist Scope = "checklist"
	// ScopeFederation is published when a federated project's per-peer sync health
	// transitions (Federation v1 F4.3, US-4.3): an undelivered batch crosses the
	// pending threshold, a peer goes unreachable, or a peer's key stops validating.
	// The frontend federation store reloads its sync-status badges on this scope.
	ScopeFederation Scope = "federation"
)

// Event is a single change notification. Payload is intentionally empty;
// subscribers refetch via REST.
type Event struct {
	Scope Scope `json:"scope"`
}

// subscriber buffer size. Bursts are absorbed; on overflow events are dropped
// rather than blocking publishers. Drops are logged.
const subscriberBufferSize = 16

type subscriber struct {
	id      uint64
	userID  int64
	origin  string
	ch      chan Event
	closeCh sync.Once
}

func (s *subscriber) close() {
	s.closeCh.Do(func() { close(s.ch) })
}

// Hub is a thread-safe in-memory fan-out keyed by user id. Zero value is not
// usable — construct with NewHub.
type Hub struct {
	mu     sync.RWMutex
	subs   map[int64]map[uint64]*subscriber
	nextID atomic.Uint64
	closed bool
	log    *slog.Logger
}

// NewHub creates an empty hub. log is used to report dropped events; if nil,
// slog.Default is used.
func NewHub(log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{
		subs: make(map[int64]map[uint64]*subscriber),
		log:  log,
	}
}

// Subscribe registers a new subscriber for userID. The returned channel
// receives events until cancel is called or the hub is closed. cancel is
// idempotent and safe to call from any goroutine. If the hub is already
// closed, the returned channel is closed immediately.
//
// An optional origin tags the subscriber with the client that owns this
// stream. Publish skips subscribers whose origin matches the publishing
// request's origin, so a client never receives the echo of its own mutation
// (see Publish). An empty origin disables this suppression for the subscriber.
func (h *Hub) Subscribe(userID int64, origin ...string) (<-chan Event, func()) {
	s := &subscriber{
		id:     h.nextID.Add(1),
		userID: userID,
		origin: firstOrigin(origin),
		ch:     make(chan Event, subscriberBufferSize),
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		s.close()
		return s.ch, func() {}
	}
	bucket, ok := h.subs[userID]
	if !ok {
		bucket = make(map[uint64]*subscriber)
		h.subs[userID] = bucket
	}
	bucket[s.id] = s
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		if bucket, ok := h.subs[userID]; ok {
			delete(bucket, s.id)
			if len(bucket) == 0 {
				delete(h.subs, userID)
			}
		}
		h.mu.Unlock()
		s.close()
	}
	return s.ch, cancel
}

// Close shuts down the hub: every active subscriber's channel is closed and
// further Subscribe calls return an already-closed channel. Idempotent.
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	subs := h.subs
	h.subs = make(map[int64]map[uint64]*subscriber)
	h.mu.Unlock()
	for _, bucket := range subs {
		for _, s := range bucket {
			s.close()
		}
	}
}

// Publish delivers an event to every subscriber of userID. Non-blocking: if a
// subscriber's buffer is full the event is dropped for that subscriber and a
// warning is logged.
//
// An optional origin identifies the client that triggered the change.
// Subscribers tagged with the same origin are skipped — a client must not
// receive the echo of its own mutation, since it already applies the change
// from the mutation's own response. An empty origin delivers to everyone.
func (h *Hub) Publish(ctx context.Context, userID int64, scope Scope, origin ...string) {
	skip := firstOrigin(origin)

	h.mu.RLock()
	bucket := h.subs[userID]
	if len(bucket) == 0 {
		h.mu.RUnlock()
		return
	}
	targets := make([]*subscriber, 0, len(bucket))
	for _, s := range bucket {
		if skip != "" && s.origin == skip {
			continue
		}
		targets = append(targets, s)
	}
	h.mu.RUnlock()

	ev := Event{Scope: scope}
	log := h.logger(ctx)
	for _, s := range targets {
		select {
		case s.ch <- ev:
		default:
			log.WarnContext(ctx, "events: subscriber buffer full, dropping event",
				slog.Int64("user_id", userID),
				slog.String("scope", string(scope)),
				slog.Uint64("subscriber_id", s.id),
			)
		}
	}
}

// SubscriberCount returns the number of active subscribers for userID. Test
// hook; not used in production paths.
func (h *Hub) SubscriberCount(userID int64) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[userID])
}

// firstOrigin extracts the optional origin from a variadic argument,
// returning "" when none was supplied.
func firstOrigin(origin []string) string {
	if len(origin) == 0 {
		return ""
	}
	return origin[0]
}

func (h *Hub) logger(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return h.log
	}
	return logging.FromContext(ctx)
}
