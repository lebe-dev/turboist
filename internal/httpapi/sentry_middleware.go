package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	sentry "github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/federation/transport"
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
				enrichSentryScope(c, hub, fiber.StatusInternalServerError, nil)
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
		enrichSentryScope(c, hub, status, err)
		hub.CaptureException(reportableError(err))
		return err
	}
}

// enrichSentryScope attaches request-scoped context so events are searchable and
// correlate with access logs: the HTTP method/route/status, the request id and
// user, the structured AppError code + details, the resolved path params, the
// inbound request (method/URL/headers, minus secrets) and a stable transaction +
// fingerprint so distinct (route, error code) pairs group into distinct issues
// instead of collapsing into one untitled "Unknown error". err is nil on the
// panic path, where the recovered value supplies the exception instead.
func enrichSentryScope(c fiber.Ctx, hub *sentry.Hub, status int, err error) {
	route := routeTemplate(c)
	transaction := c.Method() + " " + route
	hub.ConfigureScope(func(scope *sentry.Scope) {
		// Scope has no transaction setter in this SDK version, so stamp it on the
		// event directly. The transaction names the failing endpoint and is what
		// many Sentry servers title the issue by (otherwise "Unknown error").
		scope.AddEventProcessor(func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			event.Transaction = transaction
			return event
		})
		scope.SetTag("http.method", c.Method())
		scope.SetTag("http.route", route)
		scope.SetTag("http.target", c.Path())
		scope.SetTag("http.status", strconv.Itoa(status))
		if rid, ok := c.Locals(localsRequestIDKey).(string); ok && rid != "" {
			scope.SetTag("request_id", rid)
		}
		if uid := GetUserID(c); uid != 0 {
			scope.SetUser(sentry.User{ID: strconv.FormatInt(uid, 10)})
		}
		if params := routeParams(c); len(params) > 0 {
			scope.SetContext("route_params", params)
		}
		scope.SetRequest(sentryRequest(c))

		// Group by (method, route, error code) so a recurring 401 on a federation
		// route is one issue, not merged with unrelated failures behind the shared
		// middleware stack frame. The fingerprint also gives the issue a meaningful
		// identity on Sentry servers that title by transaction rather than by the
		// (Go-internal) exception type.
		fingerprint := []string{c.Method(), route, strconv.Itoa(status)}
		var appErr *AppError
		if errors.As(err, &appErr) {
			scope.SetTag("error.code", appErr.Code)
			fingerprint = append(fingerprint, appErr.Code)
			if appErr.Details != nil {
				scope.SetContext("error.details", sentry.Context{"value": appErr.Details})
			}
		}
		scope.SetFingerprint(fingerprint)
	})
}

// routeTemplate returns the registered route pattern (e.g.
// "/federation/projects/:projectClientID/events") so events group by endpoint
// rather than by the concrete path, which would scatter every id into its own
// issue. It falls back to the concrete path when no route matched (e.g. a request
// rejected by prefix middleware before routing resolved a handler).
func routeTemplate(c fiber.Ctx) string {
	if r := c.Route(); r != nil && r.Path != "" {
		return r.Path
	}
	return c.Path()
}

// routeParams resolves the matched route's path params to their concrete values
// (e.g. {"projectClientID": ""}). An empty value is itself a useful signal — it
// is what produces the tell-tale double slash in "/federation/projects//events".
func routeParams(c fiber.Ctx) sentry.Context {
	r := c.Route()
	if r == nil || len(r.Params) == 0 {
		return nil
	}
	params := make(sentry.Context, len(r.Params))
	for _, key := range r.Params {
		params[key] = c.Params(key)
	}
	return params
}

// sentryRequest reconstructs a net/http.Request from the Fiber context so Sentry
// renders the real inbound request (method, URL, query, headers) in its Request
// panel — without it the panel shows the SDK transport's own user agent. The
// Ed25519 transport signature is dropped (a secret-grade header, F0.3); Sentry's
// own NewRequest additionally strips standard sensitive headers (Authorization,
// Cookie, …) because PII reporting is left disabled.
func sentryRequest(c fiber.Ctx) *http.Request {
	r := &http.Request{
		Method:     c.Method(),
		Host:       c.Host(),
		RemoteAddr: c.IP(),
		Header:     make(http.Header),
		URL: &url.URL{
			Scheme:   c.Scheme(),
			Host:     c.Host(),
			Path:     c.Path(),
			RawQuery: string(c.Request().URI().QueryString()),
		},
	}
	for key, values := range c.GetReqHeaders() {
		if key == transport.HeaderSignature {
			continue
		}
		r.Header[key] = values
	}
	return r
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
