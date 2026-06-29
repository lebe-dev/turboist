package calendar

import (
	"errors"

	"golang.org/x/oauth2"
)

// ErrReauthRequired marks a Google Calendar failure that the user can only
// resolve by reconnecting the account — the stored refresh token was revoked,
// expired, or consent was withdrawn (the OAuth token endpoint answered with
// `invalid_grant`). This is an expected, user-actionable condition rather than a
// server bug, so callers should surface it as a dedicated client error instead
// of a 500 and keep it out of Sentry error reporting.
var ErrReauthRequired = errors.New("google calendar authorization expired; reconnect required")

// reauthErrorCodes are the RFC 6749 token-endpoint error codes that mean the
// grant is no longer usable and only re-authorization can fix it.
var reauthErrorCodes = map[string]struct{}{
	"invalid_grant":  {}, // refresh token revoked, expired, or consent withdrawn
	"invalid_client": {}, // client credentials no longer accepted by the provider
}

// IsReauthRequired reports whether err is (or wraps) a token-endpoint failure
// that requires the user to reconnect their Google Calendar account.
func IsReauthRequired(err error) bool {
	if errors.Is(err, ErrReauthRequired) {
		return true
	}
	if rErr, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		_, isReauth := reauthErrorCodes[rErr.ErrorCode]
		return isReauth
	}
	return false
}

// asReauthError tags err with ErrReauthRequired when it is a re-authorization
// failure, preserving the original cause for logging; otherwise it returns err
// unchanged. nil passes through.
func asReauthError(err error) error {
	if err != nil && IsReauthRequired(err) {
		return errors.Join(ErrReauthRequired, err)
	}
	return err
}
