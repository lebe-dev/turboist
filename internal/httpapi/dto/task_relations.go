package dto

import (
	"github.com/lebe-dev/turboist/internal/model"
)

// TaskRelationDTO is one relation as seen from the task it was loaded for.
// Direction is relative to that task — "outgoing" means it blocks Task, "incoming"
// means Task blocks it — so the same stored row serialises differently on each of
// its two endpoints. Only meaningful for type "blocks"; "related" is symmetric.
type TaskRelationDTO struct {
	ID        int64   `json:"id"`
	Type      string  `json:"type"`
	Direction string  `json:"direction"`
	Task      TaskDTO `json:"task"`
	CreatedAt string  `json:"createdAt"`
}

// taskRelationsFromModel maps the hydrated peers. Returns nil (not an empty slice)
// for no relations so TaskDTO.Relations stays `omitempty` on the list paths, which
// never hydrate it — an empty array there would falsely read as "has none".
func taskRelationsFromModel(rels []model.TaskRelation, baseURL string) []TaskRelationDTO {
	if len(rels) == 0 {
		return nil
	}
	out := make([]TaskRelationDTO, len(rels))
	for i, r := range rels {
		var peer TaskDTO
		if r.Other != nil {
			peer = TaskFromModel(*r.Other, baseURL)
		}
		out[i] = TaskRelationDTO{
			ID:        r.ID,
			Type:      string(r.Type),
			Direction: string(r.Direction),
			Task:      peer,
			CreatedAt: FormatTime(r.CreatedAt),
		}
	}
	return out
}

// CreateTaskRelationRequest is the body for POST /tasks/:id/relations.
// Direction is interpreted relative to the task in the path and is ignored for
// type "related".
type CreateTaskRelationRequest struct {
	TargetTaskID int64  `json:"targetTaskId"`
	Type         string `json:"type"`
	Direction    string `json:"direction"`
}
