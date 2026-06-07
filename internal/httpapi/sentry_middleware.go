package httpapi

import (
	"errors"
	"fmt"
	"strconv"

	sentry "github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v3"
)

// SentryCaptureMinStatus is the lowest HTTP status SentryMiddleware reports.
// 400 means every 4xx and 5xx response is captured; recovered panics are always
// captured regardless of the status they map to.
const SentryCaptureMinStatus = 400

// SentryMiddleware reports server-side failures to Sentry: any recovered panic,
// plus every request whose error maps to an HTTP status >= minStatus. It uses a
// per-request hub clone so concurrent requests never share scope state.
//
// For AppError values carrying an Internal cause the underlying error is sent —
// it holds the real failure, whereas the AppError message is the sanitized text
// shown to clients. Recovered panics are converted to a 500 AppError so the
// central ErrorHandler still renders a clean JSON envelope.
func SentryMiddleware(minStatus int) fiber.Handler {
	return func(c fiber.Ctx) (err error) {
		hub := sentry.CurrentHub().Clone()

		defer func() {
			if r := recover(); r != nil {
				enrichSentryScope(c, hub, fiber.StatusInternalServerError)
				hub.RecoverWithContext(c.Context(), r)
				err = ErrInternal("internal server error").
					WithCause(fmt.Errorf("panic recovered: %v", r))
			}
		}()

		err = c.Next()
		if err == nil {
			return nil
		}
		status := statusFromError(err)
		if status < minStatus {
			return err
		}
		enrichSentryScope(c, hub, status)
		hub.CaptureException(reportableError(err))
		return err
	}
}

// enrichSentryScope attaches request-scoped context (method, route, status,
// request id, user) so events are searchable and correlate with access logs.
func enrichSentryScope(c fiber.Ctx, hub *sentry.Hub, status int) {
	hub.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag("http.method", c.Method())
		scope.SetTag("http.route", c.Path())
		scope.SetTag("http.status", strconv.Itoa(status))
		if rid, ok := c.Locals(localsRequestIDKey).(string); ok && rid != "" {
			scope.SetTag("request_id", rid)
		}
		if uid := GetUserID(c); uid != 0 {
			scope.SetUser(sentry.User{ID: strconv.FormatInt(uid, 10)})
		}
	})
}

// statusFromError extracts the HTTP status an error maps to, mirroring the
// central ErrorHandler: AppError carries it explicitly, fiber.Error exposes
// Code, and anything else is treated as a 500.
func statusFromError(err error) int {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.HTTPStatus
	}
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return fiberErr.Code
	}
	return fiber.StatusInternalServerError
}

// reportableError unwraps an AppError to its Internal cause when present so the
// Sentry event carries the underlying failure rather than the sanitized client
// message.
func reportableError(err error) error {
	var appErr *AppError
	if errors.As(err, &appErr) && appErr.Internal != nil {
		return appErr.Internal
	}
	return err
}
