package handlers_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/handshake"
	"github.com/lebe-dev/turboist/internal/federation/nonce"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// fedRegistry is the shared in-process directory of federation instances so they
// can resolve each other's .well-known and route signed requests through
// app.Test() (no real network — the REAL signature middleware still runs).
type fedRegistry struct {
	apps map[string]*fiber.App
}

func newFedRegistry() *fedRegistry { return &fedRegistry{apps: map[string]*fiber.App{}} }

// fetcher resolves a peer's .well-known in-process. It is the independent,
// out-of-band fetch the owner-key validation (US-2.2 AC2) and the signature
// middleware both use.
func (r *fedRegistry) fetcher() peerkeys.Fetcher {
	return func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		target := r.apps[instanceURL]
		if target == nil {
			return nil, errPeerUnknown
		}
		req := httptest.NewRequest(http.MethodGet, instanceURL+peerkeys.WellKnownPath, http.NoBody)
		resp, err := target.Test(req, fiber.TestConfig{Timeout: -1})
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		var doc struct {
			PublicKey   string `json:"public_key"`
			DisplayName string `json:"display_name"`
		}
		body, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(body, &doc); err != nil {
			return nil, err
		}
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: doc.PublicKey, DisplayName: doc.DisplayName}, nil
	}
}

// Send routes a signed request to the registered app whose URL prefixes the
// request URL. Satisfies fedsvc.HandshakeSender.
func (r *fedRegistry) Send(_ context.Context, sr fedsvc.SignedRequest) (*fedsvc.SignedResponse, error) {
	for url, app := range r.apps {
		if has(sr.URL, url) {
			req := httptest.NewRequest(sr.Method, sr.URL, bytes.NewReader(sr.Body))
			for k, v := range sr.Headers {
				req.Header.Set(k, v)
			}
			resp, err := app.Test(req, fiber.TestConfig{Timeout: -1})
			if err != nil {
				return nil, err
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)
			return &fedsvc.SignedResponse{StatusCode: resp.StatusCode, Body: body}, nil
		}
	}
	return nil, errPeerUnknown
}

func has(s, prefix string) bool { return len(s) >= len(prefix) && s[:len(prefix)] == prefix }

var errPeerUnknown = peerUnknownError{}

type peerUnknownError struct{}

func (peerUnknownError) Error() string { return "peer unknown" }

// fedInstance is one in-process federation instance (owner or joiner).
type fedInstance struct {
	app          *fiber.App
	db           *sql.DB
	url          string
	jwt          string
	svc          *fedsvc.Service
	fedProjects  *repo.FederatedProjectRepo
	fedInstances *repo.FederatedInstanceRepo
	invites      *repo.FederationInviteRepo
	projects     *repo.ProjectRepo
	contexts     *repo.ContextRepo
	keys         *repo.FederationKeysRepo
	tasks        *repo.TaskRepo
	sections     *repo.ProjectSectionRepo
}

// fedInstanceOption tunes an instance built by newFedInstance. Used by the F7.7
// handshake-rate-limit test to wire a limiter onto the owner's handshake endpoint;
// nil/no options preserve the default (unthrottled) wiring every other test uses.
type fedInstanceOption func(*fedInstanceConfig)

type fedInstanceConfig struct {
	handshakeLimiter handlers.FederationRateLimiter
	log              *slog.Logger
}

// withHandshakeLimiter wires a per-peer handshake rate limiter onto the instance's
// signed handshake endpoint (Federation v1 F7.7, NFR-3).
func withHandshakeLimiter(l handlers.FederationRateLimiter) fedInstanceOption {
	return func(c *fedInstanceConfig) { c.handshakeLimiter = l }
}

// withLogger injects a logger so a test can capture every record the instance's
// handlers emit (Federation v1 F7.7 no-secret-in-logs scan).
func withLogger(l *slog.Logger) fedInstanceOption {
	return func(c *fedInstanceConfig) { c.log = l }
}

// newFedInstance builds one federation-enabled instance with the public
// .well-known, the JWT admin (enable/invite/join/preview), and the signed
// handshake group all mounted, registering it in reg so other instances can
// reach it.
func newFedInstance(t *testing.T, reg *fedRegistry, url string, opts ...fedInstanceOption) *fedInstance {
	t.Helper()
	var icfg fedInstanceConfig
	for _, opt := range opts {
		opt(&icfg)
	}
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "fed.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	issuer := auth.NewJWTIssuer([]byte("test-secret-key-32-bytes-padding!"))
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	apiTokens := repo.NewAPITokenRepo(d)
	salt := []byte("test-api-token-salt-32-bytes-pad!")
	plabels := repo.NewProjectLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	contexts := repo.NewContextRepo(d)

	cipher := crypto.NewTokenCipher(fedHandlerKey)
	fedKeys := repo.NewFederationKeysRepo(d)
	fedProjects := repo.NewFederatedProjectRepo(d)
	fedInvites := repo.NewFederationInviteRepo(d)
	fedInstances := repo.NewFederatedInstanceRepo(d)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	sections := repo.NewProjectSectionRepo(d)
	fedSnapshot := repo.NewFederationSnapshotRepo(d)

	deps := httpapi.Deps{JWTIssuer: issuer, UserRepo: users, APITokenRepo: apiTokens, APITokenSalt: salt, Log: icfg.log}
	app := httpapi.NewApp(deps)
	reg.apps[url] = app

	fh := handlers.NewFederationHandler(fedKeys, cipher, url)
	fh.RegisterPublic(app)

	svc := fedsvc.NewService(d, projects, fedProjects, fedKeys, fedInvites, fedInstances, cipher, url)
	peerCache := peerkeys.NewCache(reg.fetcher())
	svc.WithJoinDeps(reg, reg.fetcher(), peerCache, nil)
	// Snapshot bootstrap deps (Federation v1 F2.3): the owner streams snapshots,
	// the joiner applies them into a new local project.
	svc.WithSnapshotDeps(tasks, sections, contexts, fedSnapshot)
	fh.WithService(svc)
	if icfg.handshakeLimiter != nil {
		fh.WithHandshakeRateLimiter(icfg.handshakeLimiter)
	}

	api := httpapi.RegisterRoutes(app, deps)
	handlers.NewFederationAdminHandler(svc).Register(api.Group("", httpapi.RequireJWTAuth()))

	signed := app.Group("/federation", httpapi.HTTPSignatureMiddleware(httpapi.FederationSignatureDeps{
		Nonces:   nonce.NewCache(),
		PeerKeys: peerCache,
	}))
	fh.RegisterSigned(signed)

	tok, _, err := issuer.Issue(1, 1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	return &fedInstance{
		app: app, db: d, url: url, jwt: tok, svc: svc,
		fedProjects: fedProjects, fedInstances: fedInstances, invites: fedInvites,
		projects: projects, contexts: contexts, keys: fedKeys,
		tasks: tasks, sections: sections,
	}
}

// enableAndInvite creates a project on this (owner) instance, enables federation,
// and mints a single-use invite of the given grade.
func (e *fedInstance) enableAndInvite(t *testing.T, perm model.FederationPermission) (fedsvc.ParsedInvite, int64) {
	t.Helper()
	ctx := context.Background()
	cx, err := e.contexts.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := e.projects.Create(ctx, repo.CreateProject{ContextID: cx.ID, Title: "Roadmap", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := e.svc.EnableForProject(ctx, p.ID); err != nil {
		t.Fatalf("enable federation: %v", err)
	}
	res, err := e.svc.CreateInvite(ctx, p.ID, fedsvc.CreateInviteParams{Permissions: perm})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	return fedsvc.ParsedInvite{InviteID: res.InviteID, Secret: res.Secret}, p.ID
}

// join posts an Accept (join) to this instance's JWT join endpoint, targeting
// ownerURL with the parsed invite.
func (e *fedInstance) join(t *testing.T, ownerURL string, inv fedsvc.ParsedInvite) (*http.Response, []byte) {
	t.Helper()
	body, _ := json.Marshal(dto.JoinInviteRequest{
		InviteID:         inv.InviteID,
		Secret:           inv.Secret,
		OwnerInstanceURL: ownerURL,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.jwt)
	resp, err := e.app.Test(req, fiber.TestConfig{Timeout: -1})
	if err != nil {
		t.Fatalf("join test: %v", err)
	}
	out, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, out
}

// enableAndInviteOn mints another invite on an already-federated project.
func (e *fedInstance) enableAndInviteOn(t *testing.T, projectID int64, perm model.FederationPermission) (fedsvc.ParsedInvite, int64) {
	t.Helper()
	res, err := e.svc.CreateInvite(context.Background(), projectID, fedsvc.CreateInviteParams{Permissions: perm})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	return fedsvc.ParsedInvite{InviteID: res.InviteID, Secret: res.Secret}, projectID
}

// handshakeDirect drives the owner-side handshake service directly with an
// explicit advertised version set. It fabricates a syntactically-valid joiner
// identity (body key == verified key) so the only failing dimension is the
// version negotiation — the deterministic way to exercise the no-overlap branch.
func (e *fedInstance) handshakeDirect(t *testing.T, inv fedsvc.ParsedInvite, versions []int) (*handshakeResult, error) {
	t.Helper()
	const dummyKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	resp, err := e.svc.Handshake(context.Background(), fedsvc.HandshakeInput{
		Body:            handshakeBody(inv, versions),
		VerifiedPeerURL: "https://probe.example",
		VerifiedPeerKey: dummyKey,
	}, timeNowUTC())
	if err != nil {
		return nil, err
	}
	return &handshakeResult{ProjectID: resp.ProjectID}, nil
}

type handshakeResult struct{ ProjectID int64 }

// handshakeBody builds a handshake.Request for the direct (no-network) path.
func handshakeBody(inv fedsvc.ParsedInvite, versions []int) handshake.Request {
	return handshake.Request{
		InviteID:          inv.InviteID,
		Secret:            inv.Secret,
		JoinerInstanceURL: "https://probe.example",
		JoinerPublicKey:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		JoinerDisplayName: "probe.example",
		ProtocolVersions:  versions,
	}
}

func timeNowUTC() time.Time { return time.Now().UTC() }

// lyingFetcher returns a .well-known whose public key never matches any real
// owner key, so the joiner's owner-key corroboration (US-2.2 AC2) fails.
func lyingFetcher() peerkeys.Fetcher {
	return func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{
			InstanceURL: instanceURL,
			PublicKey:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			DisplayName: "liar",
		}, nil
	}
}

func errCodeOf(t *testing.T, body []byte) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode error envelope: %v (%s)", err, body)
	}
	return env.Error.Code
}
