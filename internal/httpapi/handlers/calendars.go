package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	calendar "github.com/lebe-dev/turboist/internal/service/calendar"
	"golang.org/x/oauth2"
)

const (
	msgLoadSettings             = "load settings"
	msgLoadGoogleCalendarConfig = "load google calendar config"
	msgInvalidJSON              = "invalid JSON"
	msgGCalOAuthCallbackError   = "google calendar oauth callback error"
	redirectCalendarError       = "tab=calendars&calendar=error"
)

// CalendarHandler is a thin Fiber adapter over calendar.Service.
type CalendarHandler struct {
	svc       *calendar.Service
	calendars *repo.CalendarRepo
	users     *repo.UserRepo
	baseURL   string
	log       *slog.Logger
}

// NewCalendarHandler constructs a CalendarHandler.
func NewCalendarHandler(
	svc *calendar.Service,
	calendars *repo.CalendarRepo,
	users *repo.UserRepo,
	baseURL string,
	log *slog.Logger,
) *CalendarHandler {
	return &CalendarHandler{
		svc:       svc,
		calendars: calendars,
		users:     users,
		baseURL:   strings.TrimRight(baseURL, "/"),
		log:       log,
	}
}

// RegisterPublic registers routes that do not require authentication.
func (h *CalendarHandler) RegisterPublic(app fiber.Router) {
	app.Get("/api/v1/calendars/google/callback", h.googleCallback)
}

// Register registers authenticated calendar routes under the given router.
// Mutation endpoints (Patch/Post/Delete) cover account/source/OAuth config that
// has no `calendars:write` scope by design — they are admin operations, so we
// gate them with RequireJWTAuth instead of a scope check.
func (h *CalendarHandler) Register(r fiber.Router) {
	r.Get("/", httpapi.RequireScope(auth.ScopeCalendarsRead), h.list)
	r.Patch("/settings", httpapi.RequireJWTAuth(), h.patchSettings)
	r.Get("/events", httpapi.RequireScope(auth.ScopeCalendarsRead), h.events)
	r.Patch("/google/config", httpapi.RequireJWTAuth(), h.patchGoogleConfig)
	r.Delete("/google/config", httpapi.RequireJWTAuth(), h.deleteGoogleConfig)
	r.Get("/google/start", httpapi.RequireScope(auth.ScopeCalendarsRead), h.googleStart)
	r.Post("/google/sync", httpapi.RequireJWTAuth(), h.googleSync)
	r.Patch("/sources/:id", httpapi.RequireJWTAuth(), h.patchSource)
	r.Delete("/accounts/:id", httpapi.RequireJWTAuth(), h.deleteAccount)
}

// --- HTTP response types (stay in the handler layer) ---

type calendarAccountResp struct {
	ID          int64  `json:"id"`
	Provider    string `json:"provider"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type calendarSourceResp struct {
	ID         int64  `json:"id"`
	AccountID  int64  `json:"accountId"`
	Provider   string `json:"provider"`
	ExternalID string `json:"externalId"`
	Summary    string `json:"summary"`
	Color      string `json:"color"`
	Selected   bool   `json:"selected"`
	IsPrimary  bool   `json:"isPrimary"`
}

type calendarListResp struct {
	Enabled                      bool                  `json:"enabled"`
	GoogleConfigured             bool                  `json:"googleConfigured"`
	GoogleClientIDConfigured     bool                  `json:"googleClientIdConfigured"`
	GoogleClientSecretConfigured bool                  `json:"googleClientSecretConfigured"`
	Accounts                     []calendarAccountResp `json:"accounts"`
	Sources                      []calendarSourceResp  `json:"sources"`
}

type calendarEventResp struct {
	ID          string `json:"id"`
	SourceID    int64  `json:"sourceId"`
	SourceName  string `json:"sourceName"`
	SourceColor string `json:"sourceColor"`
	Provider    string `json:"provider"`
	ExternalID  string `json:"externalId"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location"`
	Start       string `json:"start"`
	End         string `json:"end"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
	AllDay      bool   `json:"allDay"`
	HTMLLink    string `json:"htmlLink"`
}

func calendarAccountToResp(a model.CalendarAccount) calendarAccountResp {
	return calendarAccountResp{
		ID:          a.ID,
		Provider:    string(a.Provider),
		Email:       a.Email,
		DisplayName: a.DisplayName,
		CreatedAt:   model.FormatUTC(a.CreatedAt),
		UpdatedAt:   model.FormatUTC(a.UpdatedAt),
	}
}

func calendarSourceToResp(s model.CalendarSource) calendarSourceResp {
	return calendarSourceResp{
		ID:         s.ID,
		AccountID:  s.AccountID,
		Provider:   string(s.Provider),
		ExternalID: s.ExternalID,
		Summary:    s.Summary,
		Color:      s.Color,
		Selected:   s.Selected,
		IsPrimary:  s.IsPrimary,
	}
}

func calendarEventToResp(e calendar.CalendarEvent) calendarEventResp {
	return calendarEventResp{
		ID:          e.ID,
		SourceID:    e.SourceID,
		SourceName:  e.SourceName,
		SourceColor: e.SourceColor,
		Provider:    e.Provider,
		ExternalID:  e.ExternalID,
		Title:       e.Title,
		Description: e.Description,
		Location:    e.Location,
		Start:       model.FormatUTC(e.Start),
		End:         model.FormatUTC(e.End),
		StartDate:   e.StartDate,
		EndDate:     e.EndDate,
		AllDay:      e.AllDay,
		HTMLLink:    e.HTMLLink,
	}
}

// --- auth helpers ---

func (h *CalendarHandler) claimsUserID(c fiber.Ctx) (int64, *httpapi.AppError) {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return 0, httpapi.ErrAuthInvalid("missing auth claims")
	}
	return claims.UserID, nil
}

func (h *CalendarHandler) claims(c fiber.Ctx) (*auth.Claims, *httpapi.AppError) {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return nil, httpapi.ErrAuthInvalid("missing auth claims")
	}
	return claims, nil
}

// --- handlers ---

func (h *CalendarHandler) list(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	settings, err := h.users.GetSettings(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal(msgLoadSettings).WithCause(err)
	}
	accounts, err := h.calendars.ListAccounts(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("list calendar accounts").WithCause(err)
	}
	sources, err := h.calendars.ListSources(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("list calendar sources").WithCause(err)
	}
	var googleClientIDConfigured, googleClientSecretConfigured bool
	dbCfg, err := h.calendars.GetOAuthConfig(c.Context(), userID, model.CalendarProviderGoogle)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		return httpapi.ErrInternal(msgLoadGoogleCalendarConfig).WithCause(err)
	}
	if dbCfg != nil {
		googleClientIDConfigured = dbCfg.ClientID != ""
		googleClientSecretConfigured = dbCfg.ClientSecret != ""
	}
	out := calendarListResp{
		Enabled:                      settings.CalendarEnabled,
		GoogleConfigured:             googleClientIDConfigured && googleClientSecretConfigured,
		GoogleClientIDConfigured:     googleClientIDConfigured,
		GoogleClientSecretConfigured: googleClientSecretConfigured,
		Accounts:                     make([]calendarAccountResp, len(accounts)),
		Sources:                      make([]calendarSourceResp, len(sources)),
	}
	for i, a := range accounts {
		out.Accounts[i] = calendarAccountToResp(a)
	}
	for i, s := range sources {
		out.Sources[i] = calendarSourceToResp(s)
	}
	return c.JSON(out)
}

type calendarSettingsPatchReq struct {
	Enabled *bool `json:"enabled"`
}

func (h *CalendarHandler) patchSettings(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	var req calendarSettingsPatchReq
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation(msgInvalidJSON)
	}
	if req.Enabled == nil {
		return httpapi.ErrValidation("enabled is required")
	}
	settings, err := h.users.GetSettings(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal(msgLoadSettings).WithCause(err)
	}
	settings.CalendarEnabled = *req.Enabled
	if err := h.users.SetSettings(c.Context(), userID, settings); err != nil {
		return httpapi.ErrInternal("save settings").WithCause(err)
	}
	return h.list(c)
}

type googleCalendarConfigPatchReq struct {
	ClientID     *string `json:"clientId"`
	ClientSecret *string `json:"clientSecret"`
}

func (h *CalendarHandler) patchGoogleConfig(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	var req googleCalendarConfigPatchReq
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation(msgInvalidJSON)
	}
	if req.ClientID == nil {
		return httpapi.ErrValidation("clientId is required")
	}
	clientID := strings.TrimSpace(*req.ClientID)
	clientSecret := ""
	if req.ClientSecret != nil {
		clientSecret = strings.TrimSpace(*req.ClientSecret)
	}
	existing, err := h.calendars.GetOAuthConfig(c.Context(), userID, model.CalendarProviderGoogle)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		return httpapi.ErrInternal(msgLoadGoogleCalendarConfig).WithCause(err)
	}
	if clientID == "" && existing == nil {
		return httpapi.ErrValidation("clientId is required")
	}
	if clientSecret == "" && existing == nil {
		return httpapi.ErrValidation("clientSecret is required")
	}
	cipher := h.svc.Cipher()
	if clientID != "" {
		encrypted, err := cipher.Encrypt(clientID)
		if err != nil {
			return httpapi.ErrInternal("encrypt google calendar client id").WithCause(err)
		}
		clientID = encrypted
	}
	if clientSecret != "" {
		encrypted, err := cipher.Encrypt(clientSecret)
		if err != nil {
			return httpapi.ErrInternal("encrypt google calendar secret").WithCause(err)
		}
		clientSecret = encrypted
	}
	if _, err := h.calendars.UpsertOAuthConfig(c.Context(), &model.CalendarOAuthConfig{
		UserID:       userID,
		Provider:     model.CalendarProviderGoogle,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}); err != nil {
		return httpapi.ErrInternal("save google calendar config").WithCause(err)
	}
	h.svc.Cache().DeleteUser(userID)
	return h.list(c)
}

func (h *CalendarHandler) deleteGoogleConfig(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	if err := h.calendars.DeleteOAuthConfig(c.Context(), userID, model.CalendarProviderGoogle); err != nil && !errors.Is(err, repo.ErrNotFound) {
		return httpapi.ErrInternal("delete google calendar config").WithCause(err)
	}
	// Drop the connected account too: its stored refresh token was issued for
	// the credentials being removed, so leaving it behind only yields
	// `invalid_grant` on the next refresh. Sources cascade with the account.
	if err := h.calendars.DeleteAccountByProvider(c.Context(), userID, model.CalendarProviderGoogle); err != nil {
		return httpapi.ErrInternal("disconnect google calendar account").WithCause(err)
	}
	h.svc.Cache().DeleteUser(userID)
	return h.list(c)
}

func (h *CalendarHandler) googleStart(c fiber.Ctx) error {
	claims, appErr := h.claims(c)
	if appErr != nil {
		return appErr
	}
	cfg, ok, err := h.svc.OAuthConfigForUser(c.Context(), claims.UserID)
	if err != nil {
		return httpapi.ErrInternal(msgLoadGoogleCalendarConfig).WithCause(err)
	}
	if !ok {
		return httpapi.ErrValidation("Google Calendar OAuth is not configured")
	}
	state, err := randomState()
	if err != nil {
		return httpapi.ErrInternal("create oauth state").WithCause(err)
	}
	if err := h.calendars.CreateOAuthState(c.Context(), state, claims.UserID, claims.SessionID, model.CalendarProviderGoogle, 10*time.Minute); err != nil {
		return httpapi.ErrInternal("save oauth state").WithCause(err)
	}
	return c.JSON(fiber.Map{
		"url": cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce),
	})
}

func (h *CalendarHandler) googleCallback(c fiber.Ctx) error {
	if c.Query("error") != "" {
		slog.WarnContext(c.Context(), msgGCalOAuthCallbackError, "reason", "provider_error", "provider_error", c.Query("error"))
		return h.redirectToSettings(c, redirectCalendarError)
	}
	state := c.Query("state")
	code := c.Query("code")
	if state == "" || code == "" {
		slog.WarnContext(c.Context(), msgGCalOAuthCallbackError, "reason", "missing_state_or_code")
		return h.redirectToSettings(c, redirectCalendarError)
	}
	userID, err := h.calendars.ConsumeOAuthState(c.Context(), state, model.CalendarProviderGoogle)
	if errors.Is(err, repo.ErrNotFound) {
		slog.WarnContext(c.Context(), msgGCalOAuthCallbackError, "reason", "invalid_or_expired_state")
		return h.redirectToSettings(c, redirectCalendarError)
	}
	if err != nil {
		slog.WarnContext(c.Context(), msgGCalOAuthCallbackError, "reason", "consume_state_failed", "err", err)
		return h.redirectToSettings(c, redirectCalendarError)
	}
	cfg, ok, err := h.svc.OAuthConfigForUser(c.Context(), userID)
	if err != nil {
		slog.WarnContext(c.Context(), msgGCalOAuthCallbackError, "reason", "load_oauth_config_failed", "user_id", userID, "err", err)
		return h.redirectToSettings(c, redirectCalendarError)
	}
	if !ok {
		slog.WarnContext(c.Context(), msgGCalOAuthCallbackError, "reason", "oauth_not_configured", "user_id", userID)
		return h.redirectToSettings(c, redirectCalendarError)
	}
	token, err := cfg.Exchange(c.Context(), code)
	if err != nil {
		slog.WarnContext(c.Context(), msgGCalOAuthCallbackError, "reason", "token_exchange_failed", "user_id", userID, "err", err)
		return h.redirectToSettings(c, redirectCalendarError)
	}
	account, err := h.svc.SaveGoogleAccountAndSources(c.Context(), userID, cfg, token)
	if err != nil {
		slog.WarnContext(c.Context(), msgGCalOAuthCallbackError, "reason", "save_account_failed", "user_id", userID, "err", err)
		return h.redirectToSettings(c, redirectCalendarError)
	}
	if account != nil {
		settings, err := h.users.GetSettings(c.Context(), userID)
		if err == nil {
			settings.CalendarEnabled = true
			if setErr := h.users.SetSettings(c.Context(), userID, settings); setErr != nil {
				slog.WarnContext(c.Context(), "google calendar: failed to auto-enable calendar in settings", "user_id", userID, "err", setErr)
			}
		}
		h.svc.Cache().DeleteUser(userID)
	}
	slog.InfoContext(c.Context(), "google calendar connected", "user_id", userID)
	return h.redirectToSettings(c, "tab=calendars&calendar=connected")
}

func (h *CalendarHandler) redirectToSettings(c fiber.Ctx, query string) error {
	target := h.baseURL + "/settings"
	if query != "" {
		target += "?" + query
	}
	c.Set("Location", target)
	c.Status(fiber.StatusFound)
	return nil
}

func (h *CalendarHandler) googleSync(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	cfg, ok, err := h.svc.OAuthConfigForUser(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal(msgLoadGoogleCalendarConfig).WithCause(err)
	}
	if !ok {
		return httpapi.ErrValidation("Google Calendar OAuth is not configured")
	}
	account, err := h.calendars.GetAccountByProvider(c.Context(), userID, model.CalendarProviderGoogle)
	if errors.Is(err, repo.ErrNotFound) {
		return httpapi.ErrNotFound("Google Calendar is not connected")
	}
	if err != nil {
		return httpapi.ErrInternal("load calendar account").WithCause(err)
	}
	token, err := h.svc.FreshGoogleToken(c.Context(), cfg, account)
	if err != nil {
		if calendar.IsReauthRequired(err) {
			slog.WarnContext(c.Context(), "google calendar reauth required",
				"op", "handler.Calendar.googleSync", "user_id", userID, "err", err)
			return httpapi.ErrCalendarReauthRequired()
		}
		return httpapi.ErrInternal("refresh google calendar token").WithCause(err)
	}
	if _, err := h.svc.SaveGoogleAccountAndSources(c.Context(), userID, cfg, token); err != nil {
		return httpapi.ErrInternal("sync google calendar").WithCause(err)
	}
	h.svc.Cache().DeleteUser(userID)
	slog.InfoContext(c.Context(), "google calendar synced", "user_id", userID)
	return h.list(c)
}

type calendarSourcePatchReq struct {
	Selected *bool `json:"selected"`
}

func (h *CalendarHandler) patchSource(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req calendarSourcePatchReq
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation(msgInvalidJSON)
	}
	if req.Selected == nil {
		return httpapi.ErrValidation("selected is required")
	}
	src, err := h.calendars.SetSourceSelected(c.Context(), userID, id, *req.Selected)
	if errors.Is(err, repo.ErrNotFound) {
		return httpapi.ErrNotFound("calendar source not found")
	}
	if err != nil {
		return httpapi.ErrInternal("update calendar source").WithCause(err)
	}
	h.svc.Cache().DeleteUser(userID)
	return c.JSON(calendarSourceToResp(*src))
}

func (h *CalendarHandler) deleteAccount(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.calendars.DeleteAccount(c.Context(), userID, id); errors.Is(err, repo.ErrNotFound) {
		return httpapi.ErrNotFound("calendar account not found")
	} else if err != nil {
		return httpapi.ErrInternal("delete calendar account").WithCause(err)
	}
	h.svc.Cache().DeleteUser(userID)
	slog.InfoContext(c.Context(), "google calendar account disconnected", "user_id", userID, "account_id", id)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *CalendarHandler) events(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	settings, err := h.users.GetSettings(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal(msgLoadSettings).WithCause(err)
	}
	if !settings.CalendarEnabled {
		return c.JSON(fiber.Map{"items": []calendarEventResp{}})
	}
	start, end, appErr := parseEventRange(c)
	if appErr != nil {
		return appErr
	}
	cfg, ok, err := h.svc.OAuthConfigForUser(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal(msgLoadGoogleCalendarConfig).WithCause(err)
	}
	if !ok {
		return c.JSON(fiber.Map{"items": []calendarEventResp{}})
	}
	account, err := h.calendars.GetAccountByProvider(c.Context(), userID, model.CalendarProviderGoogle)
	if errors.Is(err, repo.ErrNotFound) {
		return c.JSON(fiber.Map{"items": []calendarEventResp{}})
	}
	if err != nil {
		return httpapi.ErrInternal("load calendar account").WithCause(err)
	}
	sources, err := h.calendars.ListSelectedSources(c.Context(), userID, model.CalendarProviderGoogle)
	if err != nil {
		return httpapi.ErrInternal("list selected calendar sources").WithCause(err)
	}
	cacheKey := calendar.EventsCacheKey(userID, start, end, sources)
	if items, ok := h.svc.Cache().Get(cacheKey); ok {
		resps := make([]calendarEventResp, len(items))
		for i, e := range items {
			resps[i] = calendarEventToResp(e)
		}
		return c.JSON(fiber.Map{"items": resps})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 20*time.Second)
	defer cancel()
	items, err := h.svc.FetchGoogleEvents(ctx, cfg, account, sources, start, end)
	if err != nil {
		if calendar.IsReauthRequired(err) {
			slog.WarnContext(c.Context(), "google calendar reauth required",
				"op", "handler.Calendar.events", "user_id", userID, "err", err)
			return httpapi.ErrCalendarReauthRequired()
		}
		return httpapi.ErrInternal("fetch calendar events").WithCause(err)
	}
	h.svc.Cache().Set(cacheKey, items)
	resps := make([]calendarEventResp, len(items))
	for i, e := range items {
		resps[i] = calendarEventToResp(e)
	}
	return c.JSON(fiber.Map{"items": resps})
}

func parseEventRange(c fiber.Ctx) (time.Time, time.Time, *httpapi.AppError) {
	startRaw := c.Query("start")
	endRaw := c.Query("end")
	if startRaw == "" || endRaw == "" {
		return time.Time{}, time.Time{}, httpapi.ErrValidation("start and end are required")
	}
	start, err := model.ParseUTC(startRaw)
	if err != nil {
		return time.Time{}, time.Time{}, httpapi.ErrValidation("invalid start format")
	}
	end, err := model.ParseUTC(endRaw)
	if err != nil {
		return time.Time{}, time.Time{}, httpapi.ErrValidation("invalid end format")
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, httpapi.ErrValidation("end must be after start")
	}
	if end.Sub(start) > 92*24*time.Hour {
		return time.Time{}, time.Time{}, httpapi.ErrValidation("calendar range is too large")
	}
	return start, end, nil
}

func randomState() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
