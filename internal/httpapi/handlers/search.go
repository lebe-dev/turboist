package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// SearchHandler serves GET /api/v1/search.
type SearchHandler struct {
	search  *repo.SearchRepo
	baseURL string
}

func NewSearchHandler(search *repo.SearchRepo, baseURL string) *SearchHandler {
	return &SearchHandler{search: search, baseURL: baseURL}
}

func (h *SearchHandler) Register(r fiber.Router) {
	r.Get("/search", h.search_)
}

type searchResponse struct {
	Tasks    *dto.PagedResponse[dto.TaskDTO]    `json:"tasks,omitempty"`
	Projects *dto.PagedResponse[dto.ProjectDTO] `json:"projects,omitempty"`
}

func (h *SearchHandler) search_(c fiber.Ctx) error {
	q := c.Query("q")
	if len([]rune(q)) < 2 {
		return httpapi.ErrValidation("q must be at least 2 characters")
	}
	searchType := c.Query("type", "all")
	if searchType != "tasks" && searchType != "projects" && searchType != "all" {
		return httpapi.ErrValidation("type must be tasks, projects, or all")
	}

	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	page := repo.Page{Limit: pp.Limit, Offset: pp.Offset}

	resp := searchResponse{}

	if searchType == "tasks" || searchType == "all" {
		tasks, total, err := h.search.SearchTasks(c.Context(), q, page)
		if err != nil {
			return httpapi.ErrInternal("search tasks")
		}
		r := dto.NewPagedResponse(tasksToDTO(tasks, h.baseURL), total, pp.Limit, pp.Offset)
		resp.Tasks = &r
	}

	if searchType == "projects" || searchType == "all" {
		projects, total, err := h.search.SearchProjects(c.Context(), q, page)
		if err != nil {
			return httpapi.ErrInternal("search projects")
		}
		r := dto.NewPagedResponse(projectsToDTO(projects), total, pp.Limit, pp.Offset)
		resp.Projects = &r
	}

	return c.JSON(resp)
}

func projectsToDTO(projects []model.Project) []dto.ProjectDTO {
	result := make([]dto.ProjectDTO, len(projects))
	for i, p := range projects {
		result[i] = dto.ProjectFromModel(p)
	}
	return result
}
