package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
)

// Handlers is every handler the server mounts. TOTP and Passkeys are optional:
// a nil one leaves its routes unmounted, as when the feature is not configured.
type Handlers struct {
	Auth            *AuthHandler
	TOTP            *TOTPHandler
	Events          *EventsHandler
	Calendar        *CalendarHandler
	Contexts        *ContextHandler
	Labels          *LabelHandler
	Templates       *TemplateHandler
	Sections        *SectionHandler
	Projects        *ProjectHandler
	Inbox           *InboxHandler
	TaskBulk        *TaskBulkHandler
	TaskViews       *TaskViewHandler
	TaskActions     *TaskActionHandler
	Troiki          *TroikiHandler
	TaskRelations   *TaskRelationHandler
	Tasks           *TaskHandler
	Search          *SearchHandler
	Meta            *MetaHandler
	State           *StateHandler
	Settings        *SettingsHandler
	Harpoon         *HarpoonHandler
	TodayLock       *TodayLockHandler
	AppSettings     *AppSettingsHandler
	InboxProcessing *InboxProcessingHandler
	Sync            *SyncHandler
	APITokens       *APITokensHandler
	Sessions        *SessionsHandler
	Passkeys        *PasskeysHandler
	Backup          *BackupHandler
}

// Mount wires every API route onto app: the public endpoints, /auth and the
// /api/v1 group, which also carries the OpenAPI document. Registration order is
// load-bearing — static paths go before parameterised ones, and the public
// /api/v1 routes go before the group so its middleware never sees them. The SPA
// catch-all is the caller's to add afterwards.
func Mount(app *fiber.App, deps httpapi.Deps, h Handlers) fiber.Router {
	h.Calendar.RegisterPublic(app)
	h.Events.RegisterPublic(app)

	api := httpapi.RegisterRoutes(app, deps)
	RegisterOpenAPI(api, deps.Version)
	h.Events.Register(api)

	authGroup := app.Group("/auth")
	h.Auth.RegisterAuth(authGroup, deps.JWTIssuer)
	if h.TOTP != nil {
		h.TOTP.RegisterTOTP(authGroup.Group("", httpapi.AuthMiddleware(deps.JWTIssuer)))
	}

	h.Contexts.Register(api.Group("/contexts"))
	h.Labels.Register(api.Group("/labels"))
	h.Templates.Register(api.Group("/task-templates"))
	h.Sections.Register(api.Group("/sections"))
	h.Projects.Register(api)
	h.Inbox.Register(api.Group("/inbox"))
	h.TaskBulk.Register(api)
	h.TaskViews.Register(api)
	h.TaskActions.Register(api)
	h.Troiki.Register(api)
	h.TaskRelations.Register(api)
	h.Tasks.Register(api)
	h.Search.Register(api)
	h.Meta.Register(api)
	h.State.Register(api)
	h.Settings.Register(api)
	h.Harpoon.Register(api)
	h.TodayLock.Register(api)
	h.AppSettings.Register(api)
	h.InboxProcessing.Register(api)
	h.Sync.Register(api.Group("/sync"))
	h.APITokens.Register(api.Group("/api-tokens", httpapi.RequireJWTAuth()))
	h.Sessions.Register(api.Group("/sessions", httpapi.RequireJWTAuth()))
	if h.Passkeys != nil {
		// Registering a passkey mints a password-equivalent credential, so the
		// group is JWT-only: an API token must never be able to add one.
		h.Passkeys.Register(api.Group("/passkeys", httpapi.RequireJWTAuth()))
	}
	h.Backup.Register(api.Group("", httpapi.RequireJWTAuth()))
	h.Calendar.Register(api.Group("/calendars"))
	return api
}
