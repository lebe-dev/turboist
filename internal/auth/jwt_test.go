package auth

import (
	"errors"
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
