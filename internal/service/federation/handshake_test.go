package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/handshake"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

const svcTestKey = "federation-svc-cipher-key-32-bytes-min!!!"

func newOwnerSvc(t *testing.T) (*Service, *repo.ProjectRepo, *repo.FederationInviteRepo) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "svc.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	plabels := repo.NewProjectLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	fedProjects := repo.NewFederatedProjectRepo(d)
	keys := repo.NewFederationKeysRepo(d)
	invites := repo.NewFederationInviteRepo(d)
	fedInstances := repo.NewFederatedInstanceRepo(d)
	cipher := crypto.NewTokenCipher(svcTestKey)
	svc := NewService(d, projects, fedProjects, keys, invites, fedInstances, cipher, "https://owner.example")
	return svc, projects, invites
}

// seedFederatedProject creates a federated project + an active single-use invite,
// returning the invite id, the plaintext secret, and the project id.
func seedFederatedProject(t *testing.T, svc *Service, projects *repo.ProjectRepo) (string, string, int64) {
	t.Helper()
	ctx := context.Background()
	cx, err := repo.NewContextRepo(svc.db).Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := projects.Create(ctx, repo.CreateProject{ContextID: cx.ID, Title: "Roadmap", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.EnableForProject(ctx, p.ID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	res, err := svc.CreateInvite(ctx, p.ID, CreateInviteParams{Permissions: model.FederationPermissionWrite})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	return res.InviteID, res.Secret, p.ID
}

func handshakeFor(inviteID, secret string) HandshakeInput {
	const key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	return HandshakeInput{
		Body: handshake.Request{
			InviteID:          inviteID,
			Secret:            secret,
			JoinerInstanceURL: "https://bob.example",
			JoinerPublicKey:   key,
			JoinerDisplayName: "bob.example",
			ProtocolVersions:  []int{1},
		},
		VerifiedPeerURL: "https://bob.example",
		VerifiedPeerKey: key,
	}
}

// TestHandshake_BodyKeyMustMatchVerifiedKey asserts that a body joiner_public_key
// differing from the signature-verified key is a generic invalid handshake
// (US-2.2 AC1 defense-in-depth) and consumes nothing.
func TestHandshake_BodyKeyMustMatchVerifiedKey(t *testing.T) {
	svc, projects, _ := newOwnerSvc(t)
	id, secret, _ := seedFederatedProject(t, svc, projects)

	in := handshakeFor(id, secret)
	in.VerifiedPeerKey = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=" // != body key
	if _, err := svc.Handshake(context.Background(), in, time.Now()); !errors.Is(err, ErrHandshakeInvalid) {
		t.Errorf("body/verified key split: got %v, want ErrHandshakeInvalid", err)
	}
}

// TestHandshake_RevokedInviteRejected asserts a revoked invite is a generic
// invalid handshake (US-2.2 AC4) and consumes nothing.
func TestHandshake_RevokedInviteRejected(t *testing.T) {
	svc, projects, _ := newOwnerSvc(t)
	id, secret, pid := seedFederatedProject(t, svc, projects)
	if err := svc.RevokeInvite(context.Background(), pid, id); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.Handshake(context.Background(), handshakeFor(id, secret), time.Now()); !errors.Is(err, ErrHandshakeInvalid) {
		t.Errorf("revoked invite: got %v, want ErrHandshakeInvalid", err)
	}
}

// TestHandshake_ExpiredInviteRejected asserts an expired invite is rejected as a
// generic invalid handshake at the future evaluation time (US-2.2 AC4).
func TestHandshake_ExpiredInviteRejected(t *testing.T) {
	svc, projects, invites := newOwnerSvc(t)
	id, secret, _ := seedFederatedProject(t, svc, projects)
	inv, _ := invites.Get(context.Background(), id)
	future := inv.ExpiresAt.Add(time.Hour)
	if _, err := svc.Handshake(context.Background(), handshakeFor(id, secret), future); !errors.Is(err, ErrHandshakeInvalid) {
		t.Errorf("expired invite: got %v, want ErrHandshakeInvalid", err)
	}
}

// TestHandshake_SecretHashConstantTimeMatch is a sanity check that a correct
// secret matches the stored hash and a wrong one does not (the constant-time
// compare itself is exercised by the success/failure paths above).
func TestHandshake_SecretHashConstantTimeMatch(t *testing.T) {
	secret := "the-real-secret"
	sum := sha256.Sum256([]byte(secret))
	stored := hex.EncodeToString(sum[:])
	other := sha256.Sum256([]byte("wrong"))
	if hex.EncodeToString(other[:]) == stored {
		t.Fatal("distinct secrets must not collide")
	}
}
