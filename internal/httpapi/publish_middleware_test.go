package httpapi

import (
	"reflect"
	"testing"

	"github.com/lebe-dev/turboist/internal/service/events"
)

func TestScopesForPath(t *testing.T) {
	tests := []struct {
		path string
		want []events.Scope
	}{
		{"/api/v1/tasks", []events.Scope{events.ScopeTasks, events.ScopePlan, events.ScopeInbox}},
		{"/api/v1/tasks/123", []events.Scope{events.ScopeTasks, events.ScopePlan, events.ScopeInbox}},
		{"/api/v1/tasks/123/complete", []events.Scope{events.ScopeTasks, events.ScopePlan, events.ScopeInbox}},
		{"/api/v1/inbox", []events.Scope{events.ScopeTasks, events.ScopeInbox, events.ScopePlan}},
		{"/api/v1/projects/7", []events.Scope{events.ScopeProjects, events.ScopeTasks}},
		{"/api/v1/labels", []events.Scope{events.ScopeLabels, events.ScopeTasks}},
		{"/api/v1/contexts/2", []events.Scope{events.ScopeContexts}},
		{"/api/v1/sections", []events.Scope{events.ScopeSections, events.ScopeTasks}},
		{"/api/v1/calendars/google/sync", []events.Scope{events.ScopeCalendar}},
		{"/api/v1/troiki/start", []events.Scope{events.ScopeTasks, events.ScopePlan}},
		{"/api/v1/app-settings", []events.Scope{events.ScopeLabels, events.ScopeTasks}},
		// instantiate materialises a task tree; template CRUD shares the hint
		{"/api/v1/task-templates/4/instantiate", []events.Scope{events.ScopeTasks, events.ScopePlan, events.ScopeInbox}},
		{"/api/v1/task-templates", []events.Scope{events.ScopeTasks, events.ScopePlan, events.ScopeInbox}},

		// excluded — issuing an SSE ticket is not a data mutation
		{"/api/v1/events/ticket", nil},
		// unknown / unmapped
		{"/api/v1/api-tokens", nil},
		{"/api/v1/backup", nil},
		{"/api/v1/config", nil},
		// non-API paths
		{"/healthz", nil},
		{"/auth/login", nil},
		{"", nil},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := scopesForPath(tc.path)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("path %q: got %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestIsMutatingMethod(t *testing.T) {
	mutating := []string{"POST", "PUT", "PATCH", "DELETE"}
	for _, m := range mutating {
		if !isMutatingMethod(m) {
			t.Errorf("%s should be mutating", m)
		}
	}
	for _, m := range []string{"GET", "HEAD", "OPTIONS", ""} {
		if isMutatingMethod(m) {
			t.Errorf("%s should not be mutating", m)
		}
	}
}
