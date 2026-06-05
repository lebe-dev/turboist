package inbox_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TestDBValidator_ResolvesSeededMembership asserts the production wiring resolves
// a real federated_projects row (the seeded write peer in applyEnv) and accepts a
// correctly-signed, in-window event — proving NewDBValidator threads the real
// peer-key cache + repo membership through Validate end to end.
func TestDBValidator_ResolvesSeededMembership(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	// Warm the peer-key cache with the author's published key (Put, as a handshake
	// would) so the resolver never hits the network.
	cache := peerkeys.NewCache(nil)
	if err := cache.Put("https://alice.example", base64.StdEncoding.EncodeToString(pub), "Alice"); err != nil {
		t.Fatalf("warm cache: %v", err)
	}

	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	v := inbox.NewDBValidator(env.db, repo.NewFederatedProjectRepo(env.db), cache, func() time.Time { return now })

	e := updateEvent(env, map[string]events.Field{
		"title": {Value: "Wired", HLC: hlcAt(now)},
	})
	e.EventID = eventID("wired")
	signed, err := events.Sign(e, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	res, err := v.Validate(ctx, signed, "https://alice.example")
	if err != nil {
		t.Fatalf("seeded write peer must validate: %v", err)
	}
	if res.LocalProjectID != env.projectID {
		t.Errorf("local project id: got %d, want %d", res.LocalProjectID, env.projectID)
	}
}

// TestDBValidator_UnknownProjectIsNotMember asserts an event whose
// project_client_id maps to no local federated project is rejected as ErrNotMember
// (a probing peer cannot distinguish "no such project" from "not a member").
func TestDBValidator_UnknownProjectIsNotMember(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	cache := peerkeys.NewCache(nil)
	if err := cache.Put("https://alice.example", base64.StdEncoding.EncodeToString(pub), "Alice"); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	v := inbox.NewDBValidator(env.db, repo.NewFederatedProjectRepo(env.db), cache, func() time.Time { return now })

	e := updateEvent(env, map[string]events.Field{
		"title": {Value: "Nowhere", HLC: hlcAt(now)},
	})
	e.ProjectClientID = "no-such-project"
	e.EventID = eventID("unknown-project")
	signed, err := events.Sign(e, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = v.Validate(ctx, signed, "https://alice.example")
	if !errors.Is(err, inbox.ErrNotMember) {
		t.Fatalf("unknown project must be ErrNotMember, got %v", err)
	}
}

// TestDBValidator_UnknownPeerIsNotMember asserts an event from an instance with no
// federated_projects row for the project is rejected as ErrNotMember.
func TestDBValidator_UnknownPeerIsNotMember(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	cache := peerkeys.NewCache(nil)
	if err := cache.Put("https://mallory.example", base64.StdEncoding.EncodeToString(pub), "Mallory"); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	v := inbox.NewDBValidator(env.db, repo.NewFederatedProjectRepo(env.db), cache, func() time.Time { return now })

	e := events.Event{
		EventID:         eventID("unknown-peer"),
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        env.taskClientID,
		ProjectClientID: env.projectClient,
		Author:          "https://mallory.example",
		OriginInstance:  "https://mallory.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			"title": {Value: "Intruder", HLC: hlcAt(now)},
		},
	}
	signed, err := events.Sign(e, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = v.Validate(ctx, signed, "https://mallory.example")
	if !errors.Is(err, inbox.ErrNotMember) {
		t.Fatalf("event from a non-peer instance must be ErrNotMember, got %v", err)
	}
}
