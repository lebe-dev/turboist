package auth

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const ScopeWildcard = "*"

// ValidScopes lists every concrete scope accepted on API tokens.
// Wildcard "*" is allowed but is not part of this list.
var ValidScopes = []string{
	"tasks:read", "tasks:write",
	"projects:read", "projects:write",
	"contexts:read", "contexts:write",
	"labels:read", "labels:write",
	"sections:read", "sections:write",
	"troiki:read", "troiki:write",
	"settings:read", "settings:write",
	"search:read",
	"calendars:read",
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
