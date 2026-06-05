package httpapi

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/service/events"
)

// PublishMiddleware emits coarse-grained data-change notifications after a
// successful mutating request. Frontends use these as a hint to refetch the
// affected views; the payload is intentionally empty.
//
// Returning a nil hub disables publishing — useful in tests that do not care
// about the event channel.
func PublishMiddleware(hub *events.Hub) fiber.Handler {
	return func(c fiber.Ctx) error {
		if err := c.Next(); err != nil {
			return err
		}
		if hub == nil || !isMutatingMethod(c.Method()) {
			return nil
		}
		status := c.Response().StatusCode()
		if status < 200 || status >= 300 {
			return nil
		}
		userID := GetUserID(c)
		if userID == 0 {
			return nil
		}
		// The originating client tags its mutations with X-Client-Origin; the
		// hub skips that client's own stream so it never receives the echo of
		// a change it already applied locally from the response.
		origin := c.Get(clientOriginHeader)
		for _, scope := range scopesForPath(c.Path()) {
			hub.Publish(c.Context(), userID, scope, origin)
		}
		return nil
	}
}

// clientOriginHeader carries the caller's per-tab client id on mutating
// requests. See Hub.Publish for how it suppresses self-echo.
const clientOriginHeader = "X-Client-Origin"

func isMutatingMethod(m string) bool {
	switch m {
	case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		return true
	}
	return false
}

// scopesForPath maps a /api/v1/... request path to the change scopes it can
// produce. Scopes are intentionally coarse — the frontend uses them as
// invalidation hints. When in doubt, include adjacent scopes (e.g., task
// mutations also touch plan stats and inbox counts).
//
// /api/v1/events/* is excluded because issuing a stream ticket is not a data
// mutation.
func scopesForPath(p string) []events.Scope {
	const apiPrefix = "/api/v1/"
	if !strings.HasPrefix(p, apiPrefix) {
		return nil
	}
	rest := p[len(apiPrefix):]
	if rest == "" {
		return nil
	}
	// First path segment determines the domain.
	domain, tail, _ := strings.Cut(rest, "/")
	switch domain {
	case "events":
		return nil
	case "tasks":
		// Task sub-resources publish their own coarse scope on top of the task
		// scopes so the open card refetches comments/checklist (Federation v1
		// F0.2). tail here is e.g. "42/comments" or "42/checklist/7".
		if scope, ok := taskSubResourceScope(tail); ok {
			return []events.Scope{scope, events.ScopeTasks}
		}
		return []events.Scope{events.ScopeTasks, events.ScopePlan, events.ScopeInbox}
	case "inbox":
		return []events.Scope{events.ScopeTasks, events.ScopeInbox, events.ScopePlan}
	case "projects":
		return []events.Scope{events.ScopeProjects, events.ScopeTasks}
	case "labels":
		return []events.Scope{events.ScopeLabels, events.ScopeTasks}
	case "contexts":
		return []events.Scope{events.ScopeContexts}
	case "sections":
		return []events.Scope{events.ScopeSections, events.ScopeTasks}
	case "calendars":
		return []events.Scope{events.ScopeCalendar}
	case "troiki":
		return []events.Scope{events.ScopeTasks, events.ScopePlan}
	case "app-settings":
		// Auto-label rules etc. affect how tasks are presented.
		return []events.Scope{events.ScopeLabels, events.ScopeTasks}
	}
	return nil
}

// taskSubResourceScope maps the path tail after "tasks/" (e.g. "42/comments" or
// "42/checklist/7") to its dedicated change scope. It returns false when the
// tail is not a comments/checklist sub-resource, so the caller falls back to the
// plain task scopes (Federation v1 F0.2).
func taskSubResourceScope(tail string) (events.Scope, bool) {
	_, sub, ok := strings.Cut(tail, "/")
	if !ok {
		return "", false
	}
	resource, _, _ := strings.Cut(sub, "/")
	switch resource {
	case "comments":
		return events.ScopeComments, true
	case "checklist":
		return events.ScopeChecklist, true
	}
	return "", false
}
