package service_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
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
	trels    *repo.TaskRelationsRepo
	plabels  *repo.ProjectLabelsRepo
	users    *repo.UserRepo
	appSet   *repo.AppSettingsRepo
}

func setupBackupFixtures(t *testing.T) *backupFixtures {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	trels := repo.NewTaskRelationsRepo(d)
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
		tasks:    repo.NewTaskRepo(d, tlabels, trels),
		tlabels:  tlabels,
		trels:    trels,
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
	it, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{InboxID: ptr(int64(1))},
		Title:     "think about this later",
	})
	if err != nil {
		t.Fatalf("inbox task: %v", err)
	}

	// One relation of each type, so the round-trip covers both the enum values and
	// the cross-placement (project task ↔ inbox task) case.
	if _, err := f.trels.Create(ctx, pt.ID, it.ID, model.RelationTypeBlocks); err != nil {
		t.Fatalf("blocks relation: %v", err)
	}
	if _, err := f.trels.Create(ctx, pt.ID, it.ID, model.RelationTypeRelated); err != nil {
		t.Fatalf("related relation: %v", err)
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

func TestBackupService_SanitizeDropsTaskLabelWhenLabelMissing(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()

	// Task references label_id=999 which is not present in Labels. The task
	// itself is well-formed (inbox placement), so it must survive, but the
	// link row pointing at the missing label must be dropped.
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			Tasks: []service.BackupTask{
				{
					ID: 1, Title: "labelled inbox", InboxID: ptr(int64(1)),
					Priority: string(model.PriorityNone), Status: string(model.TaskStatusOpen),
					DayPart: string(model.DayPartNone), PlanState: string(model.PlanStateNone),
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z",
				},
			},
			TaskLabels: []service.BackupTaskLabel{
				{TaskID: 1, LabelID: 999},
			},
		},
	}
	if err := f.svc.Restore(ctx, bad); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := f.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(got.Data.Tasks) != 1 {
		t.Fatalf("tasks: got %d, want 1", len(got.Data.Tasks))
	}
	if len(got.Data.TaskLabels) != 0 {
		t.Errorf("task_labels: got %d, want 0 (dangling label link must be dropped)", len(got.Data.TaskLabels))
	}
}

func TestBackupService_SanitizeDropsSectionWithoutProject(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()

	// Section references project_id=999 (missing) → section dropped.
	// Task references that section but has valid inbox placement; section_id
	// must be NULL'd out, the task itself stays.
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			ProjectSections: []service.BackupProjectSection{
				{ID: 50, ProjectID: 999, Title: "orphan", Position: 1,
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
			},
			Tasks: []service.BackupTask{
				{
					ID: 1, Title: "with ghost section", InboxID: ptr(int64(1)), SectionID: ptr(int64(50)),
					Priority: string(model.PriorityNone), Status: string(model.TaskStatusOpen),
					DayPart: string(model.DayPartNone), PlanState: string(model.PlanStateNone),
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z",
				},
			},
		},
	}
	if err := f.svc.Restore(ctx, bad); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := f.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(got.Data.ProjectSections) != 0 {
		t.Errorf("project_sections: got %d, want 0 (orphan section must be dropped)", len(got.Data.ProjectSections))
	}
	if len(got.Data.Tasks) != 1 {
		t.Fatalf("tasks: got %d, want 1", len(got.Data.Tasks))
	}
	if got.Data.Tasks[0].SectionID != nil {
		t.Errorf("task.section_id: got %v, want nil (healed)", *got.Data.Tasks[0].SectionID)
	}
}

func TestBackupService_SanitizeDropsTaskViolatingPlacement(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()

	// Task ID=1 has BOTH inbox_id and context_id set, violating the
	// (inbox XOR context) CHECK invariant — it must be dropped together with
	// any task_labels rows that referenced it. Task ID=2 is a clean inbox
	// task with the same label, expected to survive.
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			Contexts: []service.BackupContext{
				{ID: 1, Name: "ctx", Color: "blue", CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
			},
			Labels: []service.BackupLabel{
				{ID: 5, Name: "lbl", Color: "red", CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
			},
			Tasks: []service.BackupTask{
				{
					ID: 1, Title: "both placements", InboxID: ptr(int64(1)), ContextID: ptr(int64(1)),
					Priority: string(model.PriorityNone), Status: string(model.TaskStatusOpen),
					DayPart: string(model.DayPartNone), PlanState: string(model.PlanStateNone),
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z",
				},
				{
					ID: 2, Title: "clean inbox", InboxID: ptr(int64(1)),
					Priority: string(model.PriorityNone), Status: string(model.TaskStatusOpen),
					DayPart: string(model.DayPartNone), PlanState: string(model.PlanStateNone),
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z",
				},
			},
			TaskLabels: []service.BackupTaskLabel{
				{TaskID: 1, LabelID: 5},
				{TaskID: 2, LabelID: 5},
			},
		},
	}
	if err := f.svc.Restore(ctx, bad); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := f.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(got.Data.Tasks) != 1 {
		t.Fatalf("tasks: got %d, want 1 (task with both placements must be dropped)", len(got.Data.Tasks))
	}
	if got.Data.Tasks[0].ID != 2 {
		t.Errorf("survivor id: got %d, want 2", got.Data.Tasks[0].ID)
	}
	if len(got.Data.TaskLabels) != 1 {
		t.Fatalf("task_labels: got %d, want 1 (link to dropped task must be removed)", len(got.Data.TaskLabels))
	}
	if got.Data.TaskLabels[0].TaskID != 2 {
		t.Errorf("surviving task_label.task_id: got %d, want 2", got.Data.TaskLabels[0].TaskID)
	}
}

func TestBackupService_SanitizeHealsParentIDWhenParentDroppedLater(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()

	// Child task (id=7) appears in the slice BEFORE its parent (id=6). The
	// parent has both inbox_id and context_id set, so it must be dropped by
	// the placement check. The single-pass implementation kept the child's
	// parent_id pointing at the missing row, causing commit-time fk failure;
	// the multi-pass implementation must null it instead so restore succeeds.
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			Contexts: []service.BackupContext{
				{ID: 1, Name: "ctx", Color: "blue", CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
			},
			Tasks: []service.BackupTask{
				{
					ID: 7, Title: "child first", ContextID: ptr(int64(1)), ParentID: ptr(int64(6)),
					Priority: string(model.PriorityNone), Status: string(model.TaskStatusOpen),
					DayPart: string(model.DayPartNone), PlanState: string(model.PlanStateNone),
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z",
				},
				{
					ID: 6, Title: "parent both placements", InboxID: ptr(int64(1)), ContextID: ptr(int64(1)),
					Priority: string(model.PriorityNone), Status: string(model.TaskStatusOpen),
					DayPart: string(model.DayPartNone), PlanState: string(model.PlanStateNone),
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z",
				},
			},
		},
	}
	if err := f.svc.Restore(ctx, bad); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := f.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(got.Data.Tasks) != 1 {
		t.Fatalf("tasks: got %d, want 1", len(got.Data.Tasks))
	}
	survivor := got.Data.Tasks[0]
	if survivor.ID != 7 {
		t.Errorf("survivor id: got %d, want 7", survivor.ID)
	}
	if survivor.ParentID != nil {
		t.Errorf("survivor parent_id: got %v, want nil (healed against final survivor set)", *survivor.ParentID)
	}
}

func TestBackupService_SanitizeDropsProjectLabelWhenProjectMissing(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()

	// Label exists but the referenced project does not — link must be dropped.
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			Labels: []service.BackupLabel{
				{ID: 5, Name: "lbl", Color: "red", CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
			},
			ProjectLabels: []service.BackupProjectLabel{
				{ProjectID: 999, LabelID: 5},
			},
		},
	}
	if err := f.svc.Restore(ctx, bad); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := f.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(got.Data.ProjectLabels) != 0 {
		t.Errorf("project_labels: got %d, want 0 (link to missing project must be dropped)", len(got.Data.ProjectLabels))
	}
}

func TestBackupService_RestoreEmptyPayloadWipesAll(t *testing.T) {
	f := setupBackupFixtures(t)
	seedSample(t, f)
	ctx := context.Background()
	if countTasks(t, f.db) == 0 {
		t.Fatal("precondition: sample seed yielded no tasks")
	}

	empty := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data:       service.BackupData{},
	}
	if err := f.svc.Restore(ctx, empty); err != nil {
		t.Fatalf("restore empty: %v", err)
	}
	got, err := f.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(got.Data.Contexts) != 0 || len(got.Data.Labels) != 0 ||
		len(got.Data.Projects) != 0 || len(got.Data.ProjectSections) != 0 ||
		len(got.Data.Tasks) != 0 || len(got.Data.TaskLabels) != 0 ||
		len(got.Data.ProjectLabels) != 0 || len(got.Data.TaskRelations) != 0 {
		t.Errorf("data after empty restore: %#v, want all empty", got.Data)
	}
}

func TestDecodeBackup_RejectsCorruptedGzip(t *testing.T) {
	// 0x1f 0x8b is the gzip magic — the decoder routes through gzip.NewReader,
	// which must fail on garbage that follows the magic bytes.
	raw := []byte{0x1f, 0x8b, 0x00, 0xde, 0xad, 0xbe, 0xef}
	_, err := service.DecodeBackup(raw)
	if err == nil {
		t.Fatal("want error from corrupted gzip body, got nil")
	}
	if !errors.Is(err, service.ErrBadBackup) {
		t.Errorf("err not ErrBadBackup: %v", err)
	}
}

func TestDecodeBackup_RejectsDecompressionBomb(t *testing.T) {
	// A tiny gzipped payload that decompresses to >256 MiB of zero bytes must
	// be rejected before io.ReadAll exhausts process memory.
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		t.Fatalf("gzip writer: %v", err)
	}
	// 257 MiB worth of zeros — gzip squashes this to a few KiB.
	const target = 257 * 1024 * 1024
	chunk := make([]byte, 1<<20)
	for written := 0; written < target; written += len(chunk) {
		if _, err := zw.Write(chunk); err != nil {
			t.Fatalf("gzip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	_, err = service.DecodeBackup(buf.Bytes())
	if err == nil {
		t.Fatal("want error from oversized decompressed payload, got nil")
	}
	if !errors.Is(err, service.ErrBadBackup) {
		t.Errorf("err not ErrBadBackup: %v", err)
	}
}

func TestDecodeBackup_RejectsUnknownFields(t *testing.T) {
	// json.Decoder is configured with DisallowUnknownFields so foreign keys
	// (typos, schema drift, malicious tampering) surface immediately.
	raw := []byte(`{"version":1,"exportedAt":"2026-05-19T00:00:00.000Z","data":{"contexts":[],"labels":[],"projects":[],"projectSections":[],"tasks":[],"taskLabels":[],"projectLabels":[]},"mystery":true}`)
	_, err := service.DecodeBackup(raw)
	if err == nil {
		t.Fatal("want error from unknown field, got nil")
	}
	if !errors.Is(err, service.ErrBadBackup) {
		t.Errorf("err not ErrBadBackup: %v", err)
	}
}

func TestBackupService_RestoreRejectsVersionMismatch(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion + 99,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data:       service.BackupData{},
	}
	err := f.svc.Restore(ctx, bad)
	if err == nil {
		t.Fatal("want error from version mismatch, got nil")
	}
	if !errors.Is(err, service.ErrBadBackup) {
		t.Errorf("err not ErrBadBackup: %v", err)
	}
}

func TestBackupService_RestoreRejectsNilPayload(t *testing.T) {
	f := setupBackupFixtures(t)
	err := f.svc.Restore(context.Background(), nil)
	if err == nil {
		t.Fatal("want error from nil payload, got nil")
	}
	if !errors.Is(err, service.ErrBadBackup) {
		t.Errorf("err not ErrBadBackup: %v", err)
	}
}

func TestBackupService_RestoreFailsOnDanglingInboxFK(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()
	// sanitize does not touch InboxID, but tasks.inbox_id REFERENCES inbox(id).
	// The bogus inbox id survives until commit-time foreign_key_check.
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			Tasks: []service.BackupTask{
				{
					ID: 1, Title: "ghost inbox", InboxID: ptr(int64(424242)),
					Priority: string(model.PriorityNone), Status: string(model.TaskStatusOpen),
					DayPart: string(model.DayPartNone), PlanState: string(model.PlanStateNone),
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z",
				},
			},
		},
	}
	err := f.svc.Restore(ctx, bad)
	if err == nil {
		t.Fatal("want fk-violation error, got nil")
	}
	if !strings.Contains(err.Error(), "fk check failed") {
		t.Errorf("want 'fk check failed' in error, got: %v", err)
	}
}

func TestBackupService_RestoreFailsOnInsertLabelsConstraint(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()
	// Two labels with the same name violate UNIQUE(labels.name) — surfaces the
	// "labels: %w" wrap in Restore and the error path in insertLabels.
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			Labels: []service.BackupLabel{
				{ID: 1, Name: "dup", Color: "red", CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
				{ID: 2, Name: "dup", Color: "blue", CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
			},
		},
	}
	if err := f.svc.Restore(ctx, bad); err == nil {
		t.Fatal("want error from duplicate label name, got nil")
	}
}

func TestBackupService_ExportFailsWhenContextsTableMissing(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()
	// Dropping the table breaks readContexts, the very first export step, so
	// the wrapped error from backup.go's Export coordinator is exercised.
	if _, err := f.db.ExecContext(ctx, `DROP TABLE contexts`); err != nil {
		t.Fatalf("drop contexts: %v", err)
	}
	if _, err := f.svc.Export(ctx, service.ExportOptions{}); err == nil {
		t.Fatal("want export error after dropping table, got nil")
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

// Relations must survive an export/restore cycle with their ids intact — the API
// addresses a relation by id, so a restore that renumbered them would break any
// client holding one.
func TestBackupService_RoundTripPreservesTaskRelations(t *testing.T) {
	src := setupBackupFixtures(t)
	seedSample(t, src)
	ctx := context.Background()

	payload, err := src.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(payload.Data.TaskRelations) != 2 {
		t.Fatalf("exported relations: got %d, want 2", len(payload.Data.TaskRelations))
	}

	dst := setupBackupFixtures(t)
	if err := dst.svc.Restore(ctx, payload); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := dst.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(got.Data.TaskRelations) != 2 {
		t.Fatalf("restored relations: got %d, want 2", len(got.Data.TaskRelations))
	}
	for i, want := range payload.Data.TaskRelations {
		if got.Data.TaskRelations[i] != want {
			t.Errorf("relation %d: got %+v, want %+v", i, got.Data.TaskRelations[i], want)
		}
	}
	// And the restored graph must still block: a summary of zero would mean the rows
	// landed but nothing reads them.
	blocked, err := dst.tasks.Get(ctx, payload.Data.TaskRelations[0].TargetTaskID)
	if err != nil {
		t.Fatalf("get restored target: %v", err)
	}
	if blocked.RelationSummary.BlockedByOpen != 1 {
		t.Errorf("restored blocked-by: got %d, want 1", blocked.RelationSummary.BlockedByOpen)
	}
}

// Both FKs are NOT NULL, so a relation whose task was dropped by the sanitiser is
// unrestorable and must be pruned rather than aborting the whole restore.
func TestBackupService_SanitizeDropsTaskRelationWhenTaskMissing(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()

	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			Tasks: []service.BackupTask{
				{
					ID: 1, Title: "surviving inbox task", InboxID: ptr(int64(1)),
					Priority: string(model.PriorityNone), Status: string(model.TaskStatusOpen),
					DayPart: string(model.DayPartNone), PlanState: string(model.PlanStateNone),
					CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z",
				},
			},
			TaskRelations: []service.BackupTaskRelation{
				{ID: 1, SourceTaskID: 1, TargetTaskID: 999, Type: string(model.RelationTypeBlocks),
					CreatedAt: "2026-05-19T00:00:00.000Z"},
				{ID: 2, SourceTaskID: 999, TargetTaskID: 1, Type: string(model.RelationTypeRelated),
					CreatedAt: "2026-05-19T00:00:00.000Z"},
			},
		},
	}
	if err := f.svc.Restore(ctx, bad); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := f.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(got.Data.Tasks) != 1 {
		t.Fatalf("tasks: got %d, want 1", len(got.Data.Tasks))
	}
	if len(got.Data.TaskRelations) != 0 {
		t.Errorf("task_relations: got %d, want 0 (both endpoints must exist)", len(got.Data.TaskRelations))
	}
}
