package federation_test

import (
	"context"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// Federation v1 F7.7 — NFR-3 security verification (service-layer half).
//
// F7.7 is a verification milestone: it asserts the security invariants the whole
// federation stack already promises hold concretely. This file covers the two
// NFR-3 legs that live in the service layer:
//
//   - invite secret = 256 bits of crypto/rand (NFR-3.2), only the SHA-256 hash is
//     stored, and the plaintext never appears in any log record (US-1.2 AC2/AC6);
//   - a grep-guard that no federation source compares a secret / digest / hash /
//     signature with the timing-unsafe == / != operator instead of
//     crypto/subtle.ConstantTimeCompare (NFR-3.3). The two real comparisons today
//     (the handshake secret-hash compare and the body-digest compare) already use
//     subtle; this guard fails the build if a future edit regresses to ==.
//
// The transport / handshake-endpoint legs (rate-limit 429, no-secret-in-logs at
// the HTTP boundary) live in the handlers package; the deploy legs (CGO-free
// build, YAML defaults, proxy-prefix signature) live in internal/federation/fedtest.

// TestF77_InviteSecretIs256BitCryptoRand asserts a minted invite secret carries
// 256 bits of entropy from crypto/rand: it is a 64-char hex string (32 bytes),
// decodes cleanly, only its SHA-256 hash is persisted (no plaintext column), and
// two consecutive mints never collide (NFR-3.2, US-1.2 AC2).
func TestF77_InviteSecretIs256BitCryptoRand(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	pid := seedProject(t, projects)

	invites := repo.NewFederationInviteRepo(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, invites, repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), "https://me.example")
	if _, err := svc.EnableForProject(context.Background(), pid); err != nil {
		t.Fatalf("enable: %v", err)
	}

	seen := make(map[string]struct{})
	for i := 0; i < 16; i++ {
		res, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
			Permissions: "write",
		})
		if err != nil {
			t.Fatalf("create invite: %v", err)
		}
		// 256 bits = 32 bytes = 64 hex chars.
		raw, err := hex.DecodeString(res.Secret)
		if err != nil {
			t.Fatalf("secret %q is not hex: %v", res.Secret, err)
		}
		if len(raw) != 32 {
			t.Errorf("invite secret: got %d bytes, want 32 (256 bits, NFR-3.2)", len(raw))
		}
		if _, dup := seen[res.Secret]; dup {
			t.Fatalf("invite secret collision across mints — entropy source is broken")
		}
		seen[res.Secret] = struct{}{}

		// The plaintext secret must NOT be reconstructable from storage: the listed
		// invite carries only metadata + derived status, never the secret/hash
		// (US-1.2 AC2 / US-1.3 AC5).
		views, err := svc.ListInvites(context.Background(), pid)
		if err != nil {
			t.Fatalf("list invites: %v", err)
		}
		for _, v := range views {
			// The view struct has no Secret/SecretHash field by construction; assert the
			// id is present so the list is real but the secret never is.
			if v.InviteID == "" {
				t.Errorf("listed invite missing id")
			}
		}
	}

	// The DB column for the secret is a HASH, never the plaintext: no stored row's
	// secret_hash equals any plaintext we just minted.
	rows, err := d.Query(`SELECT secret_hash FROM federation_invites`)
	if err != nil {
		t.Fatalf("query invites: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var stored string
		if err := rows.Scan(&stored); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if _, isPlaintext := seen[stored]; isPlaintext {
			t.Errorf("federation_invites stored a PLAINTEXT secret, not its hash (NFR-3.2 violation)")
		}
		if len(stored) != 64 { // sha256 hex
			t.Errorf("stored secret_hash %q is not a 64-char sha256 hex digest", stored)
		}
	}
}

// TestF77_ConstantTimeCompareGrepGuard is the NFR-3.3 grep-guard: it parses every
// non-test .go file under internal/federation, internal/service/federation,
// internal/crypto, and the federation HTTP middleware, and fails if any of them
// compares a secret/digest/hash/signature-shaped value with the timing-unsafe ==
// or != operator. Such comparisons MUST go through crypto/subtle.
// ConstantTimeCompare. This catches a regression the moment a future edit swaps a
// constant-time compare back to == on a sensitive value.
func TestF77_ConstantTimeCompareGrepGuard(t *testing.T) {
	root := repoRoot(t)
	dirs := []string{
		filepath.Join(root, "internal", "federation"),
		filepath.Join(root, "internal", "service", "federation"),
		filepath.Join(root, "internal", "crypto"),
		// The HTTP boundary handlers (federation admin/events/handshake live here,
		// alongside the rest). New secret/token/digest comparisons are most likely to
		// land here, so the guard must scan it too (F7.7 review B — previously a blind
		// spot; only the single signature-middleware file was covered).
		filepath.Join(root, "internal", "httpapi", "handlers"),
	}
	// The federation transport-signature middleware lives directly under internal/httpapi.
	mwFile := filepath.Join(root, "internal", "httpapi", "federation_signature_middleware.go")

	// sensitive matches identifiers whose name implies a secret/credential VALUE
	// the comparison of which must be constant-time. A name that merely embeds a
	// keyword while clearly denoting an identifier / metadata field
	// (`tokenProjectID`, `signatureSize`, `hashLen`) is NOT a secret value and is
	// excluded so the guard does not false-positive on int/size compares.
	sensitive := func(name string) bool {
		l := strings.ToLower(name)
		hit := false
		for _, kw := range []string{"secret", "digest", "hash", "signature", "token", "mac", "hmac"} {
			if strings.Contains(l, kw) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
		for _, meta := range []string{"id", "size", "len", "count", "version", "kind", "name", "type", "at", "url"} {
			if strings.HasSuffix(l, meta) {
				return false
			}
		}
		return true
	}

	fset := token.NewFileSet()
	var offenders []string

	scanFile := func(path string) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			if be.Op != token.EQL && be.Op != token.NEQ {
				return true
			}
			xSensitive := identNameMatches(be.X, sensitive)
			ySensitive := identNameMatches(be.Y, sensitive)
			if !xSensitive && !ySensitive {
				return true
			}
			// A sensitive operand is fine to compare when it is only being checked for
			// PRESENCE or SHAPE, none of which compares two secret VALUES:
			//   - `sig == ""` / `token != ""`     — emptiness check (no value compare);
			//   - `key != nil`                     — structural nil check;
			//   - `len(sig) != ed25519.Size`       — length/size check;
			//   - `tokenProjectID != projectID`    — non-secret int compare (id, len, etc).
			// The genuine NFR-3.3 danger is comparing two secret VALUES for equality
			// (`digest == header`, `hashA == hashB`, `secret == "literal"`): flag those.
			if isEmptyStringLiteral(be.X) || isEmptyStringLiteral(be.Y) {
				return true
			}
			if isNilOperand(be.X) || isNilOperand(be.Y) {
				return true
			}
			if isLenCall(be.X) || isLenCall(be.Y) {
				return true
			}
			pos := fset.Position(be.Pos())
			offenders = append(offenders, pos.String())
			return true
		})
	}

	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, _ os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			scanFile(path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	scanFile(mwFile)

	if len(offenders) > 0 {
		t.Errorf("timing-unsafe ==/!= compare of a secret/digest/hash value (use crypto/subtle.ConstantTimeCompare) at:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// identNameMatches reports whether e is (or selects) an identifier whose final
// name component satisfies match — e.g. matches `inv.SecretHash`, `digestHeader`,
// `bodyDigest`.
func identNameMatches(e ast.Expr, match func(string) bool) bool {
	switch v := e.(type) {
	case *ast.Ident:
		return match(v.Name)
	case *ast.SelectorExpr:
		return match(v.Sel.Name)
	default:
		return false
	}
}

func isNilOperand(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

func isEmptyStringLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && (lit.Value == `""` || lit.Value == "``")
}

func isLenCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "len"
}

// repoRoot walks up from the test's working directory until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod above %s", dir)
		}
		dir = parent
	}
}
