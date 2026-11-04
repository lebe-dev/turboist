package linktitle

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

type fakeFetcher struct {
	mu   sync.Mutex
	urls []string
	fn   func(u *url.URL) (Page, error)
}

func (f *fakeFetcher) Fetch(_ context.Context, u *url.URL) (Page, error) {
	f.mu.Lock()
	f.urls = append(f.urls, u.String())
	f.mu.Unlock()
	return f.fn(u)
}

type fixture struct {
	svc      *Service
	fetch    *fakeFetcher
	tasks    *repo.TaskRepo
	queue    *repo.LinkTitleQueueRepo
	settings *repo.AppSettingsRepo
	inboxRep *repo.InboxProcessingRepo
	taskSvc  *service.TaskService
	now      time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := db.RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := repo.NewUserRepo(d)
	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	settings := repo.NewAppSettingsRepo(d)
	f := &fixture{
		fetch:    &fakeFetcher{fn: func(*url.URL) (Page, error) { return Page{Title: "Page title", Description: "Summary"}, nil }},
		tasks:    tasks,
		queue:    repo.NewLinkTitleQueueRepo(d),
		settings: settings,
		inboxRep: repo.NewInboxProcessingRepo(d, tlabels),
		now:      time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC),
	}
	f.svc = NewService(Deps{Queue: f.queue, Tasks: tasks, AppSettings: settings, Users: users, Log: slog.Default()}, f.fetch)
	f.svc.now = func() time.Time { return f.now }
	f.taskSvc = service.NewTaskService(tasks, projects, tlabels, service.NewAutoLabelsService(repo.NewLabelRepo(d), settings)).
		WithCreatedHook(f.svc)
	return f
}

func (f *fixture) enable(t *testing.T, mutate func(*model.LinkTitlesSettings)) {
	t.Helper()
	s, err := f.settings.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.LinkTitles.Enabled = true
	if mutate != nil {
		mutate(&s.LinkTitles)
	}
	if err := f.settings.Set(context.Background(), s); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) create(t *testing.T, title, desc string) *model.Task {
	t.Helper()
	inbox := int64(1)
	task, err := f.taskSvc.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{InboxID: &inbox}, Title: title, Description: desc,
	}, nil, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return task
}

func (f *fixture) queued(t *testing.T) []repo.LinkTitleJob {
	t.Helper()
	jobs, err := f.queue.Due(context.Background(), f.now.Add(24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	return jobs
}

func (f *fixture) get(t *testing.T, id int64) *model.Task {
	t.Helper()
	task, err := f.tasks.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestDisabledByDefault(t *testing.T) {
	f := newFixture(t)
	f.create(t, "https://example.com/post", "")
	if jobs := f.queued(t); len(jobs) != 0 {
		t.Fatalf("queued while disabled: %+v", jobs)
	}
}

func TestRewritesBareLink(t *testing.T) {
	f := newFixture(t)
	f.enable(t, nil)
	task := f.create(t, " https://example.com/post ", "my note")
	f.create(t, "not a link", "")
	if jobs := f.queued(t); len(jobs) != 1 || jobs[0].URL != "https://example.com/post" {
		t.Fatalf("queue = %+v", jobs)
	}

	if changed := f.svc.pass(context.Background()); changed != 1 {
		t.Fatalf("changed = %d", changed)
	}
	got := f.get(t, task.ID)
	if got.Title != "Article “Page title”" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Description != "https://example.com/post\n\nSummary\n\nmy note" {
		t.Errorf("description = %q", got.Description)
	}
	if jobs := f.queued(t); len(jobs) != 0 {
		t.Errorf("job left behind: %+v", jobs)
	}
}

func TestDomainRuleAndFallback(t *testing.T) {
	f := newFixture(t)
	f.enable(t, func(s *model.LinkTitlesSettings) {
		s.Template = "Статья «{title}»"
		s.Rules = []model.LinkTitleRule{{Domain: "youtube.com", Template: "Посмотреть «{title}»"}}
	})
	f.fetch.fn = func(u *url.URL) (Page, error) {
		if u.Hostname() == "www.youtube.com" {
			return Page{Title: "Видео"}, nil
		}
		return Page{}, &FetchError{Err: errors.New("status 404")}
	}
	video := f.create(t, "https://www.youtube.com/watch?v=1", "")
	missing := f.create(t, "https://example.com/blog/post/", "")
	f.svc.pass(context.Background())
	if got := f.get(t, video.ID).Title; got != "Посмотреть «Видео»" {
		t.Errorf("rule title = %q", got)
	}
	if got := f.get(t, missing.ID).Title; got != "Статья «example.com/blog/post»" {
		t.Errorf("fallback title = %q", got)
	}
}

func TestTransientFailureRetriesThenFallsBack(t *testing.T) {
	f := newFixture(t)
	f.enable(t, nil)
	f.fetch.fn = func(*url.URL) (Page, error) {
		return Page{}, &FetchError{Transient: true, Err: errors.New("timeout")}
	}
	task := f.create(t, "https://example.com/slow", "")

	f.svc.pass(context.Background())
	jobs := f.queued(t)
	if len(jobs) != 1 || jobs[0].Attempts != 1 || !jobs[0].NextAttemptAt.After(f.now) {
		t.Fatalf("after first failure: %+v", jobs)
	}
	if got := f.get(t, task.ID).Title; got != "https://example.com/slow" {
		t.Fatalf("task rewritten too early: %q", got)
	}
	// Not due yet: nothing happens.
	f.svc.pass(context.Background())
	if n := len(f.fetch.urls); n != 1 {
		t.Fatalf("fetched %d times before the retry was due", n)
	}
	f.now = f.now.Add(time.Hour)
	f.svc.pass(context.Background())
	f.now = f.now.Add(time.Hour)
	f.svc.pass(context.Background())
	if got := f.get(t, task.ID).Title; got != "Article “example.com/slow”" {
		t.Fatalf("after giving up: %q", got)
	}
	if jobs := f.queued(t); len(jobs) != 0 {
		t.Fatalf("job left behind: %+v", jobs)
	}
}

func TestEditedTaskIsLeftAlone(t *testing.T) {
	f := newFixture(t)
	f.enable(t, nil)
	task := f.create(t, "https://example.com/x", "")
	renamed := "My own title"
	if _, err := f.tasks.Update(context.Background(), task.ID, repo.TaskUpdate{Title: &renamed}); err != nil {
		t.Fatal(err)
	}
	f.svc.pass(context.Background())
	if got := f.get(t, task.ID).Title; got != renamed {
		t.Fatalf("title = %q", got)
	}
	if len(f.fetch.urls) != 0 {
		t.Fatal("an edited task must not be fetched")
	}
	if jobs := f.queued(t); len(jobs) != 0 {
		t.Fatalf("job left behind: %+v", jobs)
	}
}

func TestDisablingClearsQueue(t *testing.T) {
	f := newFixture(t)
	f.enable(t, nil)
	task := f.create(t, "https://example.com/x", "")
	s, _ := f.settings.Get(context.Background())
	s.LinkTitles.Enabled = false
	_ = f.settings.Set(context.Background(), s)
	f.svc.pass(context.Background())
	if got := f.get(t, task.ID).Title; got != "https://example.com/x" {
		t.Fatalf("title = %q", got)
	}
	if jobs := f.queued(t); len(jobs) != 0 {
		t.Fatalf("queue not cleared: %+v", jobs)
	}
}

func TestInboxProcessingWaitsForTitle(t *testing.T) {
	f := newFixture(t)
	f.enable(t, nil)
	link := f.create(t, "https://example.com/x", "")
	plain := f.create(t, "buy milk", "")
	pending, err := f.inboxRep.ListPending(context.Background(), f.now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Task.ID != plain.ID {
		t.Fatalf("pending = %+v", pending)
	}
	f.svc.pass(context.Background())
	pending, _ = f.inboxRep.ListPending(context.Background(), f.now, 10)
	if len(pending) != 2 || pending[0].Task.ID != link.ID {
		t.Fatalf("after rewrite pending = %+v", pending)
	}
}
