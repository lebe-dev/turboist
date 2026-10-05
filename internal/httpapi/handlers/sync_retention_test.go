package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// rawAuthedReq posts bytes verbatim. The backup endpoints exchange a whole file
// rather than a JSON object, so the body cannot go through authedReq's marshal.
func rawAuthedReq(t *testing.T, e *apiEnv, method, url string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token(t))
	return req
}

type syncErrorResp struct {
	Error struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

func parseSyncError(t *testing.T, body []byte) syncErrorResp {
	t.Helper()
	var out syncErrorResp
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse error body: %v", err)
	}
	return out
}

// backdateChangeLog ages the log rows up to seq, standing in for history that
// has been sitting on the server past the retention window.
func backdateChangeLog(t *testing.T, e *apiEnv, upToSeq int64, at time.Time) {
	t.Helper()
	if _, err := e.db.Exec(`UPDATE change_log SET changed_at = ? WHERE seq <= ?`,
		model.FormatUTC(at), upToSeq); err != nil {
		t.Fatalf("backdate change log: %v", err)
	}
}

// A restore replaces the dataset wholesale, so a client resuming from a cursor
// it took beforehand would carry over rows the backup never contained. The
// round trip through the real endpoints proves the refusal is wired end to end.
func TestSyncChanges_EpochMismatchAfterBackupRestore(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	createTestTask(t, e, c.ID, "Write the report")

	before := fetchChanges(t, e, 0, "limit=500")

	resp, archive := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/backup?settings=1", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("export backup: got %d, want 200", resp.StatusCode)
	}
	resp, body := doReq(t, e.app, rawAuthedReq(t, e, http.MethodPost, "/api/v1/restore", archive))
	if resp.StatusCode != 204 {
		t.Fatalf("restore backup: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	url := syncURL(before.Cursor, "epoch=1")
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, url, nil))
	if resp.StatusCode != 409 {
		t.Fatalf("got %d, want 409; body: %s", resp.StatusCode, body)
	}
	out := parseSyncError(t, body)
	if out.Error.Code != httpapi.CodeSyncEpochMismatch {
		t.Errorf("code: got %q, want %q", out.Error.Code, httpapi.CodeSyncEpochMismatch)
	}
	if got := out.Error.Details["epoch"]; got != float64(before.Epoch+1) {
		t.Errorf("details.epoch: got %v, want %d", got, before.Epoch+1)
	}
}

// The restored workspace is served from a snapshot instead, which starts the
// client on the new epoch with a cursor that resumes cleanly.
func TestSyncSnapshot_AfterRestoreCarriesTheNewEpoch(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	createTestTask(t, e, c.ID, "Write the report")

	resp, archive := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/backup?settings=1", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("export backup: got %d, want 200", resp.StatusCode)
	}
	resp, body := doReq(t, e.app, rawAuthedReq(t, e, http.MethodPost, "/api/v1/restore", archive))
	if resp.StatusCode != 204 {
		t.Fatalf("restore backup: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	snap, _ := fetchSnapshot(t, e)
	if snap.Epoch != 2 {
		t.Errorf("snapshot epoch: got %d, want 2", snap.Epoch)
	}
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, syncURL(snap.Cursor, "epoch=2"), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("resume after restore: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// Retention is the other way a cursor stops being answerable: the changes it
// still needs were trimmed away months after they were written.
func TestSyncChanges_CursorExpiredAfterRetentionPrune(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()
	c := createTestContext(t, e, "Work")
	createTestTask(t, e, c.ID, "Ancient news")
	stale := currentSeq(t, e)
	createTestTask(t, e, c.ID, "Recent news")

	backdateChangeLog(t, e, stale, time.Now().Add(-repo.SyncHistoryWindow-24*time.Hour))
	removed, err := repo.NewChangeLogRepo(e.db).Prune(ctx, time.Now().Add(-repo.SyncHistoryWindow))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed == 0 {
		t.Fatal("precondition: prune removed nothing")
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, syncURL(1, ""), nil))
	if resp.StatusCode != 410 {
		t.Fatalf("ancient cursor: got %d, want 410; body: %s", resp.StatusCode, body)
	}
	out := parseSyncError(t, body)
	if out.Error.Code != httpapi.CodeSyncCursorExpired {
		t.Errorf("code: got %q, want %q", out.Error.Code, httpapi.CodeSyncCursorExpired)
	}
	if got := out.Error.Details["oldestRetained"]; got != float64(stale+1) {
		t.Errorf("details.oldestRetained: got %v, want %d", got, stale+1)
	}

	// The client that was already up to date lost nothing and keeps resuming:
	// routine retention must not cost every replica a full reseed.
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, syncURL(stale, ""), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("up-to-date cursor: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// A client that omits the epoch is not let off either: the restore's own wipe
// and re-inserts push the sequence past every cursor handed out before it, and
// the truncate then leaves the boundary above them all.
func TestSyncChanges_PreRestoreCursorExpiresWithoutAnEpoch(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	createTestTask(t, e, c.ID, "Write the report")
	before := currentSeq(t, e)

	resp, archive := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/backup?settings=1", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("export backup: got %d, want 200", resp.StatusCode)
	}
	resp, body := doReq(t, e.app, rawAuthedReq(t, e, http.MethodPost, "/api/v1/restore", archive))
	if resp.StatusCode != 204 {
		t.Fatalf("restore backup: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, syncURL(before, ""), nil))
	if resp.StatusCode != 410 {
		t.Fatalf("pre-restore cursor: got %d, want 410; body: %s", resp.StatusCode, body)
	}
	if got := parseSyncError(t, body).Error.Code; got != httpapi.CodeSyncCursorExpired {
		t.Errorf("code: got %q, want %q", got, httpapi.CodeSyncCursorExpired)
	}
}
