package federation

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/snapshot"
	"github.com/lebe-dev/turboist/internal/federation/snapshottoken"
	"github.com/lebe-dev/turboist/internal/repo"
)

const snapTestKey = "snapshot-test-federation-key-32!"

func newOwnerService(t *testing.T) (*Service, int64) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "owner.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	plabels := repo.NewProjectLabelsRepo(d)
	tlabels := repo.NewTaskLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	tasks := repo.NewTaskRepo(d, tlabels)
	contexts := repo.NewContextRepo(d)
	sections := repo.NewProjectSectionRepo(d)
	fedProjects := repo.NewFederatedProjectRepo(d)
	fedInvites := repo.NewFederationInviteRepo(d)
	fedInstances := repo.NewFederatedInstanceRepo(d)
	fedKeys := repo.NewFederationKeysRepo(d)
	cipher := crypto.NewTokenCipher(snapTestKey)

	svc := NewService(d, projects, fedProjects, fedKeys, fedInvites, fedInstances, cipher, "https://owner.example")
	svc.WithSnapshotDeps(tasks, sections, contexts, repo.NewFederationSnapshotRepo(d))

	ctx := context.Background()
	cx, err := contexts.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := projects.Create(ctx, repo.CreateProject{ContextID: cx.ID, Title: "Roadmap", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.EnableForProject(ctx, p.ID); err != nil {
		t.Fatalf("enable federation: %v", err)
	}
	if _, err := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx.ID, ProjectID: &p.ID}, Title: "Task A"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	return svc, p.ID
}

// TestBuildSnapshot_ValidToken asserts the owner builds the buffer-first snapshot
// when presented a valid 15-min token, and the NDJSON carries the project + task
// + end sentinel (US-2.3 AC2/AC3).
func TestBuildSnapshot_ValidToken(t *testing.T) {
	svc, projectID := newOwnerService(t)
	ctx := context.Background()

	tok := mintToken(t, svc, projectID, time.Now())
	snap, err := svc.BuildSnapshot(ctx, projectID, tok, time.Now())
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := snapshot.WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()
	out := buf.String()
	if !strings.Contains(out, `"type":"project"`) {
		t.Errorf("snapshot missing project line:\n%s", out)
	}
	if !strings.Contains(out, "Task A") {
		t.Errorf("snapshot missing task: %s", out)
	}
	if !strings.Contains(out, `"type":"end"`) {
		t.Errorf("snapshot missing end sentinel: %s", out)
	}
}

// TestBuildSnapshot_ExpiredToken asserts an expired snapshot token is rejected
// with ErrSnapshotTokenExpired (US-2.3 AC4 — handler maps it to 401).
func TestBuildSnapshot_ExpiredToken(t *testing.T) {
	svc, projectID := newOwnerService(t)
	ctx := context.Background()

	mintedAt := time.Now().Add(-snapshottoken.TTL - time.Minute)
	tok := mintToken(t, svc, projectID, mintedAt)

	_, err := svc.BuildSnapshot(ctx, projectID, tok, time.Now())
	if !errors.Is(err, ErrSnapshotTokenExpired) {
		t.Errorf("expired token: got %v, want ErrSnapshotTokenExpired", err)
	}
}

// TestBuildSnapshot_WrongProject asserts a token minted for a different project
// id cannot be used to snapshot another project (the embedded id must match).
func TestBuildSnapshot_WrongProject(t *testing.T) {
	svc, projectID := newOwnerService(t)
	ctx := context.Background()

	tok := mintToken(t, svc, projectID+999, time.Now())
	if _, err := svc.BuildSnapshot(ctx, projectID, tok, time.Now()); err == nil {
		t.Errorf("expected rejection for token bound to a different project")
	}
}

// TestBuildSnapshot_BadToken asserts a garbage token is rejected.
func TestBuildSnapshot_BadToken(t *testing.T) {
	svc, projectID := newOwnerService(t)
	if _, err := svc.BuildSnapshot(context.Background(), projectID, "not-a-token", time.Now()); err == nil {
		t.Errorf("expected rejection for malformed token")
	}
}

func mintToken(t *testing.T, svc *Service, projectID int64, now time.Time) string {
	t.Helper()
	keys, err := svc.keys.Get(context.Background())
	if err != nil {
		t.Fatalf("load keys: %v", err)
	}
	priv, _, err := crypto.LoadInstanceKeypair(svc.cipher, keys.PublicKey, keys.PrivateSeedEnc)
	if err != nil {
		t.Fatalf("load keypair: %v", err)
	}
	tok, err := snapshottoken.Mint(priv, projectID, now)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return tok
}
