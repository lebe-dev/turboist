package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/todoist"
)

type ProjectsHandler struct {
	cache *todoist.Cache
}

func NewProjectsHandler(cache *todoist.Cache) *ProjectsHandler {
	return &ProjectsHandler{cache: cache}
}

type projectWithSections struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Sections []*todoist.Section `json:"sections"`
}

type projectsResponse struct {
	Projects []projectWithSections `json:"projects"`
}

// Projects handles GET /api/projects
func (h *ProjectsHandler) Projects(c fiber.Ctx) error {
	projects := h.cache.Projects()
	sections := h.cache.Sections()

	sectionsByProject := make(map[string][]*todoist.Section)
	for _, s := range sections {
		sectionsByProject[s.ProjectID] = append(sectionsByProject[s.ProjectID], s)
	}

	result := make([]projectWithSections, len(projects))
	for i, p := range projects {
		secs := sectionsByProject[p.ID]
		if secs == nil {
			secs = []*todoist.Section{}
		}
		result[i] = projectWithSections{
			ID:       p.ID,
			Name:     p.Name,
			Sections: secs,
		}
	}

	return c.JSON(projectsResponse{Projects: result})
}
