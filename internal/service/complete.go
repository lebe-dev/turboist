package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	rrule "github.com/teambition/rrule-go"
)

// dayBounds returns midnight in `loc` for the calendar day containing `t`,
// plus the midnight 24h later. Used to scope "snapshot already exists today"
// idempotency checks to the user's clock.
func dayBounds(t time.Time, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.UTC
	}
	local := t.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return start, start.Add(24 * time.Hour)
}

// RecurrenceError wraps RRULE parse or compute failures.
type RecurrenceError struct{ Err error }

func (e *RecurrenceError) Error() string { return "recurrence_invalid: " + e.Err.Error() }
func (e *RecurrenceError) Unwrap() error { return e.Err }

// TaskStatusFederator routes a task status change (complete/uncomplete/cancel and
// recurring advance) through the federation Emitter so a change on a task in a
// FEDERATED project emits a signed op event in the SAME tx as the domain write
// (Federation v1 F3.1, US-3.2 AC1, TASK A). CompleteService depends on this narrow
// interface (no federation types, no import cycle) — mirroring TaskCreator.
//
// StatusChange runs write in one tx and emits op=update for the task's changed
// federated fields (status / completed_at / priority). RecurrenceAdvance runs
// write in one tx and, for a recurring complete that advances the parent IN PLACE
// and CREATES a new occurrence snapshot, emits op=update for the parent's advance
// fields AND op=create for the snapshot (carrying snapClientID) together. A
// non-federated project (or a task with no project) incurs zero overhead — the
// Emitter gate runs the write closure only.
type TaskStatusFederator interface {
	StatusChange(ctx context.Context, task *model.Task, fields map[string]any, write func(tx *sql.Tx) error) error
	RecurrenceAdvance(ctx context.Context, parent *model.Task, advanceFields map[string]any, snapClientID string, snapFields map[string]any, write func(tx *sql.Tx) error) error
}

// CompleteService handles task completion, including recurring task advancement.
// Troiki capacity grants are derived from the parent project's category, so the
// service depends on ProjectRepo to look up the project on each completion.
type CompleteService struct {
	tasks    *repo.TaskRepo
	projects *repo.ProjectRepo
	users    *repo.UserRepo
	loc      *time.Location

	// fed routes status changes through the federation Emitter when wired
	// (production, FEDERATION_KEY set). nil → the service writes via the plain repo
	// path, so the single-user / federation-off path is byte-for-byte unchanged.
	fed TaskStatusFederator
}

func NewCompleteService(tasks *repo.TaskRepo, projects *repo.ProjectRepo, users *repo.UserRepo) *CompleteService {
	return &CompleteService{tasks: tasks, projects: projects, users: users, loc: time.UTC}
}

// NewCompleteServiceWithLoc constructs a CompleteService anchored to a specific
// timezone for RRULE evaluation (so e.g. daily 9 AM rules align with the user's clock).
func NewCompleteServiceWithLoc(tasks *repo.TaskRepo, projects *repo.ProjectRepo, users *repo.UserRepo, loc *time.Location) *CompleteService {
	if loc == nil {
		loc = time.UTC
	}
	return &CompleteService{tasks: tasks, projects: projects, users: users, loc: loc}
}

// WithFederation wires the federation status federator so a complete/uncomplete/
// cancel (and recurring advance) on a federated task emits its op event (US-3.2
// AC1). Returns the service for chaining. A nil federator leaves the service on
// the plain repo path.
func (s *CompleteService) WithFederation(f TaskStatusFederator) *CompleteService {
	s.fed = f
	return s
}

func (s *CompleteService) Complete(ctx context.Context, taskID int64) (*model.Task, error) {
	return s.completeAt(ctx, taskID, nil)
}

// CompleteAt marks a task completed with an explicit completion timestamp.
// Recurring tasks ignore the override (recurrence advance is always anchored to
// the current moment) — pass a nil time on those.
func (s *CompleteService) CompleteAt(ctx context.Context, taskID int64, completedAt time.Time) (*model.Task, error) {
	return s.completeAt(ctx, taskID, &completedAt)
}

func (s *CompleteService) completeAt(ctx context.Context, taskID int64, completedAt *time.Time) (*model.Task, error) {
	const op = "service.CompleteService.Complete"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID))
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		logRepoErr(ctx, op+": get task", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	if t.RecurrenceRule != nil {
		return s.advanceRecurring(ctx, t, completedAt)
	}
	if s.fed != nil {
		return s.completeFederated(ctx, t, completedAt)
	}
	// Always attempt the capacity bump even when the task is already completed:
	// if a previous Complete crashed between Update and bumpTroikiCapacity, the
	// task sits completed with an unset grant flag, and only a retry can recover
	// the lost grant. The flag-flip inside bumpTroikiCapacity is idempotent, so
	// it's a no-op when capacity was already granted.
	if t.Status != model.TaskStatusCompleted {
		status := model.TaskStatusCompleted
		updated, err := s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status, CompletedAt: completedAt})
		if err != nil {
			logRepoErr(ctx, op+": update status", err, slog.Int64("task_id", taskID))
			return nil, err
		}
		t = updated
	}
	if err := s.bumpTroikiCapacity(ctx, t); err != nil {
		log.ErrorContext(ctx, op+": bump troiki capacity", slog.Int64("task_id", taskID), slog.String("err", err.Error()))
		return nil, err
	}
	log.InfoContext(ctx, "task completed", slog.String("op", op), slog.Int64("task_id", taskID))
	return t, nil
}

// completeFederated is the federation-wired counterpart of the non-recurring
// complete tail: it runs the status=completed write AND the troiki capacity grant
// in ONE tx via the federator, emitting op=update{status:completed,completed_at}
// when the project is federated (TASK A). The capacity-grant retry semantics of
// the plain path are preserved: the grant is always attempted (idempotent) so a
// crash between the status write and the grant can recover on retry.
func (s *CompleteService) completeFederated(ctx context.Context, t *model.Task, completedAt *time.Time) (*model.Task, error) {
	const op = "service.CompleteService.Complete"
	log := logging.FromContext(ctx)

	col, err := s.troikiTargetCol(ctx, t)
	if err != nil {
		return nil, err
	}
	completedStatus := model.TaskStatusCompleted
	fields := map[string]any{
		"status":       string(completedStatus),
		"completed_at": completedTS(completedAt),
	}
	if err := s.fed.StatusChange(ctx, t, fields, func(tx *sql.Tx) error {
		if t.Status != model.TaskStatusCompleted {
			if err := s.tasks.UpdateTx(ctx, tx, t.ID, repo.TaskUpdate{Status: &completedStatus, CompletedAt: completedAt}); err != nil {
				return err
			}
		}
		if col != "" {
			if _, err := s.tasks.GrantAndBumpTroikiCapacityTx(ctx, tx, t.ID, SingleUserID, col); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		logRepoErr(ctx, op+": federated complete", err, slog.Int64("task_id", t.ID))
		return nil, err
	}
	log.InfoContext(ctx, "task completed", slog.String("op", op), slog.Int64("task_id", t.ID))
	return s.tasks.Get(ctx, t.ID)
}

// completedTS renders the completion timestamp the federated event carries,
// mirroring repo.TaskRepo.UpdateTx (server-now when no explicit time is given).
func completedTS(completedAt *time.Time) string {
	if completedAt != nil {
		return model.FormatUTC(*completedAt)
	}
	return model.FormatUTC(time.Now())
}

// troikiTargetCol resolves the user capacity column a completion of t grants into
// (important → +medium, medium → +rest), or "" when the project has no category /
// the task is outside a project. It mirrors bumpTroikiCapacity's tier mapping but
// returns the column so the grant can run inside the federated emit tx.
func (s *CompleteService) troikiTargetCol(ctx context.Context, t *model.Task) (string, error) {
	if t == nil || t.ProjectID == nil || s.projects == nil || s.users == nil {
		return "", nil
	}
	p, err := s.projects.Get(ctx, *t.ProjectID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	if p.TroikiCategory == nil {
		return "", nil
	}
	switch *p.TroikiCategory {
	case model.TroikiCategoryImportant:
		return "troiki_medium_capacity", nil
	case model.TroikiCategoryMedium:
		return "troiki_rest_capacity", nil
	default:
		return "", nil
	}
}

func (s *CompleteService) advanceRecurring(ctx context.Context, t *model.Task, completedAt *time.Time) (*model.Task, error) {
	const op = "service.CompleteService.advanceRecurring"
	log := logging.FromContext(ctx)

	// Idempotency: if we already snapped a completion for this recurring task
	// today (in the configured location), a second Complete call is a no-op —
	// otherwise rapid double-clicks, retries, or any duplicate request would
	// keep advancing the parent and snapping new rows, surfacing as duplicate
	// entries on the "completed yesterday" view.
	completionTS := time.Now()
	if completedAt != nil {
		completionTS = *completedAt
	}
	dayStart, dayEnd := dayBounds(completionTS, s.loc)
	if has, err := s.tasks.HasRecurrenceCompletionOnDay(ctx, t.ID, dayStart, dayEnd); err != nil {
		logRepoErr(ctx, op+": check recurrence completion", err, slog.Int64("task_id", t.ID))
		return nil, err
	} else if has {
		log.DebugContext(ctx, op+": skip duplicate", slog.Int64("task_id", t.ID))
		return t, nil
	}

	r, err := rrule.StrToRRule(*t.RecurrenceRule)
	if err != nil {
		log.WarnContext(ctx, op+": invalid RRULE", slog.Int64("task_id", t.ID), slog.String("err", err.Error()))
		return nil, &RecurrenceError{Err: err}
	}

	// Base: current due_at if in the future, otherwise now. Anchor to the
	// configured location so RRULE BYHOUR/BYDAY semantics follow the user's clock.
	base := time.Now().In(s.loc)
	if t.DueAt != nil && t.DueAt.After(base) {
		base = t.DueAt.In(s.loc)
	}
	r.DTStart(base)

	next := r.After(base, false)

	planNone := model.PlanStateNone
	upd := repo.TaskUpdate{PlanState: &planNone}
	terminal := next.IsZero()
	if terminal {
		status := model.TaskStatusCompleted
		upd.Status = &status
		upd.CompletedAt = completedAt
	} else {
		upd.DueAt = &next
		upd.ResetPostponeCount = true
	}

	if s.fed != nil {
		return s.advanceRecurringFederated(ctx, t, upd, terminal, completedAt, completionTS)
	}

	updated, err := s.tasks.Update(ctx, t.ID, upd)
	if err != nil {
		logRepoErr(ctx, op+": update task", err, slog.Int64("task_id", t.ID))
		return nil, err
	}
	// For non-terminal completions the parent task stays open (advanced to the
	// next occurrence), so the completed view would never show this run. Snap
	// off a completed history row so the user can see what they got done.
	if !terminal {
		if _, err := s.tasks.CreateRecurrenceCompletion(ctx, t, completionTS); err != nil {
			log.ErrorContext(ctx, op+": create recurrence completion", slog.Int64("task_id", t.ID), slog.String("err", err.Error()))
			return nil, err
		}
	}
	if terminal {
		if err := s.bumpTroikiCapacity(ctx, t); err != nil {
			log.ErrorContext(ctx, op+": bump troiki capacity", slog.Int64("task_id", t.ID), slog.String("err", err.Error()))
			return nil, err
		}
	}
	log.InfoContext(ctx, "recurring task advanced", slog.String("op", op), slog.Int64("task_id", t.ID), slog.Bool("terminal", terminal))
	return updated, nil
}

// advanceRecurringFederated is the federation-wired counterpart of the recurring
// advance tail (TASK A). Two shapes:
//   - TERMINAL: the parent transitions to completed in place → op=update
//     {status:completed,completed_at}, plus the troiki capacity grant, all in one tx.
//   - NON-TERMINAL: the parent advances IN PLACE (due_at moves, postpone reset,
//     status stays open) AND a NEW completed snapshot row is created with its OWN
//     client_id → op=update{due_at,...} on the parent + op=create{full federated set}
//     for the snapshot, emitted TOGETHER in one tx via RecurrenceAdvance.
func (s *CompleteService) advanceRecurringFederated(ctx context.Context, t *model.Task, upd repo.TaskUpdate, terminal bool, completedAt *time.Time, completionTS time.Time) (*model.Task, error) {
	const op = "service.CompleteService.advanceRecurring"
	log := logging.FromContext(ctx)

	if terminal {
		col, err := s.troikiTargetCol(ctx, t)
		if err != nil {
			return nil, err
		}
		fields := map[string]any{
			"status":       string(model.TaskStatusCompleted),
			"completed_at": completedTS(completedAt),
		}
		if err := s.fed.StatusChange(ctx, t, fields, func(tx *sql.Tx) error {
			if err := s.tasks.UpdateTx(ctx, tx, t.ID, upd); err != nil {
				return err
			}
			if col != "" {
				if _, err := s.tasks.GrantAndBumpTroikiCapacityTx(ctx, tx, t.ID, SingleUserID, col); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			logRepoErr(ctx, op+": federated terminal advance", err, slog.Int64("task_id", t.ID))
			return nil, err
		}
		log.InfoContext(ctx, "recurring task advanced", slog.String("op", op), slog.Int64("task_id", t.ID), slog.Bool("terminal", true))
		return s.tasks.Get(ctx, t.ID)
	}

	// Non-terminal: parent advances in place + a new occurrence snapshot is created.
	advanceFields := map[string]any{"due_at": model.FormatUTC(*upd.DueAt)}
	snapClientID := model.NewClientID()
	snapFields := recurrenceSnapshotFields(t, completionTS)
	if err := s.fed.RecurrenceAdvance(ctx, t, advanceFields, snapClientID, snapFields, func(tx *sql.Tx) error {
		if err := s.tasks.UpdateTx(ctx, tx, t.ID, upd); err != nil {
			return err
		}
		if _, err := s.tasks.CreateRecurrenceCompletionTx(ctx, tx, t, completionTS, snapClientID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		logRepoErr(ctx, op+": federated advance", err, slog.Int64("task_id", t.ID))
		return nil, err
	}
	log.InfoContext(ctx, "recurring task advanced", slog.String("op", op), slog.Int64("task_id", t.ID), slog.Bool("terminal", false))
	return s.tasks.Get(ctx, t.ID)
}

// recurrenceSnapshotFields builds the FULL federated field set the new occurrence
// snapshot carries in its op=create event, mirroring repo.CreateRecurrenceCompletion's
// inserted row (status=completed, no due/deadline, completed_at=completionTS). It
// stays in lock-step with the inbox task field set; local-only columns are excluded.
func recurrenceSnapshotFields(base *model.Task, completionTS time.Time) map[string]any {
	priority := base.Priority
	if priority == "" {
		priority = model.PriorityNone
	}
	return map[string]any{
		"title":             base.Title,
		"description":       base.Description,
		"priority":          string(priority),
		"status":            string(model.TaskStatusCompleted),
		"due_at":            nil,
		"due_has_time":      false,
		"deadline_at":       nil,
		"deadline_has_time": false,
		"completed_at":      model.FormatUTC(completionTS),
	}
}

// bumpTroikiCapacity grants +1 capacity to the next-tier slot when a task is
// completed inside a categorised project: important → +medium, medium → +rest.
// Rest and uncategorised projects (or tasks outside any project) have no
// effect. The grant flag on the task makes the operation idempotent across
// uncomplete/recomplete cycles — capacity is granted only once per
// (task, project-category-assignment) until the project's category is
// cleared or changed.
func (s *CompleteService) bumpTroikiCapacity(ctx context.Context, t *model.Task) error {
	if t == nil || t.ProjectID == nil || s.projects == nil || s.users == nil {
		return nil
	}
	p, err := s.projects.Get(ctx, *t.ProjectID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil
		}
		return err
	}
	if p.TroikiCategory == nil {
		return nil
	}
	var col string
	switch *p.TroikiCategory {
	case model.TroikiCategoryImportant:
		col = "troiki_medium_capacity"
	case model.TroikiCategoryMedium:
		col = "troiki_rest_capacity"
	default:
		return nil
	}
	// Single transaction: flag flip + counter bump must succeed or both roll back.
	// Otherwise a failure between them strands the grant — the flag blocks retries.
	_, err = s.tasks.GrantAndBumpTroikiCapacity(ctx, t.ID, SingleUserID, col)
	return err
}

// Uncomplete reopens a completed/cancelled task. Project-level Troiki
// categorisation means reopening a task does not affect any slot capacity, so
// no slot guard is needed here. If the parent project carries a category, the
// task's priority is re-pinned to the category-derived priority — without this,
// a task completed before the category was assigned (or moved into a
// categorised project while completed) would come back open with a stale
// priority that the frontend then locks against edits.
//
// The status transition and priority pin are performed in a single SQL
// statement that reads projects.troiki_category atomically with the UPDATE,
// eliminating a race with a concurrent SetCategory that would otherwise let
// the task come back open with a priority derived from the project's previous
// category.
func (s *CompleteService) Uncomplete(ctx context.Context, taskID int64) (*model.Task, error) {
	const op = "service.CompleteService.Uncomplete"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID))
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		logRepoErr(ctx, op+": get task", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	if t.Status == model.TaskStatusOpen {
		return t, nil
	}
	if s.fed != nil {
		return s.uncompleteFederated(ctx, t)
	}
	reopened, err := s.tasks.ReopenAndPinProjectPriority(ctx, taskID)
	if err != nil {
		logRepoErr(ctx, op+": reopen", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	log.InfoContext(ctx, "task reopened", slog.String("op", op), slog.Int64("task_id", taskID))
	return reopened, nil
}

// uncompleteFederated is the federation-wired counterpart of Uncomplete's reopen
// (TASK A). It emits op=update{status:open,completed_at:null} and, when the parent
// project carries a Troiki category (so the reopen re-pins priority), the new
// category-derived priority too — keeping emit↔local-row in agreement.
func (s *CompleteService) uncompleteFederated(ctx context.Context, t *model.Task) (*model.Task, error) {
	const op = "service.CompleteService.Uncomplete"
	log := logging.FromContext(ctx)

	fields := map[string]any{
		"status":       string(model.TaskStatusOpen),
		"completed_at": nil,
	}
	if pinned, err := s.reopenPinnedPriority(ctx, t); err != nil {
		return nil, err
	} else if pinned != "" {
		fields["priority"] = string(pinned)
	}
	if err := s.fed.StatusChange(ctx, t, fields, func(tx *sql.Tx) error {
		return s.tasks.ReopenAndPinProjectPriorityTx(ctx, tx, t.ID)
	}); err != nil {
		logRepoErr(ctx, op+": federated reopen", err, slog.Int64("task_id", t.ID))
		return nil, err
	}
	log.InfoContext(ctx, "task reopened", slog.String("op", op), slog.Int64("task_id", t.ID))
	return s.tasks.Get(ctx, t.ID)
}

// reopenPinnedPriority returns the category-derived priority a reopen of t pins it
// to (matching ReopenAndPinProjectPriority's CASE), or "" when the project has no
// category — in which case the reopen preserves the existing priority and no
// priority field is emitted.
func (s *CompleteService) reopenPinnedPriority(ctx context.Context, t *model.Task) (model.Priority, error) {
	if t == nil || t.ProjectID == nil || s.projects == nil {
		return "", nil
	}
	p, err := s.projects.Get(ctx, *t.ProjectID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	if p.TroikiCategory == nil {
		return "", nil
	}
	return PriorityForCategory(*p.TroikiCategory), nil
}

// Cancel marks a task cancelled. With project-owned Troiki categories, cancelling
// a single task does not release any slot — the project keeps its category until
// the user explicitly clears it.
func (s *CompleteService) Cancel(ctx context.Context, taskID int64) (*model.Task, error) {
	const op = "service.CompleteService.Cancel"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID))
	status := model.TaskStatusCancelled
	if s.fed != nil {
		t, err := s.tasks.Get(ctx, taskID)
		if err != nil {
			logRepoErr(ctx, op+": get task", err, slog.Int64("task_id", taskID))
			return nil, err
		}
		fields := map[string]any{
			"status":       string(status),
			"completed_at": nil,
		}
		if err := s.fed.StatusChange(ctx, t, fields, func(tx *sql.Tx) error {
			return s.tasks.UpdateTx(ctx, tx, taskID, repo.TaskUpdate{Status: &status})
		}); err != nil {
			logRepoErr(ctx, op+": federated cancel", err, slog.Int64("task_id", taskID))
			return nil, err
		}
		log.InfoContext(ctx, "task cancelled", slog.String("op", op), slog.Int64("task_id", taskID))
		return s.tasks.Get(ctx, taskID)
	}
	updated, err := s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
	if err != nil {
		logRepoErr(ctx, op+": update", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	log.InfoContext(ctx, "task cancelled", slog.String("op", op), slog.Int64("task_id", taskID))
	return updated, nil
}
