package service

import (
	"context"
	"errors"
	"strings"

	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/repo"
)

// UnknownLabelError is returned when a label name is not found and is not an auto-label.
type UnknownLabelError struct {
	Name string
}

func (e *UnknownLabelError) Error() string {
	return "service: unknown label: " + e.Name
}

// AutoLabelsService applies auto-label rules from config and resolves label names.
type AutoLabelsService struct {
	labels    *repo.LabelRepo
	cfg       *config.Config
	autoNames map[string]struct{} // lower-cased auto-label names for O(1) lookup
}

// NewAutoLabelsService constructs an AutoLabelsService.
func NewAutoLabelsService(labels *repo.LabelRepo, cfg *config.Config) *AutoLabelsService {
	m := make(map[string]struct{}, len(cfg.AutoLabels))
	for _, al := range cfg.AutoLabels {
		m[strings.ToLower(al.Label)] = struct{}{}
	}
	return &AutoLabelsService{labels: labels, cfg: cfg, autoNames: m}
}

func (s *AutoLabelsService) isAutoLabel(name string) bool {
	_, ok := s.autoNames[strings.ToLower(name)]
	return ok
}

// getOrCreate looks up a label by name. If missing and the name is an auto-label it is
// created; otherwise UnknownLabelError is returned.
func (s *AutoLabelsService) getOrCreate(ctx context.Context, name string) (int64, error) {
	l, err := s.labels.GetByName(ctx, name)
	if err == nil {
		return l.ID, nil
	}
	if !errors.Is(err, repo.ErrNotFound) {
		return 0, err
	}
	if !s.isAutoLabel(name) {
		return 0, &UnknownLabelError{Name: name}
	}
	created, err := s.labels.Create(ctx, name, "grey", false)
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			// Race condition: another request created it first.
			l, err := s.labels.GetByName(ctx, name)
			if err != nil {
				return 0, err
			}
			return l.ID, nil
		}
		return 0, err
	}
	return created.ID, nil
}

// matchAutoLabels scans title against all configured auto-label rules and returns matched IDs,
// auto-creating labels that don't exist yet.
func (s *AutoLabelsService) matchAutoLabels(ctx context.Context, title string) ([]int64, error) {
	seen := make(map[int64]struct{})
	var ids []int64
	for _, al := range s.cfg.AutoLabels {
		needle := al.Mask
		haystack := title
		if al.IgnoreCaseValue() {
			needle = strings.ToLower(needle)
			haystack = strings.ToLower(haystack)
		}
		if !strings.Contains(haystack, needle) {
			continue
		}
		id, err := s.getOrCreate(ctx, al.Label)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[id]; !dup {
			ids = append(ids, id)
			seen[id] = struct{}{}
		}
	}
	return ids, nil
}

// Apply computes the final label ID set for a task:
//
//	Final = (base ∪ matched_auto) − removed
//
// Parameters:
//   - title: scanned for auto-label rule matches
//   - currentIDs: current label IDs (ignored when explicitNames != nil)
//   - explicitNames: if non-nil, replaces the base set; each name must exist or be an auto-label
//   - removedAutoNames: label names to exclude from the final set (best-effort; ignored if not found)
func (s *AutoLabelsService) Apply(ctx context.Context, title string, currentIDs []int64, explicitNames *[]string, removedAutoNames []string) ([]int64, error) {
	base := make(map[int64]struct{})

	if explicitNames != nil {
		for _, name := range *explicitNames {
			id, err := s.getOrCreate(ctx, name)
			if err != nil {
				return nil, err
			}
			base[id] = struct{}{}
		}
	} else {
		for _, id := range currentIDs {
			base[id] = struct{}{}
		}
	}

	matched, err := s.matchAutoLabels(ctx, title)
	if err != nil {
		return nil, err
	}
	for _, id := range matched {
		base[id] = struct{}{}
	}

	for _, name := range removedAutoNames {
		l, err := s.labels.GetByName(ctx, name)
		if err == nil {
			delete(base, l.ID)
		}
	}

	result := make([]int64, 0, len(base))
	for id := range base {
		result = append(result, id)
	}
	return result, nil
}
