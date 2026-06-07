// Package obs wires application-wide observability. Currently this is Sentry
// error reporting, configured entirely through environment variables and
// disabled (a no-op) when no DSN is provided.
package obs

import (
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
		// Errors only — no performance tracing (TracesSampleRate defaults to 0).
	}); initErr != nil {
		return func() {}, initErr
	}
	return func() { sentry.Flush(flushTimeout) }, nil
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
	hub.CaptureException(err)
}
