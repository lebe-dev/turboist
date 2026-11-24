// Package obs wires application-wide observability. Currently this is Sentry
// error reporting, configured entirely through environment variables and
// disabled (a no-op) when no DSN is provided.
package obs

import (
	"reflect"
	"time"

	sentry "github.com/getsentry/sentry-go"
)

// flushTimeout bounds how long Flush blocks while draining buffered events on
// shutdown before giving up.
const flushTimeout = 2 * time.Second

// Init configures the global Sentry hub from the given DSN. A blank dsn leaves
// Sentry disabled: Init returns a no-op flush and a nil error, and every later
// CaptureError / middleware call becomes a no-op (the hub has no client).
//
// release is stamped on every event (the app version); environment groups
// events by deploy (e.g. "production"). The returned flush should be deferred
// in main so buffered events are delivered during graceful shutdown.
func Init(dsn, environment, release string) (flush func(), err error) {
	if dsn == "" {
		return func() {}, nil
	}
	if initErr := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: environment,
		Release:     release,
		// Errors only — no performance tracing.
		TracesSampleRate: 0,
		// Every event is built by FailureEvent, which carries the error's own
		// chain and a message. The goroutine stack at the capture site is not
		// the error's origin and would only add noise.
		AttachStacktrace: false,
		// Nothing request-shaped is ever attached to an event (no headers, no
		// body, no query string), so the SDK's data-collection defaults have
		// nothing to pick up: an event holds the error text, the route, the
		// user id and the request id.
		BeforeSend: dropEmptyEvents,
	}); initErr != nil {
		return func() {}, initErr
	}
	return func() { sentry.Flush(flushTimeout) }, nil
}

// dropEmptyEvents discards an event that carries neither a message nor an
// exception: Sentry shows it as "Unknown error", an issue with nothing to
// investigate and no way to tell two failures apart.
func dropEmptyEvents(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event.Message == "" && len(event.Exception) == 0 {
		return nil
	}
	return event
}

// FailureEvent builds the event reported for err: the whole unwrapped error
// chain as the exception, err.Error() as the message so the issue always has a
// readable title, and fingerprint as its grouping key. It returns nil for a nil
// err.
func FailureEvent(err error, fingerprint []string) *sentry.Event {
	if err == nil {
		return nil
	}
	event := sentry.NewEvent()
	event.Level = sentry.LevelError
	event.SetException(err, -1)
	event.Message = err.Error()
	event.Fingerprint = fingerprint
	return event
}

// CaptureError reports err to Sentry from a freshly cloned hub, which makes it
// safe to call from any goroutine (background workers, startup paths). The tags
// are attached to the event for filtering in the Sentry UI. It is a no-op when
// err is nil or when Sentry is disabled.
func CaptureError(err error, tags map[string]string) {
	if err == nil {
		return
	}
	hub := sentry.CurrentHub().Clone()
	if len(tags) > 0 {
		hub.ConfigureScope(func(scope *sentry.Scope) {
			scope.SetTags(tags)
		})
	}
	hub.CaptureEvent(FailureEvent(err, backgroundFingerprint(err, tags)))
}

// ErrorType names err's concrete Go type — the part of a failure that stays the
// same across occurrences when its message does not.
func ErrorType(err error) string {
	if err == nil {
		return ""
	}
	return reflect.TypeOf(err).String()
}

// backgroundFingerprint groups a background failure by the operation that ran
// and what broke, instead of by the goroutine stack it was reported from, which
// is the same for every call of one worker.
func backgroundFingerprint(err error, tags map[string]string) []string {
	fingerprint := []string{"background"}
	if op := tags["op"]; op != "" {
		fingerprint = append(fingerprint, op)
	}
	return append(fingerprint, ErrorType(err))
}
