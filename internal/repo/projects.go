package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

type ProjectRepo struct {
	db     *sql.DB
	labels *ProjectLabelsRepo
}

func NewProjectRepo(db *sql.DB, labels *ProjectLabelsRepo) *ProjectRepo {
	return &ProjectRepo{db: db, labels: labels}
}

func scanProject(row interface{ Scan(...any) error }) (*model.Project, error) {
	var p model.Project
	var pinned, priv, federated int
	var pinnedAt, troikiCategory, clientID, deletedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&p.ID, &p.ContextID, &p.Title, &p.Description, &p.Color, &p.Status, &p.Type, &pinned, &pinnedAt, &priv, &federated, &troikiCategory, &clientID, &deletedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	p.IsPinned = pinned == 1
	p.IsPrivate = priv == 1
	p.IsFederated = federated == 1
	p.ClientID = clientID.String
	if deletedAt.Valid {
		t, err := model.ParseUTC(deletedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse deleted_at: %w", err)
		}
		p.DeletedAt = &t
	}
	if pinnedAt.Valid {
		t, err := model.ParseUTC(pinnedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse pinned_at: %w", err)
		}
		p.PinnedAt = &t
	}
	if troikiCategory.Valid {
		c := model.TroikiCategory(troikiCategory.String)
		p.TroikiCategory = &c
	}
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	p.CreatedAt = t
	t, err = model.ParseUTC(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	p.UpdatedAt = t
	return &p, nil
}

const projectColumns = `id, context_id, title, description, color, status, project_type, is_pinned, pinned_at, is_private, is_federated, troiki_category, client_id, deleted_at, created_at, updated_at`

type CreateProject struct {
	ContextID   int64
	Title       string
	Description string
	Color       string
	Type        model.ProjectType
}

func (r *ProjectRepo) Create(ctx context.Context, in CreateProject) (*model.Project, error) {
	const op = "repo.projects.Create"
	logQuery(ctx, op, in.ContextID, string(in.Type))
	var id int64
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		newID, err := r.CreateTx(ctx, tx, in, model.NewClientID())
		if err != nil {
			return err
		}
		id = newID
		return nil
	})
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return r.Get(ctx, id)
}

// CreateTx inserts a project inside a caller's transaction with the supplied
// cross-instance client_id, so a federated create can run the domain write in
// the SAME tx as the outbox emit (atomicity, NFR-2). It returns the new
// project's local id; the caller re-reads after the tx commits. clientID must be
// a fresh ULID for a create.
func (r *ProjectRepo) CreateTx(ctx context.Context, tx *sql.Tx, in CreateProject, clientID string) (int64, error) {
	now := model.FormatUTC(time.Now())
	pt := in.Type
	if pt == "" {
		pt = model.ProjectTypeGeneric
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO projects (context_id, title, description, color, status, project_type, is_pinned, pinned_at, client_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'open', ?, 0, NULL, ?, ?, ?)`,
		in.ContextID, in.Title, in.Description, in.Color, string(pt), clientID, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrConflict
		}
		return 0, fmt.Errorf("insert project: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *ProjectRepo) Get(ctx context.Context, id int64) (*model.Project, error) {
	const op = "repo.projects.Get"
	logQuery(ctx, op, id)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+projectColumns+` FROM projects WHERE id = ? AND deleted_at IS NULL`, id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	if r.labels != nil {
		hydrated, err := r.labels.LabelsByProjectIDs(ctx, []int64{p.ID})
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		p.Labels = hydrated[p.ID]
	}
	return p, nil
}

type ProjectListFilter struct {
	ContextID *int64
	Status    *model.ProjectStatus
}

func (r *ProjectRepo) List(ctx context.Context, filter ProjectListFilter, page Page) ([]model.Project, int, error) {
	const op = "repo.projects.List"
	logQuery(ctx, op, filter, page)
	page = page.Normalize()
	conds := []string{"deleted_at IS NULL"}
	args := []any{}
	if filter.ContextID != nil {
		conds = append(conds, "context_id = ?")
		args = append(args, *filter.ContextID)
	}
	if filter.Status != nil {
		conds = append(conds, "status = ?")
		args = append(args, string(*filter.Status))
	}
	where := " WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`+where, args...).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count projects: %w", err))
	}

	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, page.Limit, page.Offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+projectColumns+` FROM projects`+where+
			` ORDER BY is_pinned DESC, pinned_at DESC, created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("list projects: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.Project, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, logErr(ctx, op, err)
	}
	if r.labels != nil && len(ids) > 0 {
		hydrated, err := r.labels.LabelsByProjectIDs(ctx, ids)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		for i := range out {
			out[i].Labels = hydrated[out[i].ID]
		}
	}
	return out, total, nil
}

type ProjectUpdate struct {
	Title       *string
	Description *string
	Color       *string
	ContextID   *int64
	IsPrivate   *bool
	Type        *model.ProjectType

	TroikiCategory      *model.TroikiCategory
	TroikiCategoryClear bool
}

func (r *ProjectRepo) Update(ctx context.Context, id int64, u ProjectUpdate) (*model.Project, error) {
	const op = "repo.projects.Update"
	logQuery(ctx, op, id)
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		return r.UpdateTx(ctx, tx, id, u)
	})
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return r.Get(ctx, id)
}

// UpdateTx applies a project field update inside a caller's transaction, so a
// federated update can run the domain write in the SAME tx as the outbox emit
// (atomicity, NFR-2). It does NOT re-read the row; the caller re-reads after the
// tx commits. A missing/already-tombstoned project returns ErrNotFound/ErrGone.
func (r *ProjectRepo) UpdateTx(ctx context.Context, tx *sql.Tx, id int64, u ProjectUpdate) error {
	sets := make([]string, 0, 4)
	args := make([]any, 0, 5)
	if u.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *u.Title)
	}
	if u.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *u.Description)
	}
	if u.Color != nil {
		sets = append(sets, "color = ?")
		args = append(args, *u.Color)
	}
	if u.ContextID != nil {
		sets = append(sets, "context_id = ?")
		args = append(args, *u.ContextID)
	}
	if u.IsPrivate != nil {
		sets = append(sets, "is_private = ?")
		pv := 0
		if *u.IsPrivate {
			pv = 1
		}
		args = append(args, pv)
	}
	if u.Type != nil {
		sets = append(sets, "project_type = ?")
		args = append(args, string(*u.Type))
	}
	if u.TroikiCategoryClear {
		sets = append(sets, "troiki_category = NULL")
	} else if u.TroikiCategory != nil {
		sets = append(sets, "troiki_category = ?")
		args = append(args, string(*u.TroikiCategory))
	}
	if len(sets) == 0 {
		// No-op update: verify the row is live so a PATCH on a tombstone surfaces
		// 410, not a silent success (US-3.7 AC2). A live row → nil.
		return requireLiveTx(ctx, tx, "projects", id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, model.FormatUTC(time.Now()))
	args = append(args, id)

	res, err := tx.ExecContext(ctx, `UPDATE projects SET `+joinSets(sets)+` WHERE id = ? AND deleted_at IS NULL`, args...)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFoundOrGoneTx(ctx, tx, "projects", id)
	}
	return nil
}

// UpdateStatus changes the project status. Transitioning out of 'open' also
// clears troiki_category — a non-open project no longer occupies a Troiki slot,
// and reopening it must require an explicit re-assignment so capacity is
// re-checked against the current state of the slot.
func (r *ProjectRepo) UpdateStatus(ctx context.Context, id int64, status model.ProjectStatus) error {
	const op = "repo.projects.UpdateStatus"
	logQuery(ctx, op, id, status)
	if err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		return r.UpdateStatusTx(ctx, tx, id, status)
	}); err != nil {
		return logErr(ctx, op, err)
	}
	return nil
}

// UpdateStatusTx changes the project status inside a caller's transaction, so a
// federated status change (archive/complete/cancel/open) can run the domain
// write in the SAME tx as the outbox emit (atomicity, NFR-2 — see
// service/federation.Emitter.EmitMutation). It mirrors UpdateStatus: leaving
// 'open' also clears troiki_category (a non-open project no longer occupies a
// Troiki slot). A missing/already-tombstoned project returns ErrNotFound. The
// troiki_category clear is a turboist-local side effect; only `status` travels
// in the federated event (the field set in fields.go projectStatusFields).
func (r *ProjectRepo) UpdateStatusTx(ctx context.Context, tx *sql.Tx, id int64, status model.ProjectStatus) error {
	now := model.FormatUTC(time.Now())
	var res sql.Result
	var err error
	if status == model.ProjectStatusOpen {
		res, err = tx.ExecContext(ctx,
			`UPDATE projects SET status = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
			string(status), now, id)
	} else {
		res, err = tx.ExecContext(ctx,
			`UPDATE projects SET status = ?, troiki_category = NULL, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
			string(status), now, id)
	}
	if err != nil {
		return fmt.Errorf("update project status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProjectRepo) SetPinned(ctx context.Context, id int64, pinned bool) error {
	const op = "repo.projects.SetPinned"
	logQuery(ctx, op, id, pinned)
	now := model.FormatUTC(time.Now())
	var res sql.Result
	var err error
	if pinned {
		res, err = r.db.ExecContext(ctx,
			`UPDATE projects SET is_pinned = 1, pinned_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, now, now, id)
	} else {
		res, err = r.db.ExecContext(ctx,
			`UPDATE projects SET is_pinned = 0, pinned_at = NULL, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, now, id)
	}
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("set pinned: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n == 0 {
		// Mirror TaskRepo.SetPinned: re-pinning a tombstoned project is a 410, not
		// a 404 (Federation v1 F0.1, US-3.7 AC2).
		return logErr(ctx, op, notFoundOrGone(ctx, r.db, "projects", id))
	}
	return nil
}

// SetFederatedTx flips the project's is_federated flag inside the given
// transaction (Federation v1 F1.1). It is tx-scoped so EnableForProject can set
// the flag and insert the federated_projects self-row atomically. Returns the
// number of rows affected so the caller can distinguish a missing/tombstoned
// project (0) from a successful update (1). Re-enabling an already-federated
// project is a no-op flip and still reports 1 row.
func (r *ProjectRepo) SetFederatedTx(ctx context.Context, tx *sql.Tx, id int64, federated bool) (int64, error) {
	const op = "repo.projects.SetFederatedTx"
	logQuery(ctx, op, id, federated)
	now := model.FormatUTC(time.Now())
	fv := 0
	if federated {
		fv = 1
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE projects SET is_federated = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		fv, now, id)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("set is_federated: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, err)
	}
	return n, nil
}

// ClearAllTroikiCategories unassigns every project from its Troiki category in
// a single UPDATE. Used by Troiki Reset.
func (r *ProjectRepo) ClearAllTroikiCategories(ctx context.Context) error {
	const op = "repo.projects.ClearAllTroikiCategories"
	logQuery(ctx, op)
	now := model.FormatUTC(time.Now())
	_, err := r.db.ExecContext(ctx,
		`UPDATE projects SET troiki_category = NULL, updated_at = ? WHERE troiki_category IS NOT NULL AND deleted_at IS NULL`,
		now)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("clear all troiki categories: %w", err))
	}
	return nil
}

// Delete soft-deletes the project and emulates the former FK cascade
// (projects → project_sections, projects → tasks). The DB ON DELETE CASCADE no
// longer fires for soft-deletes, so the child tombstones are written here in
// one transaction (Federation v1 F0.1).
func (r *ProjectRepo) Delete(ctx context.Context, id int64) error {
	const op = "repo.projects.Delete"
	logQuery(ctx, op, id)
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		return r.DeleteTx(ctx, tx, id)
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrGone) {
			return logErr(ctx, op, err)
		}
		return logErr(ctx, op, fmt.Errorf("delete project: %w", err))
	}
	return nil
}

// DeleteTx soft-deletes the project and emulates the former FK cascade (project
// → sections + tasks) inside a caller's transaction, so a federated project
// delete can run the domain write in the SAME tx as the outbox emit (atomicity,
// NFR-2 — see service/federation.Emitter.EmitDeleteCascade). Re-deleting an
// already-tombstoned project returns ErrGone (410); a missing one ErrNotFound.
func (r *ProjectRepo) DeleteTx(ctx context.Context, tx *sql.Tx, id int64) error {
	now := model.FormatUTC(time.Now())
	res, err := tx.ExecContext(ctx,
		`UPDATE projects SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, now, now, id)
	if err != nil {
		return fmt.Errorf("soft-delete project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Re-deleting an already-tombstoned project is a 410, not a 404 — consistent
		// with UpdateTx and the task/section delete handlers (US-3.7 AC2).
		return notFoundOrGoneTx(ctx, tx, "projects", id)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE tasks SET deleted_at = ?, updated_at = ? WHERE project_id = ? AND deleted_at IS NULL`,
		now, now, id); err != nil {
		return fmt.Errorf("cascade tasks: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE project_sections SET deleted_at = ?, updated_at = ? WHERE project_id = ? AND deleted_at IS NULL`,
		now, now, id); err != nil {
		return fmt.Errorf("cascade sections: %w", err)
	}
	return nil
}

func (r *ProjectRepo) SetLabels(ctx context.Context, projectID int64, labelIDs []int64) error {
	if r.labels == nil {
		return nil
	}
	return r.labels.SetForProject(ctx, projectID, labelIDs)
}

// SetTroikiCategoryIfRoom atomically assigns the troiki_category to the project
// iff the project is open and the count of currently-open projects in that
// category is below `capacity`. Returns:
//   - (true, nil)   when the assignment succeeded
//   - (false, nil)  when the slot is full or the project is no longer open
//   - (false, ErrNotFound) when the project id does not exist
func (r *ProjectRepo) SetTroikiCategoryIfRoom(ctx context.Context, id int64, cat model.TroikiCategory, capacity int) (bool, error) {
	const op = "repo.projects.SetTroikiCategoryIfRoom"
	logQuery(ctx, op, id, cat, capacity)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE projects
		 SET troiki_category = ?, updated_at = ?
		 WHERE id = ?
		   AND status = 'open'
		   AND deleted_at IS NULL
		   AND (SELECT COUNT(*) FROM projects WHERE troiki_category = ? AND status = 'open' AND deleted_at IS NULL) < ?`,
		string(cat), now, id, string(cat), capacity)
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("set project troiki category: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, logErr(ctx, op, err)
	}
	if n > 0 {
		return true, nil
	}
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE id = ? AND deleted_at IS NULL`, id).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, logErr(ctx, op, ErrNotFound)
		}
		return false, logErr(ctx, op, fmt.Errorf("set project troiki category: %w", err))
	}
	return false, nil
}

// ListByTroikiCategory returns all open projects for the given Troiki category
// along with their total count. Slot counts and the rendered list must agree,
// so this listing is intentionally not paginated.
func (r *ProjectRepo) ListByTroikiCategory(ctx context.Context, cat model.TroikiCategory) ([]model.Project, int, error) {
	const op = "repo.projects.ListByTroikiCategory"
	logQuery(ctx, op, cat)
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM projects WHERE troiki_category = ? AND status = 'open' AND deleted_at IS NULL`,
		string(cat)).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count troiki projects: %w", err))
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+projectColumns+` FROM projects
		 WHERE troiki_category = ? AND status = 'open' AND deleted_at IS NULL
		 ORDER BY is_pinned DESC, pinned_at DESC, created_at DESC`,
		string(cat))
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("list troiki projects: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.Project, 0, total)
	ids := make([]int64, 0, total)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, logErr(ctx, op, err)
	}
	if r.labels != nil && len(ids) > 0 {
		hydrated, err := r.labels.LabelsByProjectIDs(ctx, ids)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		for i := range out {
			out[i].Labels = hydrated[out[i].ID]
		}
	}
	return out, total, nil
}

func (r *ProjectRepo) CountOpenByTroikiCategory(ctx context.Context, cat model.TroikiCategory) (int, error) {
	const op = "repo.projects.CountOpenByTroikiCategory"
	logQuery(ctx, op, cat)
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM projects WHERE troiki_category = ? AND status = 'open' AND deleted_at IS NULL`,
		string(cat)).Scan(&n); err != nil {
		return 0, logErr(ctx, op, err)
	}
	return n, nil
}

func (r *ProjectRepo) ListByLabel(ctx context.Context, labelID int64, page Page) ([]model.Project, int, error) {
	const op = "repo.projects.ListByLabel"
	logQuery(ctx, op, labelID, page)
	page = page.Normalize()
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM projects p
		 JOIN project_labels pl ON pl.project_id = p.id
		 WHERE pl.label_id = ? AND p.deleted_at IS NULL`, labelID).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count projects by label: %w", err))
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id, p.context_id, p.title, p.description, p.color, p.status, p.project_type,
		        p.is_pinned, p.pinned_at, p.is_private, p.is_federated, p.troiki_category, p.client_id, p.deleted_at, p.created_at, p.updated_at
		 FROM projects p
		 JOIN project_labels pl ON pl.project_id = p.id
		 WHERE pl.label_id = ? AND p.deleted_at IS NULL
		 ORDER BY p.is_pinned DESC, p.pinned_at DESC, p.created_at DESC
		 LIMIT ? OFFSET ?`, labelID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("list projects by label: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make([]model.Project, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, logErr(ctx, op, err)
	}
	if r.labels != nil && len(ids) > 0 {
		hydrated, err := r.labels.LabelsByProjectIDs(ctx, ids)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		for i := range out {
			out[i].Labels = hydrated[out[i].ID]
		}
	}
	return out, total, nil
}
