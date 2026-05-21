package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
)

// logRepoErr emits an ERROR record for unexpected repo failures only.
// Expected/business-rule errors (NotFound, InvalidPlacement, Cycle, Conflict,
// sql.ErrNoRows) are skipped — the repo layer has already logged them at
// DEBUG, and the handler maps them to a typed 4xx. Re-logging at ERROR would
// pollute alert streams with routine client-induced 404s.
func logRepoErr(ctx context.Context, op string, err error, attrs ...any) {
	if err == nil || isExpectedRepoErr(err) {
		return
	}
	all := append([]any{slog.String("err", err.Error())}, attrs...)
	logging.FromContext(ctx).ErrorContext(ctx, op, all...)
}

func isExpectedRepoErr(err error) bool {
	return errors.Is(err, repo.ErrNotFound) ||
		errors.Is(err, repo.ErrConflict) ||
		errors.Is(err, repo.ErrInvalidPlacement) ||
		errors.Is(err, repo.ErrCycle) ||
		errors.Is(err, sql.ErrNoRows)
}
