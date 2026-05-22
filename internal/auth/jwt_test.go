package auth

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestJWT_IssueAndVerify(t *testing.T) {
	j := NewJWTIssuer([]byte("secret"))
	tok, exp, err := j.Issue(1, 42)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !strings.HasPrefix(tok, "eyJ") {
		t.Errorf("token prefix: %q", tok[:10])
	}
	if time.Until(exp) > AccessTokenTTL+time.Second {
		t.Errorf("expiry too far: %v", exp)
	}

	c, err := j.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if c.UserID != 1 {
		t.Errorf("user id: got %d, want 1", c.UserID)
	}
	if c.SessionID != 42 {
		t.Errorf("session id: got %d, want 42", c.SessionID)
	}
}

func TestJWT_Verify_WrongSecret(t *testing.T) {
	j1 := NewJWTIssuer([]byte("a"))
	j2 := NewJWTIssuer([]byte("b"))
	tok, _, _ := j1.Issue(1, 1)
	_, err := j2.Verify(tok)
	if !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("err: got %v, want ErrTokenInvalid", err)
	}
}

func TestJWT_OTPTicket_IssueAndVerify(t *testing.T) {
	j := NewJWTIssuer([]byte("secret"))
	tok, exp, err := j.IssueOTPTicket(7, "web")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if time.Until(exp) > OTPTicketTTL+time.Second {
		t.Errorf("expiry too far: %v", exp)
	}
	tk, err := j.VerifyOTPTicket(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if tk.UserID != 7 {
		t.Errorf("user id: got %d, want 7", tk.UserID)
	}
	if tk.ClientKind != "web" {
		t.Errorf("client kind: got %q, want web", tk.ClientKind)
	}
}

func TestJWT_OTPTicket_Expired(t *testing.T) {
	j := NewJWTIssuer([]byte("secret"))
	j.SetClock(func() time.Time { return time.Unix(1000, 0) })
	tok, _, _ := j.IssueOTPTicket(1, "cli")
	j.SetClock(func() time.Time { return time.Unix(1000+int64(OTPTicketTTL.Seconds())+10, 0) })
	_, err := j.VerifyOTPTicket(tok)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("err: got %v, want ErrTokenExpired", err)
	}
}

func TestJWT_OTPTicket_RejectsAccessToken(t *testing.T) {
	j := NewJWTIssuer([]byte("secret"))
	tok, _, _ := j.Issue(1, 2)
	if _, err := j.VerifyOTPTicket(tok); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("access token accepted as ticket: %v", err)
	}
}

func TestJWT_Verify_RejectsOTPTicket(t *testing.T) {
	j := NewJWTIssuer([]byte("secret"))
	tok, _, _ := j.IssueOTPTicket(1, "cli")
	// A ticket lacks "sid" so the regular Verify should treat it as invalid.
	if _, err := j.Verify(tok); err == nil {
		t.Error("ticket accepted by access-token Verify")
	}
}

func TestJWT_Verify_Expired(t *testing.T) {
	j := NewJWTIssuer([]byte("s"))
	j.SetClock(func() time.Time { return time.Now().Add(-time.Hour) })
	tok, _, _ := j.Issue(1, 1)
	j.SetClock(time.Now)
	_, err := j.Verify(tok)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("err: got %v, want ErrTokenExpired", err)
	}
}

func TestJWT_Verify_Garbage(t *testing.T) {
	j := NewJWTIssuer([]byte("s"))
	if _, err := j.Verify("not-a-jwt"); err == nil {
		t.Errorf("expected error on garbage")
	}
}

func TestJWT_Issue_LogsDebug(t *testing.T) {
	cap := newCaptureHandler()
	swapDefault(t, cap)

	j := NewJWTIssuer([]byte("secret"))
	if _, _, err := j.Issue(7, 99); err != nil {
		t.Fatalf("issue: %v", err)
	}

	var saw bool
	for _, r := range cap.snapshot() {
		if r.Level != slog.LevelDebug || r.Message != "jwt issued" {
			continue
		}
		var uid, sid int64
		r.Attrs(func(a slog.Attr) bool {
			switch a.Key {
			case "user_id":
				uid = a.Value.Int64()
			case "session_id":
				sid = a.Value.Int64()
			}
			return true
		})
		if uid == 7 && sid == 99 {
			saw = true
		}
	}
	if !saw {
		t.Error("no DEBUG 'jwt issued' record with expected ids")
	}
}

func TestJWT_Verify_Expired_LogsDebugReason(t *testing.T) {
	cap := newCaptureHandler()
	swapDefault(t, cap)

	j := NewJWTIssuer([]byte("s"))
	j.SetClock(func() time.Time { return time.Now().Add(-time.Hour) })
	tok, _, _ := j.Issue(1, 1)
	j.SetClock(time.Now)
	if _, err := j.Verify(tok); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("verify: got %v, want ErrTokenExpired", err)
	}

	var reason string
	for _, r := range cap.snapshot() {
		if r.Level != slog.LevelDebug || r.Message != "jwt verify failed" {
			continue
		}
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "reason" {
				reason = a.Value.String()
			}
			return true
		})
	}
	if reason != "expired" {
		t.Errorf("reason: got %q, want %q", reason, "expired")
	}
}

func TestJWT_Verify_Garbage_LogsDebugReason(t *testing.T) {
	cap := newCaptureHandler()
	swapDefault(t, cap)

	j := NewJWTIssuer([]byte("s"))
	if _, err := j.Verify("garbage-token"); err == nil {
		t.Fatal("expected error")
	}

	var sawDebug bool
	for _, r := range cap.snapshot() {
		if r.Level == slog.LevelDebug && r.Message == "jwt verify failed" {
			sawDebug = true
		}
	}
	if !sawDebug {
		t.Error("no DEBUG 'jwt verify failed' record")
	}
}
