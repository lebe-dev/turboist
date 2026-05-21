package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// wipeAll clears every user-owned table in FK-safe order and resets the
// autoincrement sequence so subsequent explicit IDs are honoured cleanly.
// inbox, users, sessions, api_tokens, app_settings are intentionally
// preserved: they hold account state and singleton rows that backup never
// touches.
func wipeAll(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		`DELETE FROM task_labels`,
		`DELETE FROM project_labels`,
		`DELETE FROM tasks`,
		`DELETE FROM project_sections`,
		`DELETE FROM projects`,
		`DELETE FROM labels`,
		`DELETE FROM contexts`,
		`DELETE FROM sqlite_sequence
		   WHERE name IN ('tasks','project_sections','projects','labels','contexts')`,
	}
	for _, q := range stmts {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("exec %q: %w", q, err)
		}
	}
	return nil
}

func insertContexts(ctx context.Context, tx *sql.Tx, rows []BackupContext) error {
	const q = `INSERT INTO contexts (id, name, color, is_favourite, created_at, updated_at)
	           VALUES (?, ?, ?, ?, ?, ?)`
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, q, r.ID, r.Name, r.Color, boolToInt(r.IsFavourite), r.CreatedAt, r.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func insertLabels(ctx context.Context, tx *sql.Tx, rows []BackupLabel) error {
	const q = `INSERT INTO labels (id, name, color, is_favourite, is_private, created_at, updated_at)
	           VALUES (?, ?, ?, ?, ?, ?, ?)`
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, q, r.ID, r.Name, r.Color,
			boolToInt(r.IsFavourite), boolToInt(r.IsPrivate), r.CreatedAt, r.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func insertProjects(ctx context.Context, tx *sql.Tx, rows []BackupProject) error {
	const q = `INSERT INTO projects (id, context_id, title, description, color, status,
	                                  is_pinned, pinned_at, is_private, project_type, troiki_category,
	                                  created_at, updated_at)
	           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for _, r := range rows {
		projectType := r.ProjectType
		if projectType == "" {
			projectType = "generic"
		}
		if _, err := tx.ExecContext(ctx, q,
			r.ID, r.ContextID, r.Title, r.Description, r.Color, r.Status,
			boolToInt(r.IsPinned), nullable(r.PinnedAt), boolToInt(r.IsPrivate),
			projectType, nullable(r.TroikiCategory),
			r.CreatedAt, r.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func insertProjectSections(ctx context.Context, tx *sql.Tx, rows []BackupProjectSection) error {
	const q = `INSERT INTO project_sections (id, project_id, title, position, created_at, updated_at)
	           VALUES (?, ?, ?, ?, ?, ?)`
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, q, r.ID, r.ProjectID, r.Title, r.Position, r.CreatedAt, r.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func insertTasks(ctx context.Context, tx *sql.Tx, rows []BackupTask) error {
	const q = `INSERT INTO tasks (id, title, description, inbox_id, context_id, project_id, section_id, parent_id,
	                              priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
	                              day_part, plan_state, is_pinned, pinned_at, is_private,
	                              recurrence_rule, completed_at, postpone_count, troiki_category, troiki_capacity_granted,
	                              created_at, updated_at)
	           VALUES (?, ?, ?, ?, ?, ?, ?, ?,
	                   ?, ?, ?, ?, ?, ?,
	                   ?, ?, ?, ?, ?,
	                   ?, ?, ?, ?, ?,
	                   ?, ?)`
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, q,
			r.ID, r.Title, r.Description,
			nullableID(r.InboxID), nullableID(r.ContextID), nullableID(r.ProjectID), nullableID(r.SectionID), nullableID(r.ParentID),
			r.Priority, r.Status,
			nullable(r.DueAt), boolToInt(r.DueHasTime), nullable(r.DeadlineAt), boolToInt(r.DeadlineHasTime),
			r.DayPart, r.PlanState,
			boolToInt(r.IsPinned), nullable(r.PinnedAt), boolToInt(r.IsPrivate),
			nullable(r.RecurrenceRule), nullable(r.CompletedAt), r.PostponeCount, nullable(r.TroikiCategory), boolToInt(r.TroikiCapacityGranted),
			r.CreatedAt, r.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func insertTaskLabels(ctx context.Context, tx *sql.Tx, rows []BackupTaskLabel) error {
	const q = `INSERT INTO task_labels (task_id, label_id) VALUES (?, ?)`
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, q, r.TaskID, r.LabelID); err != nil {
			return err
		}
	}
	return nil
}

func insertProjectLabels(ctx context.Context, tx *sql.Tx, rows []BackupProjectLabel) error {
	const q = `INSERT INTO project_labels (project_id, label_id) VALUES (?, ?)`
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, q, r.ProjectID, r.LabelID); err != nil {
			return err
		}
	}
	return nil
}

func applySettings(ctx context.Context, tx *sql.Tx, cfg *BackupConfig) error {
	if cfg.User != nil {
		raw, err := json.Marshal(cfg.User)
		if err != nil {
			return fmt.Errorf("encode user settings: %w", err)
		}
		now := model.FormatUTC(time.Now())
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET settings = ?, updated_at = ? WHERE id = 1`,
			string(raw), now); err != nil {
			return fmt.Errorf("update user settings: %w", err)
		}
	}
	if cfg.App != nil {
		raw, err := json.Marshal(cfg.App)
		if err != nil {
			return fmt.Errorf("encode app settings: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO app_settings (id, data) VALUES (1, ?)
			 ON CONFLICT(id) DO UPDATE SET data = excluded.data`,
			string(raw)); err != nil {
			return fmt.Errorf("upsert app settings: %w", err)
		}
	}
	return nil
}

// firstFKViolation runs PRAGMA foreign_key_check inside the transaction and
// returns a short description of the first dangling row, if any. Used to
// surface a useful error message instead of an opaque SQLITE_CONSTRAINT at
// commit time when sanitisation missed something.
func firstFKViolation(ctx context.Context, tx *sql.Tx) (string, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return "", err
	}
	defer logging.LogClose(ctx, "service.backup.firstFKViolation.rows", rows)
	if rows.Next() {
		var table, parent sql.NullString
		var rowid, fkid sql.NullInt64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return "", err
		}
		return fmt.Sprintf("table=%s rowid=%d parent=%s fkid=%d",
			table.String, rowid.Int64, parent.String, fkid.Int64), nil
	}
	return "", rows.Err()
}
