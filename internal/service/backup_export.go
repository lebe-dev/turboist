package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

func (s *BackupService) readContexts(ctx context.Context) ([]BackupContext, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, color, is_favourite, created_at, updated_at FROM contexts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer logging.LogClose(ctx, "service.backup.export.rows", rows)
	out := make([]BackupContext, 0)
	for rows.Next() {
		var c BackupContext
		var fav int
		if err := rows.Scan(&c.ID, &c.Name, &c.Color, &fav, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.IsFavourite = fav == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *BackupService) readLabels(ctx context.Context) ([]BackupLabel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, color, is_favourite, is_private, created_at, updated_at FROM labels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer logging.LogClose(ctx, "service.backup.export.rows", rows)
	out := make([]BackupLabel, 0)
	for rows.Next() {
		var l BackupLabel
		var fav, priv int
		if err := rows.Scan(&l.ID, &l.Name, &l.Color, &fav, &priv, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		l.IsFavourite = fav == 1
		l.IsPrivate = priv == 1
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *BackupService) readProjects(ctx context.Context) ([]BackupProject, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, context_id, title, description, color, status, is_pinned, pinned_at,
				is_private, project_type, troiki_category, created_at, updated_at
		 FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer logging.LogClose(ctx, "service.backup.export.rows", rows)
	out := make([]BackupProject, 0)
	for rows.Next() {
		var p BackupProject
		var pinned, priv int
		var pinnedAt, troiki sql.NullString
		if err := rows.Scan(&p.ID, &p.ContextID, &p.Title, &p.Description, &p.Color, &p.Status,
			&pinned, &pinnedAt, &priv, &p.ProjectType, &troiki, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.IsPinned = pinned == 1
		p.IsPrivate = priv == 1
		if pinnedAt.Valid {
			v := pinnedAt.String
			p.PinnedAt = &v
		}
		if troiki.Valid {
			v := troiki.String
			p.TroikiCategory = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *BackupService) readProjectSections(ctx context.Context) ([]BackupProjectSection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, title, position, created_at, updated_at
		 FROM project_sections ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer logging.LogClose(ctx, "service.backup.export.rows", rows)
	out := make([]BackupProjectSection, 0)
	for rows.Next() {
		var s BackupProjectSection
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Title, &s.Position, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (s *BackupService) readTasks(ctx context.Context) ([]BackupTask, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, description, inbox_id, context_id, project_id, section_id, parent_id,
				priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
				day_part, plan_state, is_pinned, pinned_at, is_private,
				recurrence_rule, completed_at, postpone_count, troiki_category, troiki_capacity_granted,
				created_at, updated_at
		 FROM tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer logging.LogClose(ctx, "service.backup.export.rows", rows)
	out := make([]BackupTask, 0)
	for rows.Next() {
		var t BackupTask
		var inboxID, contextID, projectID, sectionID, parentID sql.NullInt64
		var dueAt, deadlineAt, pinnedAt, recurrence, completedAt, troiki sql.NullString
		var dueHasTime, deadlineHasTime, isPinned, isPrivate, capGranted int
		if err := rows.Scan(&t.ID, &t.Title, &t.Description,
			&inboxID, &contextID, &projectID, &sectionID, &parentID,
			&t.Priority, &t.Status,
			&dueAt, &dueHasTime, &deadlineAt, &deadlineHasTime,
			&t.DayPart, &t.PlanState,
			&isPinned, &pinnedAt, &isPrivate,
			&recurrence, &completedAt, &t.PostponeCount, &troiki, &capGranted,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.DueHasTime = dueHasTime == 1
		t.DeadlineHasTime = deadlineHasTime == 1
		t.IsPinned = isPinned == 1
		t.IsPrivate = isPrivate == 1
		t.TroikiCapacityGranted = capGranted == 1
		if inboxID.Valid {
			v := inboxID.Int64
			t.InboxID = &v
		}
		if contextID.Valid {
			v := contextID.Int64
			t.ContextID = &v
		}
		if projectID.Valid {
			v := projectID.Int64
			t.ProjectID = &v
		}
		if sectionID.Valid {
			v := sectionID.Int64
			t.SectionID = &v
		}
		if parentID.Valid {
			v := parentID.Int64
			t.ParentID = &v
		}
		if dueAt.Valid {
			v := dueAt.String
			t.DueAt = &v
		}
		if deadlineAt.Valid {
			v := deadlineAt.String
			t.DeadlineAt = &v
		}
		if pinnedAt.Valid {
			v := pinnedAt.String
			t.PinnedAt = &v
		}
		if recurrence.Valid {
			v := recurrence.String
			t.RecurrenceRule = &v
		}
		if completedAt.Valid {
			v := completedAt.String
			t.CompletedAt = &v
		}
		if troiki.Valid {
			v := troiki.String
			t.TroikiCategory = &v
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *BackupService) readTaskLabels(ctx context.Context) ([]BackupTaskLabel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT task_id, label_id FROM task_labels ORDER BY task_id, label_id`)
	if err != nil {
		return nil, err
	}
	defer logging.LogClose(ctx, "service.backup.export.rows", rows)
	out := make([]BackupTaskLabel, 0)
	for rows.Next() {
		var l BackupTaskLabel
		if err := rows.Scan(&l.TaskID, &l.LabelID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *BackupService) readProjectLabels(ctx context.Context) ([]BackupProjectLabel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id, label_id FROM project_labels ORDER BY project_id, label_id`)
	if err != nil {
		return nil, err
	}
	defer logging.LogClose(ctx, "service.backup.export.rows", rows)
	out := make([]BackupProjectLabel, 0)
	for rows.Next() {
		var l BackupProjectLabel
		if err := rows.Scan(&l.ProjectID, &l.LabelID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *BackupService) readSettings(ctx context.Context) (*BackupConfig, error) {
	cfg := &BackupConfig{}

	var userRaw string
	err := s.db.QueryRowContext(ctx, `SELECT settings FROM users WHERE id = 1`).Scan(&userRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		var us model.UserSettings
		if userRaw == "" || userRaw == "{}" {
			cfg.User = &us
		} else {
			if err := json.Unmarshal([]byte(userRaw), &us); err != nil {
				return nil, fmt.Errorf("decode user settings: %w", err)
			}
			cfg.User = &us
		}
	}

	var appRaw string
	err = s.db.QueryRowContext(ctx, `SELECT data FROM app_settings WHERE id = 1`).Scan(&appRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		var as model.AppSettings
		if appRaw == "" || appRaw == "{}" {
			cfg.App = &as
		} else {
			if err := json.Unmarshal([]byte(appRaw), &as); err != nil {
				return nil, fmt.Errorf("decode app settings: %w", err)
			}
			cfg.App = &as
		}
	}
	return cfg, nil
}
