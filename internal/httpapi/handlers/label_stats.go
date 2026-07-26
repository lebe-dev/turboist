package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// labelStatsLimit caps the number of labels in the usage report. Same 500-row
// ceiling the /config bootstrap uses for the label list.
const labelStatsLimit = 500

// labelStatsPeriods are the rolling windows the usage report is bucketed by,
// in the order they appear in the response's `periods` map.
//
// Rolling (last N days ending tonight) rather than calendar-aligned: the page
// answers "how often am I using this label lately", and a calendar week that is
// one day old would make every label look unused on Tuesday morning. It also
// makes the previous-window comparison symmetric.
var labelStatsPeriods = []struct {
	key  string
	days int
}{
	{"week", 7},
	{"month", 30},
	{"quarter", 90},
}

// labelStatsRangeDTO is the resolved window for one period key.
type labelStatsRangeDTO struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Days  int    `json:"days"`
}

// labelStatsPeriodDTO holds one window's counters. `previousApplied` is the same
// counter over the equally long window immediately before it, which is what the
// frontend turns into a trend arrow.
type labelStatsPeriodDTO struct {
	Applied         int `json:"applied"`
	PreviousApplied int `json:"previousApplied"`
	Completed       int `json:"completed"`
}

// labelStatsItemDTO is one label's row: the label itself, its period counters
// keyed by period, and the period-independent totals.
type labelStatsItemDTO struct {
	Label      dto.LabelDTO                   `json:"label"`
	TotalTasks int                            `json:"totalTasks"`
	OpenTasks  int                            `json:"openTasks"`
	Overdue    int                            `json:"overdue"`
	Projects   int                            `json:"projects"`
	LastUsedAt *string                        `json:"lastUsedAt"`
	Periods    map[string]labelStatsPeriodDTO `json:"periods"`
}

// labelStatsResponse powers the /labels stats page. Every label is included,
// never-used ones with zeros: the page sorts and filters client-side so
// switching the displayed period costs no round-trip, and the zero rows are what
// the "unused labels" cleanup list is built from.
type labelStatsResponse struct {
	Ranges map[string]labelStatsRangeDTO `json:"ranges"`
	Items  []labelStatsItemDTO           `json:"items"`
}

// dayBounds returns [start of today, start of tomorrow) in the configured
// timezone, as UTC instants. Ranges end at tomorrow's start so today counts in
// full instead of cutting off at the current clock time.
func (h *LabelHandler) dayBounds() (time.Time, time.Time) {
	loc := time.UTC
	if h.cfg != nil && h.cfg.Location != nil {
		loc = h.cfg.Location
	}
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}

func (h *LabelHandler) stats(c fiber.Ctx) error {
	todayStart, end := h.dayBounds()

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

	usage, err := h.labels.UsageStats(c.Context(), ranges, todayStart, labelStatsLimit)
	if err != nil {
		return httpapi.ErrInternal("load label stats").WithCause(err)
	}

	items := make([]labelStatsItemDTO, len(usage))
	for i, u := range usage {
		periods := make(map[string]labelStatsPeriodDTO, len(labelStatsPeriods))
		for j, p := range labelStatsPeriods {
			periods[p.key] = labelStatsPeriodDTO{
				Applied:         u.Periods[j].Applied,
				PreviousApplied: u.Periods[j].PreviousApplied,
				Completed:       u.Periods[j].Completed,
			}
		}
		var lastUsedAt *string
		if u.LastUsedAt != nil {
			s := model.FormatUTC(*u.LastUsedAt)
			lastUsedAt = &s
		}
		items[i] = labelStatsItemDTO{
			Label:      dto.LabelFromModel(u.Label),
			TotalTasks: u.TotalTasks,
			OpenTasks:  u.OpenTasks,
			Overdue:    u.Overdue,
			Projects:   u.Projects,
			LastUsedAt: lastUsedAt,
			Periods:    periods,
		}
	}

	return c.JSON(labelStatsResponse{Ranges: rangeDTOs, Items: items})
}
