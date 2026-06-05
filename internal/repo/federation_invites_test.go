package repo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// TestFederationInviteRepo_CreateStoresHashNotPlaintext asserts that the invite
// secret is stored only as its SHA-256 hash (US-1.2 AC2): the schema has no
// plaintext column and the stored hash never equals the raw secret.
func TestFederationInviteRepo_CreateStoresHashNotPlaintext(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d) // context id=1 + project id=1
	r := NewFederationInviteRepo(d)

	secret := "this-is-the-256-bit-secret-in-hex-form"
	sum := sha256.Sum256([]byte(secret))
	hashHex := hex.EncodeToString(sum[:])
	exp := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	inv, err := r.Create(context.Background(), model.FederationInvite{
		InviteID:       "inv-1",
		LocalProjectID: 1,
		SecretHash:     hashHex,
		Permissions:    model.FederationPermissionWrite,
		MaxUses:        1,
		ExpiresAt:      &exp,
		CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if inv.SecretHash != hashHex {
		t.Errorf("secret_hash: got %q, want %q", inv.SecretHash, hashHex)
	}

	// US-1.2 AC2: the schema must have no plaintext-secret column.
	cols := tableColumns(t, d, "federation_invites")
	for _, col := range cols {
		switch col {
		case "secret", "invite_secret", "plaintext", "secret_plain":
			t.Errorf("federation_invites must not have a plaintext secret column, found %q", col)
		}
	}

	got, err := r.Get(context.Background(), "inv-1")
	if err != nil {
		t.Fatalf("get invite: %v", err)
	}
	if got.SecretHash == secret {
		t.Errorf("stored secret_hash equals plaintext secret — not hashed")
	}
	if got.SecretHash != hashHex {
		t.Errorf("stored secret_hash: got %q, want %q", got.SecretHash, hashHex)
	}
	if got.Permissions != model.FederationPermissionWrite {
		t.Errorf("permissions: got %q, want write", got.Permissions)
	}
	if got.MaxUses != 1 {
		t.Errorf("max_uses: got %d, want 1", got.MaxUses)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) {
		t.Errorf("expires_at: got %v, want %v", got.ExpiresAt, exp)
	}
}

// TestFederationInviteRepo_GetNotFound asserts ErrNotFound for an unknown id.
func TestFederationInviteRepo_GetNotFound(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederationInviteRepo(d)
	if _, err := r.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestFederationInviteRepo_ConsumeTx asserts ConsumeTx bumps used_count and, on
// the FINAL use, stamps consumed_at (Federation v1 F2.2, US-2.2 AC3 / US-1.2 AC3),
// inside a transaction. A single-use invite is consumed after one call; consuming
// past max_uses leaves consumed_at set.
func TestFederationInviteRepo_ConsumeTx(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederationInviteRepo(d)

	hash := func(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
	if _, err := r.Create(context.Background(), model.FederationInvite{
		InviteID: "inv-2u", LocalProjectID: 1, SecretHash: hash("s"),
		Permissions: model.FederationPermissionRead, MaxUses: 2,
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	consume := func() {
		tx, err := d.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := r.ConsumeTx(context.Background(), tx, "inv-2u", at); err != nil {
			_ = tx.Rollback()
			t.Fatalf("consume: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	// First use of a 2-use invite: used_count=1, NOT yet consumed.
	consume()
	got, _ := r.Get(context.Background(), "inv-2u")
	if got.UsedCount != 1 {
		t.Errorf("used_count after first consume: got %d, want 1", got.UsedCount)
	}
	if got.ConsumedAt != nil {
		t.Errorf("consumed_at should be nil before max_uses, got %v", got.ConsumedAt)
	}

	// Second use reaches max_uses=2: now consumed.
	consume()
	got, _ = r.Get(context.Background(), "inv-2u")
	if got.UsedCount != 2 {
		t.Errorf("used_count after second consume: got %d, want 2", got.UsedCount)
	}
	if got.ConsumedAt == nil {
		t.Error("consumed_at must be set once max_uses is reached")
	}
	if got.Status(at) != model.InviteStatusConsumed {
		t.Errorf("status after full consume: got %q, want consumed", got.Status(at))
	}
}

// TestFederationInviteRepo_ConsumeTxUnknown asserts ConsumeTx returns ErrNotFound
// for an unknown invite id.
func TestFederationInviteRepo_ConsumeTxUnknown(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederationInviteRepo(d)
	tx, _ := d.BeginTx(context.Background(), nil)
	defer func() { _ = tx.Rollback() }()
	if err := r.ConsumeTx(context.Background(), tx, "nope", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("consume unknown: got %v, want ErrNotFound", err)
	}
}

// TestFederationInviteRepo_ConsumeTxSelfGuard asserts the self-guarding UPDATE
// upholds the single-use invariant (US-1.2 AC3 / US-2.2 AC3): once a single-use
// invite is fully consumed, a further ConsumeTx matches zero rows and returns
// ErrInviteNotConsumable WITHOUT bumping used_count past max_uses. This is the
// repo-level guard the concurrent-handshake TOCTOU loser hits.
func TestFederationInviteRepo_ConsumeTxSelfGuard(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederationInviteRepo(d)

	hash := func(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
	if _, err := r.Create(context.Background(), model.FederationInvite{
		InviteID: "inv-1u", LocalProjectID: 1, SecretHash: hash("s"),
		Permissions: model.FederationPermissionRead, MaxUses: 1,
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	consume := func() error {
		tx, err := d.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if cErr := r.ConsumeTx(context.Background(), tx, "inv-1u", at); cErr != nil {
			_ = tx.Rollback()
			return cErr
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
		return nil
	}

	if err := consume(); err != nil {
		t.Fatalf("first consume: got %v, want nil", err)
	}
	// Second consume of a single-use invite must be rejected by the guard, not
	// silently bump used_count to 2.
	if err := consume(); !errors.Is(err, ErrInviteNotConsumable) {
		t.Errorf("second consume: got %v, want ErrInviteNotConsumable", err)
	}
	got, _ := r.Get(context.Background(), "inv-1u")
	if got.UsedCount != 1 {
		t.Errorf("used_count after blocked second consume: got %d, want 1", got.UsedCount)
	}
}

// TestFederationInviteRepo_ConsumeTxExpiredGuard asserts ConsumeTx refuses an
// expired invite (the WHERE expires_at > ? leg of the active guard) with
// ErrInviteNotConsumable and does not increment used_count (US-2.2 AC4).
func TestFederationInviteRepo_ConsumeTxExpiredGuard(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederationInviteRepo(d)

	hash := func(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
	exp := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := r.Create(context.Background(), model.FederationInvite{
		InviteID: "inv-exp", LocalProjectID: 1, SecretHash: hash("s"),
		Permissions: model.FederationPermissionRead, MaxUses: 1, ExpiresAt: &exp,
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	tx, _ := d.BeginTx(context.Background(), nil)
	// `at` is after expiry, so the guard must reject.
	err := r.ConsumeTx(context.Background(), tx, "inv-exp", exp.Add(time.Hour))
	_ = tx.Rollback()
	if !errors.Is(err, ErrInviteNotConsumable) {
		t.Errorf("consume expired: got %v, want ErrInviteNotConsumable", err)
	}
	got, _ := r.Get(context.Background(), "inv-exp")
	if got.UsedCount != 0 {
		t.Errorf("used_count after blocked expired consume: got %d, want 0", got.UsedCount)
	}
}

// tableColumns returns the column names of a SQLite table via PRAGMA table_info.
func tableColumns(t *testing.T, d *sql.DB, table string) []string {
	t.Helper()
	rows, err := d.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	defer logging.LogClose(context.Background(), "test.tableColumns", rows)
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	return out
}
