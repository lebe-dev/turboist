package dto

import "github.com/lebe-dev/turboist/internal/model"

type LoginRequest struct {
	Username   string           `json:"username"`
	Password   string           `json:"password"`
	ClientKind model.ClientKind `json:"clientKind"`
}

type RefreshRequest struct {
	Refresh string `json:"refresh"`
}

type UserDTO struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type AuthResponse struct {
	Access  string  `json:"access"`
	Refresh string  `json:"refresh"`
	User    UserDTO `json:"user"`
}

type RefreshResponse struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
}
