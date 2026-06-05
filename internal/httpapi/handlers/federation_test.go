package handlers_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/repo"
)

const fedHandlerKey = "federation-cipher-key-32-bytes-min-padding!"

func setupFederationApp(t *testing.T) (*fiber.App, *repo.FederationKeysRepo) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "fed.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	fedKeys := repo.NewFederationKeysRepo(d)
	cipher := crypto.NewTokenCipher(fedHandlerKey)

	app := httpapi.NewApp(httpapi.Deps{})
	h := handlers.NewFederationHandler(fedKeys, cipher, "https://me.example")
	h.RegisterPublic(app)
	return app, fedKeys
}

func TestFederationWellKnown_ReachableBeforeSetupAndCarriesDisplayName(t *testing.T) {
	app, _ := setupFederationApp(t)

	// No user has been created (no setup): the .well-known endpoint must still
	// be reachable (US-2.2 AC2) — it is mounted publicly before SetupCheck/SPA.
	req := httptest.NewRequest(http.MethodGet, "/federation/.well-known/instance", http.NoBody)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("well-known status: got %d, want 200", resp.StatusCode)
	}
	var doc struct {
		InstanceURL      string `json:"instance_url"`
		PublicKey        string `json:"public_key"`
		DisplayName      string `json:"display_name"`
		ProtocolVersions []int  `json:"protocol_versions"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if doc.InstanceURL != "https://me.example" {
		t.Errorf("instance_url: got %q, want https://me.example", doc.InstanceURL)
	}
	// display_name defaults to host(BASE_URL) since users has none (US-1.4 AC2 source).
	if doc.DisplayName != "me.example" {
		t.Errorf("display_name: got %q, want me.example", doc.DisplayName)
	}
	if len(doc.ProtocolVersions) != 1 || doc.ProtocolVersions[0] != 1 {
		t.Errorf("protocol_versions: got %v, want [1]", doc.ProtocolVersions)
	}
	// The published key is a valid Ed25519 public key.
	raw, err := base64.StdEncoding.DecodeString(doc.PublicKey)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		t.Errorf("public_key not a valid ed25519 key: err=%v len=%d", err, len(raw))
	}
}

func TestFederationWellKnown_KeypairLazyGeneratedOnce(t *testing.T) {
	app, fedKeys := setupFederationApp(t)

	first := getWellKnownKey(t, app)
	second := getWellKnownKey(t, app)
	if first != second {
		t.Errorf("public key changed across requests: %q vs %q", first, second)
	}

	stored, err := fedKeys.Get(context.Background())
	if err != nil {
		t.Fatalf("get stored: %v", err)
	}
	if stored.PublicKey != first {
		t.Errorf("stored key %q != published %q", stored.PublicKey, first)
	}
	if stored.NodeID == "" {
		t.Error("node_id not generated")
	}
}

func getWellKnownKey(t *testing.T, app *fiber.App) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/federation/.well-known/instance", http.NoBody)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	var doc struct {
		PublicKey string `json:"public_key"`
	}
	body, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(body, &doc)
	return doc.PublicKey
}
