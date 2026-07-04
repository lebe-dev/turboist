package auth

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const ScopeWildcard = "*"

// Concrete scope names. Use these constants instead of raw "<resource>:<action>"
// string literals so the set has a single source of truth.
const (
	ScopeTasksRead      = "tasks:read"
	ScopeTasksWrite     = "tasks:write"
	ScopeProjectsRead   = "projects:read"
	ScopeProjectsWrite  = "projects:write"
	ScopeContextsRead   = "contexts:read"
	ScopeContextsWrite  = "contexts:write"
	ScopeLabelsRead     = "labels:read"
	ScopeLabelsWrite    = "labels:write"
	ScopeTemplatesRead  = "templates:read"
	ScopeTemplatesWrite = "templates:write"
	ScopeSectionsRead   = "sections:read"
	ScopeSectionsWrite  = "sections:write"
	ScopeTroikiRead     = "troiki:read"
	ScopeTroikiWrite    = "troiki:write"
	ScopeSettingsRead   = "settings:read"
	ScopeSettingsWrite  = "settings:write"
	ScopeSearchRead     = "search:read"
	ScopeCalendarsRead  = "calendars:read"
)

// ValidScopes lists every concrete scope accepted on API tokens.
// Wildcard "*" is allowed but is not part of this list.
var ValidScopes = []string{
	ScopeTasksRead, ScopeTasksWrite,
	ScopeProjectsRead, ScopeProjectsWrite,
	ScopeContextsRead, ScopeContextsWrite,
	ScopeLabelsRead, ScopeLabelsWrite,
	ScopeTemplatesRead, ScopeTemplatesWrite,
	ScopeSectionsRead, ScopeSectionsWrite,
	ScopeTroikiRead, ScopeTroikiWrite,
	ScopeSettingsRead, ScopeSettingsWrite,
	ScopeSearchRead,
	ScopeCalendarsRead,
}

// ValidateScopes enforces the rules described in PLAN.md §3:
//   - non-empty
//   - every entry is in ValidScopes or equals "*"
//   - no duplicates
//   - "*" cannot coexist with other scopes
//   - "<resource>:write" requires "<resource>:read"
func ValidateScopes(scopes []string) error {
	if len(scopes) == 0 {
		return errors.New("scopes must not be empty")
	}
	if len(scopes) > len(ValidScopes)+1 {
		return fmt.Errorf("too many scopes: %d", len(scopes))
	}

	seen := make(map[string]struct{}, len(scopes))
	hasWildcard := false
	for _, s := range scopes {
		if _, dup := seen[s]; dup {
			return fmt.Errorf("duplicate scope: %s", s)
		}
		seen[s] = struct{}{}

		if s == ScopeWildcard {
			hasWildcard = true
			continue
		}
		if !slices.Contains(ValidScopes, s) {
			return fmt.Errorf("invalid scope: %s", s)
		}
	}

	if hasWildcard && len(scopes) > 1 {
		return errors.New("wildcard '*' cannot be combined with other scopes")
	}

	for s := range seen {
		resource, ok := strings.CutSuffix(s, ":write")
		if !ok {
			continue
		}
		readScope := resource + ":read"
		if _, ok := seen[readScope]; !ok {
			return fmt.Errorf("%s requires %s", s, readScope)
		}
	}

	return nil
}

// HasScope reports whether the granted set satisfies the required scope.
// A granted "*" satisfies any required scope.
func HasScope(granted []string, required string) bool {
	for _, s := range granted {
		if s == ScopeWildcard || s == required {
			return true
		}
	}
	return false
}
