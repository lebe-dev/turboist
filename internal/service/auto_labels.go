package service

import (
	"context"
	"errors"
	"strings"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// UnknownLabelError is returned when a label name referenced explicitly does
// not exist. Auto-labels never create labels — existing labels only.
type UnknownLabelError struct {
	Name string
}

func (e *UnknownLabelError) Error() string {
	return "service: unknown label: " + e.Name
}

// AutoLabelsService applies auto-label rules from app settings and resolves label names.
type AutoLabelsService struct {
	labels      *repo.LabelRepo
	appSettings *repo.AppSettingsRepo
}

// NewAutoLabelsService constructs an AutoLabelsService.
func NewAutoLabelsService(labels *repo.LabelRepo, appSettings *repo.AppSettingsRepo) *AutoLabelsService {
	return &AutoLabelsService{labels: labels, appSettings: appSettings}
}

func (s *AutoLabelsService) loadRules(ctx context.Context) ([]model.AutoLabelRule, error) {
	settings, err := s.appSettings.Get(ctx)
	if err != nil {
		return nil, err
	}
	return settings.AutoLabels, nil
}

// matchAutoLabels scans title against all configured auto-label rules and returns matched label IDs.
// Labels are not auto-created: each rule's LabelIDs are returned as-is when the mask matches.
func matchAutoLabels(rules []model.AutoLabelRule, title string) []int64 {
	seen := make(map[int64]struct{})
	var ids []int64
	for _, al := range rules {
		needle := al.Mask
		haystack := title
		if al.IgnoreCase {
			needle = strings.ToLower(needle)
			haystack = strings.ToLower(haystack)
		}
		if needle == "" || !strings.Contains(haystack, needle) {
			continue
		}
		for _, id := range al.LabelIDs {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

// resolveExisting returns the ID of an existing label by name. Auto-creation is no longer
// performed: an unknown label always yields UnknownLabelError.
func (s *AutoLabelsService) resolveExisting(ctx context.Context, name string) (int64, error) {
	l, err := s.labels.GetByName(ctx, name)
	if err == nil {
		return l.ID, nil
	}
	if errors.Is(err, repo.ErrNotFound) {
		return 0, &UnknownLabelError{Name: name}
	}
	return 0, err
}

// Apply computes the final label ID set for a task:
//
//	Final = (base ∪ matched_auto) − removed
//
// Parameters:
//   - title: scanned for auto-label rule matches
//   - currentIDs: current label IDs (ignored when explicitNames != nil)
//   - explicitNames: if non-nil, replaces the base set; each name must reference an existing label
//   - removedAutoNames: label names to exclude from the final set (best-effort; ignored if not found)
func (s *AutoLabelsService) Apply(ctx context.Context, title string, currentIDs []int64, explicitNames *[]string, removedAutoNames []string) ([]int64, error) {
	rules, err := s.loadRules(ctx)
	if err != nil {
		return nil, err
	}

	base := make(map[int64]struct{})

	if explicitNames != nil {
		for _, name := range *explicitNames {
			id, err := s.resolveExisting(ctx, name)
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

	for _, id := range matchAutoLabels(rules, title) {
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
