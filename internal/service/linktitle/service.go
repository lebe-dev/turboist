package linktitle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/obs"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service/events"
)

const (
	// userID is the single user of the installation (seeded by migration 002).
	userID = int64(1)
	// batchSize is how many jobs one pass takes.
	batchSize = 20
	// maxAttempts is how many transient failures a link gets before it is given
	// the fallback title.
	maxAttempts = 3
	// idleWake bounds the sleep between passes when nothing is due, so a missed
	// trigger costs at most this much delay.
	idleWake = 5 * time.Minute
	// defaultLocale stands in for a user who never picked a language.
	defaultLocale = "en"
)

// retryBackoff is the pause before the second and third attempt.
var retryBackoff = []time.Duration{time.Minute, 5 * time.Minute}

// Deps are the repositories the worker works through.
type Deps struct {
	Queue       *repo.LinkTitleQueueRepo
	Tasks       *repo.TaskRepo
	AppSettings *repo.AppSettingsRepo
	Users       *repo.UserRepo
	Hub         *events.Hub
	Log         *slog.Logger
}

// Service queues bare-link tasks and rewrites them in the background. It is
// always constructed; the settings toggle decides whether it does anything.
type Service struct {
	d       Deps
	fetch   PageFetcher
	log     *slog.Logger
	now     func() time.Time
	trigger chan struct{}
}

func NewService(d Deps, fetch PageFetcher) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{d: d, fetch: fetch, log: log, now: time.Now, trigger: make(chan struct{}, 1)}
}

// TaskCreated is the hook service.TaskService calls after every create. A task
// whose title is a bare public https link is queued and the worker woken; any
// failure is logged, never returned — the task itself was created fine.
func (s *Service) TaskCreated(ctx context.Context, t *model.Task) {
	const op = "service.linktitle.TaskCreated"
	u, ok := DetectURL(t.Title)
	if !ok {
		return
	}
	settings, err := s.d.AppSettings.Get(ctx)
	if err != nil {
		s.log.ErrorContext(ctx, op+": load settings", slog.Int64("task_id", t.ID), slog.String("err", err.Error()))
		return
	}
	if !settings.LinkTitles.Enabled {
		return
	}
	// The title as typed, not u.String(): the worker recognises an untouched task
	// by comparing its title with this exact text.
	if err := s.d.Queue.Enqueue(ctx, t.ID, strings.TrimSpace(t.Title), s.now()); err != nil {
		s.log.ErrorContext(ctx, op+": enqueue", slog.Int64("task_id", t.ID), slog.String("err", err.Error()))
		return
	}
	s.log.InfoContext(ctx, "link title queued", slog.String("op", op), slog.Int64("task_id", t.ID),
		slog.String("host", u.Hostname()))
	s.Wake()
}

// Wake asks the worker for a pass now. It never blocks.
func (s *Service) Wake() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Run is the background loop: a pass right away, then one per wake-up or when
// the next retry falls due, until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	for {
		s.pass(ctx)
		wait := idleWake
		if next, err := s.d.Queue.NextDue(ctx); err == nil && next != nil {
			if d := next.Sub(s.now()); d < wait {
				wait = max(d, time.Second)
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.trigger:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// pass works through every due job. It returns how many tasks were rewritten.
func (s *Service) pass(ctx context.Context) int {
	const op = "service.linktitle.pass"
	defer func() {
		// A parser bug on one odd page must not take the server down with it.
		if r := recover(); r != nil {
			err := fmt.Errorf("panic: %v", r)
			s.log.ErrorContext(ctx, op+": panic", slog.String("err", err.Error()))
			obs.CaptureError(err, map[string]string{"op": op})
		}
	}()
	settings, err := s.d.AppSettings.Get(ctx)
	if err != nil {
		s.log.ErrorContext(ctx, op+": load settings", slog.String("err", err.Error()))
		return 0
	}
	if !settings.LinkTitles.Enabled {
		// Switched off with jobs still waiting: they stay bare links.
		if err := s.d.Queue.Clear(ctx); err != nil {
			s.log.ErrorContext(ctx, op+": clear queue", slog.String("err", err.Error()))
		}
		return 0
	}
	locale := s.locale(ctx)
	changed := 0
	for ctx.Err() == nil {
		jobs, err := s.d.Queue.Due(ctx, s.now(), batchSize)
		if err != nil {
			s.log.ErrorContext(ctx, op+": list due", slog.String("err", err.Error()))
			break
		}
		if len(jobs) == 0 {
			break
		}
		for _, j := range jobs {
			if ctx.Err() != nil {
				break
			}
			if s.process(ctx, j, settings.LinkTitles, locale) {
				changed++
			}
		}
		if len(jobs) < batchSize {
			break
		}
	}
	if changed > 0 && s.d.Hub != nil {
		// No originating request: every client, the creating one included,
		// refetches.
		for _, scope := range []events.Scope{events.ScopeTasks, events.ScopeInbox, events.ScopePlan} {
			s.d.Hub.Publish(ctx, userID, scope)
		}
	}
	return changed
}

// process handles one job and reports whether the task was rewritten. Every
// path either deletes the job or reschedules it, so a job can never spin.
func (s *Service) process(ctx context.Context, j repo.LinkTitleJob, settings model.LinkTitlesSettings, locale string) bool {
	const op = "service.linktitle.process"
	log := s.log.With(slog.String("op", op), slog.Int64("task_id", j.TaskID))
	drop := func(reason string) bool {
		log.DebugContext(ctx, "link title dropped", slog.String("reason", reason))
		if err := s.d.Queue.Delete(ctx, j.TaskID); err != nil {
			log.ErrorContext(ctx, op+": delete job", slog.String("err", err.Error()))
		}
		return false
	}
	retry := func(cause error) bool {
		attempts := j.Attempts + 1
		backoff := retryBackoff[min(attempts-1, len(retryBackoff)-1)]
		if err := s.d.Queue.Reschedule(ctx, j.TaskID, attempts, s.now().Add(backoff), cause.Error()); err != nil {
			log.ErrorContext(ctx, op+": reschedule", slog.String("err", err.Error()))
		}
		log.InfoContext(ctx, "link title retry scheduled", slog.Int("attempt", attempts),
			slog.String("err", cause.Error()))
		return false
	}

	u, err := url.Parse(j.URL)
	if err != nil || ValidateURL(u) != nil {
		return drop("invalid url")
	}
	if ok, err := s.stillBare(ctx, j); err != nil {
		return retry(err)
	} else if !ok {
		return drop("task changed or gone")
	}

	page, ferr := s.fetch.Fetch(WithAcceptLanguage(ctx, locale), u)
	if ferr != nil && ctx.Err() != nil {
		// Shutdown mid-fetch is not the link's fault; the job stays as it was.
		return false
	}
	if ferr != nil && IsTransient(ferr) && j.Attempts+1 < maxAttempts {
		return retry(ferr)
	}
	title := page.Title
	if title == "" {
		title = FallbackTitle(u)
		attrs := []any{slog.String("host", u.Hostname())}
		if ferr != nil {
			attrs = append(attrs, slog.String("err", ferr.Error()))
		}
		log.InfoContext(ctx, "link title not found, using fallback", attrs...)
	}

	// Re-read: the user may have edited the task while the page loaded.
	task, err := s.d.Tasks.Get(ctx, j.TaskID)
	if errors.Is(err, repo.ErrNotFound) {
		return drop("task gone")
	}
	if err != nil {
		return retry(err)
	}
	if strings.TrimSpace(task.Title) != j.URL {
		return drop("task changed")
	}
	newTitle := RenderTitle(ResolveTemplate(settings, u, locale), title, u)
	newDesc := ComposeDescription(j.URL, page.Description, task.Description)
	if _, err := s.d.Tasks.Update(ctx, j.TaskID, repo.TaskUpdate{Title: &newTitle, Description: &newDesc}); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return drop("task gone")
		}
		return retry(err)
	}
	if err := s.d.Queue.Delete(ctx, j.TaskID); err != nil {
		log.ErrorContext(ctx, op+": delete job", slog.String("err", err.Error()))
	}
	log.InfoContext(ctx, "link title applied", slog.String("host", u.Hostname()),
		slog.Bool("found", page.Title != ""))
	return true
}

// stillBare reports whether the task still exists with the link as its title.
func (s *Service) stillBare(ctx context.Context, j repo.LinkTitleJob) (bool, error) {
	task, err := s.d.Tasks.Get(ctx, j.TaskID)
	if errors.Is(err, repo.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(task.Title) == j.URL, nil
}

func (s *Service) locale(ctx context.Context) string {
	if s.d.Users == nil {
		return defaultLocale
	}
	settings, err := s.d.Users.GetSettings(ctx, userID)
	if err != nil || settings.Locale == "" {
		return defaultLocale
	}
	return settings.Locale
}
