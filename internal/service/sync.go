package service

import (
	"context"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// SyncBundle is what /sync/pull returns: a coherent snapshot of all syncable
// entities the client needs to merge into its local store.
type SyncBundle struct {
	Now      time.Time
	Tasks    []model.Task
	Projects []model.Project
	Sections []model.ProjectSection
	Labels   []model.Label
	Contexts []model.Context
}

// SyncService orchestrates the pull endpoint. The 30-day completed-task window
// matches the FEATURE-OFFLINE-PLAN contract: initial pull avoids hauling years
// of completed history, but recent completions remain visible offline.
type SyncService struct {
	repo            *repo.SyncRepo
	completedWindow time.Duration
}

func NewSyncService(r *repo.SyncRepo) *SyncService {
	return &SyncService{repo: r, completedWindow: 30 * 24 * time.Hour}
}

// Pull collects all syncable entities. When since == nil the bundle contains
// only alive rows; when since != nil it contains every row with updated_at >
// since (including tombstones).
func (s *SyncService) Pull(ctx context.Context, since *time.Time) (*SyncBundle, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-s.completedWindow)
	tasks, err := s.repo.Tasks(ctx, since, cutoff)
	if err != nil {
		return nil, err
	}
	projects, err := s.repo.Projects(ctx, since)
	if err != nil {
		return nil, err
	}
	sections, err := s.repo.Sections(ctx, since)
	if err != nil {
		return nil, err
	}
	labels, err := s.repo.Labels(ctx, since)
	if err != nil {
		return nil, err
	}
	contexts, err := s.repo.Contexts(ctx, since)
	if err != nil {
		return nil, err
	}
	return &SyncBundle{
		Now:      now,
		Tasks:    tasks,
		Projects: projects,
		Sections: sections,
		Labels:   labels,
		Contexts: contexts,
	}, nil
}
