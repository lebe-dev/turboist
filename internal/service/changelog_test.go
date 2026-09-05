package service_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

type loggedChange struct {
	entity   string
	entityID int64
	op       string
}

func readChangeLog(t *testing.T, d *sql.DB) []loggedChange {
	t.Helper()
	rows, err := d.Query(`SELECT entity, entity_id, op FROM change_log ORDER BY seq`)
	if err != nil {
		t.Fatalf("read change log: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []loggedChange
	for rows.Next() {
		var c loggedChange
		if err := rows.Scan(&c.entity, &c.entityID, &c.op); err != nil {
			t.Fatalf("scan change log: %v", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read change log: %v", err)
	}
	return out
}

func changeLogged(rows []loggedChange, entity string, entityID int64, op string) bool {
	for _, r := range rows {
		if r.entity == entity && r.entityID == entityID && r.op == op {
			return true
		}
	}
	return false
}

func clearChangeLog(t *testing.T, d *sql.DB) {
	t.Helper()
	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("clear change log: %v", err)
	}
}

// Parking a parent in the backlog rewrites every open descendant in one bulk
// statement that names no ids. This is exactly the shape of write a hand-written
// logging call forgets, and a replica that misses it would keep showing the
// subtasks as scheduled forever — so the changelog is asserted against the
// service path, not against a single-row update.
func TestChangeLog_BacklogCascadeLogsEveryDescendant(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	plan := service.NewPlanService(tasks, ctxs, 10, 10)
	ctx := context.Background()

	c, err := ctxs.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	cid := c.ID

	parent, err := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Ship the release",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ParentID: &parent.ID},
		Title:     "Write the notes",
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	grandchild, err := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ParentID: &child.ID},
		Title:     "Proofread the notes",
	})
	if err != nil {
		t.Fatalf("create grandchild: %v", err)
	}

	clearChangeLog(t, d)
	if _, err := plan.SetPlanState(ctx, parent.ID, model.PlanStateBacklog); err != nil {
		t.Fatalf("park parent in backlog: %v", err)
	}

	rows := readChangeLog(t, d)
	for _, id := range []int64{parent.ID, child.ID, grandchild.ID} {
		if !changeLogged(rows, "task", id, "upsert") {
			t.Errorf("task %d: no upsert logged for the backlog cascade, got %v", id, rows)
		}
	}
}

// Creating a task with labels writes two tables, and the label edge is served
// inside the task payload — so both writes have to converge on one entity the
// replica can refetch.
func TestChangeLog_TaskCreationWithLabelsLogsTheTask(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	labels := repo.NewLabelRepo(d)
	ctx := context.Background()

	c, err := ctxs.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	cid := c.ID
	l, err := labels.Create(ctx, "urgent", "red", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}

	clearChangeLog(t, d)
	task, err := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Call the bank",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := tlabels.SetForTask(ctx, task.ID, []int64{l.ID}); err != nil {
		t.Fatalf("tag task: %v", err)
	}

	rows := readChangeLog(t, d)
	upserts := 0
	for _, r := range rows {
		if r.entity == "task" && r.entityID == task.ID && r.op == "upsert" {
			upserts++
		}
	}
	if upserts < 2 {
		t.Errorf("task upserts after create and tag: got %d in %v, want at least 2", upserts, rows)
	}
}

// Deleting a context takes its projects and tasks with it through foreign keys,
// a path no repository method ever names. Each vanished row still has to leave a
// tombstone, or the replica keeps rows the server has forgotten.
func TestChangeLog_ContextDeletionLogsCascadedTasks(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	ctx := context.Background()

	c, err := ctxs.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	cid := c.ID
	p, err := projects.Create(ctx, repo.CreateProject{
		ContextID: cid,
		Title:     "Release",
		Color:     "blue",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ProjectID: &p.ID},
		Title:     "Cut the tag",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	clearChangeLog(t, d)
	if err := ctxs.Delete(ctx, cid); err != nil {
		t.Fatalf("delete context: %v", err)
	}

	rows := readChangeLog(t, d)
	for _, want := range []loggedChange{
		{"task", task.ID, "delete"},
		{"project", p.ID, "delete"},
		{"context", cid, "delete"},
	} {
		if !changeLogged(rows, want.entity, want.entityID, want.op) {
			t.Errorf("%s %d: no %s logged for the cascade, got %v", want.entity, want.entityID, want.op, rows)
		}
	}
}
