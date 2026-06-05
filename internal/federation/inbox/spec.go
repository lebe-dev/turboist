package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
)

// now is the injectable wall clock for the soft-delete / ghost-row timestamps;
// production uses time.Now. (A package var keeps Apply's signature small; tests
// that need a pinned clock can swap it.)
var now = time.Now

// entitySpec describes how one federated entity type maps onto a local table:
// which table holds it and which event field names map to which columns. ONLY
// the federated field set is listed — turboist-local fields (troiki/plan/etc.)
// are deliberately absent so a peer naming them is ignored, not rejected (§3).
type entitySpec struct {
	table  string
	fields map[string]string // event field name -> SQL column
}

// columnValidators maps a constrained SQL column to a whitelist validator that
// runs on the incoming (decoded JSON) value BEFORE it reaches a raw UPDATE.
// Without this the only line of defence is the table CHECK constraint, and a
// CHECK failure rolls back the whole apply tx as an opaque, retried-forever
// error — head-of-line blocking the per-project apply queue on one bad field
// from one peer (the W-8 buggy/cross-app sender). Validating here lets the
// caller classify the failure as a permanent poison reject instead.
//
// Only the enum/palette-constrained columns are listed; free-text columns
// (title/description) and the timestamp/integer/bool columns are coerced and
// shape-checked in coerceValue and need no value whitelist.
var columnValidators = map[string]func(any) bool{
	"priority": func(v any) bool {
		s, ok := v.(string)
		return ok && model.Priority(s).IsValid()
	},
	"status": func(v any) bool {
		s, ok := v.(string)
		if !ok {
			return false
		}
		// tasks.status and projects.status share the open/completed/cancelled
		// values; projects additionally allow 'archived'. Accept the superset
		// here (the per-table CHECK is the final backstop) so a project that is
		// legitimately 'archived' on the sender is not poison-rejected.
		return model.TaskStatus(s).IsValid() || model.ProjectStatus(s).IsValid()
	},
	"color": func(v any) bool {
		s, ok := v.(string)
		return ok && isValidColor(s)
	},
}

// validNamedColors mirrors the projects.color CHECK named palette (migration
// 001). Kept local to the inbox package so it does not import httpapi.
var validNamedColors = map[string]struct{}{
	"red": {}, "orange": {}, "yellow": {}, "green": {}, "teal": {},
	"blue": {}, "purple": {}, "pink": {}, "grey": {}, "brown": {},
}

// isValidColor accepts a named palette color or a #rrggbb hex string, matching
// the projects.color CHECK constraint (migration 001).
func isValidColor(c string) bool {
	if _, ok := validNamedColors[c]; ok {
		return true
	}
	if len(c) == 7 && c[0] == '#' {
		for _, ch := range c[1:] {
			isDigit := ch >= '0' && ch <= '9'
			isLower := ch >= 'a' && ch <= 'f'
			isUpper := ch >= 'A' && ch <= 'F'
			if !isDigit && !isLower && !isUpper {
				return false
			}
		}
		return true
	}
	return false
}

// entitySpecs is the per-entity-type federated field whitelist. Comments and
// checklist items are intentionally NOT listed here in F3.1: when their F0.2
// schema is present a later milestone adds them; until then their events
// degrade gracefully (the upsert path returns early for an unknown entity type).
var entitySpecs = map[events.EntityType]entitySpec{
	events.EntityProject: {
		table: "projects",
		fields: map[string]string{
			"title":       "title",
			"description": "description",
			"color":       "color",
			"status":      "status",
		},
	},
	events.EntitySection: {
		table: "project_sections",
		fields: map[string]string{
			"title":    "title",
			"position": "position",
		},
	},
	events.EntityTask: {
		table: "tasks",
		fields: map[string]string{
			"title":             "title",
			"description":       "description",
			"priority":          "priority",
			"status":            "status",
			"due_at":            "due_at",
			"due_has_time":      "due_has_time",
			"deadline_at":       "deadline_at",
			"deadline_has_time": "deadline_has_time",
			"completed_at":      "completed_at",
		},
	},
}

// createGhost materialises a minimal local row for an entity the receiver has
// never seen, preserving the cross-instance client_id so per-field events
// resolve to it (§10.4a). The row carries safe defaults that satisfy the table's
// NOT NULL / CHECK constraints; the event's fields are applied on top by the
// caller. Ghost rows may transiently violate higher-level invariants until the
// fields land — an accepted F3.1 risk.
func (a *Applier) createGhost(ctx context.Context, tx *sql.Tx, spec entitySpec, clientID string, localProjectID int64) (int64, error) {
	nowStr := model.FormatUTC(now())
	var res sql.Result
	var err error
	switch spec.table {
	case "projects":
		var contextID int64
		if err := tx.QueryRowContext(ctx,
			`SELECT context_id FROM projects WHERE id = ?`, localProjectID).Scan(&contextID); err != nil {
			return 0, fmt.Errorf("ghost project context: %w", err)
		}
		res, err = tx.ExecContext(ctx,
			`INSERT INTO projects (context_id, title, description, color, status, project_type, is_pinned, is_federated, client_id, created_at, updated_at)
			 VALUES (?, '', '', 'grey', 'open', 'generic', 0, 1, ?, ?, ?)`,
			contextID, clientID, nowStr, nowStr)
	case "project_sections":
		res, err = tx.ExecContext(ctx,
			`INSERT INTO project_sections (project_id, title, position, client_id, created_at, updated_at)
			 VALUES (?, '', 0, ?, ?, ?)`,
			localProjectID, clientID, nowStr, nowStr)
	case "tasks":
		var contextID int64
		if err := tx.QueryRowContext(ctx,
			`SELECT context_id FROM projects WHERE id = ?`, localProjectID).Scan(&contextID); err != nil {
			return 0, fmt.Errorf("ghost task context: %w", err)
		}
		res, err = tx.ExecContext(ctx,
			`INSERT INTO tasks (title, description, context_id, project_id, priority, status, day_part, plan_state, is_pinned, client_id, created_at, updated_at)
			 VALUES ('', '', ?, ?, 'no-priority', 'open', 'none', 'none', 0, ?, ?, ?)`,
			contextID, localProjectID, clientID, nowStr, nowStr)
	default:
		return 0, fmt.Errorf("ghost: unsupported table %q", spec.table)
	}
	if err != nil {
		return 0, fmt.Errorf("create ghost %s %q: %w", spec.table, clientID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("ghost last id: %w", err)
	}
	return id, nil
}

// setColumn writes one winning field value to its column, bumping updated_at in
// the same statement so the local wall-clock mirror stays fresh. The value is a
// decoded JSON scalar (string/number/bool/nil); coerceValue normalises it to the
// SQL representation each column expects.
func setColumn(ctx context.Context, tx *sql.Tx, table, column string, localID int64, value any) error {
	sqlVal, err := coerceValue(column, value)
	if err != nil {
		return err
	}
	nowStr := model.FormatUTC(now())
	q := fmt.Sprintf(`UPDATE %s SET %s = ?, updated_at = ? WHERE id = ?`, table, column)
	if _, err := tx.ExecContext(ctx, q, sqlVal, nowStr, localID); err != nil {
		return fmt.Errorf("set %s.%s: %w", table, column, err)
	}
	return nil
}

// coerceValue normalises a decoded JSON field value to the SQL form its column
// expects. The boolean-flag and integer columns are stored as INTEGER; nullable
// timestamp columns accept nil; everything else is passed through. json numbers
// decode as json.Number (CanonicalJSON uses UseNumber) or float64.
func coerceValue(column string, value any) (any, error) {
	switch column {
	case "due_has_time", "deadline_has_time":
		return boolToInt(value), nil
	case "position":
		n, err := toInt64(value)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", column, err)
		}
		return n, nil
	case "due_at", "deadline_at", "completed_at":
		if value == nil {
			return nil, nil
		}
		return value, nil
	default:
		if value == nil {
			return "", nil
		}
		return value, nil
	}
}

func boolToInt(v any) int {
	switch t := v.(type) {
	case bool:
		if t {
			return 1
		}
		return 0
	case float64:
		if t != 0 {
			return 1
		}
		return 0
	}
	return 0
}

func toInt64(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		return int64(t), nil
	case int:
		return int64(t), nil
	case int64:
		return t, nil
	case json.Number:
		return t.Int64()
	}
	return 0, fmt.Errorf("not an integer: %T", v)
}
