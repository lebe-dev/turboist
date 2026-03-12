package handler

import (
	"crypto/subtle"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
)

const cookieName = "turboist_token"

type AuthHandler struct {
	store    *auth.SessionStore
	password string
	dev      bool
}

func NewAuthHandler(store *auth.SessionStore, password string, dev bool) *AuthHandler {
	return &AuthHandler{store: store, password: password, dev: dev}
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	var body struct {
		Password string `json:"password"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(h.password)) != 1 {
		log.Warn("login: invalid password attempt")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid password"})
	}

	token, err := h.store.CreateSession()
	if err != nil {
		log.Error("login: create session", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	log.Debug("login: session created")

	c.Cookie(&fiber.Cookie{
		Name:     cookieName,
		Value:    token,
		HTTPOnly: true,
		SameSite: "Lax",
		Secure:   !h.dev,
		Path:     "/",
	})

	return c.SendStatus(fiber.StatusOK)
}

func (h *AuthHandler) Logout(c fiber.Ctx) error {
	token := c.Cookies(cookieName)
	if token != "" {
		h.store.DeleteSession(token)
		log.Debug("logout: session deleted")
	}

	c.Cookie(&fiber.Cookie{
		Name:    cookieName,
		Value:   "",
		Expires: time.Unix(0, 0),
		Path:    "/",
	})

	return c.SendStatus(fiber.StatusOK)
}

func (h *AuthHandler) Me(c fiber.Ctx) error {
	return c.SendStatus(fiber.StatusOK)
}
