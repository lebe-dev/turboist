package handlers

import (
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

func TestShouldIncPostpone(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	old := now.Add(-time.Hour)         // task created 1h ago — past grace
	fresh := now.Add(-time.Minute)     // 1 minute old — within grace
	past := now.Add(-24 * time.Hour)   // yesterday
	future1 := now.Add(24 * time.Hour) // tomorrow
	future2 := now.Add(72 * time.Hour) // in 3 days

	cases := []struct {
		name string
		task model.Task
		upd  repo.TaskUpdate
		want bool
	}{
		{
			name: "future to further future increments",
			task: model.Task{CreatedAt: old, DueAt: &future1},
			upd:  repo.TaskUpdate{DueAt: &future2},
			want: true,
		},
		{
			name: "past to future increments",
			task: model.Task{CreatedAt: old, DueAt: &past},
			upd:  repo.TaskUpdate{DueAt: &future1},
			want: true,
		},
		{
			name: "future to past does not increment",
			task: model.Task{CreatedAt: old, DueAt: &future1},
			upd:  repo.TaskUpdate{DueAt: &past},
			want: false,
		},
		{
			name: "future to earlier future does not increment",
			task: model.Task{CreatedAt: old, DueAt: &future2},
			upd:  repo.TaskUpdate{DueAt: &future1},
			want: false,
		},
		{
			name: "first assignment (old nil) does not increment",
			task: model.Task{CreatedAt: old, DueAt: nil},
			upd:  repo.TaskUpdate{DueAt: &future1},
			want: false,
		},
		{
			name: "clearing due date does not increment",
			task: model.Task{CreatedAt: old, DueAt: &future1},
			upd:  repo.TaskUpdate{DueAtClear: true},
			want: false,
		},
		{
			name: "task fresher than 5 min does not increment",
			task: model.Task{CreatedAt: fresh, DueAt: &future1},
			upd:  repo.TaskUpdate{DueAt: &future2},
			want: false,
		},
		{
			name: "task exactly 5 min old does not increment",
			task: model.Task{CreatedAt: now.Add(-5 * time.Minute), DueAt: &future1},
			upd:  repo.TaskUpdate{DueAt: &future2},
			want: false,
		},
		{
			name: "no due change does not increment",
			task: model.Task{CreatedAt: old, DueAt: &future1},
			upd:  repo.TaskUpdate{Title: ptr("renamed")},
			want: false,
		},
		{
			name: "equal due dates do not increment",
			task: model.Task{CreatedAt: old, DueAt: &future1},
			upd:  repo.TaskUpdate{DueAt: &future1},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldIncPostpone(&tc.task, tc.upd, now)
			if got != tc.want {
				t.Errorf("shouldIncPostpone: got %v, want %v", got, tc.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
