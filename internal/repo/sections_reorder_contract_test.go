package repo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Moving a column of a project board is one of the few writes the native mobile
// client applies to its own replica before the request is sent: the user drags a
// column and the board has to settle immediately, with no connection. That means
// the ordering rule exists twice — here, and in the client's write path — and two
// implementations of one rule drift silently.
//
// testdata/sync-contract/section-reorder.json is the shared set of cases that
// stops them. This test replays each case through ProjectSectionRepo.Reorder; the
// mobile client replays the same file through its own write path and compares the
// same expected boards. Change the rule and both sides go red together.
const sectionReorderContractFile = "../../testdata/sync-contract/section-reorder.json"

type sectionReorderContract struct {
	Cases []sectionReorderCase `json:"cases"`
}

type sectionReorderCase struct {
	Name     string   `json:"name"`
	Board    []string `json:"board"`
	Move     string   `json:"move"`
	Position int      `json:"position"`
	Expected []string `json:"expected"`
}

func TestSectionReorderContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean(sectionReorderContractFile))
	if err != nil {
		t.Fatalf("read shared reorder cases: %v", err)
	}
	var contract sectionReorderContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("parse shared reorder cases: %v", err)
	}
	if len(contract.Cases) == 0 {
		t.Fatal("the shared reorder cases are empty")
	}

	for _, c := range contract.Cases {
		t.Run(c.Name, func(t *testing.T) {
			db := setupTestDB(t)
			ctx := context.Background()
			contexts := NewContextRepo(db)
			projects := NewProjectRepo(db, NewProjectLabelsRepo(db))
			sections := NewProjectSectionRepo(db)

			cx, err := contexts.Create(ctx, "Work", "blue", false)
			if err != nil {
				t.Fatalf("create context: %v", err)
			}
			project, err := projects.Create(ctx, CreateProject{ContextID: cx.ID, Title: "Board", Color: "blue"})
			if err != nil {
				t.Fatalf("create project: %v", err)
			}

			// Columns are created in the order the case draws them, so each one
			// lands at the next free position and the board starts as 0..n-1.
			ids := make(map[string]int64, len(c.Board))
			for _, title := range c.Board {
				s, err := sections.Create(ctx, project.ID, title)
				if err != nil {
					t.Fatalf("create section %q: %v", title, err)
				}
				ids[title] = s.ID
			}

			if _, err := sections.Reorder(ctx, ids[c.Move], c.Position); err != nil {
				t.Fatalf("reorder: %v", err)
			}

			listed, _, err := sections.ListByProject(ctx, project.ID, Page{Limit: 100})
			if err != nil {
				t.Fatalf("list sections: %v", err)
			}
			if len(listed) != len(c.Expected) {
				t.Fatalf("board size: got %d, want %d", len(listed), len(c.Expected))
			}
			for i, want := range c.Expected {
				if listed[i].Title != want {
					t.Errorf("column %d: got %q, want %q", i, listed[i].Title, want)
				}
				if listed[i].Position != i {
					t.Errorf("position of %q: got %d, want %d", listed[i].Title, listed[i].Position, i)
				}
			}
		})
	}
}
