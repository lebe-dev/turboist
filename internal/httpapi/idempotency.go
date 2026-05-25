package httpapi

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
)

// IdempotencyHeader is the request header that carries the client-generated
// dedup token. The frontend outbox attaches a fresh ULID per mutation and
// reuses it for every retry of that mutation until the server confirms.
const IdempotencyHeader = "Idempotency-Key"

// IdempotencyReplayHeader is set on a cached response so the client (and
// integration tests) can tell a replay from a fresh execution.
const IdempotencyReplayHeader = "Idempotency-Replayed"

// IdempotencyTTL bounds how long a cached response stays valid. 24h covers a
// phone left offline overnight; older keys are treated as never seen and
// silently overwritten on the next request.
const IdempotencyTTL = 24 * time.Hour

// IdempotencyMiddleware caches successful responses to mutating requests keyed
// by (user_id, Idempotency-Key). Retries of the same request return the
// original response byte-for-byte, so a network glitch never duplicates a
// task/project/etc.
//
// Skipped when:
//   - the method is not mutating (GET/HEAD/OPTIONS) — nothing to dedup,
//   - the client did not send Idempotency-Key — caller opted out,
//   - the request is unauthenticated — we have no user_id to key on.
//
// Only 2xx responses are cached. Errors propagate normally and are not
// memoised, so a transient validation failure does not poison the next retry.
func IdempotencyMiddleware(store *repo.IdempotencyRepo) fiber.Handler {
	return func(c fiber.Ctx) error {
		if store == nil || !isMutatingMethod(c.Method()) {
			return c.Next()
		}
		key := c.Get(IdempotencyHeader)
		if key == "" {
			return c.Next()
		}
		userID := GetUserID(c)
		if userID == 0 {
			return c.Next()
		}

		ctx := c.Context()
		notOlderThan := time.Now().Add(-IdempotencyTTL)
		if cached, err := store.Get(ctx, userID, key, notOlderThan); err == nil {
			c.Set(fiber.HeaderContentType, cached.ContentType)
			c.Set(IdempotencyReplayHeader, "true")
			logging.FromContext(ctx).DebugContext(ctx, "idempotency replay",
				slog.String("op", "httpapi.IdempotencyMiddleware"),
				slog.Int64("user_id", userID),
				slog.String("key", key),
				slog.Int("status", cached.StatusCode),
			)
			return c.Status(cached.StatusCode).Send(cached.ResponseBody)
		} else if !errors.Is(err, repo.ErrNotFound) {
			logging.FromContext(ctx).WarnContext(ctx, "idempotency lookup failed",
				slog.String("op", "httpapi.IdempotencyMiddleware"),
				slog.String("err", err.Error()),
			)
			// Lookup failure is non-fatal: run the handler without dedup
			// rather than returning 500 for a cache miss-handling bug.
			return c.Next()
		}

		if err := c.Next(); err != nil {
			return err
		}
		status := c.Response().StatusCode()
		if status < 200 || status >= 300 {
			return nil
		}
		body := append([]byte(nil), c.Response().Body()...)
		contentType := string(c.Response().Header.ContentType())
		if contentType == "" {
			contentType = fiber.MIMEApplicationJSON
		}
		if err := store.Put(ctx, repo.IdempotencyRecord{
			UserID:       userID,
			Key:          key,
			StatusCode:   status,
			ContentType:  contentType,
			ResponseBody: body,
			CreatedAt:    time.Now(),
		}); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "idempotency store failed",
				slog.String("op", "httpapi.IdempotencyMiddleware"),
				slog.String("err", err.Error()),
			)
		}
		return nil
	}
}
