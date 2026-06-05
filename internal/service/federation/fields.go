package federation

import (
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// The federated field sets below MUST stay in lock-step with the receiver: the
// snapshot field sets (internal/federation/snapshot) and the inbox apply
// whitelist (internal/federation/inbox.entitySpecs) recognise EXACTLY these
// names. Turboist-local fields (troiki_category, troiki_capacity_granted,
// project_type, day_part, plan_state, postpone_count, pin fields, is_private)
// are deliberately EXCLUDED so a peer without them is never poison-rejected
// (FEDERATION-IMPLEMENTATION-PLAN §3 DEVIATE row, W-8).
//
// Field VALUES use the same wire shapes the inbox coerceValue expects: enum and
// free-text columns as strings, *_has_time as bools, and the nullable timestamp
// columns (due_at/deadline_at/completed_at) as ISO-8601 UTC strings or nil.
//
// parent/section linkage is carried by the SNAPSHOT only (resolved via client_id
// on bootstrap), not by per-field events — inbox entitySpecs[task] does not list
// section_client_id/parent_client_id — so it is intentionally omitted from the
// emitted event field set to keep emit↔apply in agreement.

// taskCreateFields builds the full federated field set for a task op=create from
// the repo create input. A freshly created task is always status=open and never
// completed.
func taskCreateFields(in repo.CreateTask) map[string]any {
	priority := in.Priority
	if priority == "" {
		priority = model.PriorityNone
	}
	return map[string]any{
		"title":             in.Title,
		"description":       in.Description,
		"priority":          string(priority),
		"status":            string(model.TaskStatusOpen),
		"due_at":            formatNullableTime(in.DueAt),
		"due_has_time":      in.DueHasTime,
		"deadline_at":       formatNullableTime(in.DeadlineAt),
		"deadline_has_time": in.DeadlineHasTime,
		"completed_at":      nil,
	}
}

// taskUpdateFields maps the CHANGED federated columns of a TaskUpdate to their
// new values so an op=update event carries ONLY the fields the mutation touched
// (per-field LWW — disjoint edits from two instances both land, US-3.3 AC1).
// Local-only fields (day_part/plan_state/troiki/postpone/private/recurrence) are
// skipped: they are not in the federated set. Returns an empty map when nothing
// federated changed (the caller then skips the emit).
func taskUpdateFields(u repo.TaskUpdate) map[string]any {
	fields := map[string]any{}
	if u.Title != nil {
		fields["title"] = *u.Title
	}
	if u.Description != nil {
		fields["description"] = *u.Description
	}
	if u.Priority != nil {
		fields["priority"] = string(*u.Priority)
	}
	if u.Status != nil {
		fields["status"] = string(*u.Status)
		// status=completed stamps completed_at; anything else clears it — mirror
		// repo.TaskRepo.Update so the federated completed_at tracks the local row.
		if *u.Status == model.TaskStatusCompleted {
			ts := model.FormatUTC(time.Now())
			if u.CompletedAt != nil {
				ts = model.FormatUTC(*u.CompletedAt)
			}
			fields["completed_at"] = ts
		} else {
			fields["completed_at"] = nil
		}
	}
	if u.DueAtClear {
		fields["due_at"] = nil
		fields["due_has_time"] = false
	} else {
		if u.DueAt != nil {
			fields["due_at"] = model.FormatUTC(*u.DueAt)
		}
		if u.DueHasTime != nil {
			fields["due_has_time"] = *u.DueHasTime
		}
	}
	if u.DeadlineAtClear {
		fields["deadline_at"] = nil
		fields["deadline_has_time"] = false
	} else {
		if u.DeadlineAt != nil {
			fields["deadline_at"] = model.FormatUTC(*u.DeadlineAt)
		}
		if u.DeadlineHasTime != nil {
			fields["deadline_has_time"] = *u.DeadlineHasTime
		}
	}
	return fields
}

func formatNullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return model.FormatUTC(*t)
}

// projectUpdateFields maps the CHANGED federated columns of a ProjectUpdate to
// their new values. The federated project field set is title/description/color
// (+ status, which the handler changes via a separate UpdateStatus path).
// context_id, is_private, project_type and troiki_category are local-only and
// excluded. Returns an empty map when nothing federated changed.
func projectUpdateFields(u repo.ProjectUpdate) map[string]any {
	fields := map[string]any{}
	if u.Title != nil {
		fields["title"] = *u.Title
	}
	if u.Description != nil {
		fields["description"] = *u.Description
	}
	if u.Color != nil {
		fields["color"] = *u.Color
	}
	return fields
}

// projectStatusFields builds the federated field set for a project status change
// (archive/complete/cancel/open). Only `status` travels — the local
// troiki_category clear that UpdateStatus performs is a turboist-local side
// effect, not part of the federated set (lock-step with inbox entitySpecs[project]
// which maps status→projects.status; the receiver's status validator accepts the
// open/completed/cancelled/archived superset).
func projectStatusFields(status model.ProjectStatus) map[string]any {
	return map[string]any{"status": string(status)}
}

// sectionUpdateFields maps the CHANGED federated columns of a SectionUpdate to
// their new values. The federated section field set is title (+ position, which
// the handler changes via a separate Reorder path). Returns an empty map when
// nothing federated changed.
func sectionUpdateFields(u repo.SectionUpdate) map[string]any {
	fields := map[string]any{}
	if u.Title != nil {
		fields["title"] = *u.Title
	}
	return fields
}
