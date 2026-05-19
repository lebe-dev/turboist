package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// BackupSchemaVersion is the on-disk schema version for backup payloads. Bump
// when the JSON structure changes in a non-additive way.
const BackupSchemaVersion = 1

// BackupPayload is the full export envelope written to disk and consumed on
// restore. Times are encoded as the same UTC strings used in the SQLite schema
// so a round-trip preserves the database byte-for-byte (modulo autoincrement
// sequence rows). The Settings field is optional and controlled by the
// IncludeSettings export option.
type BackupPayload struct {
	Version    int           `json:"version"`
	ExportedAt string        `json:"exportedAt"`
	Data       BackupData    `json:"data"`
	Settings   *BackupConfig `json:"settings,omitempty"`
}

type BackupData struct {
	Contexts        []BackupContext        `json:"contexts"`
	Labels          []BackupLabel          `json:"labels"`
	Projects        []BackupProject        `json:"projects"`
	ProjectSections []BackupProjectSection `json:"projectSections"`
	Tasks           []BackupTask           `json:"tasks"`
	TaskLabels      []BackupTaskLabel      `json:"taskLabels"`
	ProjectLabels   []BackupProjectLabel   `json:"projectLabels"`
}

type BackupConfig struct {
	User *model.UserSettings `json:"user,omitempty"`
	App  *model.AppSettings  `json:"app,omitempty"`
}

type BackupContext struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	IsFavourite bool   `json:"isFavourite"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type BackupLabel struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	IsFavourite bool   `json:"isFavourite"`
	IsPrivate   bool   `json:"isPrivate"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type BackupProject struct {
	ID             int64   `json:"id"`
	ContextID      int64   `json:"contextId"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Color          string  `json:"color"`
	Status         string  `json:"status"`
	IsPinned       bool    `json:"isPinned"`
	PinnedAt       *string `json:"pinnedAt,omitempty"`
	IsPrivate      bool    `json:"isPrivate"`
	ProjectType    string  `json:"projectType"`
	TroikiCategory *string `json:"troikiCategory,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

type BackupProjectSection struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Title     string `json:"title"`
	Position  int    `json:"position"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type BackupTask struct {
	ID                    int64   `json:"id"`
	Title                 string  `json:"title"`
	Description           string  `json:"description"`
	InboxID               *int64  `json:"inboxId,omitempty"`
	ContextID             *int64  `json:"contextId,omitempty"`
	ProjectID             *int64  `json:"projectId,omitempty"`
	SectionID             *int64  `json:"sectionId,omitempty"`
	ParentID              *int64  `json:"parentId,omitempty"`
	Priority              string  `json:"priority"`
	Status                string  `json:"status"`
	DueAt                 *string `json:"dueAt,omitempty"`
	DueHasTime            bool    `json:"dueHasTime"`
	DeadlineAt            *string `json:"deadlineAt,omitempty"`
	DeadlineHasTime       bool    `json:"deadlineHasTime"`
	DayPart               string  `json:"dayPart"`
	PlanState             string  `json:"planState"`
	IsPinned              bool    `json:"isPinned"`
	PinnedAt              *string `json:"pinnedAt,omitempty"`
	IsPrivate             bool    `json:"isPrivate"`
	RecurrenceRule        *string `json:"recurrenceRule,omitempty"`
	CompletedAt           *string `json:"completedAt,omitempty"`
	PostponeCount         int     `json:"postponeCount"`
	TroikiCategory        *string `json:"troikiCategory,omitempty"`
	TroikiCapacityGranted bool    `json:"troikiCapacityGranted"`
	CreatedAt             string  `json:"createdAt"`
	UpdatedAt             string  `json:"updatedAt"`
}

type BackupTaskLabel struct {
	TaskID  int64 `json:"taskId"`
	LabelID int64 `json:"labelId"`
}

type BackupProjectLabel struct {
	ProjectID int64 `json:"projectId"`
	LabelID   int64 `json:"labelId"`
}

// BackupService produces and consumes BackupPayload snapshots of the database.
// It operates on raw SQL so IDs can be preserved verbatim and the entire
// restore happens inside one transaction.
type BackupService struct {
	db *sql.DB
}

func NewBackupService(db *sql.DB) *BackupService {
	return &BackupService{db: db}
}

// ExportOptions controls what is included in the backup.
type ExportOptions struct {
	// IncludeSettings adds per-user and global app settings to the payload.
	IncludeSettings bool
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

// Marshal serializes the payload into a single JSON document. The encoder is
// configured to keep IDs as raw numbers, which json.Marshal already does.
func (p *BackupPayload) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

// ErrBadBackup is returned when the input bytes are not a recognizable backup.
var ErrBadBackup = errors.New("backup: invalid payload")

// DecodeBackup parses a backup payload. It transparently accepts either a
// plain JSON document or a gzipped one (detected via the 0x1f 0x8b magic).
func DecodeBackup(raw []byte) (*BackupPayload, error) {
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: gzip header: %v", ErrBadBackup, err)
		}
		defer func() { _ = zr.Close() }()
		decoded, err := io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("%w: gzip body: %v", ErrBadBackup, err)
		}
		raw = decoded
	}
	var p BackupPayload
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadBackup, err)
	}
	if p.Version != BackupSchemaVersion {
		return nil, fmt.Errorf("%w: unsupported version %d (want %d)", ErrBadBackup, p.Version, BackupSchemaVersion)
	}
	return &p, nil
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

// sanitizePayload removes dangling references from the input data. We do this
// rather than failing the restore because such references can legitimately
// exist in older databases that were written before all the constraints were
// enforced, and the user's intent is clearly to recover what they have.
func sanitizePayload(d *BackupData) {
	contextIDs := idSet(d.Contexts, func(c BackupContext) int64 { return c.ID })
	labelIDs := idSet(d.Labels, func(l BackupLabel) int64 { return l.ID })

	// Drop projects whose context vanished (context_id is NOT NULL).
	projects := d.Projects[:0]
	for _, p := range d.Projects {
		if _, ok := contextIDs[p.ContextID]; ok {
			projects = append(projects, p)
		}
	}
	d.Projects = projects
	projectIDs := idSet(d.Projects, func(p BackupProject) int64 { return p.ID })

	// Drop sections whose project vanished.
	sections := d.ProjectSections[:0]
	for _, s := range d.ProjectSections {
		if _, ok := projectIDs[s.ProjectID]; ok {
			sections = append(sections, s)
		}
	}
	d.ProjectSections = sections
	sectionIDs := idSet(d.ProjectSections, func(s BackupProjectSection) int64 { return s.ID })

	// Tasks: null nullable refs to missing rows; drop tasks whose required
	// placement (inbox OR context) is unsatisfiable.
	taskIDs := make(map[int64]struct{}, len(d.Tasks))
	for _, t := range d.Tasks {
		taskIDs[t.ID] = struct{}{}
	}
	tasks := d.Tasks[:0]
	for _, t := range d.Tasks {
		if t.ContextID != nil {
			if _, ok := contextIDs[*t.ContextID]; !ok {
				t.ContextID = nil
			}
		}
		if t.ProjectID != nil {
			if _, ok := projectIDs[*t.ProjectID]; !ok {
				t.ProjectID = nil
				t.SectionID = nil
			}
		}
		if t.SectionID != nil {
			if _, ok := sectionIDs[*t.SectionID]; !ok {
				t.SectionID = nil
			}
		}
		if t.ParentID != nil {
			if _, ok := taskIDs[*t.ParentID]; !ok {
				t.ParentID = nil
			}
		}
		// CHECK constraint: exactly one of inbox_id / context_id must be set.
		if (t.InboxID == nil) == (t.ContextID == nil) {
			// Skip rows that no longer satisfy the placement invariant.
			delete(taskIDs, t.ID)
			continue
		}
		tasks = append(tasks, t)
	}
	d.Tasks = tasks

	// Drop link rows that lost an endpoint.
	tl := d.TaskLabels[:0]
	for _, l := range d.TaskLabels {
		if _, t := taskIDs[l.TaskID]; !t {
			continue
		}
		if _, lb := labelIDs[l.LabelID]; !lb {
			continue
		}
		tl = append(tl, l)
	}
	d.TaskLabels = tl

	pl := d.ProjectLabels[:0]
	for _, l := range d.ProjectLabels {
		if _, p := projectIDs[l.ProjectID]; !p {
			continue
		}
		if _, lb := labelIDs[l.LabelID]; !lb {
			continue
		}
		pl = append(pl, l)
	}
	d.ProjectLabels = pl
}

func idSet[T any](rows []T, key func(T) int64) map[int64]struct{} {
	m := make(map[int64]struct{}, len(rows))
	for _, r := range rows {
		m[key(r)] = struct{}{}
	}
	return m
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
	defer func() { _ = rows.Close() }()
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

// --- export readers ---

func (s *BackupService) readContexts(ctx context.Context) ([]BackupContext, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, color, is_favourite, created_at, updated_at FROM contexts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
		} else if err := json.Unmarshal([]byte(userRaw), &us); err == nil {
			cfg.User = &us
		} else {
			cfg.User = &model.UserSettings{}
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
		} else if err := json.Unmarshal([]byte(appRaw), &as); err == nil {
			cfg.App = &as
		} else {
			cfg.App = &model.AppSettings{}
		}
	}
	return cfg, nil
}

// --- restore writers ---

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

// --- helpers ---

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullable(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func nullableID(i *int64) any {
	if i == nil {
		return nil
	}
	return *i
}
