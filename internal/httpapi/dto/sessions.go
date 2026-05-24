package dto

import (
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/model"
)

// SessionDTO is the wire shape of a user session returned by /api/v1/sessions.
// userAgent keeps the raw header value so the UI can show it in a tooltip;
// displayName is the parsed "Chrome on macOS" form for the main label.
type SessionDTO struct {
	ID          int64  `json:"id"`
	ClientKind  string `json:"clientKind"`
	UserAgent   string `json:"userAgent"`
	DisplayName string `json:"displayName"`
	IPAddress   string `json:"ipAddress"`
	CreatedAt   string `json:"createdAt"`
	LastUsedAt  string `json:"lastUsedAt"`
	IsCurrent   bool   `json:"isCurrent"`
}

func SessionFromModel(s model.Session, currentSessionID int64) SessionDTO {
	return SessionDTO{
		ID:          s.ID,
		ClientKind:  string(s.ClientKind),
		UserAgent:   s.UserAgent,
		DisplayName: auth.ParseUserAgent(s.UserAgent).DisplayName(),
		IPAddress:   s.IPAddress,
		CreatedAt:   model.FormatUTC(s.CreatedAt),
		LastUsedAt:  model.FormatUTC(s.LastUsedAt),
		IsCurrent:   s.ID == currentSessionID,
	}
}
