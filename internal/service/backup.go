package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// BackupService produces and consumes BackupPayload snapshots of the database.
// It operates on raw SQL so IDs can be preserved verbatim and the entire
// restore happens inside one transaction.
type BackupService struct {
	db *sql.DB
}

func NewBackupService(db *sql.DB) *BackupService {
	return &BackupService{db: db}
}

// Export reads the full dataset and returns it as a BackupPayload. Read is
// done sequentially (not transactionally) — SQLite's WAL mode gives readers a
// consistent snapshot per statement, and backup is best-effort anyway.
func (s *BackupService) Export(ctx context.Context, opts ExportOptions) (*BackupPayload, error) {
	p := &BackupPayload{
		Version:    BackupSchemaVersion,
		ExportedAt: model.FormatUTC(time.Now()),
	}
	var err error
	if p.Data.Contexts, err = s.readContexts(ctx); err != nil {
		return nil, fmt.Errorf("export contexts: %w", err)
	}
	if p.Data.Labels, err = s.readLabels(ctx); err != nil {
		return nil, fmt.Errorf("export labels: %w", err)
	}
	if p.Data.Projects, err = s.readProjects(ctx); err != nil {
		return nil, fmt.Errorf("export projects: %w", err)
	}
	if p.Data.ProjectSections, err = s.readProjectSections(ctx); err != nil {
		return nil, fmt.Errorf("export project sections: %w", err)
	}
	if p.Data.Tasks, err = s.readTasks(ctx); err != nil {
		return nil, fmt.Errorf("export tasks: %w", err)
	}
	if p.Data.TaskLabels, err = s.readTaskLabels(ctx); err != nil {
		return nil, fmt.Errorf("export task labels: %w", err)
	}
	if p.Data.ProjectLabels, err = s.readProjectLabels(ctx); err != nil {
		return nil, fmt.Errorf("export project labels: %w", err)
	}
	if opts.IncludeSettings {
		cfg, err := s.readSettings(ctx)
		if err != nil {
			return nil, fmt.Errorf("export settings: %w", err)
		}
		p.Settings = cfg
	}
	return p, nil
}

// Restore wipes all user-owned tables and reinserts the supplied payload
// inside a single transaction. If anything fails the transaction is rolled
// back and the existing data is left intact.
func (s *BackupService) Restore(ctx context.Context, p *BackupPayload) error {
	if p == nil {
		return ErrBadBackup
	}
	if p.Version != BackupSchemaVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrBadBackup, p.Version)
	}

	// Heal dangling FKs that may exist in older databases (e.g. tasks pointing
	// at a section/project/parent that was deleted in a buggy code path).
	// Nullable references get NULL'd; orphan link-table rows are dropped.
	// projects.context_id is NOT NULL — orphan projects are dropped along with
	// their tasks and sections so the transaction can commit cleanly.
	sanitizePayload(&p.Data)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// defer_foreign_keys postpones FK checks until COMMIT, which lets us insert
	// self-referential rows (tasks.parent_id) without ordering parents before
	// children. The pragma is scoped to this transaction.
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return fmt.Errorf("defer fk: %w", err)
	}

	if err := wipeAll(ctx, tx); err != nil {
		return fmt.Errorf("wipe: %w", err)
	}
	if err := insertContexts(ctx, tx, p.Data.Contexts); err != nil {
		return fmt.Errorf("contexts: %w", err)
	}
	if err := insertLabels(ctx, tx, p.Data.Labels); err != nil {
		return fmt.Errorf("labels: %w", err)
	}
	if err := insertProjects(ctx, tx, p.Data.Projects); err != nil {
		return fmt.Errorf("projects: %w", err)
	}
	if err := insertProjectLabels(ctx, tx, p.Data.ProjectLabels); err != nil {
		return fmt.Errorf("project labels: %w", err)
	}
	if err := insertProjectSections(ctx, tx, p.Data.ProjectSections); err != nil {
		return fmt.Errorf("project sections: %w", err)
	}
	if err := insertTasks(ctx, tx, p.Data.Tasks); err != nil {
		return fmt.Errorf("tasks: %w", err)
	}
	if err := insertTaskLabels(ctx, tx, p.Data.TaskLabels); err != nil {
		return fmt.Errorf("task labels: %w", err)
	}
	if p.Settings != nil {
		if err := applySettings(ctx, tx, p.Settings); err != nil {
			return fmt.Errorf("settings: %w", err)
		}
	}
	if violation, err := firstFKViolation(ctx, tx); err != nil {
		return fmt.Errorf("fk check: %w", err)
	} else if violation != "" {
		return fmt.Errorf("fk check failed: %s", violation)
	}
	return tx.Commit()
}
