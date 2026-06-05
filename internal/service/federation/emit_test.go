package federation_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// emitEnv wires the emitter against a migrated DB with one federated project and
// one non-federated project.
type emitEnv struct {
	db          *sql.DB
	emitter     *fedsvc.Emitter
	tasks       *repo.TaskRepo
	fedProject  int64
	plainProj   int64
	fedTaskID   int64
	plainTaskID int64
}

func newEmitEnv(t *testing.T) *emitEnv {
	t.Helper()
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	ctx := context.Background()

	// Ensure a keypair + node_id exist so events can be signed.
	if _, err := keys.Ensure(ctx, crypto.NewTokenCipher(fedSvcKey), "me"); err != nil {
		t.Fatalf("ensure keys: %v", err)
	}

	cx := int64(1)
	fp, err := projects.Create(ctx, repo.CreateProject{ContextID: cx, Title: "Shared", Color: "blue"})
	if err != nil {
		t.Fatalf("create fed project: %v", err)
	}
	pp, err := projects.Create(ctx, repo.CreateProject{ContextID: cx, Title: "Private", Color: "red"})
	if err != nil {
		t.Fatalf("create plain project: %v", err)
	}

	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), "https://me.example")
	if _, err := svc.EnableForProject(ctx, fp.ID); err != nil {
		t.Fatalf("enable: %v", err)
	}

	tasks := repo.NewTaskRepo(d, repo.NewTaskLabelsRepo(d))
	ft, err := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &fp.ID}, Title: "Fed task"})
	if err != nil {
		t.Fatalf("create fed task: %v", err)
	}
	pt, err := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &pp.ID}, Title: "Plain task"})
	if err != nil {
		t.Fatalf("create plain task: %v", err)
	}

	emitter := fedsvc.NewEmitter(
		d, keys, crypto.NewTokenCipher(fedSvcKey),
		hlc.NewStore(d, mustNodeID(t, keys)),
		"https://me.example",
	)
	return &emitEnv{
		db:          d,
		emitter:     emitter,
		tasks:       tasks,
		fedProject:  fp.ID,
		plainProj:   pp.ID,
		fedTaskID:   ft.ID,
		plainTaskID: pt.ID,
	}
}

func mustNodeID(t *testing.T, keys *repo.FederationKeysRepo) string {
	t.Helper()
	k, err := keys.Get(context.Background())
	if err != nil {
		t.Fatalf("get keys: %v", err)
	}
	return k.NodeID
}

func outboxCount(t *testing.T, d *sql.DB, projectID int64) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federation_outbox WHERE local_project_id = ?`, projectID).Scan(&n); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return n
}

// TestEmitMutation_FederatedWritesOutboxAndFieldHLC asserts that a mutation on a
// task in a FEDERATED project, run through EmitMutation, performs the domain
// write, bumps entity_field_hlc, and writes ONE canonical signed event to
// federation_outbox — all in one transaction (US-3.2 AC1).
func TestEmitMutation_FederatedWritesOutboxAndFieldHLC(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()

	clientID := taskClientID(t, env.db, env.fedTaskID)
	err := env.emitter.EmitMutation(ctx, fedsvc.MutationSpec{
		LocalProjectID: env.fedProject,
		EntityType:     events.EntityTask,
		EntityID:       clientID,
		Op:             events.OpUpdate,
		Fields:         map[string]any{"title": "Renamed via emit"},
	}, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET title = ? WHERE id = ?`, "Renamed via emit", env.fedTaskID)
		return err
	})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	// Domain write happened.
	tk, err := env.tasks.Get(ctx, env.fedTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if tk.Title != "Renamed via emit" {
		t.Errorf("title: got %q, want Renamed via emit", tk.Title)
	}

	// Exactly one outbox event for the federated project.
	if got := outboxCount(t, env.db, env.fedProject); got != 1 {
		t.Errorf("outbox count: got %d, want 1", got)
	}

	// entity_field_hlc bumped for the title field.
	var fieldHLC string
	if err := env.db.QueryRow(
		`SELECT hlc FROM entity_field_hlc WHERE entity_type = 'task' AND entity_id = ? AND field_name = 'title'`,
		clientID).Scan(&fieldHLC); err != nil {
		t.Fatalf("field_hlc not written: %v", err)
	}
	if _, err := hlc.Parse(fieldHLC); err != nil {
		t.Errorf("field_hlc not canonical: %q (%v)", fieldHLC, err)
	}

	// The outbox payload is a verifiable signed event.
	payload := outboxPayload(t, env.db, env.fedProject)
	var evt events.Event
	if err := events.Unmarshal([]byte(payload), &evt); err != nil {
		t.Fatalf("decode outbox payload: %v", err)
	}
	if evt.Signature == "" {
		t.Errorf("outbox event must be signed")
	}
	if evt.OriginInstance != "https://me.example" {
		t.Errorf("origin: got %q, want https://me.example", evt.OriginInstance)
	}
}

// TestEmitMutation_NonFederatedWritesNoOutbox asserts that a mutation on a task
// in a NON-federated project performs the domain write but writes NOTHING to the
// outbox and bumps NO field HLC — federation is a scoped overlay (US-3.2 AC1).
func TestEmitMutation_NonFederatedWritesNoOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()

	clientID := taskClientID(t, env.db, env.plainTaskID)
	err := env.emitter.EmitMutation(ctx, fedsvc.MutationSpec{
		LocalProjectID: env.plainProj,
		EntityType:     events.EntityTask,
		EntityID:       clientID,
		Op:             events.OpUpdate,
		Fields:         map[string]any{"title": "Renamed plain"},
	}, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET title = ? WHERE id = ?`, "Renamed plain", env.plainTaskID)
		return err
	})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	// Domain write still happened.
	tk, err := env.tasks.Get(ctx, env.plainTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if tk.Title != "Renamed plain" {
		t.Errorf("title: got %q, want Renamed plain", tk.Title)
	}

	// No outbox event, no field HLC.
	if got := outboxCount(t, env.db, env.plainProj); got != 0 {
		t.Errorf("non-federated outbox count: got %d, want 0", got)
	}
	var n int
	if err := env.db.QueryRow(
		`SELECT COUNT(*) FROM entity_field_hlc WHERE entity_id = ?`, clientID).Scan(&n); err != nil {
		t.Fatalf("count field_hlc: %v", err)
	}
	if n != 0 {
		t.Errorf("non-federated field_hlc count: got %d, want 0", n)
	}
}

// TestEmitMutation_CommitPingFires asserts the commit-ping callback fires exactly
// once after a federated event is committed (the immediate-push trigger, NFR-1.1)
// and does NOT fire for a non-federated mutation (no outbox event to push).
func TestEmitMutation_CommitPingFires(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()

	pings := 0
	env.emitter.WithCommitPing(func() { pings++ })

	// Federated mutation → one ping.
	fedClient := taskClientID(t, env.db, env.fedTaskID)
	if err := env.emitter.EmitMutation(ctx, fedsvc.MutationSpec{
		LocalProjectID: env.fedProject,
		EntityType:     events.EntityTask,
		EntityID:       fedClient,
		Op:             events.OpUpdate,
		Fields:         map[string]any{"title": "ping me"},
	}, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET title = ? WHERE id = ?`, "ping me", env.fedTaskID)
		return err
	}); err != nil {
		t.Fatalf("emit federated: %v", err)
	}
	if pings != 1 {
		t.Errorf("federated commit ping: got %d, want 1", pings)
	}

	// Non-federated mutation → no ping (nothing to publish).
	plainClient := taskClientID(t, env.db, env.plainTaskID)
	if err := env.emitter.EmitMutation(ctx, fedsvc.MutationSpec{
		LocalProjectID: env.plainProj,
		EntityType:     events.EntityTask,
		EntityID:       plainClient,
		Op:             events.OpUpdate,
		Fields:         map[string]any{"title": "no ping"},
	}, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET title = ? WHERE id = ?`, "no ping", env.plainTaskID)
		return err
	}); err != nil {
		t.Fatalf("emit plain: %v", err)
	}
	if pings != 1 {
		t.Errorf("non-federated must not ping: got %d, want 1", pings)
	}
}

func taskClientID(t *testing.T, d *sql.DB, id int64) string {
	t.Helper()
	var c string
	if err := d.QueryRow(`SELECT client_id FROM tasks WHERE id = ?`, id).Scan(&c); err != nil {
		t.Fatalf("client_id: %v", err)
	}
	return c
}

func outboxPayload(t *testing.T, d *sql.DB, projectID int64) string {
	t.Helper()
	var p string
	if err := d.QueryRow(`SELECT payload FROM federation_outbox WHERE local_project_id = ? ORDER BY id DESC LIMIT 1`, projectID).Scan(&p); err != nil {
		t.Fatalf("outbox payload: %v", err)
	}
	return p
}
