package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// projectStatsPeriodDTO holds one window's counters for a project.
// `previousCompleted` is the same counter over the equally long window right
// before it, which the radar draws as the comparison outline.
type projectStatsPeriodDTO struct {
	Completed         int `json:"completed"`
	PreviousCompleted int `json:"previousCompleted"`
}

// projectStatsItemDTO is one project's activity, keyed by period. The project
// itself is not repeated: the client already holds the project list.
type projectStatsItemDTO struct {
	ProjectID int64                            `json:"projectId"`
	Periods   map[string]projectStatsPeriodDTO `json:"periods"`
}

// projectStatsResponse powers the activity radar on the Projects page. Only
// projects that completed something in the widest window pair are present.
type projectStatsResponse struct {
	Ranges map[string]labelStatsRangeDTO `json:"ranges"`
	Items  []projectStatsItemDTO         `json:"items"`
}

// WithLocation sets the timezone the activity windows are anchored to. Without
// it windows are cut at UTC midnight.
func (h *ProjectHandler) WithLocation(loc *time.Location) *ProjectHandler {
	h.location = loc
	return h
}

func (h *ProjectHandler) stats(c fiber.Ctx) error {
	_, end := statsDayBounds(h.location)

	ranges := make([]repo.LabelUsageRange, len(labelStatsPeriods))
	rangeDTOs := make(map[string]labelStatsRangeDTO, len(labelStatsPeriods))
	for i, p := range labelStatsPeriods {
		start := end.AddDate(0, 0, -p.days)
		ranges[i] = repo.LabelUsageRange{Start: start, End: end}
		rangeDTOs[p.key] = labelStatsRangeDTO{
			Start: model.FormatUTC(start),
			End:   model.FormatUTC(end),
			Days:  p.days,
		}
	}

	activity, err := h.projects.ActivityStats(c.Context(), ranges)
	if err != nil {
		return httpapi.ErrInternal("load project stats").WithCause(err)
	}

	items := make([]projectStatsItemDTO, len(activity))
	for i, a := range activity {
		periods := make(map[string]projectStatsPeriodDTO, len(labelStatsPeriods))
		for j, p := range labelStatsPeriods {
			periods[p.key] = projectStatsPeriodDTO{
				Completed:         a.Periods[j].Completed,
				PreviousCompleted: a.Periods[j].PreviousCompleted,
			}
		}
		items[i] = projectStatsItemDTO{ProjectID: a.ProjectID, Periods: periods}
	}
	return c.JSON(projectStatsResponse{Ranges: rangeDTOs, Items: items})
}
