package httpapi

import "fmt"

// Error code constants matching the API.md error table.
const (
	CodeValidationFailed   = "validation_failed"
	CodeAuthInvalid        = "auth_invalid"
	CodeAuthExpired        = "auth_expired"
	CodeAuthRateLimited    = "auth_rate_limited"
	CodeForbidden          = "forbidden"
	CodeNotFound           = "not_found"
	CodeConflict           = "conflict"
	CodeSetupAlreadyDone   = "setup_already_done"
	CodeLimitExceeded      = "limit_exceeded"
	CodeForbiddenPlacement = "forbidden_placement"
	CodeRecurrenceInvalid  = "recurrence_invalid"
	CodeTroikiSlotFull     = "troiki_slot_full"
	CodeTOTPInvalidCode    = "totp_invalid_code"
	CodeTOTPAlreadyEnabled = "totp_already_enabled"
	CodeTOTPNotEnabled     = "totp_not_enabled"
	CodeInternalError      = "internal_error"
)

// AppError is a structured API error carrying an HTTP status, code, message, and optional details.
type AppError struct {
	HTTPStatus int
	Code       string
	Message    string
	Details    any
	// Internal is the underlying cause. Surfaced via Unwrap and logged by the
	// central error handler for 5xx responses; never sent to clients.
	Internal error
}

func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Internal }

// WithCause attaches the underlying error so the central handler can log it.
// The cause is intentionally not added to Details — it stays server-side only.
func (e *AppError) WithCause(err error) *AppError {
	e.Internal = err
	return e
}

func newErr(status int, code, msg string, details ...any) *AppError {
	var d any
	if len(details) > 0 {
		d = details[0]
	}
	return &AppError{HTTPStatus: status, Code: code, Message: msg, Details: d}
}

func ErrValidation(msg string, details ...any) *AppError {
	return newErr(400, CodeValidationFailed, msg, details...)
}

func ErrAuthInvalid(msg string) *AppError {
	return newErr(401, CodeAuthInvalid, msg)
}

func ErrAuthExpired() *AppError {
	return newErr(401, CodeAuthExpired, "access token expired")
}

func ErrAuthRateLimited() *AppError {
	return newErr(429, CodeAuthRateLimited, "too many requests")
}

func ErrForbidden(msg string) *AppError {
	return newErr(403, CodeForbidden, msg)
}

func ErrNotFound(msg string) *AppError {
	return newErr(404, CodeNotFound, msg)
}

func ErrConflict(msg string) *AppError {
	return newErr(409, CodeConflict, msg)
}

func ErrSetupAlreadyDone() *AppError {
	return newErr(410, CodeSetupAlreadyDone, "setup already completed")
}

func ErrLimitExceeded(msg string, details ...any) *AppError {
	return newErr(422, CodeLimitExceeded, msg, details...)
}

func ErrForbiddenPlacement(msg string) *AppError {
	return newErr(422, CodeForbiddenPlacement, msg)
}

func ErrRecurrenceInvalid(msg string) *AppError {
	return newErr(422, CodeRecurrenceInvalid, msg)
}

func ErrTroikiSlotFull(msg string) *AppError {
	return newErr(409, CodeTroikiSlotFull, msg)
}

func ErrInternal(msg string) *AppError {
	return newErr(500, CodeInternalError, msg)
}

func ErrTOTPInvalidCode() *AppError {
	return newErr(401, CodeTOTPInvalidCode, "invalid TOTP code")
}

func ErrTOTPAlreadyEnabled() *AppError {
	return newErr(409, CodeTOTPAlreadyEnabled, "TOTP already enabled")
}

func ErrTOTPNotEnabled() *AppError {
	return newErr(409, CodeTOTPNotEnabled, "TOTP not enabled")
}

func ErrTOTPTicketInvalid() *AppError {
	return newErr(401, CodeAuthInvalid, "invalid or expired OTP ticket")
}
