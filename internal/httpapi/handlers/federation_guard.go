package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// FederationReadOnlyGuard is the authoritative server-side enforcement seam for
// local mutations against read-only federated projects (Federation v1 F5.2,
// US-5.1 AC4; the project-keyed half landed in F2.4, US-2.4 AC4, §9.2/§9.3).
//
// A joined read-only federated project (is_owner=0, permissions=read) is a copy
// the local user may only READ — their own edits are forbidden (the read grant
// constrains their local edits; the owner's relayed fan-out is still applied,
// enforced separately by the inbox per-event membership check). UI disabling is
// insufficient: this guard is where the rule is actually enforced, and it must
// hook EVERY mutation entry point — the project routes (F2.4), AND every task
// route: patch/delete, the action verbs (complete/uncomplete/cancel/pin/unpin/
// move/plan), the sub-resource creates (subtask/duplicate/decompose), the bulk
// verbs (bulk complete/move, group), and the section routes (patch/delete/
// reorder/create-task). The guard is keyed on the affected project: a task's
// own project, a move's DESTINATION project, or a section's parent project.
//
// The owner's own federated project (is_owner=1) and write/admin grants are
// never blocked; non-federated projects and tasks with no project (inbox tasks)
// are always a no-op. A nil guard (federation off — no FEDERATION_KEY, or no
// federated-project repo wired) is a no-op so the single-user path is untouched.
type FederationReadOnlyGuard struct {
	fedProjects *repo.FederatedProjectRepo
	tasks       *repo.TaskRepo
	sections    *repo.ProjectSectionRepo
}

// NewFederationReadOnlyGuard constructs the guard. fedProjects is required for
// the guard to do anything; a nil guard (or a guard with a nil fedProjects) is a
// safe no-op. tasks/sections are used to resolve a task or section to its
// owning project.
func NewFederationReadOnlyGuard(fedProjects *repo.FederatedProjectRepo, tasks *repo.TaskRepo, sections *repo.ProjectSectionRepo) *FederationReadOnlyGuard {
	return &FederationReadOnlyGuard{fedProjects: fedProjects, tasks: tasks, sections: sections}
}

// GuardProject rejects a local mutation against a joined read-only federated
// project with 403 federation_read_only. A nil/unwired guard, a non-federated
// project, the owner's own project, and write/admin grants all pass through.
func (g *FederationReadOnlyGuard) GuardProject(c fiber.Ctx, projectID int64) *httpapi.AppError {
	if g == nil || g.fedProjects == nil {
		return nil
	}
	surfaces, err := g.fedProjects.FederationSurfaceByProjectIDs(c.Context(), []int64{projectID})
	if err != nil {
		return httpapi.ErrInternal("resolve federation surface").WithCause(err)
	}
	s, ok := surfaces[projectID]
	if !ok || s.IsOwner {
		return nil
	}
	// A LOST copy with a read-only reason (revoked / owner-dead) is read-only
	// regardless of the original permission grant (Federation v1 F5.4, US-6.2 AC3 /
	// F5.6a, US-6.5): a write/admin peer that was revoked may no longer edit its
	// copy. A voluntarily-LEFT copy (US-6.3, F5.5) is NOT read-only — it becomes a
	// plain editable local project — so LostReason.IsReadOnly gates this, not Lost
	// alone. Checked before the read-permission gate so a revoked write peer is
	// still rejected.
	if s.Lost && s.LostReason.IsReadOnly() {
		logValidation(c, "handler.Federation.GuardReadOnly", "lost (read-only) federated mutation rejected",
			slog.Int64("project_id", projectID), slog.String("origin", s.OriginInstanceURL), slog.String("reason", string(s.LostReason)))
		return httpapi.ErrFederationReadOnly()
	}
	if s.Permissions == model.FederationPermissionRead {
		logValidation(c, "handler.Federation.GuardReadOnly", "read-only federated mutation rejected",
			slog.Int64("project_id", projectID), slog.String("origin", s.OriginInstanceURL))
		return httpapi.ErrFederationReadOnly()
	}
	return nil
}

// GuardTask resolves the task to its owning project and guards that project. A
// task with no project (an inbox/standalone task) is always local, so it is a
// no-op. A task that does not exist is a no-op here — the downstream handler
// surfaces the 404; the guard never invents one. This is the seam every
// task-keyed mutation entry point calls (Federation v1 F5.2).
func (g *FederationReadOnlyGuard) GuardTask(c fiber.Ctx, taskID int64) *httpapi.AppError {
	if g == nil || g.fedProjects == nil {
		return nil
	}
	projectID, appErr := g.taskProjectID(c, taskID)
	if appErr != nil {
		return appErr
	}
	if projectID == nil {
		return nil
	}
	return g.GuardProject(c, *projectID)
}

// GuardTasks guards a batch of tasks (bulk complete/move/group): each task's
// owning project must be writable. Inbox/standalone tasks are skipped. It stops
// at the first read-only violation so a bulk op against any read-only federated
// task is rejected whole (no partial apply).
func (g *FederationReadOnlyGuard) GuardTasks(c fiber.Ctx, taskIDs []int64) *httpapi.AppError {
	if g == nil || g.fedProjects == nil {
		return nil
	}
	for _, id := range taskIDs {
		if appErr := g.GuardTask(c, id); appErr != nil {
			return appErr
		}
	}
	return nil
}

// GuardSection resolves the section to its parent project and guards it. A
// section that does not exist is a no-op (the handler surfaces the 404).
func (g *FederationReadOnlyGuard) GuardSection(c fiber.Ctx, sectionID int64) *httpapi.AppError {
	if g == nil || g.fedProjects == nil || g.sections == nil {
		return nil
	}
	sec, err := g.sections.Get(c.Context(), sectionID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil
	}
	if err != nil {
		return httpapi.ErrInternal("resolve section project").WithCause(err)
	}
	return g.GuardProject(c, sec.ProjectID)
}

// taskProjectID resolves the project a task belongs to, or nil for an
// inbox/standalone task. A not-found task is (nil, nil): the guard defers the
// 404 to the downstream handler rather than masking it.
func (g *FederationReadOnlyGuard) taskProjectID(c fiber.Ctx, taskID int64) (*int64, *httpapi.AppError) {
	t, err := g.tasks.Get(c.Context(), taskID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, httpapi.ErrInternal("resolve task project").WithCause(err)
	}
	return t.ProjectID, nil
}
