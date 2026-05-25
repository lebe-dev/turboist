package auth

import (
	"strings"
	"testing"
)

func TestValidateScopes_Valid(t *testing.T) {
	if err := ValidateScopes([]string{"tasks:read", "tasks:write"}); err != nil {
		t.Fatalf("expected ok, got: %v", err)
	}
}

func TestValidateScopes_AllScopes(t *testing.T) {
	all := append([]string{}, ValidScopes...)
	if err := ValidateScopes(all); err != nil {
		t.Fatalf("expected ok for full ValidScopes set, got: %v", err)
	}
}

func TestValidateScopes_Wildcard(t *testing.T) {
	if err := ValidateScopes([]string{"*"}); err != nil {
		t.Fatalf("expected ok for [*], got: %v", err)
	}
}

func TestValidateScopes_Empty(t *testing.T) {
	err := ValidateScopes(nil)
	if err == nil {
		t.Fatalf("expected error for empty scopes")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error message: got %q, want it to mention 'empty'", err.Error())
	}
}

func TestValidateScopes_Invalid(t *testing.T) {
	err := ValidateScopes([]string{"foo:bar"})
	if err == nil {
		t.Fatalf("expected error for invalid scope")
	}
	if !strings.Contains(err.Error(), "foo:bar") {
		t.Errorf("error message: got %q, want it to mention scope name", err.Error())
	}
}

func TestValidateScopes_Duplicate(t *testing.T) {
	err := ValidateScopes([]string{"tasks:read", "tasks:read"})
	if err == nil {
		t.Fatalf("expected error for duplicate")
	}
	if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "tasks:read") {
		t.Errorf("error message: got %q, want it to mention 'duplicate' and scope name", err.Error())
	}
}

func TestValidateScopes_WildcardWithOther(t *testing.T) {
	err := ValidateScopes([]string{"*", "tasks:read"})
	if err == nil {
		t.Fatalf("expected error for wildcard combined with other scopes")
	}
	if !strings.Contains(err.Error(), "wildcard") {
		t.Errorf("error message: got %q, want it to mention 'wildcard'", err.Error())
	}
}

func TestValidateScopes_WriteWithoutRead(t *testing.T) {
	err := ValidateScopes([]string{"tasks:write"})
	if err == nil {
		t.Fatalf("expected error for write without read")
	}
	want := "tasks:write requires tasks:read"
	if err.Error() != want {
		t.Errorf("error message: got %q, want %q", err.Error(), want)
	}
}

func TestValidateScopes_TooMany(t *testing.T) {
	too := make([]string, 0, len(ValidScopes)+5)
	for i := 0; i < len(ValidScopes)+5; i++ {
		too = append(too, "tasks:read")
	}
	if err := ValidateScopes(too); err == nil {
		t.Fatalf("expected error for excessive scope count")
	}
}

func TestHasScope_Wildcard(t *testing.T) {
	if !HasScope([]string{"*"}, "tasks:read") {
		t.Errorf("wildcard must grant tasks:read")
	}
	if !HasScope([]string{"*"}, "anything:future") {
		t.Errorf("wildcard must grant any required scope")
	}
}

func TestHasScope_ExactMatch(t *testing.T) {
	if !HasScope([]string{"tasks:read", "projects:write"}, "tasks:read") {
		t.Errorf("exact match must succeed")
	}
}

func TestHasScope_Missing(t *testing.T) {
	if HasScope([]string{"tasks:read"}, "tasks:write") {
		t.Errorf("expected false for missing scope")
	}
	if HasScope(nil, "tasks:read") {
		t.Errorf("expected false for empty granted")
	}
}
