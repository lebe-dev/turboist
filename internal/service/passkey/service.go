package passkey

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// MaxCredentialsPerUser caps how many passkeys one account may register. High
// enough for phone + laptop + a couple of security keys, low enough that a
// scripted registration loop cannot grow the table without bound.
const MaxCredentialsPerUser = 20

// maxNameLength bounds the user-facing label of a credential.
const maxNameLength = 100

// defaultCredentialName labels a passkey the client did not name.
const defaultCredentialName = "Passkey"

// Errors returned by the Service. Handlers map these onto HTTP codes.
var (
	ErrCeremonyExpired = errors.New("passkey: ceremony expired or unknown")
	ErrInvalidResponse = errors.New("passkey: invalid authenticator response")
	ErrAlreadyExists   = errors.New("passkey: credential already registered")
	ErrLimitReached    = errors.New("passkey: credential limit reached")
	ErrNotFound        = errors.New("passkey: credential not found")
)

// Config describes the Relying Party this instance authenticates for.
type Config struct {
	// RPID is the effective domain ("todo.example.com"). Credentials are bound
	// to it: changing it invalidates every registered passkey.
	RPID string
	// RPDisplayName is shown by the platform's passkey prompt.
	RPDisplayName string
	// Origins lists every origin allowed to run a ceremony — the web origin
	// plus, for the Capacitor apps, their native origins.
	Origins []string
}

// Service performs the WebAuthn ceremonies and persists credentials.
type Service struct {
	wa         *webauthn.WebAuthn
	creds      *repo.WebAuthnRepo
	users      *repo.UserRepo
	ceremonies *ceremonyStore

	now func() time.Time
}

// NewService validates cfg and constructs the Service.
func NewService(cfg Config, creds *repo.WebAuthnRepo, users *repo.UserRepo) (*Service, error) {
	displayName := cfg.RPDisplayName
	if displayName == "" {
		displayName = "Turboist"
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: displayName,
		RPOrigins:     cfg.Origins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			// Discoverable credentials are what make the login usernameless:
			// the authenticator hands back the user handle, so the login page
			// needs no field at all.
			ResidentKey: protocol.ResidentKeyRequirementRequired,
			// "preferred" rather than "required": a passkey that verified the
			// user covers both factors, but refusing an authenticator that
			// cannot do UV would lock out plain security keys for no gain —
			// the password path remains available either way.
			UserVerification: protocol.VerificationPreferred,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("passkey: config: %w", err)
	}
	return &Service{wa: wa, creds: creds, users: users, ceremonies: newCeremonyStore(), now: time.Now}, nil
}

// RegistrationBegin is the payload the client feeds to navigator.credentials.create().
type RegistrationBegin struct {
	CeremonyID string
	Options    *protocol.CredentialCreation
}

// LoginBegin is the payload the client feeds to navigator.credentials.get().
type LoginBegin struct {
	CeremonyID string
	Options    *protocol.CredentialAssertion
}

// BeginRegistration starts enrolling a new passkey for an already-authenticated
// user. Credentials the user has registered already are excluded so the
// platform offers to replace rather than silently duplicate them.
func (s *Service) BeginRegistration(ctx context.Context, userID int64) (*RegistrationBegin, error) {
	user, err := s.loadUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(user.credentials) >= MaxCredentialsPerUser {
		return nil, ErrLimitReached
	}

	options, session, err := s.wa.BeginRegistration(user,
		webauthn.WithExclusions(webauthn.Credentials(user.WebAuthnCredentials()).CredentialDescriptors()),
	)
	if err != nil {
		return nil, fmt.Errorf("passkey: begin registration: %w", err)
	}
	id, err := s.ceremonies.Put(session, userID, s.now())
	if err != nil {
		return nil, fmt.Errorf("passkey: store ceremony: %w", err)
	}
	return &RegistrationBegin{CeremonyID: id, Options: options}, nil
}

// FinishRegistration verifies the attestation and stores the credential. body is
// the raw JSON the browser produced for the create() call.
func (s *Service) FinishRegistration(ctx context.Context, userID int64, ceremonyID, name string, body []byte) (*model.PasskeyCredential, error) {
	entry, ok := s.ceremonies.Take(ceremonyID, s.now())
	if !ok || entry.userID != userID {
		return nil, ErrCeremonyExpired
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidResponse, err)
	}
	user, err := s.loadUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(user.credentials) >= MaxCredentialsPerUser {
		return nil, ErrLimitReached
	}

	credential, err := s.wa.CreateCredential(user, entry.session, parsed)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidResponse, err)
	}

	blob, err := json.Marshal(credential)
	if err != nil {
		return nil, fmt.Errorf("passkey: marshal credential: %w", err)
	}
	stored, err := s.creds.Create(ctx, repo.CreateCredentialParams{
		UserID:       userID,
		CredentialID: base64.RawURLEncoding.EncodeToString(credential.ID),
		Name:         normalizeName(name),
		Credential:   blob,
		SignCount:    credential.Authenticator.SignCount,
	})
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			return nil, ErrAlreadyExists
		}
		return nil, fmt.Errorf("passkey: store credential: %w", err)
	}
	return stored, nil
}

// BeginLogin starts a discoverable ("usernameless") assertion: the client shows
// the platform picker and the authenticator reveals which account it holds.
func (s *Service) BeginLogin(ctx context.Context) (*LoginBegin, error) {
	options, session, err := s.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, fmt.Errorf("passkey: begin login: %w", err)
	}
	id, err := s.ceremonies.Put(session, 0, s.now())
	if err != nil {
		return nil, fmt.Errorf("passkey: store ceremony: %w", err)
	}
	return &LoginBegin{CeremonyID: id, Options: options}, nil
}

// FinishLogin verifies the assertion and returns the id of the authenticated
// user. The credential record is rewritten with the fresh signature counter, so
// a cloned authenticator is caught on its next use.
func (s *Service) FinishLogin(ctx context.Context, ceremonyID string, body []byte) (int64, error) {
	entry, ok := s.ceremonies.Take(ceremonyID, s.now())
	if !ok {
		return 0, ErrCeremonyExpired
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidResponse, err)
	}

	var resolvedUserID int64
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		userID, err := s.creds.UserIDByHandle(ctx, string(userHandle))
		if err != nil {
			return nil, err
		}
		user, err := s.loadUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		resolvedUserID = userID
		return user, nil
	}

	_, credential, err := s.wa.ValidatePasskeyLogin(handler, entry.session, parsed)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidResponse, err)
	}

	blob, err := json.Marshal(credential)
	if err != nil {
		return 0, fmt.Errorf("passkey: marshal credential: %w", err)
	}
	if err := s.creds.TouchAfterLogin(ctx,
		base64.RawURLEncoding.EncodeToString(credential.ID), blob, credential.Authenticator.SignCount); err != nil {
		return 0, fmt.Errorf("passkey: update credential: %w", err)
	}
	return resolvedUserID, nil
}

// List returns the user's registered passkeys, newest first.
func (s *Service) List(ctx context.Context, userID int64) ([]model.PasskeyCredential, error) {
	return s.creds.ListByUser(ctx, userID)
}

// Rename relabels one credential.
func (s *Service) Rename(ctx context.Context, id, userID int64, name string) (*model.PasskeyCredential, error) {
	if err := s.creds.Rename(ctx, id, userID, normalizeName(name)); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.creds.Get(ctx, id, userID)
}

// Delete removes one credential. Removing the last passkey is allowed: the
// password remains a working login, so this cannot lock the account out.
func (s *Service) Delete(ctx context.Context, id, userID int64) error {
	if err := s.creds.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// AnyRegistered reports whether the instance holds at least one passkey, so the
// login page can hide the button when there is nothing to authenticate with.
func (s *Service) AnyRegistered(ctx context.Context) (bool, error) {
	n, err := s.creds.CountAll(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// webauthnUser adapts a Turboist user (plus their stored credentials) to the
// library's User interface.
type webauthnUser struct {
	handle      []byte
	username    string
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte                         { return u.handle }
func (u *webauthnUser) WebAuthnName() string                       { return u.username }
func (u *webauthnUser) WebAuthnDisplayName() string                { return u.username }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (s *Service) loadUser(ctx context.Context, userID int64) (*webauthnUser, error) {
	user, err := s.users.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("passkey: load user: %w", err)
	}
	handle, err := s.creds.EnsureHandle(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("passkey: load handle: %w", err)
	}
	rows, err := s.creds.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("passkey: load credentials: %w", err)
	}
	credentials := make([]webauthn.Credential, 0, len(rows))
	for i := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal(rows[i].Credential, &c); err != nil {
			return nil, fmt.Errorf("passkey: decode credential %d: %w", rows[i].ID, err)
		}
		credentials = append(credentials, c)
	}
	return &webauthnUser{handle: []byte(handle), username: user.Username, credentials: credentials}, nil
}

func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultCredentialName
	}
	if len([]rune(name)) > maxNameLength {
		name = string([]rune(name)[:maxNameLength])
	}
	return name
}
