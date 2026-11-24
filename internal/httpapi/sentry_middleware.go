package httpapi

import (
	"fmt"
	"strconv"

	sentry "github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/obs"
)

// SentryMiddleware reports server-side failures to Sentry: any recovered panic,
// plus every request whose error is answered with a status shouldReportStatus
// selects. It uses a per-request hub clone so concurrent requests never share
// scope state.
//
// Classification goes through resolveAppError, the same mapping the central
// error handler renders. A refusal the API answers as a 4xx — a locked Today, a
// task held back by a blocker — is therefore never reported as a server failure,
// even when a handler wrapped it in an internal error on its way out.
//
// Recovered panics are converted to a 500 AppError so the central ErrorHandler
// still renders a clean JSON envelope.
func SentryMiddleware() fiber.Handler {
	return func(c fiber.Ctx) (err error) {
		hub := sentry.CurrentHub().Clone()

		defer func() {
			if r := recover(); r != nil {
				err = ErrInternal("internal server error").
					WithCause(fmt.Errorf("panic recovered: %v", r))
				reportFailure(c, hub, err, "panic")
			}
		}()

		err = c.Next()
		if err == nil {
			return nil
		}
		reportFailure(c, hub, err, "")
		return err
	}
}

// reportFailure sends one event for a failed request unless the client is
// answered with a status that is not worth reporting.
func reportFailure(c fiber.Ctx, hub *sentry.Hub, err error, kind string) {
	appErr := resolveAppError(err)
	if !shouldReportStatus(appErr.HTTPStatus) {
		return
	}
	route := routePattern(c)
	enrichSentryScope(c, hub, appErr, route, kind)
	event := obs.FailureEvent(err, failureFingerprint(appErr, route, kind))
	event.Transaction = route
	hub.CaptureEvent(event)
}

// shouldReportStatus decides which response statuses are worth a Sentry event.
// Server failures (5xx) and malformed requests (400) signal real bugs worth
// investigating; the other 4xx codes (401 auth, 403 forbidden, 404 not found,
// 409 conflict, 429 rate limit, ...) are expected client behavior and would
// only drown the issue feed in noise.
func shouldReportStatus(status int) bool {
	return status == fiber.StatusBadRequest || status >= fiber.StatusInternalServerError
}

// failureFingerprint groups events by what failed rather than by where the event
// was captured. An error reported from middleware is captured on the request
// goroutine, so every event carries the same middleware frames and Sentry would
// otherwise file unrelated failures under one issue.
func failureFingerprint(appErr *AppError, route, kind string) []string {
	if kind == "" {
		kind = "error"
	}
	return []string{kind, route, appErr.Code, obs.ErrorType(reportableError(appErr))}
}

// routePattern names the endpoint that failed. The concrete path carries ids,
// which would split one failure into an issue per task and inflate tag
// cardinality, so the registered pattern is preferred; the concrete path still
// travels in the request context.
func routePattern(c fiber.Ctx) string {
	if endpoint := c.Endpoint(); endpoint != nil && endpoint.Path != "" {
		return endpoint.Path
	}
	return c.Path()
}

// enrichSentryScope attaches request-scoped context (method, route, status,
// error code, request id, user) so events are searchable and correlate with
// access logs.
func enrichSentryScope(c fiber.Ctx, hub *sentry.Hub, appErr *AppError, route, kind string) {
	hub.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag("http.method", c.Method())
		scope.SetTag("http.route", route)
		scope.SetTag("http.status", strconv.Itoa(appErr.HTTPStatus))
		scope.SetTag("error.code", appErr.Code)
		scope.SetTag("error.type", obs.ErrorType(reportableError(appErr)))
		if kind != "" {
			scope.SetTag("error.kind", kind)
		}
		scope.SetContext("http", sentry.Context{
			"method": c.Method(),
			"path":   c.Path(),
			"route":  route,
			"status": appErr.HTTPStatus,
		})
		if rid, ok := c.Locals(localsRequestIDKey).(string); ok && rid != "" {
			scope.SetTag("request_id", rid)
		}
		if uid := GetUserID(c); uid != 0 {
			scope.SetUser(sentry.User{ID: strconv.FormatInt(uid, 10)})
		}
	})
}

// reportableError picks the error an event is described by: an AppError's
// Internal cause when present, since that is the real failure, otherwise the
// AppError itself.
func reportableError(appErr *AppError) error {
	if appErr.Internal != nil {
		return appErr.Internal
	}
	return appErr
}
