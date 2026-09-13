package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

func TestPruneInboxProcessingOnce_RemovesOldJournalRows(t *testing.T) {
	d := setupChangeLogDB(t)
	journal := repo.NewInboxProcessingRepo(d, repo.NewTaskLabelsRepo(d))
	ctx := context.Background()
	for _, at := range []time.Time{time.Now().Add(-100 * 24 * time.Hour), time.Now()} {
		if _, err := journal.AppendLog(ctx, model.InboxProcessingLogEntry{
			TaskTitle: "t", Outcome: model.InboxOutcomeKept, Model: "m", CreatedAt: at,
		}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	pruneInboxProcessingOnce(ctx, journal, slog.New(slog.NewTextHandler(io.Discard, nil)))

	_, total, err := journal.ListLog(ctx, repo.Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 {
		t.Errorf("journal rows after prune: got %d, want 1", total)
	}
}
