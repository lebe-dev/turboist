package federation

import (
	"context"

	fedevents "github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/service/events"
)

// HubNotifier publishes a federation-origin change to the SSE hub after the
// inbox-apply goroutine merges a remote event that altered local state
// (Federation v1 F3.2, US-3.1 AC2). It satisfies inbox.Notifier.
//
// Crucially the publish carries NO origin, so it is NOT echo-suppressed: a remote
// peer's edit is not the local user's own mutation, so every open tab must learn
// about it (unlike a local mutation, whose echo the originating tab suppresses).
// The single-user model means the audience is always user id 1.
//
// The open-card "updated remotely" affordance (a non-destructive notice rather
// than a blind clobber) is layered on the FRONTEND in F3.4; this notifier just
// drives the coarse scope refresh the SSE layer already understands.
type HubNotifier struct {
	hub    *events.Hub
	userID int64
}

// NewHubNotifier constructs the notifier. A nil hub disables notification
// (federation can run headless / in tests).
func NewHubNotifier(hub *events.Hub) *HubNotifier {
	return &HubNotifier{hub: hub, userID: 1}
}

// Notify publishes the coarse scopes touched by an applied remote event.
func (n *HubNotifier) Notify(ctx context.Context, ev inbox.Applied) {
	if n.hub == nil {
		return
	}
	for _, scope := range scopesForEntity(ev.Event.EntityType) {
		// No origin → delivered to all subscribers (federation is never the
		// caller's own echo, US-3.1 AC2).
		n.hub.Publish(ctx, n.userID, scope)
	}
}

// NotifyFederation publishes a ScopeFederation SSE so the owner's open tabs
// reload their sync-status badges after a per-peer health transition (Federation
// v1 F4.3, US-4.3): a peer's key stopped validating, a peer went unreachable, or
// an undelivered batch crossed the pending threshold. It satisfies
// StatusNotifier. Like Notify it carries no origin (a health change is not the
// user's own echo) and targets the single user id 1.
func (n *HubNotifier) NotifyFederation(ctx context.Context) {
	if n.hub == nil {
		return
	}
	n.hub.Publish(ctx, n.userID, events.ScopeFederation)
}

// scopesForEntity maps a federated entity type to the coarse SSE scopes its
// change invalidates. A project/section change refreshes the project + task
// views; a task change refreshes tasks (+ plan/inbox aggregates); comments and
// checklist items refresh their dedicated scopes plus tasks so an open card
// updates (mirrors publish_middleware.go's task sub-resource scopes).
func scopesForEntity(t fedevents.EntityType) []events.Scope {
	switch t {
	case fedevents.EntityProject:
		return []events.Scope{events.ScopeProjects, events.ScopeTasks}
	case fedevents.EntitySection:
		return []events.Scope{events.ScopeSections, events.ScopeTasks}
	case fedevents.EntityTask:
		return []events.Scope{events.ScopeTasks, events.ScopePlan, events.ScopeInbox}
	case fedevents.EntityComment:
		return []events.Scope{events.ScopeComments, events.ScopeTasks}
	case fedevents.EntityChecklistItem:
		return []events.Scope{events.ScopeChecklist, events.ScopeTasks}
	default:
		return []events.Scope{events.ScopeTasks}
	}
}
