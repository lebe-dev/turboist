package federation

import (
	"context"
	"database/sql"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// SectionMutator routes user-originated section mutations through the federation
// Emitter so a CREATE / UPDATE / DELETE of a section in a FEDERATED project emits
// the per-field HLC bump + signed outbox event the sync workers fan out
// (Federation v1 F3.1, US-3.2 AC1). A section is a federated entity
// (events.EntitySection) scoped to its project: the gate is keyed on the parent
// project's is_federated, so a section in a non-federated project incurs zero
// federation overhead. When federation is OFF the handler holds a nil
// *SectionMutator and bypasses it entirely.
type SectionMutator struct {
	emitter  *Emitter
	sections *repo.ProjectSectionRepo
}

// NewSectionMutator constructs the section federation facade over the Emitter +
// section repo. Wire emitter.WithCommitPing(worker.Ping) first so a federated
// mutation pushes immediately (NFR-1.1).
func NewSectionMutator(emitter *Emitter, sections *repo.ProjectSectionRepo) *SectionMutator {
	return &SectionMutator{emitter: emitter, sections: sections}
}

// Create inserts a section in projectID and, when that project is federated,
// emits a signed op=create event carrying the federated field set (title +
// position) — in ONE transaction with the insert (NFR-2). clientID is the
// section's cross-instance client_id (a fresh ULID); it is the event EntityID.
// Returns the new section's local id.
//
// The auto-assigned position is read once up front so it can travel in the event;
// the insert inside the emit tx derives the same value. On SetMaxOpenConns(1) the
// pool serialises writers, so the read and the insert observe the same max
// position (no concurrent append can interleave between them) — the same
// single-connection guarantee the rest of the federation store relies on (§3).
func (m *SectionMutator) Create(ctx context.Context, projectID int64, title, clientID string) (int64, error) {
	federated, err := m.projectFederated(ctx, projectID)
	if err != nil {
		return 0, err
	}
	if !federated {
		s, err := m.sections.Create(ctx, projectID, title)
		if err != nil {
			return 0, err
		}
		return s.ID, nil
	}

	position, err := m.nextPosition(ctx, projectID)
	if err != nil {
		return 0, err
	}
	var newID int64
	err = m.emitter.EmitMutation(ctx, MutationSpec{
		LocalProjectID: projectID,
		EntityType:     events.EntitySection,
		EntityID:       clientID,
		Op:             events.OpCreate,
		Fields: map[string]any{
			"title":    title,
			"position": position,
		},
	}, func(tx *sql.Tx) error {
		id, err := m.sections.CreateTx(ctx, tx, projectID, title, clientID)
		if err != nil {
			return err
		}
		newID = id
		return nil
	})
	if err != nil {
		return 0, err
	}
	return newID, nil
}

// Update applies a section field update and, when its project is federated, emits
// a signed op=update event carrying ONLY the changed federated fields (title) — in
// ONE transaction with the row update (NFR-2). section is the already-loaded
// section (the handler fetched it). A section with no ClientID, or in a
// non-federated project, or an update that changes no federated field, routes
// through the bare repo update.
func (m *SectionMutator) Update(ctx context.Context, section *model.ProjectSection, u repo.SectionUpdate) error {
	fields := sectionUpdateFields(u)
	if section.ClientID == "" || len(fields) == 0 {
		_, err := m.sections.Update(ctx, section.ID, u)
		return err
	}
	return m.emitter.EmitMutation(ctx, MutationSpec{
		LocalProjectID: section.ProjectID,
		EntityType:     events.EntitySection,
		EntityID:       section.ClientID,
		Op:             events.OpUpdate,
		Fields:         fields,
	}, func(tx *sql.Tx) error {
		return m.sections.UpdateTx(ctx, tx, section.ID, u)
	})
}

// Delete soft-deletes the section (and clears section_id on its live tasks) and,
// when its project is federated, emits an op=delete tombstone for the section
// entity — in ONE transaction (NFR-2). section is the already-loaded section.
func (m *SectionMutator) Delete(ctx context.Context, section *model.ProjectSection) error {
	if section.ClientID == "" {
		return m.sections.Delete(ctx, section.ID)
	}
	return m.emitter.EmitDeleteCascade(ctx, MutationSpec{
		LocalProjectID: section.ProjectID,
		EntityType:     events.EntitySection,
		EntityID:       section.ClientID,
		Op:             events.OpDelete,
	}, nil, func(tx *sql.Tx) error {
		return m.sections.DeleteTx(ctx, tx, section.ID)
	})
}

// projectFederated reports whether the section's parent project is federated, so
// Create can skip the position pre-read for a non-federated project.
func (m *SectionMutator) projectFederated(ctx context.Context, projectID int64) (bool, error) {
	var isFederated int
	err := m.emitter.db.QueryRowContext(ctx,
		`SELECT is_federated FROM projects WHERE id = ? AND deleted_at IS NULL`, projectID).Scan(&isFederated)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return isFederated == 1, nil
}

// nextPosition reads the position a freshly appended section will take (the
// project's current max live position + 1), matching repo.ProjectSectionRepo's
// own derivation so the emitted position equals the inserted one.
func (m *SectionMutator) nextPosition(ctx context.Context, projectID int64) (int, error) {
	var pos int
	if err := m.emitter.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM project_sections WHERE project_id = ? AND deleted_at IS NULL`,
		projectID).Scan(&pos); err != nil {
		return 0, err
	}
	return pos, nil
}
