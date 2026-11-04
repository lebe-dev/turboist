package handlers

import (
	"net/http"
	"sync"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/httpapi/openapi"
)

// openAPIRoute is the document's path inside the /api/v1 group.
const openAPIRoute = "/openapi.json"

// OpenAPIPath is where the server publishes its own OpenAPI document.
const OpenAPIPath = "/api/v1" + openAPIRoute

// OpenAPIDocument describes the API as a whole. Its version is the API's, not
// the build's, so the committed document does not change on every release.
func OpenAPIDocument() openapi.Document {
	return openapi.Document{
		Title:   "Turboist API",
		Version: "1",
		Description: "HTTP API of the Turboist task manager. Every failed request answers " +
			"with an error envelope `{error: {code, message, details}}`.",
		Error: httpapi.ErrorEnvelope{},
	}
}

// RegisterOpenAPI publishes the OpenAPI document on the authenticated /api/v1
// group: a session or any valid API token may read it, anonymous callers may
// not. No scope is required — the document describes the API, not user data.
func RegisterOpenAPI(api fiber.Router, version string) {
	var (
		once sync.Once
		body []byte
		err  error
	)
	api.Get(openAPIRoute, func(c fiber.Ctx) error {
		once.Do(func() {
			doc := OpenAPIDocument()
			if version != "" {
				doc.Version = version
			}
			body, err = openapi.JSON(doc, Operations())
		})
		if err != nil {
			return httpapi.ErrInternal("build openapi document").WithCause(err)
		}
		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return c.Send(body)
	})
}

// --- Query parameter shapes. They exist only to document what the handlers
// read with c.Query, so keep them next to the operations that use them.

type pageQuery struct {
	Limit  int `query:"limit" description:"Page size; defaults to 50, capped at 200."`
	Offset int `query:"offset" description:"Number of items to skip."`
}

type taskListQuery struct {
	pageQuery
	Status   string `query:"status" enum:"open,completed,cancelled"`
	Priority string `query:"priority" enum:"high,medium,low,no-priority"`
	LabelID  int64  `query:"labelId"`
	Q        string `query:"q" description:"Substring of the title."`
}

type projectTaskListQuery struct {
	pageQuery
	Status   string `query:"status" enum:"open,completed,cancelled"`
	Priority string `query:"priority" enum:"high,medium,low,no-priority"`
	LabelID  int64  `query:"labelId"`
}

type projectListQuery struct {
	pageQuery
	ContextID int64  `query:"contextId"`
	Status    string `query:"status" enum:"open,completed,archived,cancelled"`
}

type statusPageQuery struct {
	pageQuery
	Status string `query:"status"`
}

type labelListQuery struct {
	pageQuery
	Q string `query:"q" description:"Substring of the label name."`
}

type viewFilterQuery struct {
	ContextID int64  `query:"contextId"`
	ProjectID int64  `query:"projectId"`
	LabelID   int64  `query:"labelId"`
	Priority  string `query:"priority" enum:"high,medium,low,no-priority"`
}

type pagedViewQuery struct {
	pageQuery
	viewFilterQuery
}

type completedViewQuery struct {
	Limit  int `query:"limit" description:"Page size; defaults to 50, capped at 1000."`
	Offset int `query:"offset"`
	Days   int `query:"days" description:"How many days back to look."`
	viewFilterQuery
}

type searchQuery struct {
	pageQuery
	Q    string `query:"q" required:"true"`
	Type string `query:"type" enum:"all,tasks,projects" default:"all"`
}

type taskGetQuery struct {
	Subtasks  bool `query:"subtasks" description:"Embed the subtask tree."`
	Relations bool `query:"relations" description:"Embed task relations."`
}

type syncChangesQuery struct {
	Since int64 `query:"since" description:"Cursor returned by the previous page."`
	Limit int64 `query:"limit"`
	Epoch int64 `query:"epoch" description:"Epoch the cursor belongs to; a mismatch asks for a fresh snapshot."`
}

type blockerCheckQuery struct {
	Candidates string `query:"candidates" required:"true" description:"Comma-separated task ids."`
}

type calendarEventsQuery struct {
	Start string `query:"start" required:"true" format:"date-time"`
	End   string `query:"end" required:"true" format:"date-time"`
}

type eventsStreamQuery struct {
	Ticket string `query:"ticket" required:"true" description:"Single-use ticket from POST /api/v1/events/ticket."`
}

type googleCallbackQuery struct {
	State string `query:"state"`
	Code  string `query:"code"`
	Error string `query:"error"`
}

type backupQuery struct {
	Settings bool `query:"settings" description:"Include user settings in the export."`
}

// userState is the free-form JSON object the clients keep per user.
type userState map[string]any

// backupFile is the restore payload: a backup exported by GET /api/v1/backup.
type backupFile map[string]any

type (
	pagedTasks     = dto.PagedResponse[dto.TaskDTO]
	pagedProjects  = dto.PagedResponse[dto.ProjectDTO]
	pagedContexts  = dto.PagedResponse[dto.ContextDTO]
	pagedLabels    = dto.PagedResponse[dto.LabelDTO]
	pagedSections  = dto.PagedResponse[dto.SectionDTO]
	pagedTemplates = dto.PagedResponse[dto.TaskTemplateDTO]
)

const (
	tagSystem    = "system"
	tagAuth      = "auth"
	tagAccount   = "account"
	tagTasks     = "tasks"
	tagViews     = "views"
	tagProjects  = "projects"
	tagContexts  = "contexts"
	tagLabels    = "labels"
	tagSections  = "sections"
	tagTemplates = "templates"
	tagInbox     = "inbox"
	tagTroiki    = "troiki"
	tagSettings  = "settings"
	tagCalendars = "calendars"
	tagSync      = "sync"
	tagEvents    = "events"
)

const (
	get   = http.MethodGet
	post  = http.MethodPost
	put   = http.MethodPut
	patch = http.MethodPatch
	del   = http.MethodDelete
)

func scopes(s ...string) []string { return s }

// syncScopes guards both sync endpoints: a replica needs to read everything.
var syncScopes = scopes(auth.ScopeTasksRead, auth.ScopeProjectsRead, auth.ScopeSectionsRead,
	auth.ScopeContextsRead, auth.ScopeLabelsRead, auth.ScopeTemplatesRead, auth.ScopeSettingsRead)

// Operations describes every route Mount registers. A test matches this table
// against the mounted routes, so a route cannot be added or removed without
// its description following.
func Operations() []openapi.Operation {
	const (
		api  = openapi.AuthAPI
		jwt  = openapi.AuthJWT
		none = openapi.AuthNone
	)
	tr, tw := auth.ScopeTasksRead, auth.ScopeTasksWrite
	pr, pw := auth.ScopeProjectsRead, auth.ScopeProjectsWrite
	cr, cw := auth.ScopeContextsRead, auth.ScopeContextsWrite
	lr, lw := auth.ScopeLabelsRead, auth.ScopeLabelsWrite
	sr, sw := auth.ScopeSectionsRead, auth.ScopeSectionsWrite
	mr, mw := auth.ScopeTemplatesRead, auth.ScopeTemplatesWrite
	setr, setw := auth.ScopeSettingsRead, auth.ScopeSettingsWrite

	ops := []openapi.Operation{
		// --- system
		{Method: get, Path: "/healthz", Tag: tagSystem, Auth: none, Summary: "Liveness probe", Response: httpapi.HealthResponse{}},
		{Method: get, Path: "/version", Tag: tagSystem, Auth: none, Summary: "Server version", Response: httpapi.VersionResponse{}},
		{Method: get, Path: "/api/config", Tag: tagSystem, Auth: none, Summary: "Public client bootstrap config", Response: httpapi.PublicConfigResponse{}},
		{Method: get, Path: OpenAPIPath, Tag: tagSystem, Auth: api, Summary: "This OpenAPI document", Response: map[string]any{}},
		{Method: get, Path: "/api/v1/config", Tag: tagSystem, Auth: api, Summary: "Everything the client shell needs at start",
			Description: "Answers 304 when If-None-Match carries the current ETag.",
			Scopes:      scopes(setr, tr, pr, lr, cr, auth.ScopeTroikiRead, mr), Response: configResp{}},

		// --- auth
		{Method: post, Path: "/auth/setup", Tag: tagAuth, Auth: none, Summary: "Create the single user (first run only)", Request: dto.LoginRequest{}, Response: dto.AuthResponse{}},
		{Method: post, Path: "/auth/login", Tag: tagAuth, Auth: none, Summary: "Sign in with username and password",
			Description: "Answers with an OTP challenge instead of tokens when TOTP is enabled.",
			Request:     dto.LoginRequest{}, Response: openapi.OneOf(dto.AuthResponse{}, dto.OTPChallengeResponse{})},
		{Method: post, Path: "/auth/login/otp", Tag: tagAuth, Auth: none, Summary: "Finish sign-in with a TOTP or recovery code", Request: dto.OTPLoginRequest{}, Response: dto.AuthResponse{}},
		{Method: post, Path: "/auth/passkey/login/begin", Tag: tagAuth, Auth: none, Summary: "Start a passkey sign-in ceremony", Request: dto.PasskeyLoginBeginRequest{}, Response: dto.PasskeyCeremonyResponse{}},
		{Method: post, Path: "/auth/passkey/login/finish", Tag: tagAuth, Auth: none, Summary: "Finish a passkey sign-in ceremony", Request: dto.PasskeyLoginFinishRequest{}, Response: dto.AuthResponse{}},
		{Method: post, Path: "/auth/refresh", Tag: tagAuth, Auth: none, Summary: "Rotate the refresh token",
			Description: "The web client sends the refresh token as an HttpOnly cookie; native clients send it in the body.",
			Request:     dto.RefreshRequest{}, Response: dto.RefreshResponse{}},
		{Method: post, Path: "/auth/logout", Tag: tagAuth, Auth: jwt, Summary: "End the current session"},
		{Method: post, Path: "/auth/logout-all", Tag: tagAuth, Auth: jwt, Summary: "End every session"},
		{Method: post, Path: "/auth/logout-others", Tag: tagAuth, Auth: jwt, Summary: "End every session but the current one"},
		{Method: get, Path: "/auth/me", Tag: tagAuth, Auth: jwt, Summary: "Current user", Response: dto.MeResponse{}},
		{Method: post, Path: "/auth/totp/setup", Tag: tagAuth, Auth: jwt, Summary: "Generate a TOTP secret", Response: dto.TOTPSetupResponse{}},
		{Method: post, Path: "/auth/totp/confirm", Tag: tagAuth, Auth: jwt, Summary: "Enable TOTP", Request: dto.TOTPCodeRequest{}, Response: dto.TOTPConfirmResponse{}},
		{Method: post, Path: "/auth/totp/disable", Tag: tagAuth, Auth: jwt, Summary: "Disable TOTP", Request: dto.TOTPCodeRequest{}},

		// --- account
		{Method: get, Path: "/api/v1/sessions", Tag: tagAccount, Auth: jwt, Summary: "List sessions", Response: []dto.SessionDTO{}},
		{Method: del, Path: "/api/v1/sessions/:id", Tag: tagAccount, Auth: jwt, Summary: "Revoke a session"},
		{Method: get, Path: "/api/v1/api-tokens", Tag: tagAccount, Auth: jwt, Summary: "List API tokens", Response: []apiTokenResp{}},
		{Method: post, Path: "/api/v1/api-tokens", Tag: tagAccount, Auth: jwt, Summary: "Issue an API token", Request: apiTokenCreateReq{}, Response: apiTokenCreateResp{}, Status: http.StatusCreated},
		{Method: del, Path: "/api/v1/api-tokens/:id", Tag: tagAccount, Auth: jwt, Summary: "Revoke an API token"},
		{Method: get, Path: "/api/v1/passkeys", Tag: tagAccount, Auth: jwt, Summary: "List passkeys", Response: []dto.PasskeyDTO{}},
		{Method: post, Path: "/api/v1/passkeys/register/begin", Tag: tagAccount, Auth: jwt, Summary: "Start a passkey registration ceremony", Response: dto.PasskeyCeremonyResponse{}},
		{Method: post, Path: "/api/v1/passkeys/register/finish", Tag: tagAccount, Auth: jwt, Summary: "Finish a passkey registration ceremony", Request: dto.PasskeyRegisterFinishRequest{}, Response: dto.PasskeyDTO{}, Status: http.StatusCreated},
		{Method: patch, Path: "/api/v1/passkeys/:id", Tag: tagAccount, Auth: jwt, Summary: "Rename a passkey", Request: dto.PasskeyRenameRequest{}, Response: dto.PasskeyDTO{}},
		{Method: del, Path: "/api/v1/passkeys/:id", Tag: tagAccount, Auth: jwt, Summary: "Remove a passkey"},
		{Method: get, Path: "/api/v1/backup", Tag: tagAccount, Auth: jwt, Summary: "Export a backup",
			Description: "A gzip-encoded JSON file (Content-Encoding: gzip).",
			Query:       backupQuery{}, Response: backupFile{}},
		{Method: post, Path: "/api/v1/restore", Tag: tagAccount, Auth: jwt, Summary: "Restore from a backup, replacing all data",
			Description: "Accepts the file GET /api/v1/backup produced, plain or gzip-compressed.",
			Request:     backupFile{}},

		// --- events
		{Method: post, Path: "/api/v1/events/ticket", Tag: tagEvents, Auth: jwt, Summary: "Issue a single-use ticket for the event stream", Request: ticketRequest{}, Response: ticketResponse{}},
		{Method: get, Path: "/api/v1/events", Tag: tagEvents, Auth: none, Summary: "Server-sent refetch hints",
			Description: "A text/event-stream; authenticated by the ticket query parameter rather than a header.",
			Query:       eventsStreamQuery{}, Response: "", ResponseContentType: "text/event-stream"},

		// --- tasks
		{Method: get, Path: "/api/v1/tasks/:id", Tag: tagTasks, Auth: api, Scopes: scopes(tr), Summary: "Get a task", Query: taskGetQuery{}, Response: dto.TaskDTO{}},
		{Method: patch, Path: "/api/v1/tasks/:id", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Update a task", Request: dto.PatchTaskRequest{}, Response: dto.TaskDTO{}},
		{Method: del, Path: "/api/v1/tasks/:id", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Delete a task and its subtasks"},
		{Method: get, Path: "/api/v1/tasks/:id/subtasks", Tag: tagTasks, Auth: api, Scopes: scopes(tr), Summary: "List direct subtasks", Response: pagedTasks{}},
		{Method: post, Path: "/api/v1/tasks/:id/subtasks", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Create a subtask", Request: dto.CreateTaskRequest{}, Response: dto.TaskDTO{}, Status: http.StatusCreated},
		{Method: get, Path: "/api/v1/tasks/:id/template-draft", Tag: tagTasks, Auth: api, Scopes: scopes(tr), Summary: "Draft a template from a task subtree", Response: dto.TaskTemplateDTO{}},
		{Method: post, Path: "/api/v1/tasks/:id/duplicate", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Duplicate a task", Response: dto.TaskDTO{}, Status: http.StatusCreated},
		{Method: post, Path: "/api/v1/tasks/:id/decompose", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Split a task into several", Request: dto.DecomposeTaskRequest{}, Response: dto.DecomposeTaskResponse{}, Status: http.StatusCreated},
		{Method: post, Path: "/api/v1/tasks/:id/complete", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Complete a task",
			Description: "A recurring task advances to its next occurrence instead.", Request: CompleteRequest{}, Response: dto.TaskDTO{}},
		{Method: post, Path: "/api/v1/tasks/:id/uncomplete", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Reopen a task", Response: dto.TaskDTO{}},
		{Method: post, Path: "/api/v1/tasks/:id/cancel", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Cancel a task", Response: dto.TaskDTO{}},
		{Method: post, Path: "/api/v1/tasks/:id/pin", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Pin a task", Response: dto.TaskDTO{}},
		{Method: post, Path: "/api/v1/tasks/:id/unpin", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Unpin a task", Response: dto.TaskDTO{}},
		{Method: post, Path: "/api/v1/tasks/:id/move", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Move a task", Request: MoveRequest{}, Response: dto.TaskDTO{}},
		{Method: post, Path: "/api/v1/tasks/:id/plan", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Plan a task for a day, the week or the backlog", Request: PlanRequest{}, Response: dto.TaskDTO{}},
		{Method: post, Path: "/api/v1/tasks/:id/relations", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Relate two tasks", Request: dto.CreateTaskRelationRequest{}, Response: dto.TaskDTO{}},
		{Method: get, Path: "/api/v1/tasks/:id/relations/blocker-check", Tag: tagTasks, Auth: api, Scopes: scopes(tr), Summary: "Check which candidates cannot block a task", Query: blockerCheckQuery{}, Response: dto.BlockerCheckResponse{}},
		{Method: del, Path: "/api/v1/tasks/:id/relations/:relationId", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Remove a relation", Response: dto.TaskDTO{}},
		{Method: post, Path: "/api/v1/tasks/bulk/complete", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Complete several tasks", Request: BulkIDsRequest{}, Response: bulkResponse{}},
		{Method: post, Path: "/api/v1/tasks/bulk/move", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Move several tasks", Request: BulkMoveRequest{}, Response: bulkResponse{}},
		{Method: post, Path: "/api/v1/tasks/bulk/priority", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Set the priority of several tasks", Request: BulkPriorityRequest{}, Response: bulkResponse{}},
		{Method: post, Path: "/api/v1/tasks/group", Tag: tagTasks, Auth: api, Scopes: scopes(tw), Summary: "Group tasks under a new parent", Request: dto.GroupTasksRequest{}, Response: GroupTasksResponse{}, Status: http.StatusCreated},
		{Method: get, Path: "/api/v1/search", Tag: tagTasks, Auth: api, Scopes: scopes(auth.ScopeSearchRead), Summary: "Search tasks and projects", Query: searchQuery{}, Response: searchResponse{},
			Description: "Case-insensitive substring match (every script, not just ASCII) on the title, a label name " +
				"or the description. Results are ranked title hits first, then label hits, then description hits; " +
				"open before finished within a rank."},

		// --- views
		{Method: get, Path: "/api/v1/tasks/today", Tag: tagViews, Auth: api, Scopes: scopes(tr), Summary: "Today", Query: pagedViewQuery{}, Response: todayBundleResponse{}},
		{Method: get, Path: "/api/v1/tasks/tomorrow", Tag: tagViews, Auth: api, Scopes: scopes(tr), Summary: "Tomorrow", Query: pagedViewQuery{}, Response: pagedTasks{}},
		{Method: get, Path: "/api/v1/tasks/overdue", Tag: tagViews, Auth: api, Scopes: scopes(tr), Summary: "Overdue", Query: pagedViewQuery{}, Response: pagedTasks{}},
		{Method: get, Path: "/api/v1/tasks/week", Tag: tagViews, Auth: api, Scopes: scopes(tr), Summary: "This week", Query: viewFilterQuery{}, Response: viewResponse{}},
		{Method: get, Path: "/api/v1/tasks/backlog", Tag: tagViews, Auth: api, Scopes: scopes(tr), Summary: "Backlog", Query: viewFilterQuery{}, Response: viewResponse{}},
		{Method: get, Path: "/api/v1/tasks/pinned", Tag: tagViews, Auth: api, Scopes: scopes(tr), Summary: "Pinned", Query: viewFilterQuery{}, Response: viewResponse{}},
		{Method: get, Path: "/api/v1/tasks/completed", Tag: tagViews, Auth: api, Scopes: scopes(tr), Summary: "Completion history", Query: completedViewQuery{}, Response: pagedTasks{}},
		{Method: get, Path: "/api/v1/stats/sidebar", Tag: tagViews, Auth: api, Scopes: scopes(tr), Summary: "Sidebar counters", Response: sidebarStatsResponse{}},
		{Method: get, Path: "/api/v1/stats/week-summary", Tag: tagViews, Auth: api, Scopes: scopes(tr), Summary: "Week summary", Response: weekSummaryResponse{}},

		// --- inbox
		{Method: get, Path: "/api/v1/inbox", Tag: tagInbox, Auth: api, Scopes: scopes(tr), Summary: "Inbox", Response: inboxResponse{}},
		{Method: post, Path: "/api/v1/inbox/tasks", Tag: tagInbox, Auth: api, Scopes: scopes(tw), Summary: "Capture a task into the inbox", Request: dto.CreateTaskRequest{}, Response: dto.TaskDTO{}, Status: http.StatusCreated},
		{Method: get, Path: "/api/v1/inbox/processing", Tag: tagInbox, Auth: api, Scopes: scopes(setr), Summary: "Automatic inbox processing status", Response: inboxProcessingStatusDTO{}},
		{Method: post, Path: "/api/v1/inbox/processing/run", Tag: tagInbox, Auth: api, Scopes: scopes(tw), Summary: "Process the inbox now", Response: inboxRunResp{}, Status: http.StatusAccepted},
		{Method: post, Path: "/api/v1/inbox/processing/preview", Tag: tagInbox, Auth: api, Scopes: scopes(setr), Summary: "Render the processing prompt", Request: inboxPromptReq{}, Response: inboxPreviewResp{}},
		{Method: get, Path: "/api/v1/inbox/processing/log", Tag: tagInbox, Auth: api, Scopes: scopes(tr), Summary: "Processing log", Query: pageQuery{}, Response: dto.PagedResponse[inboxProcessingLogDTO]{}},
		{Method: post, Path: "/api/v1/inbox/processing/log/:id/revert", Tag: tagInbox, Auth: api, Scopes: scopes(tw), Summary: "Undo one processing decision", Response: dto.TaskDTO{}},

		// --- projects
		{Method: get, Path: "/api/v1/projects", Tag: tagProjects, Auth: api, Scopes: scopes(pr), Summary: "List projects", Query: projectListQuery{}, Response: pagedProjects{}},
		{Method: get, Path: "/api/v1/projects/stats", Tag: tagProjects, Auth: api, Scopes: scopes(pr), Summary: "Project activity statistics", Response: projectStatsResponse{}},
		{Method: get, Path: "/api/v1/projects/:id", Tag: tagProjects, Auth: api, Scopes: scopes(pr), Summary: "Get a project", Response: dto.ProjectDTO{}},
		{Method: patch, Path: "/api/v1/projects/:id", Tag: tagProjects, Auth: api, Scopes: scopes(pw), Summary: "Update a project", Request: dto.PatchProjectRequest{}, Response: dto.ProjectDTO{}},
		{Method: del, Path: "/api/v1/projects/:id", Tag: tagProjects, Auth: api, Scopes: scopes(pw), Summary: "Delete a project"},
		{Method: get, Path: "/api/v1/projects/:id/bundle", Tag: tagProjects, Auth: api, Scopes: scopes(pr, sr, tr), Summary: "A project with its sections and tasks", Response: projectBundleResponse{}},
		{Method: get, Path: "/api/v1/projects/:id/sections", Tag: tagProjects, Auth: api, Scopes: scopes(sr), Summary: "List sections of a project", Query: pageQuery{}, Response: pagedSections{}},
		{Method: post, Path: "/api/v1/projects/:id/sections", Tag: tagProjects, Auth: api, Scopes: scopes(sw), Summary: "Create a section", Request: dto.CreateSectionRequest{}, Response: dto.SectionDTO{}, Status: http.StatusCreated},
		{Method: get, Path: "/api/v1/projects/:id/tasks", Tag: tagProjects, Auth: api, Scopes: scopes(tr), Summary: "List tasks of a project", Query: projectTaskListQuery{}, Response: pagedTasks{}},
		{Method: post, Path: "/api/v1/projects/:id/tasks", Tag: tagProjects, Auth: api, Scopes: scopes(tw), Summary: "Create a task in a project", Request: dto.CreateTaskRequest{}, Response: dto.TaskDTO{}, Status: http.StatusCreated},
		{Method: post, Path: "/api/v1/projects/:id/complete", Tag: tagProjects, Auth: api, Scopes: scopes(pw), Summary: "Complete a project", Response: dto.ProjectDTO{}},
		{Method: post, Path: "/api/v1/projects/:id/uncomplete", Tag: tagProjects, Auth: api, Scopes: scopes(pw), Summary: "Reopen a project", Response: dto.ProjectDTO{}},
		{Method: post, Path: "/api/v1/projects/:id/cancel", Tag: tagProjects, Auth: api, Scopes: scopes(pw), Summary: "Cancel a project", Response: dto.ProjectDTO{}},
		{Method: post, Path: "/api/v1/projects/:id/archive", Tag: tagProjects, Auth: api, Scopes: scopes(pw), Summary: "Archive a project", Response: dto.ProjectDTO{}},
		{Method: post, Path: "/api/v1/projects/:id/unarchive", Tag: tagProjects, Auth: api, Scopes: scopes(pw), Summary: "Unarchive a project", Response: dto.ProjectDTO{}},
		{Method: post, Path: "/api/v1/projects/:id/pin", Tag: tagProjects, Auth: api, Scopes: scopes(pw), Summary: "Pin a project", Response: dto.ProjectDTO{}},
		{Method: post, Path: "/api/v1/projects/:id/unpin", Tag: tagProjects, Auth: api, Scopes: scopes(pw), Summary: "Unpin a project", Response: dto.ProjectDTO{}},
		{Method: post, Path: "/api/v1/projects/:id/troiki", Tag: tagTroiki, Auth: api, Scopes: scopes(pw), Summary: "Put a project into a troiki bucket", Request: SetTroikiCategoryRequest{}, Response: dto.ProjectDTO{}},

		// --- contexts
		{Method: get, Path: "/api/v1/contexts", Tag: tagContexts, Auth: api, Scopes: scopes(cr), Summary: "List contexts", Query: pageQuery{}, Response: pagedContexts{}},
		{Method: post, Path: "/api/v1/contexts", Tag: tagContexts, Auth: api, Scopes: scopes(cw), Summary: "Create a context", Request: dto.CreateContextRequest{}, Response: dto.ContextDTO{}, Status: http.StatusCreated},
		{Method: get, Path: "/api/v1/contexts/:id", Tag: tagContexts, Auth: api, Scopes: scopes(cr), Summary: "Get a context", Response: dto.ContextDTO{}},
		{Method: patch, Path: "/api/v1/contexts/:id", Tag: tagContexts, Auth: api, Scopes: scopes(cw), Summary: "Update a context", Request: dto.PatchContextRequest{}, Response: dto.ContextDTO{}},
		{Method: del, Path: "/api/v1/contexts/:id", Tag: tagContexts, Auth: api, Scopes: scopes(cw), Summary: "Delete a context"},
		{Method: get, Path: "/api/v1/contexts/:id/projects", Tag: tagContexts, Auth: api, Scopes: scopes(pr), Summary: "List projects of a context", Query: statusPageQuery{}, Response: pagedProjects{}},
		{Method: post, Path: "/api/v1/contexts/:id/projects", Tag: tagContexts, Auth: api, Scopes: scopes(pw), Summary: "Create a project in a context", Request: dto.CreateProjectRequest{}, Response: dto.ProjectDTO{}, Status: http.StatusCreated},
		{Method: get, Path: "/api/v1/contexts/:id/tasks", Tag: tagContexts, Auth: api, Scopes: scopes(tr), Summary: "List tasks of a context", Query: taskListQuery{}, Response: pagedTasks{}},
		{Method: post, Path: "/api/v1/contexts/:id/tasks", Tag: tagContexts, Auth: api, Scopes: scopes(tw), Summary: "Create a task in a context", Request: dto.CreateTaskRequest{}, Response: dto.TaskDTO{}, Status: http.StatusCreated},

		// --- labels
		{Method: get, Path: "/api/v1/labels", Tag: tagLabels, Auth: api, Scopes: scopes(lr), Summary: "List labels", Query: labelListQuery{}, Response: pagedLabels{}},
		{Method: post, Path: "/api/v1/labels", Tag: tagLabels, Auth: api, Scopes: scopes(lw), Summary: "Create a label", Request: dto.CreateLabelRequest{}, Response: dto.LabelDTO{}, Status: http.StatusCreated},
		{Method: get, Path: "/api/v1/labels/stats", Tag: tagLabels, Auth: api, Scopes: scopes(lr), Summary: "Label usage statistics", Response: labelStatsResponse{}},
		{Method: get, Path: "/api/v1/labels/:id", Tag: tagLabels, Auth: api, Scopes: scopes(lr), Summary: "Get a label", Response: dto.LabelDTO{}},
		{Method: patch, Path: "/api/v1/labels/:id", Tag: tagLabels, Auth: api, Scopes: scopes(lw), Summary: "Update a label", Request: dto.PatchLabelRequest{}, Response: dto.LabelDTO{}},
		{Method: del, Path: "/api/v1/labels/:id", Tag: tagLabels, Auth: api, Scopes: scopes(lw), Summary: "Delete a label"},
		{Method: get, Path: "/api/v1/labels/:id/tasks", Tag: tagLabels, Auth: api, Scopes: scopes(tr), Summary: "List tasks with a label", Query: pageQuery{}, Response: pagedTasks{}},
		{Method: get, Path: "/api/v1/labels/:id/projects", Tag: tagLabels, Auth: api, Scopes: scopes(pr), Summary: "List projects with a label", Query: pageQuery{}, Response: pagedProjects{}},

		// --- sections
		{Method: get, Path: "/api/v1/sections/:id", Tag: tagSections, Auth: api, Scopes: scopes(sr), Summary: "Get a section", Response: dto.SectionDTO{}},
		{Method: patch, Path: "/api/v1/sections/:id", Tag: tagSections, Auth: api, Scopes: scopes(sw), Summary: "Update a section", Request: dto.PatchSectionRequest{}, Response: dto.SectionDTO{}},
		{Method: del, Path: "/api/v1/sections/:id", Tag: tagSections, Auth: api, Scopes: scopes(sw), Summary: "Delete a section"},
		{Method: get, Path: "/api/v1/sections/:id/tasks", Tag: tagSections, Auth: api, Scopes: scopes(tr), Summary: "List tasks of a section", Query: pageQuery{}, Response: pagedTasks{}},
		{Method: post, Path: "/api/v1/sections/:id/tasks", Tag: tagSections, Auth: api, Scopes: scopes(tw), Summary: "Create a task in a section", Request: dto.CreateTaskRequest{}, Response: dto.TaskDTO{}, Status: http.StatusCreated},
		{Method: post, Path: "/api/v1/sections/:id/reorder", Tag: tagSections, Auth: api, Scopes: scopes(sw), Summary: "Move a section within its project", Request: dto.ReorderSectionRequest{}, Response: dto.SectionDTO{}},

		// --- templates
		{Method: get, Path: "/api/v1/task-templates", Tag: tagTemplates, Auth: api, Scopes: scopes(mr), Summary: "List task templates", Response: pagedTemplates{}},
		{Method: post, Path: "/api/v1/task-templates", Tag: tagTemplates, Auth: api, Scopes: scopes(mw), Summary: "Create a task template", Request: dto.TaskTemplateRequest{}, Response: dto.TaskTemplateDTO{}, Status: http.StatusCreated},
		{Method: get, Path: "/api/v1/task-templates/:id", Tag: tagTemplates, Auth: api, Scopes: scopes(mr), Summary: "Get a task template", Response: dto.TaskTemplateDTO{}},
		{Method: patch, Path: "/api/v1/task-templates/:id", Tag: tagTemplates, Auth: api, Scopes: scopes(mw), Summary: "Update a task template", Request: dto.TaskTemplateRequest{}, Response: dto.TaskTemplateDTO{}},
		{Method: del, Path: "/api/v1/task-templates/:id", Tag: tagTemplates, Auth: api, Scopes: scopes(mw), Summary: "Delete a task template"},
		{Method: post, Path: "/api/v1/task-templates/:id/instantiate", Tag: tagTemplates, Auth: api, Scopes: scopes(tw), Summary: "Create tasks from a template", Request: dto.InstantiateTemplateRequest{}, Response: dto.InstantiateTemplateResponse{}, Status: http.StatusCreated},

		// --- troiki
		{Method: get, Path: "/api/v1/troiki", Tag: tagTroiki, Auth: api, Scopes: scopes(auth.ScopeTroikiRead), Summary: "Daily plan by troiki buckets", Response: troikiViewDTO{}},
		{Method: post, Path: "/api/v1/troiki/start", Tag: tagTroiki, Auth: api, Scopes: scopes(auth.ScopeTroikiWrite), Summary: "Start a troiki cycle", Response: troikiViewDTO{}},
		{Method: post, Path: "/api/v1/troiki/reset", Tag: tagTroiki, Auth: api, Scopes: scopes(auth.ScopeTroikiWrite), Summary: "Reset the troiki cycle", Response: troikiViewDTO{}},

		// --- settings
		{Method: get, Path: "/api/v1/settings", Tag: tagSettings, Auth: api, Scopes: scopes(setr), Summary: "User settings", Response: settingsResp{}},
		{Method: patch, Path: "/api/v1/settings", Tag: tagSettings, Auth: api, Scopes: scopes(setw), Summary: "Update user settings", Request: settingsPatchReq{}, Response: settingsResp{}},
		{Method: get, Path: "/api/v1/state", Tag: tagSettings, Auth: api, Scopes: scopes(setr), Summary: "Client UI state", Response: userState{}},
		{Method: patch, Path: "/api/v1/state", Tag: tagSettings, Auth: api, Scopes: scopes(setw), Summary: "Merge keys into the client UI state", Request: userState{}, Response: userState{}},
		{Method: get, Path: "/api/v1/harpoon", Tag: tagSettings, Auth: api, Scopes: scopes(setr), Summary: "Jump pair", Response: harpoonResp{}},
		{Method: post, Path: "/api/v1/harpoon/attach", Tag: tagSettings, Auth: api, Scopes: scopes(setw), Summary: "Put an item into the jump pair", Request: harpoonRefReq{}, Response: harpoonResp{}},
		{Method: post, Path: "/api/v1/harpoon/detach", Tag: tagSettings, Auth: api, Scopes: scopes(setw), Summary: "Remove an item from the jump pair", Request: harpoonRefReq{}, Response: harpoonResp{}},
		{Method: get, Path: "/api/v1/app-settings", Tag: tagSettings, Auth: api, Scopes: scopes(setr), Summary: "Installation settings", Response: appSettingsResp{}},
		{Method: put, Path: "/api/v1/app-settings/auto-labels", Tag: tagSettings, Auth: api, Scopes: scopes(setw), Summary: "Replace auto-label rules", Request: autoLabelsPutReq{}, Response: appSettingsResp{}},
		{Method: put, Path: "/api/v1/app-settings/project-suggestions", Tag: tagSettings, Auth: api, Scopes: scopes(setw), Summary: "Replace project suggestion rules", Request: projectSuggestionsPutReq{}, Response: appSettingsResp{}},
		{Method: put, Path: "/api/v1/app-settings/inbox-processing", Tag: tagSettings, Auth: api, Scopes: scopes(setw), Summary: "Update inbox processing settings", Request: inboxSettingsPutReq{}, Response: appSettingsResp{}},

		// --- calendars
		{Method: get, Path: "/api/v1/calendars", Tag: tagCalendars, Auth: api, Scopes: scopes(auth.ScopeCalendarsRead), Summary: "Connected calendars", Response: calendarListResp{}},
		{Method: patch, Path: "/api/v1/calendars/settings", Tag: tagCalendars, Auth: jwt, Summary: "Update calendar settings", Request: calendarSettingsPatchReq{}, Response: calendarListResp{}},
		{Method: get, Path: "/api/v1/calendars/events", Tag: tagCalendars, Auth: api, Scopes: scopes(auth.ScopeCalendarsRead), Summary: "Calendar events in a range", Query: calendarEventsQuery{}, Response: calendarEventsResp{}},
		{Method: patch, Path: "/api/v1/calendars/google/config", Tag: tagCalendars, Auth: jwt, Summary: "Set Google OAuth credentials", Request: googleCalendarConfigPatchReq{}, Response: calendarListResp{}},
		{Method: del, Path: "/api/v1/calendars/google/config", Tag: tagCalendars, Auth: jwt, Summary: "Remove Google OAuth credentials and disconnect", Response: calendarListResp{}},
		{Method: get, Path: "/api/v1/calendars/google/start", Tag: tagCalendars, Auth: api, Scopes: scopes(auth.ScopeCalendarsRead), Summary: "Start connecting Google Calendar", Response: googleStartResp{}},
		{Method: get, Path: "/api/v1/calendars/google/callback", Tag: tagCalendars, Auth: none, Summary: "Google OAuth redirect target",
			Description: "Redirects the browser back to the settings page.", Query: googleCallbackQuery{}, Status: http.StatusFound},
		{Method: post, Path: "/api/v1/calendars/google/sync", Tag: tagCalendars, Auth: jwt, Summary: "Refresh the list of Google calendars", Response: calendarListResp{}},
		{Method: patch, Path: "/api/v1/calendars/sources/:id", Tag: tagCalendars, Auth: jwt, Summary: "Select or deselect a calendar", Request: calendarSourcePatchReq{}, Response: calendarSourceResp{}},
		{Method: del, Path: "/api/v1/calendars/accounts/:id", Tag: tagCalendars, Auth: jwt, Summary: "Disconnect a calendar account"},

		// --- sync
		{Method: get, Path: "/api/v1/sync/changes", Tag: tagSync, Auth: api, Scopes: syncScopes, Summary: "Page of the change log after a cursor", Query: syncChangesQuery{}, Response: syncChangesResp{}},
		{Method: get, Path: "/api/v1/sync/snapshot", Tag: tagSync, Auth: api, Scopes: syncScopes, Summary: "Full snapshot to seed a replica", Response: syncSnapshotResp{}},
	}
	return ops
}
