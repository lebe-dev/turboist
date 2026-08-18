package dto

import (
	"encoding/json"

	"github.com/lebe-dev/turboist/internal/model"
)

// PasskeyDTO is one registered passkey as shown in settings. The credential
// blob itself is never exposed — it is of no use to the client.
type PasskeyDTO struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt *string `json:"lastUsedAt"`
}

func PasskeyFromModel(c model.PasskeyCredential) PasskeyDTO {
	out := PasskeyDTO{
		ID:        c.ID,
		Name:      c.Name,
		CreatedAt: model.FormatUTC(c.CreatedAt),
	}
	if c.LastUsedAt != nil {
		s := model.FormatUTC(*c.LastUsedAt)
		out.LastUsedAt = &s
	}
	return out
}

// PasskeyCeremonyResponse carries the WebAuthn options straight through to
// navigator.credentials.create()/get(). Options is the library's own
// serialisation ({"publicKey": {...}}), passed through verbatim so no field is
// lost in a re-shaping layer. CeremonyID addresses the server-side challenge.
type PasskeyCeremonyResponse struct {
	CeremonyID string `json:"ceremonyId"`
	Options    any    `json:"options"`
}

// PasskeyRegisterFinishRequest completes an enrollment. Credential is the raw
// PublicKeyCredential JSON the browser produced.
type PasskeyRegisterFinishRequest struct {
	CeremonyID string          `json:"ceremonyId"`
	Name       string          `json:"name"`
	Credential json.RawMessage `json:"credential"`
}

// PasskeyLoginBeginRequest starts a discoverable login. No username: the
// authenticator reveals which account it holds.
type PasskeyLoginBeginRequest struct {
	ClientKind model.ClientKind `json:"clientKind"`
}

// PasskeyLoginFinishRequest completes a discoverable login.
type PasskeyLoginFinishRequest struct {
	CeremonyID string           `json:"ceremonyId"`
	ClientKind model.ClientKind `json:"clientKind"`
	Credential json.RawMessage  `json:"credential"`
}

// PasskeyRenameRequest relabels a stored credential.
type PasskeyRenameRequest struct {
	Name string `json:"name"`
}
