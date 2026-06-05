// Package handshake defines the federation handshake wire shapes shared by the
// owner (verify + consume) and the joiner (sign + send) sides (Federation v1
// F2.2, US-2.2).
//
// The handshake is the moment two instances exchange trust: the joiner proves
// possession of an invite secret and its own Ed25519 key (the request is signed
// with the SAME pinned transport signature as every other signed request — there
// is no second signing scheme, US-2.2 AC1), and the owner returns its own key,
// display name, the negotiated protocol version, and a short-lived snapshot token
// the joiner uses to bootstrap (the snapshot itself lands in F2.3).
package handshake

// Path is the concrete owner-side handshake endpoint path. It is mounted behind
// the HTTP-signature middleware (not under /api/v1) so a joiner can reach it
// before any account exists on the owner. The joiner signs over this exact
// concrete path (R4 — concrete path, not the Fiber route template).
const Path = "/federation/handshake"

// Request is the handshake body the joiner POSTs to the owner (US-2.2 AC1). It
// is signed by the transport signature (the body digest is line 7 of the pinned
// canonical string) — so tampering with any field invalidates the signature.
//
// JoinerPublicKey is carried in the body AND used to sign the request; the owner
// enforces that the body key equals the middleware-verified transport key
// (defense-in-depth against a body/header split). ProtocolVersions is the
// joiner's advertised supported-version set, intersected with the owner's at
// handshake time (US-9.1 AC1).
type Request struct {
	InviteID          string `json:"invite_id"`
	Secret            string `json:"secret"`
	JoinerInstanceURL string `json:"joiner_instance_url"`
	JoinerPublicKey   string `json:"joiner_public_key"`
	JoinerDisplayName string `json:"joiner_display_name"`
	ProtocolVersions  []int  `json:"protocol_versions"`
}

// Response is what the owner returns on a successful handshake (US-2.2 AC2/AC3).
// The joiner validates OwnerPublicKey against an independent .well-known fetch
// (US-2.2 AC2) before trusting it, persists the owner identity, warms its
// peer-key cache (US-2.2 AC6), and stores SnapshotToken for the F2.3 bootstrap.
//
// ProjectID is the OWNER's local project id (the joiner stores it as the remote
// project id). SnapshotURL + SnapshotToken (15-min signed) drive the snapshot
// bootstrap that lands in F2.3. PermissionsGranted is the grade the invite
// carried; ProtocolVersion is the negotiated common version.
type Response struct {
	ProjectID          int64  `json:"project_id"`
	ProjectName        string `json:"project_name"`
	OwnerPublicKey     string `json:"owner_public_key"`
	OwnerDisplayName   string `json:"owner_display_name"`
	SnapshotURL        string `json:"snapshot_url"`
	SnapshotToken      string `json:"snapshot_token"`
	PermissionsGranted string `json:"permissions"`
	ProtocolVersion    int    `json:"protocol_version"`
}
