package httpapi

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
)

const (
	idempotencyKeyHeader    = "Idempotency-Key"
	idempotencyReplayHeader = "X-Idempotent-Replay"

	idempotencyKeyMinLen = 8
	idempotencyKeyMaxLen = 128
)

// IdempotencyMiddleware makes mutating requests that carry an Idempotency-Key
// safe to retry: the first request runs the handler and stores its 2xx
// response; a replay with the same key returns that stored response verbatim
// without re-running the handler. It is registered after APIAuthMiddleware (it
// needs the resolved user id) and before PublishMiddleware, so a replay
// short-circuits before Publish and never re-emits an SSE event.
//
// A nil repo disables the middleware entirely — a pass-through used in tests
// that do not exercise idempotency, mirroring PublishMiddleware's nil-hub
// escape hatch.
func IdempotencyMiddleware(idem *repo.IdempotencyRepo) fiber.Handler {
	return func(c fiber.Ctx) error {
		if idem == nil {
			return c.Next()
		}
		key := c.Get(idempotencyKeyHeader)
		if key == "" || !isMutatingMethod(c.Method()) {
			return c.Next()
		}
		ctx := c.Context()
		log := logging.FromContext(ctx)
		if !isValidIdempotencyKey(key) {
			log.WarnContext(ctx, "idempotency: rejected malformed key",
				slog.String("op", "middleware.Idempotency"),
				slog.String("key", maskIdempotencyKey(key)),
			)
			return ErrValidation("invalid Idempotency-Key header")
		}

		userID := GetUserID(c)
		rec, exists, err := idem.Reserve(ctx, key, userID, c.Method(), c.Path())
		if err != nil {
			return err
		}
		if exists {
			if rec.Status == 0 {
				// A concurrent request with the same key is still executing.
				log.WarnContext(ctx, "idempotency: duplicate request in flight",
					slog.String("op", "middleware.Idempotency"),
					slog.String("key", maskIdempotencyKey(key)),
					slog.String("method", c.Method()),
					slog.String("path", c.Path()),
				)
				return ErrIdempotencyInFlight()
			}
			// Replay: return the stored response. Neither the handler nor any
			// downstream middleware (PublishMiddleware) runs.
			log.InfoContext(ctx, "idempotency: replaying stored response",
				slog.String("op", "middleware.Idempotency"),
				slog.String("key", maskIdempotencyKey(key)),
				slog.Int("status", rec.Status),
			)
			c.Set(idempotencyReplayHeader, "true")
			c.Response().Header.SetContentType(fiber.MIMEApplicationJSON)
			return c.Status(rec.Status).SendString(rec.Response)
		}

		if err := c.Next(); err != nil {
			// The handler failed before producing a response — free the
			// reservation so an honest retry with the same key re-runs it.
			_ = idem.Release(ctx, key)
			return err
		}

		status := c.Response().StatusCode()
		if status >= 200 && status < 300 {
			return idem.Complete(ctx, key, status, c.Response().Body())
		}
		// Non-2xx responses are not cached: release the reservation so a
		// corrected retry with the same key re-runs the handler.
		return idem.Release(ctx, key)
	}
}

// isValidIdempotencyKey enforces a length of 8..128 and the charset
// [A-Za-z0-9_-] (covers UUIDs and opaque client-generated tokens).
func isValidIdempotencyKey(key string) bool {
	if len(key) < idempotencyKeyMinLen || len(key) > idempotencyKeyMaxLen {
		return false
	}
	for i := 0; i < len(key); i++ {
		ch := key[i]
		switch {
		case ch >= 'A' && ch <= 'Z':
		case ch >= 'a' && ch <= 'z':
		case ch >= '0' && ch <= '9':
		case ch == '_' || ch == '-':
		default:
			return false
		}
	}
	return true
}

// maskIdempotencyKey trims a key to its first 6 characters for logging so the
// full key never lands in logs, mirroring the token-masking convention.
func maskIdempotencyKey(key string) string {
	if len(key) <= 6 {
		return key
	}
	return key[:6] + "…"
}
