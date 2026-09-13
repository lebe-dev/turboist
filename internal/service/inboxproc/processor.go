// Package inboxproc files open Inbox tasks into projects with an LLM.
//
// A background loop wakes up every Config.Interval, picks the Inbox tasks that
// still need a decision (see repo.InboxProcessingRepo.ListPending), asks an
// OpenAI-compatible chat model where each belongs and applies the answer through
// the regular services, so every placement invariant holds exactly as for a
// manual move. Every decision is journaled and a sorted one can be reverted.
package inboxproc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/obs"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	"github.com/lebe-dev/turboist/internal/service/events"
)

const (
	// userID is the single user of the installation (seeded by migration 002).
	userID = int64(1)
	// inboxID is the single Inbox row (seeded by migration 001).
	inboxID = int64(1)
	// maxTaskAttempts is how many failed decisions a task gets before it sleeps
	// until the user edits it.
	maxTaskAttempts = 5
	// maxProviderBackoff caps the pause after the provider refused or failed.
	maxProviderBackoff = 30 * time.Minute
	// catalogueFetchPage is the page size used to read the whole catalogue.
	catalogueFetchPage = 200
	// defaultLocale stands in for a user who never picked a language.
	defaultLocale = "en"
)

// ErrTemplate is a prompt that does not parse or does not render against the
// live catalogue.
var ErrTemplate = errors.New("inboxproc: invalid prompt template")

// Deps are the repositories and services the processor works through.
type Deps struct {
	Tasks       *repo.TaskRepo
	TaskLabels  *repo.TaskLabelsRepo
	State       *repo.InboxProcessingRepo
	Contexts    *repo.ContextRepo
	Projects    *repo.ProjectRepo
	Labels      *repo.LabelRepo
	AppSettings *repo.AppSettingsRepo
	Users       *repo.UserRepo
	Move        *service.MoveService
	Hub         *events.Hub
	Location    *time.Location
	Log         *slog.Logger
}

// RunSummary counts what one run decided.
type RunSummary struct {
	Sorted int
	Kept   int
	Failed int
}

// Status is the in-memory state of the processor, surfaced to the settings UI.
type Status struct {
	Running        bool
	LastRunAt      *time.Time
	LastRunSummary *RunSummary
	LastError      *string
	BackoffUntil   *time.Time
}

// Processor is the Inbox triage job. It is constructed even when the feature is
// disabled, so the journal, revert and prompt preview keep working.
type Processor struct {
	cfg Config
	llm Classifier
	d   Deps
	log *slog.Logger
	now func() time.Time

	trigger chan struct{}
	// mu is only ever TryLock'ed: a scheduled and a manual run never overlap,
	// and the loser simply does not run.
	mu sync.Mutex
	// requested is raised by TriggerNow and cleared once a run finishes, so the
	// UI polling right after "run now" sees the run before the loop picks it up.
	requested atomic.Bool
	running   atomic.Bool

	statusMu        sync.Mutex
	status          Status
	providerStrikes int
}

func NewProcessor(cfg Config, llm Classifier, d Deps) *Processor {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	if d.Location == nil {
		d.Location = time.UTC
	}
	return &Processor{
		cfg:     cfg,
		llm:     llm,
		d:       d,
		log:     log,
		now:     time.Now,
		trigger: make(chan struct{}, 1),
	}
}

// Enabled reports whether the background job may call the model.
func (p *Processor) Enabled() bool {
	return p.cfg.Enabled && p.llm != nil
}

// Config returns the configuration the processor was built with.
func (p *Processor) Config() Config {
	return p.cfg
}

// Run is the background loop: one pass right away, then one per interval or
// per TriggerNow, until ctx is cancelled.
func (p *Processor) Run(ctx context.Context) {
	if !p.Enabled() {
		return
	}
	ctx = logging.WithLogger(ctx, p.log)
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()
	p.runOnce(ctx, false)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runOnce(ctx, false)
		case <-p.trigger:
			p.runOnce(ctx, true)
		}
	}
}

// TriggerNow asks the loop for an immediate run without waiting for it. It
// returns false when the processor is disabled.
func (p *Processor) TriggerNow() bool {
	if !p.Enabled() {
		return false
	}
	p.requested.Store(true)
	select {
	case p.trigger <- struct{}{}:
	default:
		// A run is already queued; it will cover this request too.
	}
	return true
}

// RunOnce performs a scheduled run. ran is false when another run was already
// in flight.
func (p *Processor) RunOnce(ctx context.Context) (summary RunSummary, ran bool) {
	return p.runOnce(ctx, false)
}

// Status returns a copy of the processor's in-memory status.
func (p *Processor) Status() Status {
	p.statusMu.Lock()
	s := p.status
	p.statusMu.Unlock()
	s.Running = p.running.Load() || p.requested.Load()
	return s
}

// PendingCount is how many Inbox tasks are waiting for a decision.
func (p *Processor) PendingCount(ctx context.Context) (int, error) {
	return p.d.State.PendingCount(ctx, p.now())
}

func (p *Processor) runOnce(ctx context.Context, manual bool) (RunSummary, bool) {
	const op = "inboxproc.Processor.runOnce"
	if !p.Enabled() {
		return RunSummary{}, false
	}
	if !p.mu.TryLock() {
		p.log.DebugContext(ctx, "inbox processing run skipped: another run is in flight", slog.String("op", op))
		return RunSummary{}, false
	}
	defer p.mu.Unlock()
	p.running.Store(true)
	defer func() {
		p.running.Store(false)
		p.requested.Store(false)
	}()

	now := p.now()
	if !manual {
		if until := p.backoffUntil(); until != nil && now.Before(*until) {
			p.log.DebugContext(ctx, "inbox processing run skipped: provider backoff", slog.String("op", op),
				slog.Time("until", *until))
			return RunSummary{}, true
		}
	}

	summary, err := p.process(ctx, now)
	if summary.Sorted > 0 && p.d.Hub != nil {
		// No origin: this change has no originating tab, every client refetches.
		for _, scope := range []events.Scope{events.ScopeTasks, events.ScopeInbox, events.ScopePlan} {
			p.d.Hub.Publish(ctx, userID, scope)
		}
	}
	if err != nil && ctx.Err() != nil {
		// Shutdown in the middle of a run is not a provider problem.
		return summary, true
	}
	p.finish(ctx, now, summary, err)
	return summary, true
}

// process runs one batch. A returned error aborts the run: the provider is
// unavailable or the prompt cannot be used.
func (p *Processor) process(ctx context.Context, now time.Time) (RunSummary, error) {
	const op = "inboxproc.Processor.process"
	var summary RunSummary
	pending, err := p.d.State.ListPending(ctx, now, p.cfg.BatchLimit)
	if err != nil {
		return summary, fmt.Errorf("list pending inbox tasks: %w", err)
	}
	if len(pending) == 0 {
		return summary, nil
	}
	p.log.InfoContext(ctx, "inbox processing run started", slog.String("op", op), slog.Int("pending", len(pending)))

	catalogue, err := p.loadCatalogue(ctx)
	if err != nil {
		return summary, err
	}
	tpl, err := p.savedTemplate(ctx)
	if err != nil {
		return summary, err
	}
	locale := p.locale(ctx)

	for _, item := range pending {
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}
		outcome, err := p.decide(ctx, now, catalogue, tpl, locale, item)
		if err != nil {
			return summary, err
		}
		switch outcome {
		case model.InboxOutcomeSorted:
			summary.Sorted++
		case model.InboxOutcomeKept:
			summary.Kept++
		case model.InboxOutcomeFailed:
			summary.Failed++
		}
	}
	return summary, nil
}

// decide classifies one task and records the result. The empty outcome means
// the task was skipped (changed or gone while the model was thinking).
func (p *Processor) decide(ctx context.Context, now time.Time, catalogue *Catalogue, tpl *template.Template,
	locale string, item PendingItem) (model.InboxOutcome, error) {
	const op = "inboxproc.Processor.decide"
	task := item.Task
	system, err := RenderSystem(tpl, catalogue.PromptData(now, p.d.Location, locale, task))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplate, err)
	}
	user, err := UserMessage(task, p.d.Location)
	if err != nil {
		return p.fail(ctx, now, item, p.cfg.Model, Completion{}, nil, err)
	}

	callCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	completion, err := p.llm.Classify(callCtx, system, user)
	cancel()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrProvider) {
		return "", err
	}
	modelName := completion.Model
	if modelName == "" {
		modelName = p.cfg.Model
	}
	if err != nil {
		return p.fail(ctx, now, item, modelName, completion, nil, err)
	}

	decision, err := ParseDecision(completion.Content)
	if err != nil {
		return p.fail(ctx, now, item, modelName, completion, nil, err)
	}
	verdict, err := decision.Validate(catalogue, now, p.d.Location)
	if err != nil {
		return p.fail(ctx, now, item, modelName, completion, &decision, err)
	}
	for _, w := range verdict.Warnings {
		p.log.WarnContext(ctx, "inbox processing ignored part of the answer", slog.String("op", op),
			slog.Int64("task_id", task.ID), slog.String("warning", w))
	}
	if verdict.Keep {
		return p.keep(ctx, now, item, modelName, completion, verdict)
	}
	return p.apply(ctx, now, item, modelName, completion, verdict)
}

// PendingItem is one task handed to a run, with its earlier state if any.
type PendingItem = repo.PendingInboxTask

func (p *Processor) finish(ctx context.Context, now time.Time, summary RunSummary, err error) {
	const op = "inboxproc.Processor.finish"
	p.statusMu.Lock()
	defer p.statusMu.Unlock()
	at := now
	s := summary
	p.status.LastRunAt = &at
	p.status.LastRunSummary = &s
	if err == nil {
		p.status.LastError = nil
		p.status.BackoffUntil = nil
		p.providerStrikes = 0
		if summary != (RunSummary{}) {
			p.log.InfoContext(ctx, "inbox processing run finished", slog.String("op", op),
				slog.Int("sorted", summary.Sorted), slog.Int("kept", summary.Kept), slog.Int("failed", summary.Failed))
		}
		return
	}

	msg := err.Error()
	p.status.LastError = &msg
	if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrProvider) {
		p.providerStrikes++
		pause := backoffFor(p.cfg.Interval, p.providerStrikes, maxProviderBackoff)
		until := now.Add(pause)
		p.status.BackoffUntil = &until
		p.log.WarnContext(ctx, "inbox processing paused after a provider failure", slog.String("op", op),
			slog.String("err", msg), slog.Duration("pause", pause))
		return
	}
	if errors.Is(err, ErrTemplate) {
		// A configuration problem the settings page shows; reporting it to Sentry
		// on every tick would only bury real failures.
		p.log.WarnContext(ctx, "inbox processing prompt cannot be used", slog.String("op", op), slog.String("err", msg))
		return
	}
	obs.CaptureError(err, map[string]string{"op": op})
	p.log.ErrorContext(ctx, "inbox processing run failed", slog.String("op", op), slog.String("err", msg))
}

func (p *Processor) backoffUntil() *time.Time {
	p.statusMu.Lock()
	defer p.statusMu.Unlock()
	return p.status.BackoffUntil
}

// backoffFor is base·2^strikes, capped. strikes starts at 1.
func backoffFor(base time.Duration, strikes int, limit time.Duration) time.Duration {
	d := base
	for i := 0; i < strikes; i++ {
		d *= 2
		if d >= limit {
			return limit
		}
	}
	return d
}

func (p *Processor) loadCatalogue(ctx context.Context) (*Catalogue, error) {
	var contexts []model.Context
	for offset := 0; ; offset += catalogueFetchPage {
		page, total, err := p.d.Contexts.List(ctx, repo.Page{Limit: catalogueFetchPage, Offset: offset})
		if err != nil {
			return nil, fmt.Errorf("load contexts: %w", err)
		}
		contexts = append(contexts, page...)
		if len(page) == 0 || offset+len(page) >= total {
			break
		}
	}
	open := model.ProjectStatusOpen
	var projects []model.Project
	for offset := 0; ; offset += catalogueFetchPage {
		page, total, err := p.d.Projects.List(ctx, repo.ProjectListFilter{Status: &open}, repo.Page{Limit: catalogueFetchPage, Offset: offset})
		if err != nil {
			return nil, fmt.Errorf("load projects: %w", err)
		}
		projects = append(projects, page...)
		if len(page) == 0 || offset+len(page) >= total {
			break
		}
	}
	var labels []model.Label
	for offset := 0; ; offset += catalogueFetchPage {
		page, total, err := p.d.Labels.List(ctx, repo.LabelListFilter{}, repo.Page{Limit: catalogueFetchPage, Offset: offset})
		if err != nil {
			return nil, fmt.Errorf("load labels: %w", err)
		}
		labels = append(labels, page...)
		if len(page) == 0 || offset+len(page) >= total {
			break
		}
	}
	return NewCatalogue(contexts, projects, labels), nil
}

func (p *Processor) savedPrompt(ctx context.Context) (string, error) {
	settings, err := p.d.AppSettings.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("load app settings: %w", err)
	}
	return settings.InboxProcessing.Prompt, nil
}

func (p *Processor) savedTemplate(ctx context.Context) (*template.Template, error) {
	text, err := p.savedPrompt(ctx)
	if err != nil {
		return nil, err
	}
	tpl, err := ParsePrompt(text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTemplate, err)
	}
	return tpl, nil
}

func (p *Processor) locale(ctx context.Context) string {
	settings, err := p.d.Users.GetSettings(ctx, userID)
	if err != nil || settings.Locale == "" {
		return defaultLocale
	}
	return settings.Locale
}

// Preview renders a prompt against the live catalogue and the oldest open Inbox
// task, or a built-in example when the Inbox is empty. A nil prompt previews the
// saved one; an empty one the built-in default.
func (p *Processor) Preview(ctx context.Context, prompt *string) (string, error) {
	var text string
	if prompt != nil {
		text = *prompt
	} else {
		saved, err := p.savedPrompt(ctx)
		if err != nil {
			return "", err
		}
		text = saved
	}
	data, err := p.previewData(ctx)
	if err != nil {
		return "", err
	}
	tpl, err := ParsePrompt(text)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplate, err)
	}
	rendered, err := RenderSystem(tpl, data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplate, err)
	}
	return rendered, nil
}

// ValidatePrompt checks a prompt the same way Preview renders it. An empty
// prompt stands for the built-in default.
func (p *Processor) ValidatePrompt(ctx context.Context, prompt string) error {
	data, err := p.previewData(ctx)
	if err != nil {
		return err
	}
	if err := ValidatePrompt(prompt, data); err != nil {
		return fmt.Errorf("%w: %v", ErrTemplate, err)
	}
	return nil
}

func (p *Processor) previewData(ctx context.Context) (PromptData, error) {
	now := p.now()
	catalogue, err := p.loadCatalogue(ctx)
	if err != nil {
		return PromptData{}, err
	}
	task := ExampleTask(now)
	oldest, err := p.d.State.OldestInboxTask(ctx)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		return PromptData{}, err
	}
	if oldest != nil {
		task = *oldest
	}
	return catalogue.PromptData(now, p.d.Location, p.locale(ctx), task), nil
}
