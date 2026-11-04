package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestProjectRepo_ActivityStats(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Now()

	other, err := f.projects.Create(ctx, CreateProject{ContextID: f.contextID, Title: "other", Color: "red"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	idle, err := f.projects.Create(ctx, CreateProject{ContextID: f.contextID, Title: "idle", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	finish := func(projectID int64, at time.Time, status model.TaskStatus) {
		t.Helper()
		task, err := f.tasks.Create(ctx, CreateTask{
			Placement: Placement{ContextID: &f.contextID, ProjectID: &projectID},
			Title:     "t",
		})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		if _, err := f.tasks.Update(ctx, task.ID, TaskUpdate{Status: &status, CompletedAt: &at}); err != nil {
			t.Fatalf("complete task: %v", err)
		}
	}

	// f.projectID: two this week, one 10 days ago (previous week, this month),
	// one cancelled this week (must not count).
	finish(f.projectID, now.AddDate(0, 0, -1), model.TaskStatusCompleted)
	finish(f.projectID, now.AddDate(0, 0, -3), model.TaskStatusCompleted)
	finish(f.projectID, now.AddDate(0, 0, -10), model.TaskStatusCompleted)
	finish(f.projectID, now.AddDate(0, 0, -2), model.TaskStatusCancelled)
	// other: one completion 45 days ago — the month's previous window only.
	finish(other.ID, now.AddDate(0, 0, -45), model.TaskStatusCompleted)

	rows, err := f.projects.ActivityStats(ctx, labelUsageRanges(now))
	if err != nil {
		t.Fatalf("activity stats: %v", err)
	}
	byProject := map[int64]ProjectActivity{}
	for _, r := range rows {
		byProject[r.ProjectID] = r
	}
	if _, ok := byProject[idle.ID]; ok {
		t.Errorf("idle project must not be reported")
	}

	main := byProject[f.projectID]
	if got := main.Periods[weekIdx]; got.Completed != 2 || got.PreviousCompleted != 1 {
		t.Errorf("week: got %+v, want completed=2 previous=1", got)
	}
	if got := main.Periods[monthIdx]; got.Completed != 3 || got.PreviousCompleted != 0 {
		t.Errorf("month: got %+v, want completed=3 previous=0", got)
	}

	oth := byProject[other.ID]
	if got := oth.Periods[monthIdx]; got.Completed != 0 || got.PreviousCompleted != 1 {
		t.Errorf("other month: got %+v, want completed=0 previous=1", got)
	}
	if got := oth.Periods[quarterIdx]; got.Completed != 1 {
		t.Errorf("other quarter: got %+v, want completed=1", got)
	}
}
