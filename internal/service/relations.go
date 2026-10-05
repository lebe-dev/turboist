package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

var (
	// ErrRelationSelf is returned when both ends of a relation are the same task.
	ErrRelationSelf = errors.New("service: cannot relate a task to itself")
	// ErrRelationCycle is returned when a `blocks` relation would close a loop in
	// the blocking graph, leaving every task in it permanently uncompletable.
	ErrRelationCycle = errors.New("service: relation would create a blocking cycle")
)

// TaskBlockedError reports a refused completion and names the open blockers so the
// UI can link to them. A typed error (like RecurrenceError) rather than a sentinel
// because the ids are part of the answer; handlers match it with errors.As.
type TaskBlockedError struct{ BlockerIDs []int64 }

func (e *TaskBlockedError) Error() string {
	ids := make([]string, len(e.BlockerIDs))
	for i, id := range e.BlockerIDs {
		ids[i] = strconv.FormatInt(id, 10)
	}
	return "task_blocked: blocked by " + strings.Join(ids, ", ")
}

// RelationService owns the invariants of the task relation graph: no self-links, no
// duplicate pairs, no blocking cycles, and a canonical ordering for the symmetric
// `related` type.
type RelationService struct {
	tasks     *repo.TaskRepo
	relations *repo.TaskRelationsRepo
}

func NewRelationService(tasks *repo.TaskRepo, relations *repo.TaskRelationsRepo) *RelationService {
	return &RelationService{tasks: tasks, relations: relations}
}

// Add links taskID to otherID and returns taskID refetched with its relations
// hydrated, so the caller has nothing left to fetch.
//
// `direction` is interpreted relative to taskID and only matters for `blocks`:
// outgoing means taskID blocks otherID, incoming means otherID blocks taskID.
// `related` is symmetric, so the pair is normalised to (lower id, higher id) —
// that is what lets the UNIQUE constraint reject A↔B added from either side.
func (s *RelationService) Add(
	ctx context.Context,
	taskID, otherID int64,
	relType model.RelationType,
	direction model.RelationDirection,
) (*model.Task, error) {
	const op = "service.RelationService.Add"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op,
		slog.Int64("task_id", taskID),
		slog.Int64("other_task_id", otherID),
		slog.String("type", string(relType)),
		slog.String("direction", string(direction)))

	if taskID == otherID {
		log.WarnContext(ctx, op+": self relation", slog.Int64("task_id", taskID))
		return nil, ErrRelationSelf
	}
	// Existence of both ends is checked up front: the FKs would catch it, but as an
	// opaque constraint error the handler could not turn into a clean 404.
	if _, err := s.tasks.Get(ctx, taskID); err != nil {
		logRepoErr(ctx, op+": get task", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	if _, err := s.tasks.Get(ctx, otherID); err != nil {
		logRepoErr(ctx, op+": get other task", err, slog.Int64("other_task_id", otherID))
		return nil, err
	}

	sourceID, targetID := s.orient(taskID, otherID, relType, direction)

	if relType == model.RelationTypeBlocks {
		cycle, err := s.relations.WouldCycle(ctx, sourceID, targetID)
		if err != nil {
			logRepoErr(ctx, op+": check cycle", err, slog.Int64("task_id", taskID))
			return nil, err
		}
		if cycle {
			log.WarnContext(ctx, op+": blocking cycle",
				slog.Int64("source_task_id", sourceID), slog.Int64("target_task_id", targetID))
			return nil, ErrRelationCycle
		}
	}

	if _, err := s.relations.Create(ctx, sourceID, targetID, relType); err != nil {
		logRepoErr(ctx, op+": create relation", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	log.InfoContext(ctx, "task relation created",
		slog.String("op", op),
		slog.Int64("source_task_id", sourceID),
		slog.Int64("target_task_id", targetID),
		slog.String("type", string(relType)))
	return s.tasks.GetWithRelations(ctx, taskID)
}

// BlockerRefusal names why a candidate cannot be made a blocker of a task. The
// values are the wire spelling of the blocker-check endpoint.
type BlockerRefusal string

const (
	BlockerRefusalSelf     BlockerRefusal = "relation_self"
	BlockerRefusalNotFound BlockerRefusal = "not_found"
	BlockerRefusalExists   BlockerRefusal = "relation_exists"
	BlockerRefusalCycle    BlockerRefusal = "relation_cycle"
)

// MaxBlockerCandidates caps one blocker check. The check answers a drag gesture
// over a task's subtasks, so a screenful is all it is ever asked about.
const MaxBlockerCandidates = 500

// CheckBlockers answers, for each candidate, whether Add(taskID, candidate,
// blocks, incoming) would be refused — without writing anything. Only refused
// candidates appear in the result. It exists so a drag-and-drop gesture can show
// the refusal while the pointer is still over the target instead of after the
// drop; Add stays the authority and re-checks everything on write.
func (s *RelationService) CheckBlockers(
	ctx context.Context,
	taskID int64,
	candidateIDs []int64,
) (map[int64]BlockerRefusal, error) {
	const op = "service.RelationService.CheckBlockers"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID), slog.Int("candidates", len(candidateIDs)))

	if _, err := s.tasks.Get(ctx, taskID); err != nil {
		logRepoErr(ctx, op+": get task", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	existing, err := s.tasks.ExistingIDs(ctx, candidateIDs)
	if err != nil {
		logRepoErr(ctx, op+": existing candidates", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	blockers, err := s.relations.DirectBlockerIDs(ctx, taskID)
	if err != nil {
		logRepoErr(ctx, op+": direct blockers", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	// "candidate blocks taskID" closes a loop exactly when taskID already holds the
	// candidate back, directly or through a chain — the same test WouldCycle runs
	// for one pair, done once for the whole batch.
	downstream, err := s.relations.BlockingReachableFrom(ctx, taskID)
	if err != nil {
		logRepoErr(ctx, op+": walk blocking graph", err, slog.Int64("task_id", taskID))
		return nil, err
	}

	refused := make(map[int64]BlockerRefusal)
	for _, id := range candidateIDs {
		switch {
		case id == taskID:
			refused[id] = BlockerRefusalSelf
		case !containsID(existing, id):
			refused[id] = BlockerRefusalNotFound
		case containsID(blockers, id):
			refused[id] = BlockerRefusalExists
		case containsID(downstream, id):
			refused[id] = BlockerRefusalCycle
		}
	}
	return refused, nil
}

func containsID(set map[int64]struct{}, id int64) bool {
	_, ok := set[id]
	return ok
}

// orient maps a (task, peer, type, direction) request onto the directed row to
// store. `related` ignores direction and sorts the pair; `blocks` honours it.
func (s *RelationService) orient(
	taskID, otherID int64,
	relType model.RelationType,
	direction model.RelationDirection,
) (sourceID, targetID int64) {
	if relType == model.RelationTypeRelated {
		if taskID < otherID {
			return taskID, otherID
		}
		return otherID, taskID
	}
	if direction == model.RelationDirectionIncoming {
		return otherID, taskID
	}
	return taskID, otherID
}

// Remove deletes a relation from taskID's side and returns taskID refetched with
// its remaining relations hydrated.
func (s *RelationService) Remove(ctx context.Context, taskID, relationID int64) (*model.Task, error) {
	const op = "service.RelationService.Remove"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID), slog.Int64("relation_id", relationID))
	if err := s.relations.Delete(ctx, relationID, taskID); err != nil {
		logRepoErr(ctx, op+": delete relation", err,
			slog.Int64("task_id", taskID), slog.Int64("relation_id", relationID))
		return nil, err
	}
	log.InfoContext(ctx, "task relation removed",
		slog.String("op", op), slog.Int64("task_id", taskID), slog.Int64("relation_id", relationID))
	return s.tasks.GetWithRelations(ctx, taskID)
}
