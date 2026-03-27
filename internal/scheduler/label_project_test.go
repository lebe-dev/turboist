package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

type mockProjectMover struct {
	tasks    []*todoist.Task
	projects []*todoist.Project
	inboxID  string
	moves    map[string]string // taskID → projectID from last batch call
	err      error
}

func (m *mockProjectMover) Tasks() []*todoist.Task       { return m.tasks }
func (m *mockProjectMover) Projects() []*todoist.Project { return m.projects }
func (m *mockProjectMover) InboxProjectID() string       { return m.inboxID }
func (m *mockProjectMover) BatchMoveTasksToProject(_ context.Context, moves map[string]string) error {
	if m.err != nil {
		return m.err
	}
	m.moves = moves
	return nil
}

func lpTask(id, projectID string, labels ...string) *todoist.Task {
	return &todoist.Task{ID: id, Content: "task " + id, ProjectID: projectID, Labels: labels}
}

func lpSubtask(id, projectID, parentID string, labels ...string) *todoist.Task {
	return &todoist.Task{ID: id, Content: "sub " + id, ProjectID: projectID, ParentID: &parentID, Labels: labels}
}

func lpMappings(pairs ...string) []config.LabelProjectMapping {
	out := make([]config.LabelProjectMapping, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, config.LabelProjectMapping{Label: pairs[i], Project: pairs[i+1]})
	}
	return out
}

func TestLabelProjectSync_MovesToMappedProject(t *testing.T) {
	m := &mockProjectMover{
		tasks:    []*todoist.Task{lpTask("1", "inbox", "health")},
		projects: []*todoist.Project{{ID: "p1", Name: "Personal"}},
		inboxID:  "inbox",
	}
	lp := NewLabelProjectSync(m, lpMappings("health", "Personal"))
	lp.Job(context.Background())

	if len(m.moves) != 1 {
		t.Fatalf("got %d moves, want 1", len(m.moves))
	}
	if m.moves["1"] != "p1" {
		t.Errorf("got move %v, want {1: p1}", m.moves)
	}
}

func TestLabelProjectSync_NoMatchGoesToInbox(t *testing.T) {
	m := &mockProjectMover{
		tasks:    []*todoist.Task{lpTask("2", "p1")}, // no matching label
		projects: []*todoist.Project{{ID: "p1", Name: "Work"}},
		inboxID:  "inbox",
	}
	lp := NewLabelProjectSync(m, lpMappings("work", "Work"))
	lp.Job(context.Background())

	if len(m.moves) != 1 {
		t.Fatalf("got %d moves, want 1", len(m.moves))
	}
	if m.moves["2"] != "inbox" {
		t.Errorf("got project %q, want inbox", m.moves["2"])
	}
}

func TestLabelProjectSync_AlreadyInCorrectProject_NoMove(t *testing.T) {
	m := &mockProjectMover{
		tasks:    []*todoist.Task{lpTask("3", "p1", "work")},
		projects: []*todoist.Project{{ID: "p1", Name: "Work"}},
		inboxID:  "inbox",
	}
	lp := NewLabelProjectSync(m, lpMappings("work", "Work"))
	lp.Job(context.Background())

	if m.moves != nil {
		t.Fatalf("got %d moves, want 0", len(m.moves))
	}
}

func TestLabelProjectSync_SubtasksSkipped(t *testing.T) {
	m := &mockProjectMover{
		tasks:    []*todoist.Task{lpSubtask("4", "p1", "parent1", "work")},
		projects: []*todoist.Project{{ID: "p2", Name: "Work"}},
		inboxID:  "inbox",
	}
	lp := NewLabelProjectSync(m, lpMappings("work", "Work"))
	lp.Job(context.Background())

	if m.moves != nil {
		t.Fatalf("subtask should not be moved, got %d moves", len(m.moves))
	}
}

func TestLabelProjectSync_FirstLabelWins(t *testing.T) {
	m := &mockProjectMover{
		tasks:    []*todoist.Task{lpTask("5", "inbox", "health", "work")},
		projects: []*todoist.Project{{ID: "p1", Name: "Personal"}, {ID: "p2", Name: "Work"}},
		inboxID:  "inbox",
	}
	// "health" mapping comes first — should win
	lp := NewLabelProjectSync(m, lpMappings("health", "Personal", "work", "Work"))
	lp.Job(context.Background())

	if len(m.moves) != 1 {
		t.Fatalf("got %d moves, want 1", len(m.moves))
	}
	if m.moves["5"] != "p1" {
		t.Errorf("got project %q, want p1 (Personal)", m.moves["5"])
	}
}

func TestLabelProjectSync_UnknownProjectInMapping_FallsToInbox(t *testing.T) {
	m := &mockProjectMover{
		tasks:    []*todoist.Task{lpTask("6", "p1", "health")},
		projects: []*todoist.Project{{ID: "p1", Name: "Work"}}, // "Personal" not in cache
		inboxID:  "inbox",
	}
	lp := NewLabelProjectSync(m, lpMappings("health", "Personal"))
	lp.Job(context.Background())

	// Unknown project → falls through to inbox
	if len(m.moves) != 1 {
		t.Fatalf("got %d moves, want 1", len(m.moves))
	}
	if m.moves["6"] != "inbox" {
		t.Errorf("got project %q, want inbox", m.moves["6"])
	}
}

func TestLabelProjectSync_EmptyInboxID_SkipsTask(t *testing.T) {
	m := &mockProjectMover{
		tasks:    []*todoist.Task{lpTask("7", "p1")}, // no matching label
		projects: []*todoist.Project{},
		inboxID:  "", // inbox not in cache
	}
	lp := NewLabelProjectSync(m, lpMappings("work", "Work"))
	lp.Job(context.Background())

	if m.moves != nil {
		t.Fatalf("task should be skipped when inbox ID is empty, got %d moves", len(m.moves))
	}
}

func TestLabelProjectSync_BatchMoveError_LogsAndContinues(t *testing.T) {
	m := &mockProjectMover{
		tasks: []*todoist.Task{
			lpTask("8", "inbox", "health"),
			lpTask("9", "inbox", "work"),
		},
		projects: []*todoist.Project{
			{ID: "p1", Name: "Personal"},
			{ID: "p2", Name: "Work"},
		},
		inboxID: "inbox",
		err:     errors.New("api error"),
	}
	lp := NewLabelProjectSync(m, lpMappings("health", "Personal", "work", "Work"))
	// Should not panic; error is logged
	lp.Job(context.Background())
}
