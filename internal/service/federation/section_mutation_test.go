package federation_test

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// TestSectionMutator_CreateUpdateDeleteFederated drives create → update → delete
// of a section in a FEDERATED project and asserts each emits the right signed
// event (op=create with title/position, op=update with the changed title,
// op=delete with the _deleted HLC).
func TestSectionMutator_CreateUpdateDeleteFederated(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	sections := repo.NewProjectSectionRepo(env.db)
	mut := fedsvc.NewSectionMutator(env.emitter, sections)

	clientID := "sec-fed-client"
	id, err := mut.Create(ctx, env.fedProject, "Backlog", clientID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sec, err := sections.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sec.Title != "Backlog" {
		t.Errorf("title: got %q", sec.Title)
	}

	evts := outboxEvents(t, env, env.fedProject)
	if len(evts) != 1 {
		t.Fatalf("create outbox count: got %d, want 1", len(evts))
	}
	ce := evts[0]
	if ce.Op != events.OpCreate || ce.EntityType != events.EntitySection {
		t.Errorf("create event: op=%q type=%q", ce.Op, ce.EntityType)
	}
	if ce.EntityID != clientID {
		t.Errorf("create entity_id: got %q, want %q", ce.EntityID, clientID)
	}
	if f, ok := ce.Fields["title"]; !ok || f.Value != "Backlog" {
		t.Errorf("create title field: got %+v", ce.Fields["title"])
	}
	if _, ok := ce.Fields["position"]; !ok {
		t.Errorf("create must carry position field: got %v", ce.Fields)
	}

	newTitle := "Renamed section"
	if err := mut.Update(ctx, sec, repo.SectionUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("update: %v", err)
	}
	evts = outboxEvents(t, env, env.fedProject)
	if len(evts) != 2 {
		t.Fatalf("after update outbox count: got %d, want 2", len(evts))
	}
	ue := evts[1]
	if ue.Op != events.OpUpdate {
		t.Errorf("update op: got %q", ue.Op)
	}
	if f, ok := ue.Fields["title"]; !ok || f.Value != newTitle {
		t.Errorf("update title field: got %+v", ue.Fields["title"])
	}

	if err := mut.Delete(ctx, sec); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := sections.Get(ctx, id); err == nil {
		t.Errorf("section must be soft-deleted")
	}
	evts = outboxEvents(t, env, env.fedProject)
	if len(evts) != 3 {
		t.Fatalf("after delete outbox count: got %d, want 3", len(evts))
	}
	de := evts[2]
	if de.Op != events.OpDelete || de.EntityType != events.EntitySection {
		t.Errorf("delete event: op=%q type=%q", de.Op, de.EntityType)
	}
	if f, ok := de.Fields[events.FieldDeleted]; !ok || f.HLC == "" {
		t.Errorf("delete must carry _deleted HLC: got %+v", de.Fields)
	}
}

// TestSectionMutator_NonFederatedNoOutbox asserts section create/update/delete in
// a LOCAL-ONLY project writes the domain rows but ZERO outbox events.
func TestSectionMutator_NonFederatedNoOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	sections := repo.NewProjectSectionRepo(env.db)
	mut := fedsvc.NewSectionMutator(env.emitter, sections)

	id, err := mut.Create(ctx, env.plainProj, "Plain section", "sec-plain-client")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sec, err := sections.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	newTitle := "Plain renamed"
	if err := mut.Update(ctx, sec, repo.SectionUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := mut.Delete(ctx, sec); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n := outboxCount(t, env.db, env.plainProj); n != 0 {
		t.Errorf("non-federated section outbox: got %d, want 0", n)
	}
}
