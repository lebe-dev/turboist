package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lebe-dev/turboist/internal/logging"
)

// VacuumInto produces a federation-aware physical backup of the ENTIRE database
// via SQLite `VACUUM INTO` (Federation v1 F6.5, US-8.5). Unlike the logical JSON
// Export — which only captures the user-owned task/project tables — this is a
// byte-for-byte copy of every table, so it INCLUDES the federation bookkeeping
// (federated_projects/instances, the outbox/inbox, entity_field_hlc) AND the
// instance keypair (federation_keys). Restoring it under the SAME BASE_URL keeps
// this instance's federation identity intact (no re-handshake); restoring under a
// CHANGED BASE_URL is handled by the startup identity check (CheckRestoreIdentity),
// which marks the federation mappings read-only history rather than deleting them
// (US-8.5 AC2).
//
// VACUUM INTO writes a fresh, defragmented copy to destPath. It holds the lone
// connection for the duration (SetMaxOpenConns(1)), so it is an OFF-PEAK / admin-
// triggered action — documented, not a routine hot-path call (R1 / §16). destPath
// must be a path that does NOT yet exist (SQLite refuses to overwrite). On success
// the file at destPath is the complete backup.
func (s *BackupService) VacuumInto(ctx context.Context, destPath string) error {
	const op = "service.BackupService.VacuumInto"
	log := logging.FromContext(ctx)
	if destPath == "" {
		return fmt.Errorf("%s: destPath is required", op)
	}
	// VACUUM INTO refuses to overwrite an existing file; ensure a clean slate.
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("%s: destination %q already exists", op, destPath)
	}
	log.InfoContext(ctx, "federation-aware backup started",
		slog.String("op", op), slog.String("dest", destPath))

	// VACUUM INTO cannot use a bound parameter for the path; the path is quoted as a
	// SQLite string literal (single quotes doubled) so a path containing a quote is
	// safe. destPath is operator-supplied (not request data), but we quote defensively.
	quoted := "'" + sqlQuoteLiteral(destPath) + "'"
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO "+quoted); err != nil {
		return fmt.Errorf("%s: vacuum into: %w", op, err)
	}
	log.InfoContext(ctx, "federation-aware backup finished",
		slog.String("op", op), slog.String("dest", destPath))
	return nil
}

// VacuumIntoBytes runs VacuumInto into a temp file and returns the backup bytes,
// cleaning the temp file up afterwards. It is the convenience the download handler
// uses: the VACUUM copy is a self-contained SQLite file the operator stores and can
// later restore by placing it at DATA_PATH. The temp file lives under dir (or the
// OS temp dir when dir is empty).
func (s *BackupService) VacuumIntoBytes(ctx context.Context, dir string) ([]byte, error) {
	const op = "service.BackupService.VacuumIntoBytes"
	if dir == "" {
		dir = os.TempDir()
	}
	tmp, err := os.MkdirTemp(dir, "turboist-fed-backup-*")
	if err != nil {
		return nil, fmt.Errorf("%s: temp dir: %w", op, err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	dest := filepath.Join(tmp, "backup.db")
	if err := s.VacuumInto(ctx, dest); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		return nil, fmt.Errorf("%s: read backup: %w", op, err)
	}
	return b, nil
}

// sqlQuoteLiteral doubles single quotes for safe inclusion in a SQLite string
// literal (VACUUM INTO does not accept a bound parameter).
func sqlQuoteLiteral(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\'' {
			out = append(out, '\'')
		}
		out = append(out, r)
	}
	return string(out)
}
