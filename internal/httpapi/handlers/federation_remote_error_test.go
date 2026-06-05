package handlers

import (
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// TestMapRemoteHandshakeError_UpstreamVsRejection asserts that an owner 5xx
// (a transient upstream fault, e.g. a mid-build DB failure) maps to the retryable
// 502 federation_upstream error, NOT the generic 401 invite-rejection — while
// 4xx auth/invite codes keep collapsing to the generic rejection (F2.3 #8).
func TestMapRemoteHandshakeError_UpstreamVsRejection(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		code       string
		wantStatus int
		wantCode   string
	}{
		{"owner 500", 500, "", 502, httpapi.CodeFederationUpstream},
		{"owner 503", 503, "", 502, httpapi.CodeFederationUpstream},
		{"owner 500 with unrelated code", 500, "internal_error", 502, httpapi.CodeFederationUpstream},
		{"wrong secret 401", 401, "federation_signature_invalid", 401, httpapi.CodeFederationSignatureInvalid},
		{"unknown invite 401", 401, "", 401, httpapi.CodeFederationSignatureInvalid},
		{"version unsupported 400", 400, httpapi.CodeFederationVersionUnsupported, 400, httpapi.CodeFederationVersionUnsupported},
		{"stale invite 410", 410, httpapi.CodeGone, 410, httpapi.CodeGone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mapRemoteHandshakeError(&fedsvc.RemoteHandshakeError{StatusCode: tc.statusCode, Code: tc.code})
			appErr, ok := err.(*httpapi.AppError)
			if !ok {
				t.Fatalf("mapped error type: got %T, want *httpapi.AppError", err)
			}
			if appErr.HTTPStatus != tc.wantStatus {
				t.Errorf("status: got %d, want %d", appErr.HTTPStatus, tc.wantStatus)
			}
			if appErr.Code != tc.wantCode {
				t.Errorf("code: got %q, want %q", appErr.Code, tc.wantCode)
			}
		})
	}
}
