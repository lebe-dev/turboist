package snapshot

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrInvalidHLC is returned when the snapshot carries an as_of or per-field HLC
// string that does not parse as a canonical HLC (F2.3 #7). Because per-field LWW
// compares HLC strings purely lexically (hlc.CompareString never errors), a
// malformed/empty/non-zero-padded HLC from a signature-trusted-but-buggy owner
// would silently corrupt the F3.2 pull cursor / LWW ordering; we reject it and
// roll the whole apply back rather than persist it verbatim.
var ErrInvalidHLC = errors.New("snapshot: malformed HLC string")

// ErrNoEnd is returned when the NDJSON stream ends without the terminating
// `{"type":"end"}` sentinel — the snapshot is incomplete and the whole apply is
// rolled back (US-2.3 AC5, no partial bootstrap).
var ErrNoEnd = errors.New("snapshot: stream ended without end sentinel")

// ErrNoProject is returned when the first line is not the project line; the
// snapshot is malformed and nothing is applied.
var ErrNoProject = errors.New("snapshot: first line is not a project")

// ApplyDeps are the joiner-side collaborators the consume path needs. They are
// repos; the apply runs all writes inside ONE db.WithTx so a mid-stream failure
// rolls everything back (US-2.3 AC5).
type ApplyDeps struct {
	DB          *sql.DB
	Projects    *repo.ProjectRepo
	Sections    *repo.ProjectSectionRepo
	Tasks       *repo.TaskRepo
	Contexts    *repo.ContextRepo
	FedProjects *repo.FederatedProjectRepo
	Snapshot    *repo.FederationSnapshotRepo
}

// ApplyParams carry the per-bootstrap inputs.
type ApplyParams struct {
	// OwnerInstanceURL is the origin instance this snapshot came from; it becomes
	// federated_projects.peer_instance_url + origin_instance_url on the joiner row.
	OwnerInstanceURL string
	// RemoteProjectID is the owner's project id (stored as the remote id). Optional.
	RemoteProjectID string
	// Permissions is the grade the joiner was granted at handshake.
	Permissions model.FederationPermission
	// ProtocolVersion is the negotiated version.
	ProtocolVersion int
	// Reader streams the NDJSON snapshot body.
	Reader io.Reader
	// Now is injectable for deterministic joined_at/timestamps; nil → time.Now.
	Now func() time.Time
}

// ApplyResult is the outcome of a successful bootstrap.
type ApplyResult struct {
	LocalProjectID int64
	AsOfHLC        string
}

// Apply replays an NDJSON snapshot into a brand-new local federated project
// (Federation v1 F2.3, US-2.3). It creates the local project mapped to
// (OwnerInstanceURL, project client_id), applies sections + live tasks preserving
// their cross-instance client_ids, records tombstones + field_hlc, writes the
// federated_projects mapping (is_owner=0) with last_received_hlc=as_of, and does
// it all in ONE transaction. There is no resume: any malformed line or write
// failure rolls the whole bootstrap back (US-2.3 AC5).
func Apply(ctx context.Context, deps ApplyDeps, params ApplyParams) (*ApplyResult, error) {
	now := params.Now
	if now == nil {
		now = time.Now
	}
	if deps.Snapshot == nil && deps.DB != nil {
		deps.Snapshot = repo.NewFederationSnapshotRepo(deps.DB)
	}

	snap, err := parseStream(params.Reader)
	if err != nil {
		return nil, err
	}

	// Validate every wire HLC before persisting anything: as_of_hlc (recorded as
	// the tombstone field HLC and the mapping's last_received_hlc / pull cursor)
	// and each per-field HLC line. A parse failure aborts the whole apply — no
	// partial bootstrap, consistent with the AC5 mid-stream rollback (F2.3 #7).
	if err := validateHLCs(snap); err != nil {
		return nil, err
	}

	var res ApplyResult
	err = db.WithTx(ctx, deps.DB, func(tx *sql.Tx) error {
		contextID, err := ensureContextTx(ctx, tx)
		if err != nil {
			return err
		}
		localProjectID, err := deps.Snapshot.InsertProjectTx(ctx, tx, repo.SnapshotProject{
			ClientID:    snap.Project.ClientID,
			ContextID:   contextID,
			Title:       snap.Project.Title,
			Description: snap.Project.Description,
			Color:       snap.Project.Color,
			Status:      snap.Project.Status,
			CreatedAt:   snap.Project.CreatedAt,
			UpdatedAt:   snap.Project.UpdatedAt,
		})
		if err != nil {
			return err
		}

		sectionLocalByClient := map[string]int64{}
		for _, s := range snap.Sections {
			sid, err := deps.Snapshot.InsertSectionTx(ctx, tx, localProjectID, repo.SnapshotSection{
				ClientID:  s.ClientID,
				Title:     s.Title,
				Position:  s.Position,
				CreatedAt: s.CreatedAt,
				UpdatedAt: s.UpdatedAt,
			})
			if err != nil {
				return err
			}
			if s.ClientID != "" {
				sectionLocalByClient[s.ClientID] = sid
			}
		}

		// taskLocalByClient maps each applied task's portable client_id to its new
		// local int64 so a subtask can resolve its parent. The owner emits tasks in
		// id-asc order, so a parent is always applied before its children — a single
		// forward pass resolves every parent link. A subtask whose parent is absent
		// (e.g. the parent was tombstoned and dropped from the snapshot) gracefully
		// becomes top-level rather than failing the bootstrap.
		taskLocalByClient := map[string]int64{}
		for _, tk := range snap.Tasks {
			var sectionID *int64
			if tk.SectionClientID != "" {
				if sid, ok := sectionLocalByClient[tk.SectionClientID]; ok {
					sectionID = &sid
				}
			}
			var parentID *int64
			if tk.ParentClientID != "" {
				if pid, ok := taskLocalByClient[tk.ParentClientID]; ok {
					parentID = &pid
				}
			}
			localTaskID, err := deps.Snapshot.InsertTaskTx(ctx, tx, localProjectID, repo.SnapshotTask{
				ClientID:        tk.ClientID,
				ContextID:       contextID,
				Title:           tk.Title,
				Description:     tk.Description,
				Priority:        tk.Priority,
				Status:          tk.Status,
				DueAt:           tk.DueAt,
				DueHasTime:      tk.DueHasTime,
				DeadlineAt:      tk.DeadlineAt,
				DeadlineHasTime: tk.DeadlineHasTime,
				CompletedAt:     tk.CompletedAt,
				SectionID:       sectionID,
				ParentID:        parentID,
				CreatedAt:       tk.CreatedAt,
				UpdatedAt:       tk.UpdatedAt,
			})
			if err != nil {
				return err
			}
			if tk.ClientID != "" {
				taskLocalByClient[tk.ClientID] = localTaskID
			}
		}

		// Tombstones: record the synthetic _deleted field HLC for each deleted
		// entity so a later stale update from any peer cannot resurrect it
		// (US-2.3 AC3 / §7.2). The joiner intentionally does NOT create a live row
		// for a tombstoned entity.
		for _, tomb := range snap.Tombstones {
			if err := deps.Snapshot.InsertFieldHLCTx(ctx, tx, tomb.EntityType, tomb.EntityID, "_deleted", snap.AsOfHLC); err != nil {
				return err
			}
		}

		for _, fh := range snap.FieldHLCs {
			if err := deps.Snapshot.InsertFieldHLCTx(ctx, tx, fh.EntityType, fh.EntityID, fh.Field, fh.HLC); err != nil {
				return err
			}
		}

		perm := params.Permissions
		if !perm.IsValid() {
			perm = model.FederationPermissionRead
		}
		ver := params.ProtocolVersion
		if ver == 0 {
			ver = 1
		}
		if err := deps.FedProjects.UpsertPeerRowTx(ctx, tx, model.FederatedProject{
			LocalProjectID:    localProjectID,
			PeerInstanceURL:   params.OwnerInstanceURL,
			RemoteProjectID:   params.RemoteProjectID,
			IsOwner:           false,
			OriginInstanceURL: params.OwnerInstanceURL,
			Permissions:       perm,
			ProtocolVersion:   ver,
			LastReceivedHLC:   snap.AsOfHLC,
			JoinedAt:          now(),
		}); err != nil {
			return err
		}

		res.LocalProjectID = localProjectID
		res.AsOfHLC = snap.AsOfHLC
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// validateHLCs checks that the snapshot's as_of HLC and every per-field HLC line
// parse as canonical HLCs. Any failure returns ErrInvalidHLC so the apply aborts
// before persisting (F2.3 #7). The tombstone field HLC reuses as_of, so the
// single as_of check covers it too.
func validateHLCs(snap *Snapshot) error {
	if _, err := hlc.Parse(snap.AsOfHLC); err != nil {
		return fmt.Errorf("%w: as_of_hlc %q: %v", ErrInvalidHLC, snap.AsOfHLC, err)
	}
	for _, fh := range snap.FieldHLCs {
		if _, err := hlc.Parse(fh.HLC); err != nil {
			return fmt.Errorf("%w: field_hlc %s/%s/%s %q: %v", ErrInvalidHLC, fh.EntityType, fh.EntityID, fh.Field, fh.HLC, err)
		}
	}
	return nil
}

// parseStream decodes the NDJSON body into a Snapshot, validating that the first
// line is the project and the last is the end sentinel. A malformed line returns
// an error (no partial Snapshot escapes).
func parseStream(r io.Reader) (*Snapshot, error) {
	if r == nil {
		return nil, errors.New("snapshot: nil reader")
	}
	snap := &Snapshot{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	first := true
	sawEnd := false
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			return nil, fmt.Errorf("snapshot: decode line: %w", err)
		}
		if first && probe.Type != lineProject {
			return nil, ErrNoProject
		}
		first = false
		if err := applyProbe(snap, probe.Type, line, &sawEnd); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("snapshot: read stream: %w", err)
	}
	if !sawEnd {
		return nil, ErrNoEnd
	}
	return snap, nil
}

// applyProbe decodes one typed line into the accumulating Snapshot.
func applyProbe(snap *Snapshot, typ string, line []byte, sawEnd *bool) error {
	switch typ {
	case lineProject:
		var l struct {
			Entity ProjectLine `json:"entity"`
		}
		if err := json.Unmarshal(line, &l); err != nil {
			return fmt.Errorf("snapshot: decode project: %w", err)
		}
		snap.Project = l.Entity
	case lineSection:
		var l struct {
			Entity SectionLine `json:"entity"`
		}
		if err := json.Unmarshal(line, &l); err != nil {
			return fmt.Errorf("snapshot: decode section: %w", err)
		}
		snap.Sections = append(snap.Sections, l.Entity)
	case lineTask:
		var l struct {
			Entity TaskLine `json:"entity"`
		}
		if err := json.Unmarshal(line, &l); err != nil {
			return fmt.Errorf("snapshot: decode task: %w", err)
		}
		snap.Tasks = append(snap.Tasks, l.Entity)
	case lineTombstone:
		var l Tombstone
		if err := json.Unmarshal(line, &l); err != nil {
			return fmt.Errorf("snapshot: decode tombstone: %w", err)
		}
		snap.Tombstones = append(snap.Tombstones, l)
	case lineFieldHLC:
		var l FieldHLC
		if err := json.Unmarshal(line, &l); err != nil {
			return fmt.Errorf("snapshot: decode field_hlc: %w", err)
		}
		snap.FieldHLCs = append(snap.FieldHLCs, l)
	case lineEnd:
		var l struct {
			AsOfHLC string `json:"as_of_hlc"`
		}
		if err := json.Unmarshal(line, &l); err != nil {
			return fmt.Errorf("snapshot: decode end: %w", err)
		}
		snap.AsOfHLC = l.AsOfHLC
		*sawEnd = true
	default:
		// Unknown line types are ignored for forward-compat (F6.1 refines this);
		// a v1 peer never emits them.
	}
	return nil
}

// ensureContextTx returns a context id to attach the joiner's federated project
// to: the first existing live context, or a freshly created "Federated" context
// when the joiner has none. The federated project must hang off some context
// (the projects.context_id FK is NOT NULL).
func ensureContextTx(ctx context.Context, tx *sql.Tx) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM contexts WHERE deleted_at IS NULL ORDER BY id ASC LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("snapshot: find context: %w", err)
	}
	nowStr := model.FormatUTC(time.Now())
	res, err := tx.ExecContext(ctx,
		`INSERT INTO contexts (name, color, is_favourite, client_id, created_at, updated_at)
		 VALUES ('Federated', 'blue', 0, ?, ?, ?)`,
		model.NewClientID(), nowStr, nowStr)
	if err != nil {
		return 0, fmt.Errorf("snapshot: create federated context: %w", err)
	}
	return res.LastInsertId()
}
