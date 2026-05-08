package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrInvalidGroupRequest signals a malformed grouping request (empty/duplicate
// childIds, exceeds the cap, etc.). Handlers map this to a 422 validation
// error; the wrapped message carries the user-facing reason.
var ErrInvalidGroupRequest = errors.New("invalid group request")

func newInvalidGroupErr(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalidGroupRequest, msg)
}

// GroupService creates a new parent task and adopts existing tasks under it.
// Adopted children inherit the new parent's labels and priority (overwrite,
// not merge). Operations are best-effort: a child failure does not roll back
// the new parent or already-moved siblings; the caller receives the per-child
// outcome and can react accordingly.
type GroupService struct {
	tasks      *TaskService
	moves      *MoveService
	taskRepo   *repo.TaskRepo
	taskLabels *repo.TaskLabelsRepo
}

func NewGroupService(tasks *TaskService, moves *MoveService, taskRepo *repo.TaskRepo, taskLabels *repo.TaskLabelsRepo) *GroupService {
	return &GroupService{tasks: tasks, moves: moves, taskRepo: taskRepo, taskLabels: taskLabels}
}

// GroupInput carries the parent's creation fields and the children to adopt.
type GroupInput struct {
	Parent            repo.CreateTask
	ExplicitLabels    []string
	RemovedAutoLabels []string
	ChildIDs          []int64
}

// GroupFailure pairs a child id with the error that prevented its adoption.
type GroupFailure struct {
	ID  int64
	Err error
}

// GroupResult is returned by Group: the new parent plus per-child outcomes.
type GroupResult struct {
	Parent       *model.Task
	SucceededIDs []int64
	Failed       []GroupFailure
}

const maxGroupChildren = 100

// Group validates the request, creates the parent, then re-parents each child
// and overwrites its labels and priority to match the parent.
func (s *GroupService) Group(ctx context.Context, in GroupInput) (*GroupResult, error) {
	if len(in.ChildIDs) == 0 {
		return nil, newInvalidGroupErr("childIds must not be empty")
	}
	if len(in.ChildIDs) > maxGroupChildren {
		return nil, newInvalidGroupErr("too many childIds")
	}
	seen := make(map[int64]struct{}, len(in.ChildIDs))
	for _, id := range in.ChildIDs {
		if id <= 0 {
			return nil, newInvalidGroupErr("invalid childId")
		}
		if _, dup := seen[id]; dup {
			return nil, newInvalidGroupErr("duplicate childId")
		}
		seen[id] = struct{}{}
	}
	if in.Parent.InboxID != nil || (in.Parent.ContextID == nil && in.Parent.ProjectID == nil) {
		return nil, repo.ErrInvalidPlacement
	}

	parent, err := s.tasks.Create(ctx, in.Parent, in.ExplicitLabels, in.RemovedAutoLabels)
	if err != nil {
		return nil, err
	}

	target := repo.Placement{
		ContextID: parent.ContextID,
		ProjectID: parent.ProjectID,
		SectionID: parent.SectionID,
		ParentID:  &parent.ID,
	}

	parentLabelIDs := make([]int64, len(parent.Labels))
	for i, l := range parent.Labels {
		parentLabelIDs[i] = l.ID
	}
	parentPriority := parent.Priority

	res := &GroupResult{Parent: parent, SucceededIDs: make([]int64, 0, len(in.ChildIDs))}
	for _, id := range in.ChildIDs {
		if id == parent.ID {
			res.Failed = append(res.Failed, GroupFailure{ID: id, Err: fmt.Errorf("cannot adopt the new parent")})
			continue
		}
		if _, err := s.moves.Move(ctx, id, target); err != nil {
			res.Failed = append(res.Failed, GroupFailure{ID: id, Err: err})
			continue
		}
		if err := s.taskLabels.SetForTask(ctx, id, parentLabelIDs); err != nil {
			res.Failed = append(res.Failed, GroupFailure{ID: id, Err: err})
			continue
		}
		if _, err := s.taskRepo.Update(ctx, id, repo.TaskUpdate{Priority: &parentPriority}); err != nil {
			res.Failed = append(res.Failed, GroupFailure{ID: id, Err: err})
			continue
		}
		res.SucceededIDs = append(res.SucceededIDs, id)
	}

	return res, nil
}
