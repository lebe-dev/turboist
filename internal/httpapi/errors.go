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
	CodeInternalError      = "internal_error"
)

// AppError is a structured API error carrying an HTTP status, code, message, and optional details.
type AppError struct {
	HTTPStatus int
	Code       string
	Message    string
	Details    any
}

func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
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
