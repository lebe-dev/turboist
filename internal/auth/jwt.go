package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const AccessTokenTTL = 15 * time.Minute

// OTPTicketTTL is the lifetime of the short-lived ticket issued after a
// password check when the user has TOTP enabled. The client must complete
// the OTP step within this window.
const OTPTicketTTL = 5 * time.Minute

// otpTicketPurpose is the "prp" claim value distinguishing OTP tickets from
// regular access tokens so the verifier can refuse cross-use.
const otpTicketPurpose = "otp"

var (
	ErrTokenInvalid = errors.New("auth: token invalid")
	ErrTokenExpired = errors.New("auth: token expired")
)

// OTPTicket carries the subject of a verified OTP ticket.
type OTPTicket struct {
	UserID     int64
	ClientKind string
}

type Claims struct {
	UserID    int64
	SessionID int64
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type JWTIssuer struct {
	secret []byte
	now    func() time.Time
}

func NewJWTIssuer(secret []byte) *JWTIssuer {
	return &JWTIssuer{secret: secret, now: time.Now}
}

// SetClock overrides the clock for testing.
func (j *JWTIssuer) SetClock(now func() time.Time) {
	j.now = now
}

func (j *JWTIssuer) Issue(userID, sessionID int64) (string, time.Time, error) {
	now := j.now().UTC()
	exp := now.Add(AccessTokenTTL)
	claims := jwt.MapClaims{
		"sub": strconv.FormatInt(userID, 10),
		"sid": strconv.FormatInt(sessionID, 10),
		"iat": now.Unix(),
		"exp": exp.Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(j.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	slog.Default().Debug("jwt issued",
		slog.String("op", "auth.JWTIssuer.Issue"),
		slog.Int64("user_id", userID),
		slog.Int64("session_id", sessionID),
		slog.Time("exp", exp),
	)
	return signed, exp, nil
}

// IssueOTPTicket signs a short-lived ticket binding userID and clientKind so
// the OTP completion endpoint can issue a regular session without re-reading
// the request body's client kind. The ticket is signed with the same HS256
// secret as access tokens but carries a "prp" claim that distinguishes it.
func (j *JWTIssuer) IssueOTPTicket(userID int64, clientKind string) (string, time.Time, error) {
	now := j.now().UTC()
	exp := now.Add(OTPTicketTTL)
	claims := jwt.MapClaims{
		"sub": strconv.FormatInt(userID, 10),
		"ck":  clientKind,
		"prp": otpTicketPurpose,
		"iat": now.Unix(),
		"exp": exp.Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(j.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign otp ticket: %w", err)
	}
	return signed, exp, nil
}

// VerifyOTPTicket parses and validates a ticket previously issued by
// IssueOTPTicket. Returns ErrTokenExpired when the ticket has expired and
// ErrTokenInvalid for any other failure including wrong purpose.
func (j *JWTIssuer) VerifyOTPTicket(tokenStr string) (*OTPTicket, error) {
	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}
	if !parsed.Valid {
		return nil, ErrTokenInvalid
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrTokenInvalid
	}
	if prp, _ := mc["prp"].(string); prp != otpTicketPurpose {
		return nil, ErrTokenInvalid
	}
	subStr, ok := mc["sub"].(string)
	if !ok {
		return nil, ErrTokenInvalid
	}
	sub, err := strconv.ParseInt(subStr, 10, 64)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	ck, _ := mc["ck"].(string)
	return &OTPTicket{UserID: sub, ClientKind: ck}, nil
}

func (j *JWTIssuer) Verify(tokenStr string) (*Claims, error) {
	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			slog.Default().Debug("jwt verify failed",
				slog.String("op", "auth.JWTIssuer.Verify"),
				slog.String("reason", "expired"),
			)
			return nil, ErrTokenExpired
		}
		slog.Default().Debug("jwt verify failed",
			slog.String("op", "auth.JWTIssuer.Verify"),
			slog.String("reason", "malformed"),
			slog.String("err", err.Error()),
		)
		return nil, ErrTokenInvalid
	}
	if !parsed.Valid {
		slog.Default().Debug("jwt verify failed",
			slog.String("op", "auth.JWTIssuer.Verify"),
			slog.String("reason", "invalid"),
		)
		return nil, ErrTokenInvalid
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrTokenInvalid
	}
	subStr, ok := mc["sub"].(string)
	if !ok {
		return nil, ErrTokenInvalid
	}
	sub, err := strconv.ParseInt(subStr, 10, 64)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	sidStr, ok := mc["sid"].(string)
	if !ok {
		return nil, ErrTokenInvalid
	}
	sid, err := strconv.ParseInt(sidStr, 10, 64)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	iatF, _ := mc["iat"].(float64)
	expF, _ := mc["exp"].(float64)
	return &Claims{
		UserID:    sub,
		SessionID: sid,
		IssuedAt:  time.Unix(int64(iatF), 0).UTC(),
		ExpiresAt: time.Unix(int64(expF), 0).UTC(),
	}, nil
}
