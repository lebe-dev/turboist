// Package snapshot builds and applies a project bootstrap snapshot (Federation
// v1 F2.3, US-2.3). The owner serialises a federated project's current state to
// NDJSON; a freshly joined peer replays it into a new local project.
//
// Read path is BUFFER-FIRST by decision (§3 / R1): the owner takes a consistent
// read of the project (project → sections → live tasks → tombstones → field_hlc)
// into an in-memory Snapshot, RELEASES the lone writer connection, and only then
// streams the NDJSON from the buffer. On SetMaxOpenConns(1) streaming inside a
// long-held read transaction would stall every app write for the whole bootstrap
// (up to 60s, NFR-1.4); buffering eliminates that availability regression.
//
// The NDJSON stream is:
//
//	{"type":"project","entity":{...}}
//	{"type":"section","entity":{...}}      (0+)
//	{"type":"task","entity":{...}}         (0+, live only)
//	{"type":"tombstone","entity_type":"task","entity_id":"<client_id>"}  (0+)
//	{"type":"field_hlc","entity_type":"...","entity_id":"...","field":"...","hlc":"..."}  (0+)
//	{"type":"end","as_of_hlc":"..."}
package snapshot

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/repo"
)

// Line type tags for the NDJSON stream.
const (
	lineProject   = "project"
	lineSection   = "section"
	lineTask      = "task"
	lineTombstone = "tombstone"
	lineFieldHLC  = "field_hlc"
	lineEnd       = "end"
)

// entityTypeTask / entityTypeProject / entityTypeSection are the per-field HLC
// and tombstone entity_type values.
const (
	entityTypeProject = "project"
	entityTypeSection = "section"
	entityTypeTask    = "task"
)

// ProjectLine is the federated project field set carried in the snapshot. Only
// the synced fields travel; troiki/local-only fields are excluded (§3).
type ProjectLine struct {
	ClientID    string `json:"client_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// SectionLine is the federated section field set.
type SectionLine struct {
	ClientID  string `json:"client_id"`
	Title     string `json:"title"`
	Position  int    `json:"position"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// TaskLine is the federated task field set. SectionClientID maps a task to its
// section by the section's cross-instance client_id (resolved to the local int64
// on apply). ParentClientID maps a subtask to its parent task by the parent's
// cross-instance client_id (resolved to the local int64 on apply), preserving the
// subtask hierarchy across instances (a project task may carry parent_id — the
// schema only forbids inbox_id+parent_id together, 001_schema.sql:118).
// Troiki/plan/day-part/postpone/pin fields are local-only (§3).
type TaskLine struct {
	ClientID        string  `json:"client_id"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	Priority        string  `json:"priority"`
	Status          string  `json:"status"`
	DueAt           *string `json:"due_at,omitempty"`
	DueHasTime      bool    `json:"due_has_time"`
	DeadlineAt      *string `json:"deadline_at,omitempty"`
	DeadlineHasTime bool    `json:"deadline_has_time"`
	CompletedAt     *string `json:"completed_at,omitempty"`
	SectionClientID string  `json:"section_client_id,omitempty"`
	ParentClientID  string  `json:"parent_client_id,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// Tombstone marks a soft-deleted entity by its cross-instance client_id so a
// peer never resurrects it on a later stale update (US-2.3 AC3 / §7.2).
type Tombstone struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
}

// FieldHLC is one per-field HLC row (§5.4). It is REQUIRED in the snapshot —
// without it the joiner cannot merge future events correctly (§7.2).
type FieldHLC struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Field      string `json:"field"`
	HLC        string `json:"hlc"`
}

// Snapshot is the buffered, consistent as-of read of a federated project. It is
// produced by Build (holding no DB connection afterwards) and serialised by
// WriteNDJSON.
type Snapshot struct {
	Project    ProjectLine
	Sections   []SectionLine
	Tasks      []TaskLine
	Tombstones []Tombstone
	FieldHLCs  []FieldHLC
	AsOfHLC    string
}

// Build reads a consistent snapshot of the federated project into memory and
// returns it, releasing the writer connection before any streaming (buffer-first
// — §3 / R1). nodeID seeds the synthetic as_of HLC when the project carries no
// field_hlc rows yet (a freshly federated project). ErrProjectNotFound is
// returned when the project does not exist or is a tombstone.
func Build(ctx context.Context, database *sql.DB, projectID int64, nodeID string) (*Snapshot, error) {
	snap := &Snapshot{}
	// One short read transaction takes the whole consistent read; it is released
	// the moment Build returns, long before WriteNDJSON streams the buffer.
	err := db.WithTx(ctx, database, func(tx *sql.Tx) error {
		proj, projClientID, err := readProject(ctx, tx, projectID)
		if err != nil {
			return err
		}
		snap.Project = proj

		sections, sectionClientByID, err := readSections(ctx, tx, projectID)
		if err != nil {
			return err
		}
		snap.Sections = sections

		tasks, err := readLiveTasks(ctx, tx, projectID, sectionClientByID)
		if err != nil {
			return err
		}
		snap.Tasks = tasks

		tombstones, err := readTombstones(ctx, tx, projectID)
		if err != nil {
			return err
		}
		snap.Tombstones = tombstones

		entityIDs := collectEntityIDs(projClientID, sections, tasks, tombstones)
		fieldHLCs, maxHLC, err := readFieldHLCs(ctx, tx, entityIDs)
		if err != nil {
			return err
		}
		snap.FieldHLCs = fieldHLCs
		snap.AsOfHLC = maxHLC
		return nil
	})
	if err != nil {
		return nil, err
	}
	if snap.AsOfHLC == "" {
		// A freshly federated project with no per-field history yet — mint a
		// baseline as_of so the joiner has a non-empty cursor (last_received_hlc).
		snap.AsOfHLC = hlc.HLC{PhysicalMS: 0, Logical: 0, NodeID: nodeID}.String()
	}
	return snap, nil
}

// WriteNDJSON streams the buffered snapshot to w as NDJSON, project first and the
// end sentinel last. It performs NO database access — it serialises only the
// in-memory buffer (the buffer-first contract).
func WriteNDJSON(w *bufio.Writer, snap *Snapshot) error {
	if err := writeLine(w, map[string]any{"type": lineProject, "entity": snap.Project}); err != nil {
		return err
	}
	for _, s := range snap.Sections {
		if err := writeLine(w, map[string]any{"type": lineSection, "entity": s}); err != nil {
			return err
		}
	}
	for _, tk := range snap.Tasks {
		if err := writeLine(w, map[string]any{"type": lineTask, "entity": tk}); err != nil {
			return err
		}
	}
	for _, tomb := range snap.Tombstones {
		if err := writeLine(w, map[string]any{"type": lineTombstone, "entity_type": tomb.EntityType, "entity_id": tomb.EntityID}); err != nil {
			return err
		}
	}
	for _, fh := range snap.FieldHLCs {
		if err := writeLine(w, map[string]any{"type": lineFieldHLC, "entity_type": fh.EntityType, "entity_id": fh.EntityID, "field": fh.Field, "hlc": fh.HLC}); err != nil {
			return err
		}
	}
	return writeLine(w, map[string]any{"type": lineEnd, "as_of_hlc": snap.AsOfHLC})
}

func writeLine(w *bufio.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal snapshot line: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	return w.WriteByte('\n')
}

// readProject reads the federated project field set; returns ErrProjectNotFound
// when missing/tombstoned. The second return is the project client_id (used to
// scope which field_hlc rows belong to this project).
func readProject(ctx context.Context, tx *sql.Tx, projectID int64) (ProjectLine, string, error) {
	var p ProjectLine
	err := tx.QueryRowContext(ctx,
		`SELECT client_id, title, description, color, status, created_at, updated_at
		   FROM projects WHERE id = ? AND deleted_at IS NULL`, projectID).
		Scan(&p.ClientID, &p.Title, &p.Description, &p.Color, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectLine{}, "", repo.ErrNotFound
	}
	if err != nil {
		return ProjectLine{}, "", fmt.Errorf("read snapshot project: %w", err)
	}
	return p, p.ClientID, nil
}

// readSections reads live sections and a map from local section id → client_id so
// tasks can be linked to their section by the portable client_id.
func readSections(ctx context.Context, tx *sql.Tx, projectID int64) ([]SectionLine, map[int64]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, client_id, title, position, created_at, updated_at
		   FROM project_sections WHERE project_id = ? AND deleted_at IS NULL
		  ORDER BY position ASC`, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("read snapshot sections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SectionLine, 0)
	byID := map[int64]string{}
	for rows.Next() {
		var id int64
		var s SectionLine
		if err := rows.Scan(&id, &s.ClientID, &s.Title, &s.Position, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, nil, err
		}
		byID[id] = s.ClientID
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, byID, nil
}

// readLiveTasks reads the live (non-tombstoned) tasks placed in the project,
// carrying only the federated field set and linking each to its section by the
// section's client_id.
func readLiveTasks(ctx context.Context, tx *sql.Tx, projectID int64, sectionClientByID map[int64]string) ([]TaskLine, error) {
	// LEFT JOIN to the parent task (only when the parent is itself live) so a
	// subtask carries its parent's portable client_id; the relationship is
	// resolved back to a local int64 on apply (US-2.3, parent_id is not flattened).
	rows, err := tx.QueryContext(ctx,
		`SELECT t.client_id, t.title, t.description, t.priority, t.status,
		        t.due_at, t.due_has_time, t.deadline_at, t.deadline_has_time, t.completed_at, t.section_id,
		        p.client_id, t.created_at, t.updated_at
		   FROM tasks t
		   LEFT JOIN tasks p ON p.id = t.parent_id AND p.deleted_at IS NULL
		  WHERE t.project_id = ? AND t.deleted_at IS NULL
		  ORDER BY t.id ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("read snapshot tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]TaskLine, 0)
	for rows.Next() {
		var tk TaskLine
		var dueAt, deadlineAt, completedAt, parentClientID sql.NullString
		var dueHasTime, deadlineHasTime int
		var sectionID sql.NullInt64
		if err := rows.Scan(&tk.ClientID, &tk.Title, &tk.Description, &tk.Priority, &tk.Status,
			&dueAt, &dueHasTime, &deadlineAt, &deadlineHasTime, &completedAt, &sectionID,
			&parentClientID, &tk.CreatedAt, &tk.UpdatedAt); err != nil {
			return nil, err
		}
		tk.DueAt = nullStrPtr(dueAt)
		tk.DueHasTime = dueHasTime == 1
		tk.DeadlineAt = nullStrPtr(deadlineAt)
		tk.DeadlineHasTime = deadlineHasTime == 1
		tk.CompletedAt = nullStrPtr(completedAt)
		if sectionID.Valid {
			tk.SectionClientID = sectionClientByID[sectionID.Int64]
		}
		if parentClientID.Valid {
			tk.ParentClientID = parentClientID.String
		}
		out = append(out, tk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// readTombstones reads the soft-deleted tasks AND sections of the project as
// tombstones (by client_id) so the joiner records them and never resurrects them
// (§7.2). Each table is read and its cursor fully closed before the next query —
// two simultaneous open cursors would stall the single writer connection
// (SetMaxOpenConns(1)). Section tombstones are what let an owner-deleted section
// converge on the joiner instead of lingering as a ghost (Federation v1 F4.2).
func readTombstones(ctx context.Context, tx *sql.Tx, projectID int64) ([]Tombstone, error) {
	out := make([]Tombstone, 0)
	read := func(table, entityType string) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT client_id FROM `+table+` WHERE project_id = ? AND deleted_at IS NOT NULL ORDER BY id ASC`,
			projectID)
		if err != nil {
			return fmt.Errorf("read snapshot %s tombstones: %w", table, err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var clientID string
			if err := rows.Scan(&clientID); err != nil {
				return err
			}
			if clientID == "" {
				continue
			}
			out = append(out, Tombstone{EntityType: entityType, EntityID: clientID})
		}
		return rows.Err()
	}
	if err := read("tasks", entityTypeTask); err != nil {
		return nil, err
	}
	if err := read("project_sections", entityTypeSection); err != nil {
		return nil, err
	}
	return out, nil
}

// readFieldHLCs reads every per-field HLC row whose entity_id is one of this
// project's entities, and returns the lexically-max HLC seen (the snapshot's
// as_of, §7.4 — all lines correspond to one HLC moment).
func readFieldHLCs(ctx context.Context, tx *sql.Tx, entityIDs map[string]struct{}) ([]FieldHLC, string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT entity_type, entity_id, field_name, hlc FROM entity_field_hlc ORDER BY hlc ASC`)
	if err != nil {
		return nil, "", fmt.Errorf("read snapshot field_hlc: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]FieldHLC, 0)
	var maxHLC string
	for rows.Next() {
		var fh FieldHLC
		if err := rows.Scan(&fh.EntityType, &fh.EntityID, &fh.Field, &fh.HLC); err != nil {
			return nil, "", err
		}
		if _, ok := entityIDs[fh.EntityID]; !ok {
			continue
		}
		out = append(out, fh)
		if hlc.CompareString(fh.HLC, maxHLC) > 0 {
			maxHLC = fh.HLC
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return out, maxHLC, nil
}

// collectEntityIDs gathers the client_ids of every entity in the snapshot so
// field_hlc rows can be scoped to this project.
func collectEntityIDs(projectClientID string, sections []SectionLine, tasks []TaskLine, tombstones []Tombstone) map[string]struct{} {
	ids := map[string]struct{}{}
	if projectClientID != "" {
		ids[projectClientID] = struct{}{}
	}
	for _, s := range sections {
		if s.ClientID != "" {
			ids[s.ClientID] = struct{}{}
		}
	}
	for _, tk := range tasks {
		if tk.ClientID != "" {
			ids[tk.ClientID] = struct{}{}
		}
	}
	for _, tomb := range tombstones {
		if tomb.EntityID != "" {
			ids[tomb.EntityID] = struct{}{}
		}
	}
	return ids
}

func nullStrPtr(ns sql.NullString) *string {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	v := ns.String
	return &v
}
