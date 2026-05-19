package service_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

type backupFixtures struct {
	db       *sql.DB
	svc      *service.BackupService
	ctxs     *repo.ContextRepo
	labels   *repo.LabelRepo
	projects *repo.ProjectRepo
	sections *repo.ProjectSectionRepo
	tasks    *repo.TaskRepo
	tlabels  *repo.TaskLabelsRepo
	plabels  *repo.ProjectLabelsRepo
	users    *repo.UserRepo
	appSet   *repo.AppSettingsRepo
}

func setupBackupFixtures(t *testing.T) *backupFixtures {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	plabels := repo.NewProjectLabelsRepo(d)
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return &backupFixtures{
		db:       d,
		svc:      service.NewBackupService(d),
		ctxs:     repo.NewContextRepo(d),
		labels:   repo.NewLabelRepo(d),
		projects: repo.NewProjectRepo(d, plabels),
		sections: repo.NewProjectSectionRepo(d),
		tasks:    repo.NewTaskRepo(d, tlabels),
		tlabels:  tlabels,
		plabels:  plabels,
		users:    users,
		appSet:   repo.NewAppSettingsRepo(d),
	}
}

// seedSample creates a small but representative dataset that exercises the
// nullable / pointer fields, label associations, sections and the inbox vs
// context placement branches.
func seedSample(t *testing.T, f *backupFixtures) {
	t.Helper()
	ctx := context.Background()

	work, err := f.ctxs.Create(ctx, "work", "blue", true)
	if err != nil {
		t.Fatalf("ctx: %v", err)
	}
	urgent, err := f.labels.Create(ctx, "urgent", "red", true)
	if err != nil {
		t.Fatalf("label urgent: %v", err)
	}
	billing, err := f.labels.Create(ctx, "billing", "green", false)
	if err != nil {
		t.Fatalf("label billing: %v", err)
	}

	proj, err := f.projects.Create(ctx, repo.CreateProject{ContextID: work.ID, Title: "Q3 roadmap", Color: "purple"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if err := f.plabels.SetForProject(ctx, proj.ID, []int64{urgent.ID}); err != nil {
		t.Fatalf("project labels: %v", err)
	}

	sec, err := f.sections.Create(ctx, proj.ID, "Backend")
	if err != nil {
		t.Fatalf("section: %v", err)
	}

	// project task with a label
	pt, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{
			ContextID: ptr(work.ID),
			ProjectID: ptr(proj.ID),
			SectionID: ptr(sec.ID),
		},
		Title:    "wire backup endpoint",
		Priority: model.PriorityHigh,
	})
	if err != nil {
		t.Fatalf("project task: %v", err)
	}
	if err := f.tlabels.SetForTask(ctx, pt.ID, []int64{urgent.ID, billing.ID}); err != nil {
		t.Fatalf("task labels: %v", err)
	}

	// inbox task to exercise the (inbox_id IS NOT NULL XOR context_id) branch
	if _, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{InboxID: ptr(int64(1))},
		Title:     "think about this later",
	}); err != nil {
		t.Fatalf("inbox task: %v", err)
	}
}

func TestBackupService_RoundTrip(t *testing.T) {
	f := setupBackupFixtures(t)
	seedSample(t, f)
	ctx := context.Background()

	first, err := f.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	rawFirst, err := first.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded, err := service.DecodeBackup(rawFirst)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Restore into a fresh DB and verify the export matches byte-for-byte
	// (modulo ExportedAt which is regenerated on each call).
	f2 := setupBackupFixtures(t)
	if err := f2.svc.Restore(ctx, decoded); err != nil {
		t.Fatalf("restore: %v", err)
	}
	second, err := f2.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	second.ExportedAt = first.ExportedAt

	rawSecond, err := second.Marshal()
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if !bytes.Equal(rawFirst, rawSecond) {
		t.Errorf("round-trip mismatch:\nfirst=%s\nsecond=%s", rawFirst, rawSecond)
	}
}

func TestBackupService_RestoreReplacesExistingData(t *testing.T) {
	src := setupBackupFixtures(t)
	seedSample(t, src)
	ctx := context.Background()
	payload, err := src.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export source: %v", err)
	}

	dst := setupBackupFixtures(t)
	if _, err := dst.ctxs.Create(ctx, "personal", "green", false); err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	if _, err := dst.labels.Create(ctx, "old", "grey", false); err != nil {
		t.Fatalf("seed dst label: %v", err)
	}
	if err := dst.svc.Restore(ctx, payload); err != nil {
		t.Fatalf("restore: %v", err)
	}

	got, err := dst.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("verify export: %v", err)
	}
	if len(got.Data.Contexts) != 1 || got.Data.Contexts[0].Name != "work" {
		t.Errorf("contexts after restore: got %#v, want single context 'work'", got.Data.Contexts)
	}
	for _, l := range got.Data.Labels {
		if l.Name == "old" {
			t.Errorf("stale label survived wipe: %v", l)
		}
	}
}

func TestBackupService_SettingsToggle(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()
	if err := f.users.SetSettings(ctx, 1, &model.UserSettings{
		Locale:     "ru",
		PublicView: true,
		BannerText: "hello",
	}); err != nil {
		t.Fatalf("set user settings: %v", err)
	}
	if err := f.appSet.Set(ctx, &model.AppSettings{AutoLabels: []model.AutoLabelRule{
		{Mask: "fix", LabelIDs: []int64{}, IgnoreCase: true},
	}}); err != nil {
		t.Fatalf("set app settings: %v", err)
	}

	off, err := f.svc.Export(ctx, service.ExportOptions{IncludeSettings: false})
	if err != nil {
		t.Fatalf("export off: %v", err)
	}
	if off.Settings != nil {
		t.Errorf("settings included when toggle off: %#v", off.Settings)
	}

	on, err := f.svc.Export(ctx, service.ExportOptions{IncludeSettings: true})
	if err != nil {
		t.Fatalf("export on: %v", err)
	}
	if on.Settings == nil || on.Settings.User == nil || on.Settings.User.Locale != "ru" {
		t.Errorf("user settings missing or wrong: %#v", on.Settings)
	}
	if on.Settings.App == nil || len(on.Settings.App.AutoLabels) != 1 {
		t.Errorf("app settings missing: %#v", on.Settings)
	}
}

func TestBackupService_RestoreAppliesSettings(t *testing.T) {
	src := setupBackupFixtures(t)
	ctx := context.Background()
	if err := src.users.SetSettings(ctx, 1, &model.UserSettings{
		Locale:     "ru",
		BannerText: "from backup",
	}); err != nil {
		t.Fatalf("set src user settings: %v", err)
	}
	if err := src.appSet.Set(ctx, &model.AppSettings{AutoLabels: []model.AutoLabelRule{}}); err != nil {
		t.Fatalf("set src app settings: %v", err)
	}
	payload, err := src.svc.Export(ctx, service.ExportOptions{IncludeSettings: true})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	dst := setupBackupFixtures(t)
	if err := dst.svc.Restore(ctx, payload); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := dst.users.GetSettings(ctx, 1)
	if err != nil {
		t.Fatalf("get dst settings: %v", err)
	}
	if got.Locale != "ru" || got.BannerText != "from backup" {
		t.Errorf("settings not applied: %#v", got)
	}
}

func TestDecodeBackup_AcceptsGzip(t *testing.T) {
	src := setupBackupFixtures(t)
	seedSample(t, src)
	payload, err := src.svc.Export(context.Background(), service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, err := payload.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	if _, err := zw.Write(raw); err != nil {
		t.Fatalf("write gzip: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	decoded, err := service.DecodeBackup(buf.Bytes())
	if err != nil {
		t.Fatalf("decode gzipped: %v", err)
	}
	if len(decoded.Data.Tasks) != len(payload.Data.Tasks) {
		t.Errorf("decoded tasks count: got %d, want %d", len(decoded.Data.Tasks), len(payload.Data.Tasks))
	}
}

func TestDecodeBackup_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{name: "empty", body: []byte("")},
		{name: "non-json", body: []byte("not a json")},
		{name: "wrong version", body: mustJSON(t, service.BackupPayload{Version: 999})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.DecodeBackup(tc.body); err == nil {
				t.Fatal("want error, got nil")
			} else if !errors.Is(err, service.ErrBadBackup) {
				t.Errorf("err not ErrBadBackup: %v", err)
			}
		})
	}
}

func TestBackupService_RestoreSanitizesDanglingRefs(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()

	// Payload with three pathological tasks:
	//   - one references a non-existent project (project_id should be NULL'd,
	//     but it also lacks both inbox and context → must be dropped)
	//   - one references a non-existent parent (parent_id NULL'd, stays)
	//   - one is well-formed (acts as the dropped one's would-be parent)
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			Tasks: []service.BackupTask{
				{
					ID: 1, Title: "ghost project", ProjectID: ptr(int64(424242)),
					Priority: string(model.PriorityNone), Status: string(model.TaskStatusOpen),
					DayPart: string(model.DayPartNone), PlanState: string(model.PlanStateNone),
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z",
				},
				{
					ID: 2, Title: "ghost parent", InboxID: ptr(int64(1)), ParentID: ptr(int64(999)),
					Priority: string(model.PriorityNone), Status: string(model.TaskStatusOpen),
					DayPart: string(model.DayPartNone), PlanState: string(model.PlanStateNone),
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z",
				},
			},
		},
	}
	if err := f.svc.Restore(ctx, bad); err != nil {
		t.Fatalf("restore should heal dangling refs, got: %v", err)
	}
	got, err := f.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(got.Data.Tasks) != 1 {
		t.Fatalf("tasks after sanitize: got %d, want 1", len(got.Data.Tasks))
	}
	survivor := got.Data.Tasks[0]
	if survivor.ID != 2 {
		t.Errorf("survivor id: got %d, want 2", survivor.ID)
	}
	if survivor.ParentID != nil {
		t.Errorf("survivor parent_id: got %v, want nil (healed)", *survivor.ParentID)
	}
}

func TestBackupService_RestoreRollsBackOnInsertError(t *testing.T) {
	f := setupBackupFixtures(t)
	seedSample(t, f)
	ctx := context.Background()
	beforeCount := countTasks(t, f.db)
	if beforeCount == 0 {
		t.Fatal("precondition: sample seed yielded no tasks")
	}

	// Two contexts with the same name violate the UNIQUE(name) index. The
	// failure happens mid-transaction and proves wipe+insert rolls back.
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			Contexts: []service.BackupContext{
				{ID: 1, Name: "dup", Color: "blue", CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
				{ID: 2, Name: "dup", Color: "red", CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
			},
		},
	}
	if err := f.svc.Restore(ctx, bad); err == nil {
		t.Fatal("want restore to fail on duplicate context name")
	}
	if got := countTasks(t, f.db); got != beforeCount {
		t.Errorf("tasks after failed restore: got %d, want %d (rollback failed)", got, beforeCount)
	}
}

func TestBackupService_ExportReturnsErrorOnCorruptedUserSettings(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()
	// Inject malformed JSON directly into the users.settings column to bypass
	// the repo's marshalling. The export path must surface this rather than
	// silently substituting an empty UserSettings.
	if _, err := f.db.ExecContext(ctx,
		`UPDATE users SET settings = ? WHERE id = 1`, `{not valid json`); err != nil {
		t.Fatalf("inject bad user settings: %v", err)
	}
	_, err := f.svc.Export(ctx, service.ExportOptions{IncludeSettings: true})
	if err == nil {
		t.Fatal("want error from corrupted user settings, got nil")
	}
}

func TestBackupService_ExportReturnsErrorOnCorruptedAppSettings(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()
	// Seed an app_settings row with malformed JSON. Export must fail rather
	// than masking the corruption with an empty AppSettings.
	if _, err := f.db.ExecContext(ctx,
		`INSERT INTO app_settings (id, data) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET data = excluded.data`, `{"AutoLabels":`); err != nil {
		t.Fatalf("inject bad app settings: %v", err)
	}
	_, err := f.svc.Export(ctx, service.ExportOptions{IncludeSettings: true})
	if err == nil {
		t.Fatal("want error from corrupted app settings, got nil")
	}
}

func TestBackupService_ExportHandlesEmptyUserSettings(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()
	// Empty string and "{}" must remain valid — the corrupted-settings guard
	// must not regress these "no settings yet" cases.
	for _, raw := range []string{"", "{}"} {
		if _, err := f.db.ExecContext(ctx,
			`UPDATE users SET settings = ? WHERE id = 1`, raw); err != nil {
			t.Fatalf("set settings to %q: %v", raw, err)
		}
		payload, err := f.svc.Export(ctx, service.ExportOptions{IncludeSettings: true})
		if err != nil {
			t.Fatalf("export with settings=%q: %v", raw, err)
		}
		if payload.Settings == nil || payload.Settings.User == nil {
			t.Errorf("settings=%q: expected empty user settings present, got %#v",
				raw, payload.Settings)
		}
	}
}

func countTasks(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal helper: %v", err)
	}
	return b
}

func ptr[T any](v T) *T { return &v }
