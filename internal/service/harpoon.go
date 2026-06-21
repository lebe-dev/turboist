package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// maxHarpoon is the size of the jump pair. The user can hop between exactly two
// references; harpooning a third evicts the oldest (FIFO).
const maxHarpoon = 2

// HarpoonSlot is a hydrated harpoon reference: the persisted ref plus the
// entity's current title, resolved on read so it never goes stale.
type HarpoonSlot struct {
	Kind  model.HarpoonKind
	ID    int64
	Title string
}

// HarpoonService manages the user's "jump pair" — an ordered set of at most two
// task/project references persisted in UserSettings.Harpoon. Reads hydrate
// titles from the task/project repos and self-heal dangling references left
// behind by deleted entities.
type HarpoonService struct {
	users    *repo.UserRepo
	tasks    *repo.TaskRepo
	projects *repo.ProjectRepo
}

func NewHarpoonService(users *repo.UserRepo, tasks *repo.TaskRepo, projects *repo.ProjectRepo) *HarpoonService {
	return &HarpoonService{users: users, tasks: tasks, projects: projects}
}

// Get returns the current jump pair, hydrated and self-healed. If any stored
// reference points at a deleted entity it is dropped and the cleaned set is
// persisted before returning.
func (s *HarpoonService) Get(ctx context.Context, userID int64) ([]HarpoonSlot, error) {
	const op = "service.HarpoonService.Get"
	settings, err := s.users.GetSettings(ctx, userID)
	if err != nil {
		logRepoErr(ctx, op+": get settings", err, slog.Int64("user_id", userID))
		return nil, err
	}
	return s.hydrate(ctx, userID, settings)
}

// Attach adds ref to the jump pair. A reference already present is a no-op
// (idempotent). When the pair is full the oldest slot is evicted (FIFO). The
// target must exist, otherwise repo.ErrNotFound is returned.
func (s *HarpoonService) Attach(ctx context.Context, userID int64, ref model.HarpoonRef) ([]HarpoonSlot, error) {
	const op = "service.HarpoonService.Attach"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.String("kind", string(ref.Kind)), slog.Int64("id", ref.ID))

	if _, err := s.title(ctx, ref); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			log.WarnContext(ctx, op+": target not found", slog.String("kind", string(ref.Kind)), slog.Int64("id", ref.ID))
		}
		return nil, err
	}

	settings, err := s.users.GetSettings(ctx, userID)
	if err != nil {
		logRepoErr(ctx, op+": get settings", err, slog.Int64("user_id", userID))
		return nil, err
	}

	next := make([]model.HarpoonRef, 0, maxHarpoon)
	for _, r := range settings.Harpoon {
		if r == ref {
			continue // drop existing occurrence so it re-appends at the end
		}
		next = append(next, r)
	}
	next = append(next, ref)
	if len(next) > maxHarpoon {
		next = next[len(next)-maxHarpoon:] // evict oldest
	}
	settings.Harpoon = next

	if err := s.users.SetSettings(ctx, userID, settings); err != nil {
		logRepoErr(ctx, op+": save settings", err, slog.Int64("user_id", userID))
		return nil, err
	}
	log.InfoContext(ctx, "harpoon attached", slog.String("op", op), slog.String("kind", string(ref.Kind)), slog.Int64("id", ref.ID))
	return s.hydrate(ctx, userID, settings)
}

// Detach removes ref from the jump pair. Removing an absent reference is a
// no-op (idempotent).
func (s *HarpoonService) Detach(ctx context.Context, userID int64, ref model.HarpoonRef) ([]HarpoonSlot, error) {
	const op = "service.HarpoonService.Detach"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.String("kind", string(ref.Kind)), slog.Int64("id", ref.ID))

	settings, err := s.users.GetSettings(ctx, userID)
	if err != nil {
		logRepoErr(ctx, op+": get settings", err, slog.Int64("user_id", userID))
		return nil, err
	}

	next := make([]model.HarpoonRef, 0, len(settings.Harpoon))
	for _, r := range settings.Harpoon {
		if r == ref {
			continue
		}
		next = append(next, r)
	}
	settings.Harpoon = next

	if err := s.users.SetSettings(ctx, userID, settings); err != nil {
		logRepoErr(ctx, op+": save settings", err, slog.Int64("user_id", userID))
		return nil, err
	}
	log.InfoContext(ctx, "harpoon detached", slog.String("op", op), slog.String("kind", string(ref.Kind)), slog.Int64("id", ref.ID))
	return s.hydrate(ctx, userID, settings)
}

// hydrate resolves titles for each stored ref. References whose target no
// longer exists are dropped; if any were dropped the cleaned set is persisted.
func (s *HarpoonService) hydrate(ctx context.Context, userID int64, settings *model.UserSettings) ([]HarpoonSlot, error) {
	slots := make([]HarpoonSlot, 0, len(settings.Harpoon))
	alive := make([]model.HarpoonRef, 0, len(settings.Harpoon))
	for _, ref := range settings.Harpoon {
		title, err := s.title(ctx, ref)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				continue // self-heal: drop reference to a deleted entity
			}
			return nil, err
		}
		slots = append(slots, HarpoonSlot{Kind: ref.Kind, ID: ref.ID, Title: title})
		alive = append(alive, ref)
	}

	if len(alive) != len(settings.Harpoon) {
		settings.Harpoon = alive
		if err := s.users.SetSettings(ctx, userID, settings); err != nil {
			logRepoErr(ctx, "service.HarpoonService.hydrate: prune settings", err, slog.Int64("user_id", userID))
			return nil, err
		}
	}
	return slots, nil
}

// title resolves the current title of ref, returning repo.ErrNotFound when the
// target does not exist or the kind is unknown.
func (s *HarpoonService) title(ctx context.Context, ref model.HarpoonRef) (string, error) {
	switch ref.Kind {
	case model.HarpoonKindTask:
		t, err := s.tasks.Get(ctx, ref.ID)
		if err != nil {
			return "", err
		}
		return t.Title, nil
	case model.HarpoonKindProject:
		p, err := s.projects.Get(ctx, ref.ID)
		if err != nil {
			return "", err
		}
		return p.Title, nil
	default:
		return "", repo.ErrNotFound
	}
}
