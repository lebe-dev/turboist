package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const AccessTokenTTL = 15 * time.Minute

var (
	ErrTokenInvalid = errors.New("auth: token invalid")
	ErrTokenExpired = errors.New("auth: token expired")
)

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
	return signed, exp, nil
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
