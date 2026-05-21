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

type TOTPSetupResponse struct {
	Secret      string `json:"secret"`
	OtpauthURL  string `json:"otpauthUrl"`
	QRPngBase64 string `json:"qrPngBase64"`
}

type TOTPCodeRequest struct {
	Code string `json:"code"`
}

type TOTPConfirmResponse struct {
	RecoveryCodes []string `json:"recoveryCodes"`
}
