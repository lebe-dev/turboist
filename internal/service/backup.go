package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
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
	const op = "service.BackupService.Export"
	log := logging.FromContext(ctx)
	log.InfoContext(ctx, "backup export started", slog.String("op", op), slog.Bool("include_settings", opts.IncludeSettings))
	p := &BackupPayload{
		Version:    BackupSchemaVersion,
		ExportedAt: model.FormatUTC(time.Now()),
	}
	var err error
	if p.Data.Contexts, err = s.readContexts(ctx); err != nil {
		log.ErrorContext(ctx, op+": read contexts", slog.String("err", err.Error()))
		return nil, fmt.Errorf("export contexts: %w", err)
	}
	if p.Data.Labels, err = s.readLabels(ctx); err != nil {
		log.ErrorContext(ctx, op+": read labels", slog.String("err", err.Error()))
		return nil, fmt.Errorf("export labels: %w", err)
	}
	if p.Data.Projects, err = s.readProjects(ctx); err != nil {
		log.ErrorContext(ctx, op+": read projects", slog.String("err", err.Error()))
		return nil, fmt.Errorf("export projects: %w", err)
	}
	if p.Data.ProjectSections, err = s.readProjectSections(ctx); err != nil {
		log.ErrorContext(ctx, op+": read project sections", slog.String("err", err.Error()))
		return nil, fmt.Errorf("export project sections: %w", err)
	}
	if p.Data.Tasks, err = s.readTasks(ctx); err != nil {
		log.ErrorContext(ctx, op+": read tasks", slog.String("err", err.Error()))
		return nil, fmt.Errorf("export tasks: %w", err)
	}
	if p.Data.TaskLabels, err = s.readTaskLabels(ctx); err != nil {
		log.ErrorContext(ctx, op+": read task labels", slog.String("err", err.Error()))
		return nil, fmt.Errorf("export task labels: %w", err)
	}
	if p.Data.ProjectLabels, err = s.readProjectLabels(ctx); err != nil {
		log.ErrorContext(ctx, op+": read project labels", slog.String("err", err.Error()))
		return nil, fmt.Errorf("export project labels: %w", err)
	}
	if opts.IncludeSettings {
		cfg, err := s.readSettings(ctx)
		if err != nil {
			log.ErrorContext(ctx, op+": read settings", slog.String("err", err.Error()))
			return nil, fmt.Errorf("export settings: %w", err)
		}
		p.Settings = cfg
	}
	log.InfoContext(ctx, "backup export finished",
		slog.String("op", op),
		slog.Int("contexts", len(p.Data.Contexts)),
		slog.Int("labels", len(p.Data.Labels)),
		slog.Int("projects", len(p.Data.Projects)),
		slog.Int("project_sections", len(p.Data.ProjectSections)),
		slog.Int("tasks", len(p.Data.Tasks)),
		slog.Int("task_labels", len(p.Data.TaskLabels)),
		slog.Int("project_labels", len(p.Data.ProjectLabels)),
	)
	return p, nil
}

// Restore wipes all user-owned tables and reinserts the supplied payload
// inside a single transaction. If anything fails the transaction is rolled
// back and the existing data is left intact.
func (s *BackupService) Restore(ctx context.Context, p *BackupPayload) error {
	const op = "service.BackupService.Restore"
	log := logging.FromContext(ctx)
	if p == nil {
		log.WarnContext(ctx, op+": nil payload")
		return ErrBadBackup
	}
	if p.Version != BackupSchemaVersion {
		log.WarnContext(ctx, op+": unsupported version", slog.Int("version", p.Version))
		return fmt.Errorf("%w: unsupported version %d", ErrBadBackup, p.Version)
	}

	log.InfoContext(ctx, "backup restore started",
		slog.String("op", op),
		slog.Int("contexts", len(p.Data.Contexts)),
		slog.Int("labels", len(p.Data.Labels)),
		slog.Int("projects", len(p.Data.Projects)),
		slog.Int("project_sections", len(p.Data.ProjectSections)),
		slog.Int("tasks", len(p.Data.Tasks)),
		slog.Int("task_labels", len(p.Data.TaskLabels)),
		slog.Int("project_labels", len(p.Data.ProjectLabels)),
	)

	preProjects := len(p.Data.Projects)
	preSections := len(p.Data.ProjectSections)
	preTasks := len(p.Data.Tasks)
	preTaskLabels := len(p.Data.TaskLabels)
	preProjectLabels := len(p.Data.ProjectLabels)

	// Heal dangling FKs that may exist in older databases (e.g. tasks pointing
	// at a section/project/parent that was deleted in a buggy code path).
	// Nullable references get NULL'd; orphan link-table rows are dropped.
	// projects.context_id is NOT NULL — orphan projects are dropped along with
	// their tasks and sections so the transaction can commit cleanly.
	sanitizePayload(&p.Data)

	if d := preProjects - len(p.Data.Projects); d > 0 {
		log.WarnContext(ctx, op+": projects sanitised", slog.Int("dropped", d))
	}
	if d := preSections - len(p.Data.ProjectSections); d > 0 {
		log.WarnContext(ctx, op+": sections sanitised", slog.Int("dropped", d))
	}
	if d := preTasks - len(p.Data.Tasks); d > 0 {
		log.WarnContext(ctx, op+": tasks sanitised", slog.Int("dropped", d))
	}
	if d := preTaskLabels - len(p.Data.TaskLabels); d > 0 {
		log.WarnContext(ctx, op+": task labels sanitised", slog.Int("dropped", d))
	}
	if d := preProjectLabels - len(p.Data.ProjectLabels); d > 0 {
		log.WarnContext(ctx, op+": project labels sanitised", slog.Int("dropped", d))
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		log.ErrorContext(ctx, op+": begin tx", slog.String("err", err.Error()))
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// defer_foreign_keys postpones FK checks until COMMIT, which lets us insert
	// self-referential rows (tasks.parent_id) without ordering parents before
	// children. The pragma is scoped to this transaction.
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		log.ErrorContext(ctx, op+": defer fk", slog.String("err", err.Error()))
		return fmt.Errorf("defer fk: %w", err)
	}

	if err := wipeAll(ctx, tx); err != nil {
		log.ErrorContext(ctx, op+": wipe", slog.String("err", err.Error()))
		return fmt.Errorf("wipe: %w", err)
	}
	if err := insertContexts(ctx, tx, p.Data.Contexts); err != nil {
		log.ErrorContext(ctx, op+": insert contexts", slog.String("err", err.Error()))
		return fmt.Errorf("contexts: %w", err)
	}
	if err := insertLabels(ctx, tx, p.Data.Labels); err != nil {
		log.ErrorContext(ctx, op+": insert labels", slog.String("err", err.Error()))
		return fmt.Errorf("labels: %w", err)
	}
	if err := insertProjects(ctx, tx, p.Data.Projects); err != nil {
		log.ErrorContext(ctx, op+": insert projects", slog.String("err", err.Error()))
		return fmt.Errorf("projects: %w", err)
	}
	if err := insertProjectLabels(ctx, tx, p.Data.ProjectLabels); err != nil {
		log.ErrorContext(ctx, op+": insert project labels", slog.String("err", err.Error()))
		return fmt.Errorf("project labels: %w", err)
	}
	if err := insertProjectSections(ctx, tx, p.Data.ProjectSections); err != nil {
		log.ErrorContext(ctx, op+": insert project sections", slog.String("err", err.Error()))
		return fmt.Errorf("project sections: %w", err)
	}
	if err := insertTasks(ctx, tx, p.Data.Tasks); err != nil {
		log.ErrorContext(ctx, op+": insert tasks", slog.String("err", err.Error()))
		return fmt.Errorf("tasks: %w", err)
	}
	if err := insertTaskLabels(ctx, tx, p.Data.TaskLabels); err != nil {
		log.ErrorContext(ctx, op+": insert task labels", slog.String("err", err.Error()))
		return fmt.Errorf("task labels: %w", err)
	}
	if p.Settings != nil {
		if err := applySettings(ctx, tx, p.Settings); err != nil {
			log.ErrorContext(ctx, op+": apply settings", slog.String("err", err.Error()))
			return fmt.Errorf("settings: %w", err)
		}
	}
	if violation, err := firstFKViolation(ctx, tx); err != nil {
		log.ErrorContext(ctx, op+": fk check", slog.String("err", err.Error()))
		return fmt.Errorf("fk check: %w", err)
	} else if violation != "" {
		log.ErrorContext(ctx, op+": fk violation", slog.String("violation", violation))
		return fmt.Errorf("fk check failed: %s", violation)
	}
	if err := tx.Commit(); err != nil {
		log.ErrorContext(ctx, op+": commit", slog.String("err", err.Error()))
		return err
	}
	log.InfoContext(ctx, "backup restore finished", slog.String("op", op))
	return nil
}
