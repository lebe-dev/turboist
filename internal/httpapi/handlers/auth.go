package handlers

import (
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service/totp"
)

const (
	sessionLimit        = 5
	refreshCookieName   = "refresh"
	refreshCookiePath   = "/auth/refresh"
	refreshCookieMaxAge = 30 * 24 * 60 * 60 // 30 days in seconds
)

const (
	opAuthSetup    = "handler.Auth.Setup"
	opAuthLogin    = "handler.Auth.Login"
	opAuthLoginOTP = "handler.Auth.LoginOTP"
	opAuthRefresh  = "handler.Auth.Refresh"

	msgInvalidCredentials = "invalid credentials"
)

// AuthHandler implements all /auth/* endpoints.
type AuthHandler struct {
	users        *repo.UserRepo
	sessions     *repo.SessionRepo
	jwt          *auth.JWTIssuer
	limiter      *auth.IPLimiter
	theft        *theftCache
	argon2Params auth.Argon2Params
	// totp is nil when the TOTP feature is disabled (no TOTP_SECRET_KEY). When
	// non-nil, the login flow becomes two-step for accounts that have enrolled.
	totp *totp.Service
}

// NewAuthHandler constructs an AuthHandler.
func NewAuthHandler(
	users *repo.UserRepo,
	sessions *repo.SessionRepo,
	jwt *auth.JWTIssuer,
	limiter *auth.IPLimiter,
	argon2Params auth.Argon2Params,
) *AuthHandler {
	return &AuthHandler{
		users:        users,
		sessions:     sessions,
		jwt:          jwt,
		limiter:      limiter,
		theft:        newTheftCache(),
		argon2Params: argon2Params,
	}
}

// WithTOTP enables the two-step login flow. Pass nil (or skip the call) to
// keep TOTP disabled.
func (h *AuthHandler) WithTOTP(svc *totp.Service) *AuthHandler {
	h.totp = svc
	return h
}

// Stop releases background goroutines started by this handler.
func (h *AuthHandler) Stop() { h.theft.stop() }

// RegisterAuth wires /auth routes onto r. Protected routes (logout, me) use jwtIssuer middleware.
//
// Setup discovery is no longer a dedicated endpoint — `SetupCheckMiddleware`
// on the /api/v1 group answers `/api/v1/config` with HTTP 503 `setup_required`
// while the instance has no admin user, and the frontend reacts to that code.
func (h *AuthHandler) RegisterAuth(r fiber.Router, jwtIssuer *auth.JWTIssuer) {
	r.Post("/setup", h.setup)
	r.Post("/login", h.login)
	r.Post("/login/otp", h.loginOTP)
	r.Post("/refresh", h.refresh)
	r.Post("/logout", httpapi.AuthMiddleware(jwtIssuer), h.logout)
	r.Post("/logout-all", httpapi.AuthMiddleware(jwtIssuer), h.logoutAll)
	r.Post("/logout-others", httpapi.AuthMiddleware(jwtIssuer), h.logoutOthers)
	r.Get("/me", httpapi.AuthMiddleware(jwtIssuer), h.me)
}

func (h *AuthHandler) setup(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	if !h.limiter.Allow(c.IP()) {
		log.WarnContext(ctx, "auth: setup rate limited",
			slog.String("op", opAuthSetup),
			slog.String("ip", c.IP()),
		)
		return httpapi.ErrAuthRateLimited()
	}

	exists, err := h.users.Exists(ctx)
	if err != nil {
		return httpapi.ErrInternal("check user existence").WithCause(err)
	}
	if exists {
		log.WarnContext(ctx, "auth: setup already done",
			slog.String("op", opAuthSetup),
		)
		return httpapi.ErrSetupAlreadyDone()
	}

	var req dto.LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		log.WarnContext(ctx, "auth: setup invalid body",
			slog.String("op", opAuthSetup),
			slog.String("err", err.Error()),
		)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if err := validateLoginRequest(req); err != nil {
		log.WarnContext(ctx, "auth: setup validation failed",
			slog.String("op", opAuthSetup),
			slog.String("code", err.Code),
		)
		return err
	}

	hash, err := auth.HashPassword(req.Password, h.argon2Params)
	if err != nil {
		return httpapi.ErrInternal("hash password").WithCause(err)
	}
	user, err := h.users.Create(ctx, req.Username, hash)
	if err != nil {
		return httpapi.ErrInternal("create user").WithCause(err)
	}

	log.InfoContext(ctx, "auth: setup complete",
		slog.String("op", opAuthSetup),
		slog.Int64("user_id", user.ID),
		slog.String("client_kind", string(req.ClientKind)),
	)
	return h.issueSession(c, user, req.ClientKind)
}

func (h *AuthHandler) login(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	if !h.limiter.Allow(c.IP()) {
		log.WarnContext(ctx, "auth: login rate limited",
			slog.String("op", opAuthLogin),
			slog.String("ip", c.IP()),
		)
		return httpapi.ErrAuthRateLimited()
	}

	var req dto.LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		log.WarnContext(ctx, "auth: login invalid body",
			slog.String("op", opAuthLogin),
			slog.String("err", err.Error()),
		)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if err := validateLoginRequest(req); err != nil {
		log.WarnContext(ctx, "auth: login validation failed",
			slog.String("op", opAuthLogin),
			slog.String("code", err.Code),
		)
		return err
	}

	user, err := h.users.GetByUsername(ctx, req.Username)
	if err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrInternal("lookup user").WithCause(err)
		}
		log.WarnContext(ctx, "auth: login unknown user",
			slog.String("op", opAuthLogin),
			slog.String("client_kind", string(req.ClientKind)),
		)
		// Avoid username enumeration: return same error for not found vs wrong password.
		return httpapi.ErrAuthInvalid(msgInvalidCredentials)
	}
	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		if errors.Is(err, auth.ErrInvalidHash) || errors.Is(err, auth.ErrUnsupportedHashAlgo) {
			// Stored hash is malformed or uses an unsupported algorithm. Keep the
			// client-facing response identical to a wrong-password reply to avoid
			// account enumeration; surface the underlying cause server-side only.
			log.ErrorContext(ctx, "auth: login stored hash invalid",
				slog.String("op", opAuthLogin),
				slog.Int64("user_id", user.ID),
				slog.String("err", err.Error()),
			)
			return httpapi.ErrAuthInvalid(msgInvalidCredentials)
		}
		log.WarnContext(ctx, "auth: login wrong password",
			slog.String("op", opAuthLogin),
			slog.Int64("user_id", user.ID),
			slog.String("client_kind", string(req.ClientKind)),
		)
		return httpapi.ErrAuthInvalid(msgInvalidCredentials)
	}

	if user.TOTPEnabled {
		if h.totp == nil {
			// Fail closed: this account has 2FA enabled but the TOTP service is
			// unavailable (e.g. TOTP_SECRET_KEY missing on this deploy). Refusing
			// the login prevents a misconfigured restart from silently bypassing
			// 2FA for already-enrolled users.
			log.ErrorContext(ctx, "auth: login refused — totp service unavailable for enrolled user",
				slog.String("op", opAuthLogin),
				slog.Int64("user_id", user.ID),
			)
			return httpapi.ErrInternal("totp service unavailable")
		}
		ticket, _, terr := h.jwt.IssueOTPTicket(user.ID, string(req.ClientKind))
		if terr != nil {
			return httpapi.ErrInternal("issue otp ticket").WithCause(terr)
		}
		log.InfoContext(ctx, "auth: login awaiting otp",
			slog.String("op", opAuthLogin),
			slog.Int64("user_id", user.ID),
			slog.String("client_kind", string(req.ClientKind)),
		)
		return c.JSON(dto.OTPChallengeResponse{OTPRequired: true, Ticket: ticket})
	}

	log.InfoContext(ctx, "auth: login ok",
		slog.String("op", opAuthLogin),
		slog.Int64("user_id", user.ID),
		slog.String("client_kind", string(req.ClientKind)),
	)
	return h.issueSession(c, user, req.ClientKind)
}

// loginOTP completes the two-step login: it verifies the short-lived ticket
// issued by /auth/login and the user-entered TOTP (or recovery) code, then
// issues a regular session.
func (h *AuthHandler) loginOTP(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	if !h.limiter.Allow(c.IP()) {
		log.WarnContext(ctx, "auth: login/otp rate limited",
			slog.String("op", opAuthLoginOTP),
			slog.String("ip", c.IP()),
		)
		return httpapi.ErrAuthRateLimited()
	}
	if h.totp == nil {
		// TOTP feature disabled server-side; nothing should ever produce a ticket.
		return httpapi.ErrAuthInvalid("otp login disabled")
	}

	var req dto.OTPLoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		log.WarnContext(ctx, "auth: login/otp invalid body",
			slog.String("op", opAuthLoginOTP),
			slog.String("err", err.Error()),
		)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if strings.TrimSpace(req.Ticket) == "" {
		return httpapi.ErrValidation("ticket is required")
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return httpapi.ErrValidation("code is required")
	}

	ticket, err := h.jwt.VerifyOTPTicket(req.Ticket)
	if err != nil {
		log.WarnContext(ctx, "auth: login/otp invalid ticket",
			slog.String("op", opAuthLoginOTP),
			slog.String("reason", err.Error()),
		)
		return httpapi.ErrTOTPTicketInvalid()
	}
	clientKind := model.ClientKind(ticket.ClientKind)
	if !clientKind.IsValid() {
		log.WarnContext(ctx, "auth: login/otp invalid client kind in ticket",
			slog.String("op", opAuthLoginOTP),
			slog.String("client_kind", ticket.ClientKind),
		)
		return httpapi.ErrTOTPTicketInvalid()
	}

	// Load the user before consuming any code. A failure here (e.g. row
	// missing, transient DB error) would otherwise burn the recovery code
	// without completing login — fine on average, but catastrophic if the
	// user is on their last recovery code with no working TOTP device.
	user, err := h.users.Get(ctx, ticket.UserID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrTOTPTicketInvalid()
		}
		return httpapi.ErrInternal("lookup user").WithCause(err)
	}

	verr := h.totp.Verify(ctx, ticket.UserID, code)
	if verr != nil {
		if !errors.Is(verr, totp.ErrInvalidCode) {
			if errors.Is(verr, totp.ErrNotEnabled) {
				log.WarnContext(ctx, "auth: login/otp user not enrolled",
					slog.String("op", opAuthLoginOTP),
					slog.Int64("user_id", ticket.UserID),
				)
				return httpapi.ErrTOTPTicketInvalid()
			}
			return httpapi.ErrInternal("verify otp").WithCause(verr)
		}
		if rerr := h.totp.ConsumeRecoveryCode(ctx, ticket.UserID, code); rerr != nil {
			if errors.Is(rerr, totp.ErrInvalidCode) || errors.Is(rerr, totp.ErrNotEnabled) {
				log.WarnContext(ctx, "auth: login/otp invalid code",
					slog.String("op", opAuthLoginOTP),
					slog.Int64("user_id", ticket.UserID),
				)
				return httpapi.ErrTOTPInvalidCode()
			}
			return httpapi.ErrInternal("consume recovery code").WithCause(rerr)
		}
		log.InfoContext(ctx, "auth: login/otp recovery code used",
			slog.String("op", opAuthLoginOTP),
			slog.Int64("user_id", ticket.UserID),
		)
	}
	log.InfoContext(ctx, "auth: login/otp ok",
		slog.String("op", opAuthLoginOTP),
		slog.Int64("user_id", user.ID),
		slog.String("client_kind", string(clientKind)),
	)
	return h.issueSession(c, user, clientKind)
}

func (h *AuthHandler) refresh(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	if !h.limiter.Allow(c.IP()) {
		log.WarnContext(ctx, "auth: refresh rate limited",
			slog.String("op", opAuthRefresh),
			slog.String("ip", c.IP()),
		)
		return httpapi.ErrAuthRateLimited()
	}

	// Cookie-first, then body.
	token := c.Cookies(refreshCookieName)
	if token == "" {
		var req dto.RefreshRequest
		if err := c.Bind().JSON(&req); err == nil {
			token = req.Refresh
		}
	}
	if token == "" {
		log.WarnContext(ctx, "auth: refresh missing token",
			slog.String("op", opAuthRefresh),
		)
		return httpapi.ErrAuthInvalid("missing refresh token")
	}

	tokenHash := auth.HashRefreshToken(token)

	// Theft detection: old hash arriving after rotation → revoke session.
	// After Rotate the old hash is no longer in DB, so we look up the session ID
	// from the theft cache (recorded at rotation time) and revoke it directly.
	if sid, ok := h.theft.wasRotated(tokenHash); ok {
		log.WarnContext(ctx, "auth: refresh token reuse",
			slog.String("op", opAuthRefresh),
			slog.Int64("session_id", sid),
		)
		if err := h.sessions.Revoke(ctx, sid); err != nil && !errors.Is(err, repo.ErrNotFound) {
			log.ErrorContext(ctx, "auth: refresh reuse revoke failed",
				slog.String("op", opAuthRefresh),
				slog.Int64("session_id", sid),
				slog.String("err", err.Error()),
			)
		}
		return httpapi.ErrAuthInvalid("refresh token reuse detected")
	}

	session, err := h.sessions.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrInternal("lookup session").WithCause(err)
		}
		log.WarnContext(ctx, "auth: refresh token unknown",
			slog.String("op", opAuthRefresh),
		)
		return httpapi.ErrAuthInvalid("invalid or expired refresh token")
	}
	if !session.IsActive(time.Now()) {
		log.WarnContext(ctx, "auth: refresh token revoked or expired",
			slog.String("op", opAuthRefresh),
			slog.Int64("session_id", session.ID),
			slog.Int64("user_id", session.UserID),
		)
		return httpapi.ErrAuthInvalid("refresh token revoked or expired")
	}

	newToken, newHash, err := auth.GenerateRefreshToken()
	if err != nil {
		return httpapi.ErrInternal("generate refresh token").WithCause(err)
	}
	newExp := auth.RefreshExpiry(time.Now())
	if err := h.sessions.Rotate(ctx, session.ID, newHash, newExp); err != nil {
		return httpapi.ErrInternal("rotate session").WithCause(err)
	}

	// Mark old hash as rotated for theft detection window.
	h.theft.record(tokenHash, session.ID)

	access, _, err := h.jwt.Issue(session.UserID, session.ID)
	if err != nil {
		return httpapi.ErrInternal("issue access token").WithCause(err)
	}

	if session.ClientKind == model.ClientWeb {
		setRefreshCookie(c, newToken)
	}

	// Embed the user so a booting client does not have to follow up with
	// GET /auth/me: that was a serial round-trip on the critical path, and this
	// is the same indexed PK read /auth/me performs.
	user, err := h.users.Get(ctx, session.UserID)
	if err != nil {
		return httpapi.ErrInternal("get user").WithCause(err)
	}

	log.InfoContext(ctx, "auth: refresh ok",
		slog.String("op", opAuthRefresh),
		slog.Int64("user_id", session.UserID),
		slog.Int64("session_id", session.ID),
		slog.String("client_kind", string(session.ClientKind)),
	)
	return c.JSON(dto.RefreshResponse{
		Access:  access,
		Refresh: newToken,
		User:    dto.UserDTO{ID: user.ID, Username: user.Username, TOTPEnabled: user.TOTPEnabled},
	})
}

func (h *AuthHandler) logout(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	claims := httpapi.GetClaims(c)
	if claims == nil {
		log.WarnContext(ctx, "auth: logout missing claims",
			slog.String("op", "handler.Auth.Logout"),
		)
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	if err := h.sessions.Revoke(ctx, claims.SessionID); err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrInternal("revoke session").WithCause(err)
		}
	}
	clearRefreshCookie(c)
	log.InfoContext(ctx, "auth: logout ok",
		slog.String("op", "handler.Auth.Logout"),
		slog.Int64("user_id", claims.UserID),
		slog.Int64("session_id", claims.SessionID),
	)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AuthHandler) logoutAll(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	claims := httpapi.GetClaims(c)
	if claims == nil {
		log.WarnContext(ctx, "auth: logoutAll missing claims",
			slog.String("op", "handler.Auth.LogoutAll"),
		)
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	if err := h.sessions.RevokeAllForUser(ctx, claims.UserID); err != nil {
		return httpapi.ErrInternal("revoke all sessions").WithCause(err)
	}
	clearRefreshCookie(c)
	log.InfoContext(ctx, "auth: logoutAll ok",
		slog.String("op", "handler.Auth.LogoutAll"),
		slog.Int64("user_id", claims.UserID),
	)
	return c.SendStatus(fiber.StatusNoContent)
}

// logoutOthers revokes every active session for the user except the one that
// issued the current request. Lets a user invalidate forgotten/shared-device
// sessions without losing their own.
func (h *AuthHandler) logoutOthers(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	claims := httpapi.GetClaims(c)
	if claims == nil {
		log.WarnContext(ctx, "auth: logoutOthers missing claims",
			slog.String("op", "handler.Auth.LogoutOthers"),
		)
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	if err := h.sessions.RevokeAllForUserExcept(ctx, claims.UserID, claims.SessionID); err != nil {
		return httpapi.ErrInternal("revoke other sessions").WithCause(err)
	}
	log.InfoContext(ctx, "auth: logoutOthers ok",
		slog.String("op", "handler.Auth.LogoutOthers"),
		slog.Int64("user_id", claims.UserID),
		slog.Int64("session_id", claims.SessionID),
	)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AuthHandler) me(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	user, err := h.users.Get(c.Context(), claims.UserID)
	if err != nil {
		return httpapi.ErrInternal("get user").WithCause(err)
	}
	return c.JSON(fiber.Map{"user": dto.UserDTO{ID: user.ID, Username: user.Username, TOTPEnabled: user.TOTPEnabled}})
}

// issueSession creates a session, enforces the per-client limit, issues tokens,
// and sets the refresh cookie for web clients.
func (h *AuthHandler) issueSession(c fiber.Ctx, user *model.User, kind model.ClientKind) error {
	token, tokenHash, err := auth.GenerateRefreshToken()
	if err != nil {
		return httpapi.ErrInternal("generate refresh token").WithCause(err)
	}

	session, err := h.sessions.Create(c.Context(), repo.CreateSessionParams{
		UserID:     user.ID,
		TokenHash:  tokenHash,
		ClientKind: kind,
		UserAgent:  truncateString(c.Get("User-Agent"), 512),
		IPAddress:  truncateString(c.IP(), 64),
		ExpiresAt:  auth.RefreshExpiry(time.Now()),
	})
	if err != nil {
		return httpapi.ErrInternal("create session").WithCause(err)
	}

	if err := h.sessions.EnforceLimit(c.Context(), user.ID, kind, sessionLimit); err != nil {
		ctx := c.Context()
		if rerr := h.sessions.Revoke(ctx, session.ID); rerr != nil && !errors.Is(rerr, repo.ErrNotFound) {
			logging.FromContext(ctx).ErrorContext(ctx,
				"auth: rollback session after enforce-limit failed",
				slog.String("op", "handler.Auth.issueSession"),
				slog.Int64("session_id", session.ID),
				slog.String("err", rerr.Error()),
			)
		}
		return httpapi.ErrInternal("enforce session limit").WithCause(err)
	}

	access, _, err := h.jwt.Issue(user.ID, session.ID)
	if err != nil {
		return httpapi.ErrInternal("issue access token").WithCause(err)
	}

	if kind == model.ClientWeb {
		setRefreshCookie(c, token)
	}

	return c.JSON(dto.AuthResponse{
		Access:  access,
		Refresh: token,
		User:    dto.UserDTO{ID: user.ID, Username: user.Username, TOTPEnabled: user.TOTPEnabled},
	})
}

func validateLoginRequest(req dto.LoginRequest) *httpapi.AppError {
	if req.Username == "" {
		return httpapi.ErrValidation("username is required")
	}
	if req.Password == "" {
		return httpapi.ErrValidation("password is required")
	}
	if !req.ClientKind.IsValid() {
		return httpapi.ErrValidation("clientKind must be web, ios, android, or cli")
	}
	return nil
}

func setRefreshCookie(c fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
		MaxAge:   refreshCookieMaxAge,
	})
}

func clearRefreshCookie(c fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
		MaxAge:   -1,
	})
}

// theftCache stores recently rotated token hashes for 1 minute to detect reuse.
// The session ID is captured at rotation time so theft detection can revoke
// the session even though the old hash is no longer in the DB.
type theftCacheEntry struct {
	sessionID int64
	expires   time.Time
}

type theftCache struct {
	mu       sync.Mutex
	entries  map[string]theftCacheEntry
	stopCh   chan struct{}
	stopOnce sync.Once
}

func newTheftCache() *theftCache {
	tc := &theftCache{entries: make(map[string]theftCacheEntry), stopCh: make(chan struct{})}
	go tc.gc()
	return tc
}

func (tc *theftCache) stop() {
	tc.stopOnce.Do(func() { close(tc.stopCh) })
}

func (tc *theftCache) record(hash string, sessionID int64) {
	tc.mu.Lock()
	tc.entries[hash] = theftCacheEntry{sessionID: sessionID, expires: time.Now().Add(time.Minute)}
	tc.mu.Unlock()
}

func (tc *theftCache) wasRotated(hash string) (int64, bool) {
	tc.mu.Lock()
	e, ok := tc.entries[hash]
	tc.mu.Unlock()
	if !ok || !time.Now().Before(e.expires) {
		return 0, false
	}
	return e.sessionID, true
}

func (tc *theftCache) gc() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-tc.stopCh:
			return
		case <-t.C:
			tc.mu.Lock()
			now := time.Now()
			for k, e := range tc.entries {
				if now.After(e.expires) {
					delete(tc.entries, k)
				}
			}
			tc.mu.Unlock()
		}
	}
}

func truncateString(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}
