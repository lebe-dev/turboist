package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestTaskRepo_Views_TodayTomorrowOverdue(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	loc := time.UTC
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, loc)
	todayStart := time.Date(2026, 4, 27, 0, 0, 0, 0, loc)
	yesterday := now.Add(-48 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)

	mk := func(title string, due time.Time) *model.Task {
		t.Helper()
		dueCopy := due
		task, err := f.tasks.Create(ctx, CreateTask{
			Placement: Placement{ContextID: &f.contextID},
			Title:     title,
			DueAt:     &dueCopy,
		})
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		return task
	}

	overdueTask := mk("overdue", yesterday)
	todayTask := mk("today", now)
	tomorrowTask := mk("tomorrow", tomorrow)

	today, _, err := f.tasks.ListToday(ctx, todayStart, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("today: %v", err)
	}
	if len(today) != 1 || today[0].ID != todayTask.ID {
		t.Errorf("today: %+v", today)
	}

	tom, _, err := f.tasks.ListTomorrow(ctx, todayStart, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("tomorrow: %v", err)
	}
	if len(tom) != 1 || tom[0].ID != tomorrowTask.ID {
		t.Errorf("tomorrow: %+v", tom)
	}

	od, _, err := f.tasks.ListOverdue(ctx, todayStart, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("overdue: %v", err)
	}
	if len(od) != 1 || od[0].ID != overdueTask.ID {
		t.Errorf("overdue: %+v", od)
	}
}

func TestTaskRepo_Views_WeekAndBacklog(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	week := model.PlanStateWeek
	backlog := model.PlanStateBacklog
	wt, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "w", PlanState: week})
	bt, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "b", PlanState: backlog})

	wList, _, err := f.tasks.ListWeek(ctx, TaskFilter{})
	if err != nil {
		t.Fatalf("week: %v", err)
	}
	if len(wList) != 1 || wList[0].ID != wt.ID {
		t.Errorf("week: %+v", wList)
	}
	bList, _, err := f.tasks.ListBacklog(ctx, TaskFilter{})
	if err != nil {
		t.Fatalf("backlog: %v", err)
	}
	if len(bList) != 1 || bList[0].ID != bt.ID {
		t.Errorf("backlog: %+v", bList)
	}
}

func TestTaskRepo_Views_TodayInNonUTC(t *testing.T) {
	// Verify the today window honors a non-UTC todayStart computed by the
	// service: a task due 2026-04-27 23:00 in Tokyo (=14:00 UTC) lands inside
	// "today" for Tokyo even though the underlying due_at is stored UTC.
	f := newTaskFixture(t)
	ctx := context.Background()
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skip("no tzdata")
	}
	todayStartTokyo := time.Date(2026, 4, 27, 0, 0, 0, 0, tokyo).UTC()
	dueLate := time.Date(2026, 4, 27, 23, 0, 0, 0, tokyo)

	dueCopy := dueLate
	task, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "late tokyo",
		DueAt:     &dueCopy,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	items, _, err := f.tasks.ListToday(ctx, todayStartTokyo, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || items[0].ID != task.ID {
		t.Errorf("expected today match, got %+v", items)
	}
}

func TestTaskRepo_Views_FilterByLabel(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	todayStart := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)

	l, _ := f.labels.Create(ctx, "lbl", "blue", false)
	tagged, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "tagged",
		DueAt:     &now,
	})
	plainNow := now
	if _, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "plain",
		DueAt:     &plainNow,
	}); err != nil {
		t.Fatalf("create plain: %v", err)
	}
	if err := f.tlabels.SetForTask(ctx, tagged.ID, []int64{l.ID}); err != nil {
		t.Fatalf("set label: %v", err)
	}

	items, _, err := f.tasks.ListToday(ctx, todayStart, TaskFilter{LabelID: &l.ID}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || items[0].ID != tagged.ID {
		t.Errorf("filter by label: %+v", items)
	}
}
