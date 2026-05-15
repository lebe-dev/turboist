package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	calendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

type CalendarHandler struct {
	calendars          *repo.CalendarRepo
	users              *repo.UserRepo
	baseURL            string
	googleClientID     string
	googleClientSecret string
}

func NewCalendarHandler(calendars *repo.CalendarRepo, users *repo.UserRepo, baseURL, googleClientID, googleClientSecret string) *CalendarHandler {
	return &CalendarHandler{
		calendars:          calendars,
		users:              users,
		baseURL:            strings.TrimRight(baseURL, "/"),
		googleClientID:     googleClientID,
		googleClientSecret: googleClientSecret,
	}
}

func (h *CalendarHandler) RegisterPublic(app fiber.Router) {
	app.Get("/api/v1/calendars/google/callback", h.googleCallback)
}

func (h *CalendarHandler) Register(r fiber.Router) {
	r.Get("/", h.list)
	r.Patch("/settings", h.patchSettings)
	r.Get("/events", h.events)
	r.Get("/google/start", h.googleStart)
	r.Post("/google/sync", h.googleSync)
	r.Patch("/sources/:id", h.patchSource)
	r.Delete("/accounts/:id", h.deleteAccount)
}

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
	Enabled          bool                  `json:"enabled"`
	GoogleConfigured bool                  `json:"googleConfigured"`
	Accounts         []calendarAccountResp `json:"accounts"`
	Sources          []calendarSourceResp  `json:"sources"`
}

type calendarEventResp struct {
	ID          string `json:"id"`
	SourceID    int64  `json:"sourceId"`
	SourceName  string `json:"sourceName"`
	SourceColor string `json:"sourceColor"`
	Provider    string `json:"provider"`
	ExternalID  string `json:"externalId"`
	Title       string `json:"title"`
	Location    string `json:"location"`
	Start       string `json:"start"`
	End         string `json:"end"`
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

func (h *CalendarHandler) googleOAuthConfig() (*oauth2.Config, bool) {
	if h.googleClientID == "" || h.googleClientSecret == "" {
		return nil, false
	}
	return &oauth2.Config{
		ClientID:     h.googleClientID,
		ClientSecret: h.googleClientSecret,
		RedirectURL:  h.baseURL + "/api/v1/calendars/google/callback",
		Scopes:       []string{calendar.CalendarReadonlyScope},
		Endpoint:     google.Endpoint,
	}, true
}

func (h *CalendarHandler) claimsUserID(c fiber.Ctx) (int64, *httpapi.AppError) {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return 0, httpapi.ErrAuthInvalid("missing auth claims")
	}
	return claims.UserID, nil
}

func (h *CalendarHandler) list(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	settings, err := h.users.GetSettings(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("load settings")
	}
	accounts, err := h.calendars.ListAccounts(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("list calendar accounts")
	}
	sources, err := h.calendars.ListSources(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("list calendar sources")
	}
	out := calendarListResp{
		Enabled:          settings.CalendarEnabled,
		GoogleConfigured: h.googleClientID != "" && h.googleClientSecret != "",
		Accounts:         make([]calendarAccountResp, len(accounts)),
		Sources:          make([]calendarSourceResp, len(sources)),
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
		return httpapi.ErrValidation("invalid JSON")
	}
	if req.Enabled == nil {
		return httpapi.ErrValidation("enabled is required")
	}
	settings, err := h.users.GetSettings(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("load settings")
	}
	settings.CalendarEnabled = *req.Enabled
	if err := h.users.SetSettings(c.Context(), userID, settings); err != nil {
		return httpapi.ErrInternal("save settings")
	}
	return h.list(c)
}

func (h *CalendarHandler) googleStart(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	cfg, ok := h.googleOAuthConfig()
	if !ok {
		return httpapi.ErrValidation("Google Calendar OAuth is not configured")
	}
	state, err := randomState()
	if err != nil {
		return httpapi.ErrInternal("create oauth state")
	}
	if err := h.calendars.CreateOAuthState(c.Context(), state, userID, model.CalendarProviderGoogle, 10*time.Minute); err != nil {
		return httpapi.ErrInternal("save oauth state")
	}
	return c.JSON(fiber.Map{
		"url": cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce),
	})
}

func (h *CalendarHandler) googleCallback(c fiber.Ctx) error {
	cfg, ok := h.googleOAuthConfig()
	if !ok {
		return httpapi.ErrValidation("Google Calendar OAuth is not configured")
	}
	if c.Query("error") != "" {
		return h.redirectToSettings(c, "calendar=error")
	}
	state := c.Query("state")
	code := c.Query("code")
	if state == "" || code == "" {
		return httpapi.ErrValidation("missing OAuth state or code")
	}
	userID, err := h.calendars.ConsumeOAuthState(c.Context(), state, model.CalendarProviderGoogle)
	if errors.Is(err, repo.ErrNotFound) {
		return httpapi.ErrValidation("invalid or expired OAuth state")
	}
	if err != nil {
		return httpapi.ErrInternal("consume oauth state")
	}
	token, err := cfg.Exchange(c.Context(), code)
	if err != nil {
		return httpapi.ErrInternal("exchange google token")
	}
	account, err := h.saveGoogleAccountAndSources(c.Context(), userID, cfg, token)
	if err != nil {
		return err
	}
	if account != nil {
		settings, err := h.users.GetSettings(c.Context(), userID)
		if err == nil {
			settings.CalendarEnabled = true
			_ = h.users.SetSettings(c.Context(), userID, settings)
		}
	}
	return h.redirectToSettings(c, "calendar=connected")
}

func (h *CalendarHandler) redirectToSettings(c fiber.Ctx, query string) error {
	target := h.baseURL + "/settings"
	if query != "" {
		target += "?" + query
	}
	c.Set("Location", target)
	return c.SendStatus(fiber.StatusFound)
}

func (h *CalendarHandler) googleSync(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	cfg, ok := h.googleOAuthConfig()
	if !ok {
		return httpapi.ErrValidation("Google Calendar OAuth is not configured")
	}
	account, err := h.calendars.GetAccountByProvider(c.Context(), userID, model.CalendarProviderGoogle)
	if errors.Is(err, repo.ErrNotFound) {
		return httpapi.ErrNotFound("Google Calendar is not connected")
	}
	if err != nil {
		return httpapi.ErrInternal("load calendar account")
	}
	if _, err := h.saveGoogleAccountAndSources(c.Context(), userID, cfg, accountToken(account)); err != nil {
		return err
	}
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
		return httpapi.ErrValidation("invalid JSON")
	}
	if req.Selected == nil {
		return httpapi.ErrValidation("selected is required")
	}
	src, err := h.calendars.SetSourceSelected(c.Context(), userID, id, *req.Selected)
	if errors.Is(err, repo.ErrNotFound) {
		return httpapi.ErrNotFound("calendar source not found")
	}
	if err != nil {
		return httpapi.ErrInternal("update calendar source")
	}
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
		return httpapi.ErrInternal("delete calendar account")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *CalendarHandler) events(c fiber.Ctx) error {
	userID, appErr := h.claimsUserID(c)
	if appErr != nil {
		return appErr
	}
	settings, err := h.users.GetSettings(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("load settings")
	}
	if !settings.CalendarEnabled {
		return c.JSON(fiber.Map{"items": []calendarEventResp{}})
	}
	start, end, appErr := parseEventRange(c)
	if appErr != nil {
		return appErr
	}
	cfg, ok := h.googleOAuthConfig()
	if !ok {
		return c.JSON(fiber.Map{"items": []calendarEventResp{}})
	}
	account, err := h.calendars.GetAccountByProvider(c.Context(), userID, model.CalendarProviderGoogle)
	if errors.Is(err, repo.ErrNotFound) {
		return c.JSON(fiber.Map{"items": []calendarEventResp{}})
	}
	if err != nil {
		return httpapi.ErrInternal("load calendar account")
	}
	sources, err := h.calendars.ListSelectedSources(c.Context(), userID, model.CalendarProviderGoogle)
	if err != nil {
		return httpapi.ErrInternal("list selected calendar sources")
	}
	items, err := h.fetchGoogleEvents(c.Context(), cfg, account, sources, start, end)
	if err != nil {
		return httpapi.ErrInternal("fetch calendar events")
	}
	return c.JSON(fiber.Map{"items": items})
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

func (h *CalendarHandler) saveGoogleAccountAndSources(ctx context.Context, userID int64, cfg *oauth2.Config, token *oauth2.Token) (*model.CalendarAccount, *httpapi.AppError) {
	svc, err := googleCalendarService(ctx, cfg, token)
	if err != nil {
		return nil, httpapi.ErrInternal("create google calendar client")
	}
	list, err := svc.CalendarList.List().MinAccessRole("reader").Do()
	if err != nil {
		return nil, httpapi.ErrInternal("load google calendars")
	}
	email := ""
	display := "Google Calendar"
	for _, item := range list.Items {
		if item.Primary {
			email = item.Id
			display = item.Summary
			break
		}
	}
	account, err := h.calendars.UpsertAccount(ctx, &model.CalendarAccount{
		UserID:       userID,
		Provider:     model.CalendarProviderGoogle,
		Email:        email,
		DisplayName:  display,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	})
	if err != nil {
		return nil, httpapi.ErrInternal("save calendar account")
	}
	sources := make([]model.CalendarSource, 0, len(list.Items))
	for _, item := range list.Items {
		if item.Id == "" {
			continue
		}
		color := item.BackgroundColor
		if color == "" {
			color = item.ForegroundColor
		}
		sources = append(sources, model.CalendarSource{
			ExternalID: item.Id,
			Summary:    item.Summary,
			Color:      color,
			Selected:   true,
			IsPrimary:  item.Primary,
		})
	}
	if err := h.calendars.UpsertSources(ctx, account, sources); err != nil {
		return nil, httpapi.ErrInternal("save calendar list")
	}
	return account, nil
}

func (h *CalendarHandler) fetchGoogleEvents(ctx context.Context, cfg *oauth2.Config, account *model.CalendarAccount, sources []model.CalendarSource, start, end time.Time) ([]calendarEventResp, error) {
	svc, err := googleCalendarService(ctx, cfg, accountToken(account))
	if err != nil {
		return nil, err
	}
	out := []calendarEventResp{}
	for _, source := range sources {
		events, err := svc.Events.List(source.ExternalID).
			SingleEvents(true).
			ShowDeleted(false).
			OrderBy("startTime").
			TimeMin(start.Format(time.RFC3339)).
			TimeMax(end.Format(time.RFC3339)).
			MaxResults(250).
			Do()
		if err != nil {
			return nil, err
		}
		for _, ev := range events.Items {
			if ev.Status == "cancelled" {
				continue
			}
			item, ok := googleEventToResp(ev, source)
			if ok {
				out = append(out, item)
			}
		}
	}
	return out, nil
}

func googleCalendarService(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (*calendar.Service, error) {
	client := cfg.Client(ctx, token)
	return calendar.NewService(ctx, option.WithHTTPClient(client))
}

func accountToken(a *model.CalendarAccount) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  a.AccessToken,
		RefreshToken: a.RefreshToken,
		Expiry:       a.Expiry,
		TokenType:    "Bearer",
	}
}

func googleEventToResp(ev *calendar.Event, source model.CalendarSource) (calendarEventResp, bool) {
	start, end, allDay, ok := googleEventTimes(ev)
	if !ok {
		return calendarEventResp{}, false
	}
	title := ev.Summary
	if title == "" {
		title = "(No title)"
	}
	return calendarEventResp{
		ID:          string(model.CalendarProviderGoogle) + ":" + source.ExternalID + ":" + ev.Id,
		SourceID:    source.ID,
		SourceName:  source.Summary,
		SourceColor: source.Color,
		Provider:    string(model.CalendarProviderGoogle),
		ExternalID:  ev.Id,
		Title:       title,
		Location:    ev.Location,
		Start:       model.FormatUTC(start),
		End:         model.FormatUTC(end),
		AllDay:      allDay,
		HTMLLink:    ev.HtmlLink,
	}, true
}

func googleEventTimes(ev *calendar.Event) (time.Time, time.Time, bool, bool) {
	if ev.Start == nil || ev.End == nil {
		return time.Time{}, time.Time{}, false, false
	}
	if ev.Start.DateTime != "" {
		start, err := time.Parse(time.RFC3339, ev.Start.DateTime)
		if err != nil {
			return time.Time{}, time.Time{}, false, false
		}
		end, err := time.Parse(time.RFC3339, ev.End.DateTime)
		if err != nil {
			end = start
		}
		return start, end, false, true
	}
	if ev.Start.Date != "" {
		start, err := time.Parse("2006-01-02", ev.Start.Date)
		if err != nil {
			return time.Time{}, time.Time{}, false, false
		}
		end, err := time.Parse("2006-01-02", ev.End.Date)
		if err != nil {
			end = start.Add(24 * time.Hour)
		}
		return start, end, true, true
	}
	return time.Time{}, time.Time{}, false, false
}

func randomState() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
