package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/snapshottoken"
	"github.com/lebe-dev/turboist/internal/federation/transport"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// seedTask adds a task to the owner's federated project (with a context, as the
// placement CHECK requires).
func seedTask(t *testing.T, owner *fedInstance, projectID int64, title string) *model.Task {
	t.Helper()
	ctx := context.Background()
	var contextID int64
	if err := owner.db.QueryRow(`SELECT id FROM contexts ORDER BY id ASC LIMIT 1`).Scan(&contextID); err != nil {
		t.Fatalf("find context: %v", err)
	}
	tk, err := owner.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &contextID, ProjectID: &projectID},
		Title:     title,
	})
	if err != nil {
		t.Fatalf("seed task %q: %v", title, err)
	}
	return tk
}

// TestSnapshot_JoinBootstrapsLocalProject asserts the end-to-end F2.3 flow: a
// signed handshake then a signed snapshot pull create a local federated project
// on the joiner carrying the owner's tasks (US-2.3 AC2/AC3), through the REAL
// signature middleware and the buffer-first owner snapshot endpoint. A
// soft-deleted owner task is NOT resurrected on the joiner (US-2.3 AC3).
func TestSnapshot_JoinBootstrapsLocalProject(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)
	joiner := newFedInstance(t, reg, joinerURL)

	inv, projectID := owner.enableAndInvite(t, model.FederationPermissionWrite)
	seedTask(t, owner, projectID, "Snapshot task one")
	seedTask(t, owner, projectID, "Snapshot task two")
	gone := seedTask(t, owner, projectID, "Gone")
	if err := owner.tasks.Delete(context.Background(), gone.ID); err != nil {
		t.Fatalf("soft-delete owner task: %v", err)
	}

	resp, body := joiner.join(t, ownerURL, inv)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("join: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	ctx := context.Background()
	// The joiner now has a LOCAL federated project mapped to the owner.
	fp, err := joiner.fedProjects.Get(ctx, mustJoinProjectID(t, body), ownerURL)
	if err != nil {
		t.Fatalf("joiner federated mapping missing: %v", err)
	}
	if fp.IsOwner {
		t.Error("joiner mapping should be is_owner=0")
	}
	if fp.LastReceivedHLC == "" {
		t.Error("joiner mapping missing last_received_hlc (snapshot as_of not stored)")
	}

	// The two live tasks landed locally; the deleted one did NOT (no resurrection).
	localPID := mustJoinProjectID(t, body)
	tasks, _, err := joiner.tasks.ListByProject(ctx, localPID, repo.TaskFilter{}, repo.Page{Limit: 100})
	if err != nil {
		t.Fatalf("list joiner tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("joiner live tasks: got %d, want 2", len(tasks))
	}
	titles := map[string]bool{}
	for _, tk := range tasks {
		titles[tk.Title] = true
	}
	if !titles["Snapshot task one"] || !titles["Snapshot task two"] {
		t.Errorf("joiner tasks missing expected titles: %v", titles)
	}
	if titles["Gone"] {
		t.Error("soft-deleted owner task resurrected on joiner")
	}

	// The local project is marked federated and carries the owner's title.
	lp, err := joiner.projects.Get(ctx, localPID)
	if err != nil {
		t.Fatalf("load joiner project: %v", err)
	}
	if !lp.IsFederated {
		t.Error("joiner project should be is_federated")
	}
	if lp.Title != "Roadmap" {
		t.Errorf("joiner project title: got %q, want Roadmap", lp.Title)
	}
}

// TestSnapshot_ExpiredTokenRejected asserts the owner snapshot endpoint rejects an
// expired token with 401 (US-2.3 AC4), end-to-end through the real signature
// middleware. The signed snapshot GET is built exactly as the joiner builds it.
func TestSnapshot_ExpiredTokenRejected(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)
	joiner := newFedInstance(t, reg, joinerURL)

	_, projectID := owner.enableAndInvite(t, model.FederationPermissionRead)
	seedTask(t, owner, projectID, "task")

	// Mint a token that expired before now (owner's own key).
	ownerKeys, err := owner.keys.Get(context.Background())
	if err != nil {
		t.Fatalf("owner keys: %v", err)
	}
	priv, _, err := crypto.LoadInstanceKeypair(crypto.NewTokenCipher(fedHandlerKey), ownerKeys.PublicKey, ownerKeys.PrivateSeedEnc)
	if err != nil {
		t.Fatalf("load owner keypair: %v", err)
	}
	expired, err := snapshottoken.Mint(priv, projectID, time.Now().Add(-snapshottoken.TTL-time.Minute))
	if err != nil {
		t.Fatalf("mint expired token: %v", err)
	}

	resp, body := snapshotGet(t, reg, joiner, ownerURL, projectID, expired)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired token: got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

// TestSnapshot_BufferFirstDoesNotBlockWrites asserts that while a snapshot is
// being streamed from the owner, concurrent owner app writes proceed — the
// buffer-first read released the lone connection before streaming (NFR-1.4 / R1).
func TestSnapshot_BufferFirstDoesNotBlockWrites(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)
	joiner := newFedInstance(t, reg, joinerURL)

	inv, projectID := owner.enableAndInvite(t, model.FederationPermissionWrite)
	for i := 0; i < 20; i++ {
		seedTask(t, owner, projectID, "bulk")
	}

	// A full signed join (handshake + snapshot pull) completes; the snapshot
	// stream did not deadlock the single connection.
	done := make(chan struct{})
	go func() {
		resp, _ := joiner.join(t, ownerURL, inv)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("join during concurrent writes: got %d", resp.StatusCode)
		}
		close(done)
	}()

	// Concurrent owner writes proceed (they would block forever if the snapshot
	// held the connection across the whole stream).
	for i := 0; i < 5; i++ {
		seedTask(t, owner, projectID, "concurrent")
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("join/snapshot deadlocked the single connection — buffer-first violated")
	}
}

// snapshotGet signs and sends a snapshot GET exactly as the joiner does (the
// pinned transport signature with the token in the query), routing it to the
// owner app through the in-process registry and the real signature middleware.
func snapshotGet(t *testing.T, reg *fedRegistry, joiner *fedInstance, ownerURL string, projectID int64, token string) (*http.Response, []byte) {
	t.Helper()
	keys, err := joiner.keys.Ensure(context.Background(), crypto.NewTokenCipher(fedHandlerKey), "bob.example")
	if err != nil {
		t.Fatalf("joiner keys: %v", err)
	}
	priv, _, err := crypto.LoadInstanceKeypair(crypto.NewTokenCipher(fedHandlerKey), keys.PublicKey, keys.PrivateSeedEnc)
	if err != nil {
		t.Fatalf("joiner keypair: %v", err)
	}

	path := snapshotPath(projectID)
	full := ownerURL + path + "?token=" + token
	ts := model.FormatUTC(time.Now().UTC())
	nonce := "0123456789abcdef0123456789abcdef"
	ver := "1"
	digest := transport.BodyDigest(nil)
	sig := transport.SignB64(priv, transport.SignatureParams{
		Method:          "GET",
		Path:            path,
		InstanceURL:     joiner.url,
		Timestamp:       ts,
		Nonce:           nonce,
		ProtocolVersion: ver,
		BodyDigest:      digest,
	})

	req := httptest.NewRequest(http.MethodGet, full, http.NoBody)
	req.Header.Set(transport.HeaderInstance, joiner.url)
	req.Header.Set(transport.HeaderTimestamp, ts)
	req.Header.Set(transport.HeaderNonce, nonce)
	req.Header.Set(transport.HeaderProtocolVer, ver)
	req.Header.Set(transport.HeaderDigest, digest)
	req.Header.Set(transport.HeaderSignature, sig)

	ownerApp := reg.apps[ownerURL]
	resp, err := ownerApp.Test(req, fiber.TestConfig{Timeout: -1})
	if err != nil {
		t.Fatalf("snapshot GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp, out
}

func snapshotPath(projectID int64) string {
	return "/federation/projects/" + strconv.FormatInt(projectID, 10) + "/snapshot"
}

// mustJoinProjectID decodes the joiner-side local project id from a join result.
func mustJoinProjectID(t *testing.T, body []byte) int64 {
	t.Helper()
	var got struct {
		ProjectID int64 `json:"projectId"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode join result: %v (%s)", err, body)
	}
	if got.ProjectID == 0 {
		t.Fatalf("join result missing projectId: %s", body)
	}
	return got.ProjectID
}
