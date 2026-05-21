package repo

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
)

// logQuery emits a DEBUG record before issuing a SQL operation. op should be
// the stable identifier "repo.<table>.<method>".
func logQuery(ctx context.Context, op string, args ...any) {
	logging.FromContext(ctx).DebugContext(ctx, "repo query",
		slog.String("op", op),
		slog.Any("args", args))
}

// logErr classifies a SQL error and emits the appropriate log record. Expected
// "not found" conditions (sql.ErrNoRows, ErrNotFound, ErrConflict) are logged at
// DEBUG so lookups can be traced without polluting WARN/ERROR streams.
// Any other error is logged at ERROR with op + err. The original error is
// returned for caller convenience.
func logErr(ctx context.Context, op string, err error) error {
	if err == nil {
		return nil
	}
	log := logging.FromContext(ctx)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) {
		log.DebugContext(ctx, "repo lookup miss",
			slog.String("op", op),
			slog.String("err", err.Error()))
		return err
	}
	log.ErrorContext(ctx, "repo error",
		slog.String("op", op),
		slog.String("err", err.Error()))
	return err
}
