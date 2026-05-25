package calendar

import (
	"context"
	"errors"
	"html"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

var (
	// htmlScriptRe matches <script>…</script> and <style>…</style> blocks
	// including their content, which must be removed rather than converted to text.
	htmlScriptRe = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	// htmlBlockRe matches block-level and line-break tags that should become newlines.
	htmlBlockRe = regexp.MustCompile(`(?i)<(/?(br|p|div|li|tr|h[1-6]|blockquote|pre|ul|ol|hr)[^>]*)>`)
	// htmlTagRe strips any remaining HTML tags.
	// Note: does not handle '>' inside attribute values (e.g. title="a > b"),
	// which is not produced by Google Calendar's rich-text editor.
	htmlTagRe = regexp.MustCompile(`<[^>]+>`)
)

// Service encapsulates the business logic for Google Calendar integration.
type Service struct {
	calendars *repo.CalendarRepo
	users     *repo.UserRepo
	cipher    *crypto.TokenCipher
	cache     *EventCache
	baseURL   string
	log       *slog.Logger
}

// NewService constructs a Service. calendarTokenKey is used to derive the AES
// encryption key for OAuth tokens stored in the DB.
func NewService(
	calendars *repo.CalendarRepo,
	users *repo.UserRepo,
	baseURL, calendarTokenKey string,
	log *slog.Logger,
) *Service {
	return &Service{
		calendars: calendars,
		users:     users,
		cipher:    crypto.NewTokenCipher(calendarTokenKey),
		cache:     NewEventCache(2 * time.Minute),
		baseURL:   strings.TrimRight(baseURL, "/"),
		log:       log,
	}
}

// Cipher returns the underlying TokenCipher so handlers can encrypt/decrypt
// individual values (e.g. per-user OAuth config).
func (s *Service) Cipher() *crypto.TokenCipher {
	return s.cipher
}

// Cache returns the underlying EventCache so handlers can invalidate entries.
func (s *Service) Cache() *EventCache {
	return s.cache
}

// OAuthConfig builds an oauth2.Config from the given credentials. Returns
// (nil, false) when either credential is empty.
func (s *Service) OAuthConfig(clientID, clientSecret string) (*oauth2.Config, bool) {
	if clientID == "" || clientSecret == "" {
		return nil, false
	}
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  s.baseURL + "/api/v1/calendars/google/callback",
		Scopes:       []string{gcal.CalendarReadonlyScope},
		Endpoint:     google.Endpoint,
	}, true
}

// OAuthConfigForUser returns the per-user OAuth config stored in the DB.
// Returns (nil, false, nil) when no credentials are configured.
func (s *Service) OAuthConfigForUser(ctx context.Context, userID int64) (*oauth2.Config, bool, error) {
	dbCfg, err := s.calendars.GetOAuthConfig(ctx, userID, model.CalendarProviderGoogle)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	clientID, err := s.cipher.Decrypt(dbCfg.ClientID)
	if err != nil {
		return nil, false, err
	}
	clientSecret, err := s.cipher.Decrypt(dbCfg.ClientSecret)
	if err != nil {
		return nil, false, err
	}
	if err := s.ensureEncryptedOAuthConfig(ctx, dbCfg, clientID, clientSecret); err != nil {
		return nil, false, err
	}
	cfg, ok := s.OAuthConfig(clientID, clientSecret)
	return cfg, ok, nil
}

// ensureEncryptedOAuthConfig re-encrypts plain-text credentials stored before
// encryption was introduced. It is idempotent when values are already encrypted.
func (s *Service) ensureEncryptedOAuthConfig(ctx context.Context, cfg *model.CalendarOAuthConfig, clientID, clientSecret string) error {
	if crypto.IsEncrypted(cfg.ClientID) && crypto.IsEncrypted(cfg.ClientSecret) {
		return nil
	}
	encryptedID, err := s.cipher.Encrypt(clientID)
	if err != nil {
		return err
	}
	encryptedSecret, err := s.cipher.Encrypt(clientSecret)
	if err != nil {
		return err
	}
	_, err = s.calendars.UpsertOAuthConfig(ctx, &model.CalendarOAuthConfig{
		UserID:       cfg.UserID,
		Provider:     cfg.Provider,
		ClientID:     encryptedID,
		ClientSecret: encryptedSecret,
	})
	return err
}

// FreshGoogleToken returns a valid (potentially refreshed) token for the given
// account. If the token was refreshed or was stored in plain-text, the new
// encrypted value is persisted back to the DB.
func (s *Service) FreshGoogleToken(ctx context.Context, cfg *oauth2.Config, account *model.CalendarAccount) (*oauth2.Token, error) {
	token := accountToken(account)
	needsEncryption := !crypto.IsEncrypted(token.AccessToken) || !crypto.IsEncrypted(token.RefreshToken)
	if err := s.DecryptToken(token); err != nil {
		return nil, err
	}
	src := cfg.TokenSource(ctx, token)
	fresh, err := src.Token()
	if err != nil {
		return nil, err
	}
	if tokenChanged(token, fresh) || needsEncryption {
		account.AccessToken = fresh.AccessToken
		account.RefreshToken = fresh.RefreshToken
		if account.RefreshToken == "" {
			account.RefreshToken = token.RefreshToken
		}
		account.Expiry = fresh.Expiry
		if err := s.EncryptAccountTokens(account); err != nil {
			return nil, err
		}
		if _, err := s.calendars.UpdateAccountToken(ctx, account); err != nil {
			return nil, err
		}
	}
	return fresh, nil
}

// SaveGoogleAccountAndSources connects to the Google Calendar API with the
// given token, retrieves the user's calendar list, and upserts the account and
// all calendar sources in the DB.
func (s *Service) SaveGoogleAccountAndSources(ctx context.Context, userID int64, cfg *oauth2.Config, token *oauth2.Token) (*model.CalendarAccount, error) {
	svc, err := newGoogleCalendarClient(ctx, token)
	if err != nil {
		return nil, err
	}
	list, err := svc.CalendarList.List().MinAccessRole("reader").Do()
	if err != nil {
		return nil, err
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
	accountInput := &model.CalendarAccount{
		UserID:       userID,
		Provider:     model.CalendarProviderGoogle,
		Email:        email,
		DisplayName:  display,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}
	if err := s.EncryptAccountTokens(accountInput); err != nil {
		return nil, err
	}
	account, err := s.calendars.UpsertAccount(ctx, accountInput)
	if err != nil {
		return nil, err
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
	if err := s.calendars.UpsertSources(ctx, account, sources); err != nil {
		return nil, err
	}
	return account, nil
}

// FetchGoogleEvents fetches events from all given sources in [start, end) and
// returns them as domain CalendarEvent values.
func (s *Service) FetchGoogleEvents(ctx context.Context, cfg *oauth2.Config, account *model.CalendarAccount, sources []model.CalendarSource, start, end time.Time) ([]CalendarEvent, error) {
	fresh, err := s.FreshGoogleToken(ctx, cfg, account)
	if err != nil {
		return nil, err
	}
	svc, err := newGoogleCalendarClient(ctx, fresh)
	if err != nil {
		return nil, err
	}
	out := []CalendarEvent{}
	for _, source := range sources {
		pageToken := ""
		for {
			call := svc.Events.List(source.ExternalID).
				SingleEvents(true).
				ShowDeleted(false).
				OrderBy("startTime").
				TimeMin(start.Format(time.RFC3339)).
				TimeMax(end.Format(time.RFC3339)).
				MaxResults(250)
			if pageToken != "" {
				call.PageToken(pageToken)
			}
			events, err := call.Do()
			if err != nil {
				return nil, err
			}
			for _, ev := range events.Items {
				if ev.Status == "cancelled" {
					continue
				}
				item, ok := googleEventToCalendarEvent(ev, source)
				if ok {
					out = append(out, item)
				}
			}
			if events.NextPageToken == "" {
				break
			}
			pageToken = events.NextPageToken
		}
	}
	return out, nil
}

// EncryptAccountTokens encrypts both tokens on account in-place.
func (s *Service) EncryptAccountTokens(account *model.CalendarAccount) error {
	access, err := s.cipher.Encrypt(account.AccessToken)
	if err != nil {
		return err
	}
	refresh, err := s.cipher.Encrypt(account.RefreshToken)
	if err != nil {
		return err
	}
	account.AccessToken = access
	account.RefreshToken = refresh
	return nil
}

// DecryptToken decrypts both tokens on token in-place.
func (s *Service) DecryptToken(token *oauth2.Token) error {
	access, err := s.cipher.Decrypt(token.AccessToken)
	if err != nil {
		return err
	}
	refresh, err := s.cipher.Decrypt(token.RefreshToken)
	if err != nil {
		return err
	}
	token.AccessToken = access
	token.RefreshToken = refresh
	return nil
}

// --- private helpers ---

func newGoogleCalendarClient(ctx context.Context, token *oauth2.Token) (*gcal.Service, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	return gcal.NewService(ctx, option.WithHTTPClient(client))
}

func accountToken(a *model.CalendarAccount) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  a.AccessToken,
		RefreshToken: a.RefreshToken,
		Expiry:       a.Expiry,
		TokenType:    "Bearer",
	}
}

func tokenChanged(old, fresh *oauth2.Token) bool {
	return old.AccessToken != fresh.AccessToken ||
		(old.RefreshToken != "" && fresh.RefreshToken != "" && old.RefreshToken != fresh.RefreshToken) ||
		!old.Expiry.Equal(fresh.Expiry)
}

func googleEventToCalendarEvent(ev *gcal.Event, source model.CalendarSource) (CalendarEvent, bool) {
	start, end, startDate, endDate, allDay, ok := googleEventTimes(ev)
	if !ok {
		return CalendarEvent{}, false
	}
	title := ev.Summary
	if title == "" {
		title = "(No title)"
	}
	return CalendarEvent{
		ID:          string(model.CalendarProviderGoogle) + ":" + source.ExternalID + ":" + ev.Id,
		SourceID:    source.ID,
		SourceName:  source.Summary,
		SourceColor: source.Color,
		Provider:    string(model.CalendarProviderGoogle),
		ExternalID:  ev.Id,
		Title:       title,
		Description: stripHTML(ev.Description),
		Location:    ev.Location,
		Start:       start,
		End:         end,
		StartDate:   startDate,
		EndDate:     endDate,
		AllDay:      allDay,
		HTMLLink:    ev.HtmlLink,
	}, true
}

// stripHTML converts an HTML string to plain text suitable for display.
// Block-level tags are converted to newlines so paragraph structure is
// preserved (useful when the text is shown in a textarea for task prefill).
// <script> and <style> blocks are removed entirely including their content.
// The result is capped at 1000 characters to avoid pathologically large
// descriptions blowing up the JSON response and the in-memory cache.
func stripHTML(s string) string {
	const maxLen = 1000
	s = htmlScriptRe.ReplaceAllString(s, "")
	s = htmlBlockRe.ReplaceAllString(s, "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	// Normalise each line, drop blank lines.
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Join(strings.Fields(p), " ")
		if p != "" {
			out = append(out, p)
		}
	}
	result := strings.Join(out, "\n")
	if len(result) > maxLen {
		result = result[:maxLen] + "…"
	}
	return result
}

func googleEventTimes(ev *gcal.Event) (time.Time, time.Time, string, string, bool, bool) {
	if ev.Start == nil || ev.End == nil {
		return time.Time{}, time.Time{}, "", "", false, false
	}
	if ev.Start.DateTime != "" {
		start, err := time.Parse(time.RFC3339, ev.Start.DateTime)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", false, false
		}
		end, err := time.Parse(time.RFC3339, ev.End.DateTime)
		if err != nil {
			end = start
		}
		return start, end, "", "", false, true
	}
	if ev.Start.Date != "" {
		start, err := time.Parse("2006-01-02", ev.Start.Date)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", false, false
		}
		endDate := ev.End.Date
		end, err := time.Parse("2006-01-02", endDate)
		if err != nil {
			end = start.Add(24 * time.Hour)
			endDate = end.Format("2006-01-02")
		}
		return start, end, ev.Start.Date, endDate, true, true
	}
	return time.Time{}, time.Time{}, "", "", false, false
}
