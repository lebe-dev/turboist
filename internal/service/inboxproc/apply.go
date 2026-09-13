package inboxproc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// maxStoredError bounds the error text kept in the state row and the journal.
const maxStoredError = 1000

// apply files a task according to a "sort" verdict. There is no transaction
// across the repositories (as with the HTTP handlers): a failure after the move
// leaves the task in its project without the marker and is recorded as failed.
func (p *Processor) apply(ctx context.Context, now time.Time, catalogue *Catalogue, item PendingItem, modelName string,
	completion Completion, v Verdict) (model.InboxOutcome, error) {
	const op = "inboxproc.Processor.apply"
	fingerprint := model.InboxFingerprint(item.Task.Title, item.Task.Description)

	// The model may have taken seconds; the user may have filed or edited the
	// task meanwhile. Their change wins and nothing is written.
	current, err := p.d.Tasks.Get(ctx, item.Task.ID)
	if errors.Is(err, repo.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return p.fail(ctx, now, item, modelName, completion, nil, err)
	}
	if current.InboxID == nil || current.Status != model.TaskStatusOpen ||
		model.InboxFingerprint(current.Title, current.Description) != fingerprint {
		p.log.InfoContext(ctx, "inbox processing skipped a task changed during the request", slog.String("op", op),
			slog.Int64("task_id", current.ID), slog.String("task_title", current.Title), slog.String("model", modelName))
		return "", nil
	}

	before := model.InboxTaskBefore{
		LabelIDs:   labelIDs(current.Labels),
		Priority:   current.Priority,
		DueAt:      current.DueAt,
		DueHasTime: current.DueHasTime,
	}

	contextID, projectID := v.Project.ContextID, v.Project.ID
	if _, err := p.d.Move.Move(ctx, current.ID, repo.Placement{ContextID: &contextID, ProjectID: &projectID}); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return "", nil
		}
		return p.fail(ctx, now, item, modelName, completion, nil, fmt.Errorf("move task: %w", err))
	}

	if added := missingIDs(before.LabelIDs, v.LabelIDs); len(added) > 0 {
		if err := p.d.TaskLabels.SetForTask(ctx, current.ID, append(append([]int64{}, before.LabelIDs...), added...)); err != nil {
			return p.fail(ctx, now, item, modelName, completion, nil, fmt.Errorf("set labels: %w", err))
		}
	}

	// A priority or due date the user set at capture time is their decision; the
	// model only fills gaps. The daily-plan bucket's priority was already applied
	// by the move.
	update := repo.TaskUpdate{AutoSortedAt: &now}
	if v.Priority != nil && current.Priority == model.PriorityNone {
		update.Priority = v.Priority
	}
	if v.DueAt != nil && current.DueAt == nil {
		hasTime := false
		update.DueAt = v.DueAt
		update.DueHasTime = &hasTime
	}
	filed, err := p.d.Tasks.Update(ctx, current.ID, update)
	if err != nil {
		return p.fail(ctx, now, item, modelName, completion, nil, fmt.Errorf("update task: %w", err))
	}

	confidence := v.Confidence
	entry := p.logEntry(now, current, model.InboxOutcomeSorted, modelName, completion)
	entry.Reason = v.Reason
	entry.Confidence = &confidence
	entry.Before = before
	entry.After = &model.InboxTaskAfter{
		ContextID: contextID,
		ProjectID: projectID,
		LabelIDs:  labelIDs(filed.Labels),
		Priority:  filed.Priority,
		DueAt:     filed.DueAt,
	}
	journalID, err := p.d.State.AppendLog(ctx, entry)
	if err != nil {
		p.log.ErrorContext(ctx, "inbox processing could not journal a sorted task", slog.String("op", op),
			slog.Int64("task_id", current.ID), slog.String("err", err.Error()))
	}
	if err := p.d.State.DeleteState(ctx, current.ID); err != nil {
		p.log.ErrorContext(ctx, "inbox processing could not clear task state", slog.String("op", op),
			slog.Int64("task_id", current.ID), slog.String("err", err.Error()))
	}
	// One line holds the whole move, so the log alone tells where a task went and
	// why; journal_id ties it to the row the settings page reverts.
	p.log.InfoContext(ctx, "inbox processing filed a task", slog.String("op", op),
		slog.Int64("journal_id", journalID),
		slog.Int64("task_id", current.ID),
		slog.String("task_title", current.Title),
		slog.String("from", "inbox"),
		slog.Int64("context_id", contextID),
		slog.String("context", catalogue.ContextName(contextID)),
		slog.Int64("project_id", projectID),
		slog.String("project", v.Project.Title),
		slog.Any("labels_added", labelNamesOf(filed.Labels, missingIDs(before.LabelIDs, labelIDs(filed.Labels)))),
		slog.Any("labels", labelNames(filed.Labels)),
		slog.String("priority", string(filed.Priority)),
		slog.String("due_date", formatDue(filed.DueAt, filed.DueHasTime, p.d.Location)),
		slog.Float64("confidence", v.Confidence),
		slog.String("reason", v.Reason),
		slog.String("model", modelName))
	return model.InboxOutcomeSorted, nil
}

// keep leaves the task in the Inbox and remembers the wording it was kept for.
func (p *Processor) keep(ctx context.Context, now time.Time, item PendingItem, modelName string,
	completion Completion, v Verdict) (model.InboxOutcome, error) {
	const op = "inboxproc.Processor.keep"
	task := item.Task
	if err := p.d.State.UpsertState(ctx, model.InboxProcessingState{
		TaskID:      task.ID,
		Fingerprint: model.InboxFingerprint(task.Title, task.Description),
		Status:      model.InboxStateKept,
		UpdatedAt:   now,
	}); err != nil {
		if isGone(err) {
			return "", nil
		}
		return "", fmt.Errorf("store kept state: %w", err)
	}
	confidence := v.Confidence
	entry := p.logEntry(now, &task, model.InboxOutcomeKept, modelName, completion)
	entry.Reason = v.Reason
	entry.Confidence = &confidence
	entry.Before = beforeOf(&task)
	journalID, err := p.d.State.AppendLog(ctx, entry)
	if err != nil {
		p.log.ErrorContext(ctx, "inbox processing could not journal a kept task", slog.String("op", op),
			slog.Int64("task_id", task.ID), slog.String("err", err.Error()))
	}
	p.log.InfoContext(ctx, "inbox processing kept a task in the inbox", slog.String("op", op),
		slog.Int64("journal_id", journalID),
		slog.Int64("task_id", task.ID),
		slog.String("task_title", task.Title),
		slog.Float64("confidence", v.Confidence),
		slog.String("reason", v.Reason),
		slog.String("model", modelName))
	return model.InboxOutcomeKept, nil
}

// fail records a decision that could not be made or applied and schedules the
// retry: Interval·2^(attempts-1), and no retry at all after maxTaskAttempts
// until the task is edited.
func (p *Processor) fail(ctx context.Context, now time.Time, item PendingItem, modelName string,
	completion Completion, decision *Decision, cause error) (model.InboxOutcome, error) {
	const op = "inboxproc.Processor.fail"
	task := item.Task
	fingerprint := model.InboxFingerprint(task.Title, task.Description)
	attempts := 1
	if item.State != nil && item.State.Status == model.InboxStateFailed && item.State.Fingerprint == fingerprint {
		attempts = item.State.Attempts + 1
	}
	var next *time.Time
	if attempts < maxTaskAttempts {
		at := now.Add(p.cfg.Interval << (attempts - 1))
		next = &at
	}
	msg := truncate(cause.Error(), maxStoredError)
	if err := p.d.State.UpsertState(ctx, model.InboxProcessingState{
		TaskID:        task.ID,
		Fingerprint:   fingerprint,
		Status:        model.InboxStateFailed,
		Attempts:      attempts,
		NextAttemptAt: next,
		LastError:     &msg,
		UpdatedAt:     now,
	}); err != nil {
		if isGone(err) {
			return "", nil
		}
		return "", fmt.Errorf("store failed state: %w", err)
	}
	entry := p.logEntry(now, &task, model.InboxOutcomeFailed, modelName, completion)
	entry.Before = beforeOf(&task)
	entry.Error = &msg
	if decision != nil {
		entry.Reason = decision.Reason
		confidence := decision.Confidence
		entry.Confidence = &confidence
	}
	journalID, err := p.d.State.AppendLog(ctx, entry)
	if err != nil {
		p.log.ErrorContext(ctx, "inbox processing could not journal a failed task", slog.String("op", op),
			slog.Int64("task_id", task.ID), slog.String("err", err.Error()))
	}
	nextAttempt := ""
	if next != nil {
		nextAttempt = next.In(p.d.Location).Format(time.RFC3339)
	}
	p.log.WarnContext(ctx, "inbox processing could not decide on a task", slog.String("op", op),
		slog.Int64("journal_id", journalID),
		slog.Int64("task_id", task.ID),
		slog.String("task_title", task.Title),
		slog.Int("attempts", attempts),
		slog.String("next_attempt_at", nextAttempt),
		slog.String("model", modelName),
		slog.String("err", msg))
	return model.InboxOutcomeFailed, nil
}

func (p *Processor) logEntry(now time.Time, task *model.Task, outcome model.InboxOutcome, modelName string,
	completion Completion) model.InboxProcessingLogEntry {
	id := task.ID
	entry := model.InboxProcessingLogEntry{
		TaskID:    &id,
		TaskTitle: task.Title,
		Outcome:   outcome,
		Model:     modelName,
		CreatedAt: now,
	}
	if completion.PromptTokens > 0 || completion.CompletionTokens > 0 {
		pt, ct := completion.PromptTokens, completion.CompletionTokens
		entry.PromptTokens = &pt
		entry.CompletionTokens = &ct
	}
	return entry
}

func beforeOf(task *model.Task) model.InboxTaskBefore {
	return model.InboxTaskBefore{
		LabelIDs:   labelIDs(task.Labels),
		Priority:   task.Priority,
		DueAt:      task.DueAt,
		DueHasTime: task.DueHasTime,
	}
}

// isGone reports a write that failed because the task was deleted meanwhile
// (the state row's foreign key no longer resolves).
func isGone(err error) bool {
	return errors.Is(err, repo.ErrNotFound) || repo.IsForeignKeyViolation(err)
}

func labelIDs(labels []model.Label) []int64 {
	out := make([]int64, 0, len(labels))
	for _, l := range labels {
		out = append(out, l.ID)
	}
	return out
}

// missingIDs returns the ids of want that are not in have, in want's order.
func missingIDs(have, want []int64) []int64 {
	set := make(map[int64]struct{}, len(have))
	for _, id := range have {
		set[id] = struct{}{}
	}
	var out []int64
	for _, id := range want {
		if _, ok := set[id]; ok {
			continue
		}
		set[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// labelNamesOf names the ids among labels, in the order of ids.
func labelNamesOf(labels []model.Label, ids []int64) []string {
	byID := make(map[int64]string, len(labels))
	for _, l := range labels {
		byID[l.ID] = l.Name
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := byID[id]; ok {
			out = append(out, name)
		}
	}
	return out
}

// formatDue renders a due date for a log line: a bare date for a whole-day
// due, RFC 3339 in the server timezone when it has a time, "" when there is none.
func formatDue(due *time.Time, hasTime bool, loc *time.Location) string {
	if due == nil {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	if hasTime {
		return due.In(loc).Format(time.RFC3339)
	}
	return due.In(loc).Format("2006-01-02")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
