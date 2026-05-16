package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

type autoLabelSpec struct {
	mask       string
	labelNames []string
	ignoreCase bool
}

func setupAutoLabels(t *testing.T, specs []autoLabelSpec) (*service.AutoLabelsService, *repo.LabelRepo) {
	t.Helper()
	d := setupTestDB(t)
	labels := repo.NewLabelRepo(d)
	appSettings := repo.NewAppSettingsRepo(d)
	if specs != nil {
		rules := make([]model.AutoLabelRule, 0, len(specs))
		for _, sp := range specs {
			ids := make([]int64, 0, len(sp.labelNames))
			for _, name := range sp.labelNames {
				l, err := labels.GetByName(context.Background(), name)
				if err != nil {
					created, err := labels.Create(context.Background(), name, "grey", false)
					if err != nil {
						t.Fatalf("seed label %q: %v", name, err)
					}
					l = created
				}
				ids = append(ids, l.ID)
			}
			rules = append(rules, model.AutoLabelRule{Mask: sp.mask, LabelIDs: ids, IgnoreCase: sp.ignoreCase})
		}
		if err := appSettings.Set(context.Background(), &model.AppSettings{AutoLabels: rules}); err != nil {
			t.Fatalf("seed app settings: %v", err)
		}
	}
	return service.NewAutoLabelsService(labels, appSettings), labels
}

func TestAutoLabelsService_Apply_MatchedAutoLabel(t *testing.T) {
	svc, labels := setupAutoLabels(t, []autoLabelSpec{
		{mask: "urgent", labelNames: []string{"urgent"}, ignoreCase: true},
	})
	ctx := context.Background()

	ids, err := svc.Apply(ctx, "this is urgent stuff", nil, nil, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("ids: got %v, want one", ids)
	}
	l, err := labels.GetByName(ctx, "urgent")
	if err != nil {
		t.Fatalf("expected pre-seeded label, got: %v", err)
	}
	if l.ID != ids[0] {
		t.Errorf("id: got %d, want %d", ids[0], l.ID)
	}
}

func TestAutoLabelsService_Apply_NoMatch(t *testing.T) {
	svc, _ := setupAutoLabels(t, []autoLabelSpec{
		{mask: "urgent", labelNames: []string{"urgent"}, ignoreCase: true},
	})
	ctx := context.Background()

	ids, err := svc.Apply(ctx, "calm task", nil, nil, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids: got %v, want []", ids)
	}
}

func TestAutoLabelsService_Apply_CaseSensitive(t *testing.T) {
	svc, _ := setupAutoLabels(t, []autoLabelSpec{
		{mask: "Urgent", labelNames: []string{"urgent"}, ignoreCase: false},
	})
	ctx := context.Background()

	ids, err := svc.Apply(ctx, "this is urgent", nil, nil, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids: got %v, want [] (case-sensitive)", ids)
	}

	ids2, err := svc.Apply(ctx, "this is Urgent", nil, nil, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(ids2) != 1 {
		t.Errorf("ids: got %v, want one", ids2)
	}
}

func TestAutoLabelsService_Apply_MultipleLabelsPerRule(t *testing.T) {
	svc, _ := setupAutoLabels(t, []autoLabelSpec{
		{mask: "bug", labelNames: []string{"bug", "triage"}, ignoreCase: true},
	})
	ctx := context.Background()

	ids, err := svc.Apply(ctx, "bug report", nil, nil, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("ids: got %v, want 2", ids)
	}
}

func TestAutoLabelsService_Apply_RemovedAutoLabel(t *testing.T) {
	svc, _ := setupAutoLabels(t, []autoLabelSpec{
		{mask: "urgent", labelNames: []string{"urgent"}, ignoreCase: true},
	})
	ctx := context.Background()

	ids, err := svc.Apply(ctx, "urgent task", nil, nil, []string{"urgent"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids: got %v, want [] (auto-label removed)", ids)
	}
}

func TestAutoLabelsService_Apply_ExplicitKnown(t *testing.T) {
	svc, labels := setupAutoLabels(t, nil)
	ctx := context.Background()

	created, _ := labels.Create(ctx, "manual", "blue", false)
	names := []string{"manual"}
	ids, err := svc.Apply(ctx, "any title", nil, &names, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(ids) != 1 || ids[0] != created.ID {
		t.Errorf("ids: got %v, want [%d]", ids, created.ID)
	}
}

func TestAutoLabelsService_Apply_ExplicitUnknown(t *testing.T) {
	svc, _ := setupAutoLabels(t, nil)
	ctx := context.Background()

	names := []string{"nope"}
	_, err := svc.Apply(ctx, "x", nil, &names, nil)
	var unknown *service.UnknownLabelError
	if !errors.As(err, &unknown) {
		t.Errorf("err: got %v, want UnknownLabelError", err)
	}
	if unknown != nil && unknown.Error() == "" {
		t.Error("Error() should not be empty")
	}
}

func TestAutoLabelsService_Apply_PreservesCurrent(t *testing.T) {
	svc, labels := setupAutoLabels(t, nil)
	ctx := context.Background()

	a, _ := labels.Create(ctx, "a", "blue", false)
	b, _ := labels.Create(ctx, "b", "blue", false)
	ids, err := svc.Apply(ctx, "x", []int64{a.ID, b.ID}, nil, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("ids: got %v, want 2 ids", ids)
	}
}
