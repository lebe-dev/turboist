package handlers_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func TestBackupHandler_ExportReturnsGzippedAttachment(t *testing.T) {
	env := setupAPIEnv(t)

	req := env.authedReq(t, http.MethodGet, "/api/v1/backup", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type: got %q, want application/json", ct)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Errorf("content-encoding: got %q, want gzip", enc)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "attachment;") || !strings.Contains(cd, "turboist-backup-") || !strings.Contains(cd, ".json") {
		t.Errorf("content-disposition: got %q", cd)
	}

	// Fiber's app.Test does not auto-decompress; do it manually to verify the body.
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = zr.Close() }()
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var payload service.BackupPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.Version != service.BackupSchemaVersion {
		t.Errorf("version: got %d, want %d", payload.Version, service.BackupSchemaVersion)
	}
	if payload.Settings != nil {
		t.Errorf("settings must be absent without ?settings=1")
	}
}

func TestBackupHandler_ExportIncludesSettingsToggle(t *testing.T) {
	env := setupAPIEnv(t)

	req := env.authedReq(t, http.MethodGet, "/api/v1/backup?settings=1", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	body, err := io.ReadAll(zr)
	_ = zr.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var payload service.BackupPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.Settings == nil {
		t.Fatal("settings must be present when ?settings=1")
	}
}

func TestBackupHandler_RestoreAcceptsPlainAndGzip(t *testing.T) {
	env := setupAPIEnv(t)
	// Seed something so the export is non-empty and round-trip is observable.
	svc := service.NewBackupService(env.db)
	payload, err := svc.Export(t.Context(), service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, err := payload.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Plain JSON body.
	plainReq := httptest.NewRequest(http.MethodPost, "/api/v1/restore", bytes.NewReader(raw))
	plainReq.Header.Set("Content-Type", "application/json")
	plainReq.Header.Set("Authorization", "Bearer "+env.token(t))
	plainResp, err := env.app.Test(plainReq)
	if err != nil {
		t.Fatal(err)
	}
	if plainResp.StatusCode != http.StatusNoContent {
		t.Errorf("plain restore: got %d, want %d", plainResp.StatusCode, http.StatusNoContent)
	}

	// Gzipped body.
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = zw.Write(raw)
	_ = zw.Close()
	gzReq := httptest.NewRequest(http.MethodPost, "/api/v1/restore", &buf)
	gzReq.Header.Set("Content-Type", "application/octet-stream")
	gzReq.Header.Set("Authorization", "Bearer "+env.token(t))
	gzResp, err := env.app.Test(gzReq)
	if err != nil {
		t.Fatal(err)
	}
	if gzResp.StatusCode != http.StatusNoContent {
		t.Errorf("gzip restore: got %d, want %d", gzResp.StatusCode, http.StatusNoContent)
	}
}

func TestBackupHandler_RestoreRejectsBadJSON(t *testing.T) {
	env := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/restore", strings.NewReader("not a backup"))
	req.Header.Set("Authorization", "Bearer "+env.token(t))
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestBackupHandler_ExportRequiresAuth(t *testing.T) {
	env := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backup", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestBackupHandler_RestoreRequiresAuth(t *testing.T) {
	env := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/restore", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestBackupHandler_RestoreRejectsEmptyBody(t *testing.T) {
	env := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/restore", nil)
	req.Header.Set("Authorization", "Bearer "+env.token(t))
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestBackupHandler_ExportRoundTripViaHTTP(t *testing.T) {
	env := setupAPIEnv(t)

	ctx := t.Context()
	work, err := env.ctxs.Create(ctx, "work", "blue", true)
	if err != nil {
		t.Fatalf("seed context: %v", err)
	}
	if _, err := env.labels.Create(ctx, "urgent", "red", true); err != nil {
		t.Fatalf("seed label: %v", err)
	}
	if _, err := env.projects.Create(ctx, repo.CreateProject{ContextID: work.ID, Title: "Q3", Color: "purple"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	exportReq := env.authedReq(t, http.MethodGet, "/api/v1/backup?settings=1", nil)
	exportResp, err := env.app.Test(exportReq)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if exportResp.StatusCode != http.StatusOK {
		t.Fatalf("export status: got %d, want %d", exportResp.StatusCode, http.StatusOK)
	}

	zr, err := gzip.NewReader(exportResp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	raw, err := io.ReadAll(zr)
	_ = zr.Close()
	if err != nil {
		t.Fatalf("read export: %v", err)
	}

	// Server-side decode validates the payload survives the network format.
	payload, err := service.DecodeBackup(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Version != service.BackupSchemaVersion {
		t.Errorf("version: got %d, want %d", payload.Version, service.BackupSchemaVersion)
	}
	if got := len(payload.Data.Contexts); got != 1 {
		t.Errorf("contexts: got %d, want 1", got)
	}
	if got := len(payload.Data.Labels); got != 1 {
		t.Errorf("labels: got %d, want 1", got)
	}
	if got := len(payload.Data.Projects); got != 1 {
		t.Errorf("projects: got %d, want 1", got)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/restore", bytes.NewReader(raw))
	restoreReq.Header.Set("Content-Type", "application/json")
	restoreReq.Header.Set("Authorization", "Bearer "+env.token(t))
	restoreResp, err := env.app.Test(restoreReq)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restoreResp.StatusCode != http.StatusNoContent {
		t.Errorf("restore status: got %d, want %d", restoreResp.StatusCode, http.StatusNoContent)
	}

	ctxs, _, err := env.ctxs.List(ctx, repo.Page{})
	if err != nil {
		t.Fatalf("list contexts: %v", err)
	}
	if len(ctxs) != 1 || ctxs[0].Name != "work" {
		t.Errorf("contexts after restore: got %+v, want one named 'work'", ctxs)
	}
}
