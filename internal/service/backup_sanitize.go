package service

// sanitizePayload removes dangling references from the input data. We do this
// rather than failing the restore because such references can legitimately
// exist in older databases that were written before all the constraints were
// enforced, and the user's intent is clearly to recover what they have.
//
// WARNING: this function mutates the input *BackupData in place — slices
// (Projects, ProjectSections, Tasks, TaskLabels, TaskRelations, ProjectLabels)
// are rewritten with shorter lengths backed by the same underlying arrays. Do not
// reuse the payload after calling sanitizePayload.
func sanitizePayload(d *BackupData) {
	contextIDs := idSet(d.Contexts, func(c BackupContext) int64 { return c.ID })
	labelIDs := idSet(d.Labels, func(l BackupLabel) int64 { return l.ID })

	// Drop projects whose context vanished (context_id is NOT NULL).
	projects := d.Projects[:0]
	for _, p := range d.Projects {
		if _, ok := contextIDs[p.ContextID]; ok {
			projects = append(projects, p)
		}
	}
	d.Projects = projects
	projectIDs := idSet(d.Projects, func(p BackupProject) int64 { return p.ID })

	// Drop sections whose project vanished.
	sections := d.ProjectSections[:0]
	for _, s := range d.ProjectSections {
		if _, ok := projectIDs[s.ProjectID]; ok {
			sections = append(sections, s)
		}
	}
	d.ProjectSections = sections
	sectionIDs := idSet(d.ProjectSections, func(s BackupProjectSection) int64 { return s.ID })

	// Tasks: null nullable refs to missing rows; drop tasks whose required
	// placement (inbox OR context) is unsatisfiable. Done in three passes so
	// the parent_id heal sees the final survivor set — a child appearing
	// before its parent in the slice could otherwise keep a parent_id that
	// gets dropped later in the same pass.
	for i := range d.Tasks {
		t := &d.Tasks[i]
		if t.ContextID != nil {
			if _, ok := contextIDs[*t.ContextID]; !ok {
				t.ContextID = nil
			}
		}
		if t.ProjectID != nil {
			if _, ok := projectIDs[*t.ProjectID]; !ok {
				t.ProjectID = nil
				t.SectionID = nil
			}
		}
		if t.SectionID != nil {
			if _, ok := sectionIDs[*t.SectionID]; !ok {
				t.SectionID = nil
			}
		}
	}
	// CHECK constraint: exactly one of inbox_id / context_id must be set.
	taskIDs := make(map[int64]struct{}, len(d.Tasks))
	for _, t := range d.Tasks {
		if (t.InboxID == nil) == (t.ContextID == nil) {
			continue
		}
		taskIDs[t.ID] = struct{}{}
	}
	tasks := d.Tasks[:0]
	for _, t := range d.Tasks {
		if _, ok := taskIDs[t.ID]; !ok {
			continue
		}
		if t.ParentID != nil {
			if _, ok := taskIDs[*t.ParentID]; !ok {
				t.ParentID = nil
			}
		}
		tasks = append(tasks, t)
	}
	d.Tasks = tasks

	// Drop link rows that lost an endpoint.
	tl := d.TaskLabels[:0]
	for _, l := range d.TaskLabels {
		if _, t := taskIDs[l.TaskID]; !t {
			continue
		}
		if _, lb := labelIDs[l.LabelID]; !lb {
			continue
		}
		tl = append(tl, l)
	}
	d.TaskLabels = tl

	// A relation needs both of its tasks; either one dropped above makes the row
	// unrestorable (both FKs are NOT NULL).
	trel := d.TaskRelations[:0]
	for _, r := range d.TaskRelations {
		if _, s := taskIDs[r.SourceTaskID]; !s {
			continue
		}
		if _, t := taskIDs[r.TargetTaskID]; !t {
			continue
		}
		trel = append(trel, r)
	}
	d.TaskRelations = trel

	pl := d.ProjectLabels[:0]
	for _, l := range d.ProjectLabels {
		if _, p := projectIDs[l.ProjectID]; !p {
			continue
		}
		if _, lb := labelIDs[l.LabelID]; !lb {
			continue
		}
		pl = append(pl, l)
	}
	d.ProjectLabels = pl
}

func idSet[T any](rows []T, key func(T) int64) map[int64]struct{} {
	m := make(map[int64]struct{}, len(rows))
	for _, r := range rows {
		m[key(r)] = struct{}{}
	}
	return m
}
