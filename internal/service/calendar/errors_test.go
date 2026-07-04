package calendar

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/oauth2"
)

func TestIsReauthRequired(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"invalid_grant", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}, true},
		{"invalid_client", &oauth2.RetrieveError{ErrorCode: "invalid_client"}, true},
		{"wrapped invalid_grant", fmt.Errorf("token source: %w", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}), true},
		{"transient oauth code", &oauth2.RetrieveError{ErrorCode: "temporarily_unavailable"}, false},
		{"sentinel", ErrReauthRequired, true},
		{"unrelated", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReauthRequired(tt.err); got != tt.want {
				t.Fatalf("IsReauthRequired(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestAsReauthError(t *testing.T) {
	cause := &oauth2.RetrieveError{ErrorCode: "invalid_grant"}
	got := asReauthError(cause)
	if !errors.Is(got, ErrReauthRequired) {
		t.Fatalf("expected wrapped error to match ErrReauthRequired")
	}
	if !errors.Is(got, cause) {
		t.Fatalf("expected wrapped error to preserve the original cause for logging")
	}

	plain := errors.New("network down")
	if asReauthError(plain) != plain {
		t.Fatalf("expected non-reauth error to pass through unchanged")
	}
	if asReauthError(nil) != nil {
		t.Fatalf("expected nil to pass through")
	}
}
