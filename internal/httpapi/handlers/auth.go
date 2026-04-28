package handlers

import (
	"errors"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

const (
	sessionLimit        = 5
	refreshCookieName   = "refresh"
	refreshCookiePath   = "/auth/refresh"
	refreshCookieMaxAge = 30 * 24 * 60 * 60 // 30 days in seconds
)

// AuthHandler implements all /auth/* endpoints.
type AuthHandler struct {
	users    *repo.UserRepo
	sessions *repo.SessionRepo
	jwt      *auth.JWTIssuer
	limiter  *auth.IPLimiter
	theft    *theftCache
}

// NewAuthHandler constructs an AuthHandler.
func NewAuthHandler(
	users *repo.UserRepo,
	sessions *repo.SessionRepo,
	jwt *auth.JWTIssuer,
	limiter *auth.IPLimiter,
) *AuthHandler {
	return &AuthHandler{
		users:    users,
		sessions: sessions,
		jwt:      jwt,
		limiter:  limiter,
		theft:    newTheftCache(),
	}
}

// RegisterAuth wires /auth routes onto r. Protected routes (logout, me) use jwtIssuer middleware.
func (h *AuthHandler) RegisterAuth(r fiber.Router, jwtIssuer *auth.JWTIssuer) {
	r.Get("/setup-required", h.setupRequired)
	r.Post("/setup", h.setup)
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
	r.Post("/logout", httpapi.AuthMiddleware(jwtIssuer), h.logout)
	r.Post("/logout-all", httpapi.AuthMiddleware(jwtIssuer), h.logoutAll)
	r.Get("/me", httpapi.AuthMiddleware(jwtIssuer), h.me)
}

func (h *AuthHandler) setupRequired(c fiber.Ctx) error {
	exists, err := h.users.Exists(c.Context())
	if err != nil {
		return httpapi.ErrInternal("check user existence")
	}
	return c.JSON(fiber.Map{"required": !exists})
}

func (h *AuthHandler) setup(c fiber.Ctx) error {
	if !h.limiter.Allow(c.IP()) {
		return httpapi.ErrAuthRateLimited()
	}

	exists, err := h.users.Exists(c.Context())
	if err != nil {
		return httpapi.ErrInternal("check user existence")
	}
	if exists {
		return httpapi.ErrSetupAlreadyDone()
	}

	var req dto.LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if err := validateLoginRequest(req); err != nil {
		return err
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return httpapi.ErrInternal("hash password")
	}
	user, err := h.users.Create(c.Context(), req.Username, hash)
	if err != nil {
		return httpapi.ErrInternal("create user")
	}

	return h.issueSession(c, user, req.ClientKind)
}

func (h *AuthHandler) login(c fiber.Ctx) error {
	if !h.limiter.Allow(c.IP()) {
		return httpapi.ErrAuthRateLimited()
	}

	var req dto.LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if err := validateLoginRequest(req); err != nil {
		return err
	}

	user, err := h.users.GetByUsername(c.Context(), req.Username)
	if err != nil {
		// Avoid username enumeration: return same error for not found vs wrong password.
		return httpapi.ErrAuthInvalid("invalid credentials")
	}
	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		return httpapi.ErrAuthInvalid("invalid credentials")
	}

	return h.issueSession(c, user, req.ClientKind)
}

func (h *AuthHandler) refresh(c fiber.Ctx) error {
	// Cookie-first, then body.
	token := c.Cookies(refreshCookieName)
	if token == "" {
		var req dto.RefreshRequest
		if err := c.Bind().JSON(&req); err == nil {
			token = req.Refresh
		}
	}
	if token == "" {
		return httpapi.ErrAuthInvalid("missing refresh token")
	}

	tokenHash := auth.HashRefreshToken(token)

	// Theft detection: old hash arriving after rotation → revoke session.
	// After Rotate the old hash is no longer in DB, so we look up the session ID
	// from the theft cache (recorded at rotation time) and revoke it directly.
	if sid, ok := h.theft.wasRotated(tokenHash); ok {
		_ = h.sessions.Revoke(c.Context(), sid)
		return httpapi.ErrAuthInvalid("refresh token reuse detected")
	}

	session, err := h.sessions.GetByTokenHash(c.Context(), tokenHash)
	if err != nil {
		return httpapi.ErrAuthInvalid("invalid or expired refresh token")
	}
	if !session.IsActive(time.Now()) {
		return httpapi.ErrAuthInvalid("refresh token revoked or expired")
	}

	newToken, newHash, err := auth.GenerateRefreshToken()
	if err != nil {
		return httpapi.ErrInternal("generate refresh token")
	}
	newExp := auth.RefreshExpiry(time.Now())
	if err := h.sessions.Rotate(c.Context(), session.ID, newHash, newExp); err != nil {
		return httpapi.ErrInternal("rotate session")
	}

	// Mark old hash as rotated for theft detection window.
	h.theft.record(tokenHash, session.ID)

	access, _, err := h.jwt.Issue(session.UserID, session.ID)
	if err != nil {
		return httpapi.ErrInternal("issue access token")
	}

	if session.ClientKind == model.ClientWeb {
		setRefreshCookie(c, newToken)
	}

	return c.JSON(dto.RefreshResponse{Access: access, Refresh: newToken})
}

func (h *AuthHandler) logout(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	if err := h.sessions.Revoke(c.Context(), claims.SessionID); err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrInternal("revoke session")
		}
	}
	clearRefreshCookie(c)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AuthHandler) logoutAll(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	if err := h.sessions.RevokeAllForUser(c.Context(), claims.UserID); err != nil {
		return httpapi.ErrInternal("revoke all sessions")
	}
	clearRefreshCookie(c)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AuthHandler) me(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	user, err := h.users.Get(c.Context(), claims.UserID)
	if err != nil {
		return httpapi.ErrInternal("get user")
	}
	return c.JSON(fiber.Map{"user": dto.UserDTO{ID: user.ID, Username: user.Username}})
}

// issueSession creates a session, enforces the per-client limit, issues tokens,
// and sets the refresh cookie for web clients.
func (h *AuthHandler) issueSession(c fiber.Ctx, user *model.User, kind model.ClientKind) error {
	token, tokenHash, err := auth.GenerateRefreshToken()
	if err != nil {
		return httpapi.ErrInternal("generate refresh token")
	}

	session, err := h.sessions.Create(c.Context(), repo.CreateSessionParams{
		UserID:     user.ID,
		TokenHash:  tokenHash,
		ClientKind: kind,
		UserAgent:  c.Get("User-Agent"),
		ExpiresAt:  auth.RefreshExpiry(time.Now()),
	})
	if err != nil {
		return httpapi.ErrInternal("create session")
	}

	if err := h.sessions.EnforceLimit(c.Context(), user.ID, kind, sessionLimit); err != nil {
		return httpapi.ErrInternal("enforce session limit")
	}

	access, _, err := h.jwt.Issue(user.ID, session.ID)
	if err != nil {
		return httpapi.ErrInternal("issue access token")
	}

	if kind == model.ClientWeb {
		setRefreshCookie(c, token)
	}

	return c.JSON(dto.AuthResponse{
		Access:  access,
		Refresh: token,
		User:    dto.UserDTO{ID: user.ID, Username: user.Username},
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
		return httpapi.ErrValidation("clientKind must be web, ios, or cli")
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
	mu      sync.Mutex
	entries map[string]theftCacheEntry
}

func newTheftCache() *theftCache {
	tc := &theftCache{entries: make(map[string]theftCacheEntry)}
	go tc.gc()
	return tc
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
	for range t.C {
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
