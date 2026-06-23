package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

var (
	// ErrWebAuthnNotConfigured is returned when WebAuthn is used without BASE_DOMAIN set.
	ErrWebAuthnNotConfigured = errors.New("WebAuthn requires BASE_DOMAIN to be configured")
	// ErrWebAuthnSession is returned when the registration session token is invalid/expired.
	ErrWebAuthnSession = errors.New("WebAuthn session expired, start over")
	ErrWebAuthnFailed  = errors.New("WebAuthn verification failed")
)

// NewWebAuthn builds the relying party from the public domain. Returns (nil, nil) when
// baseDomain is empty — WebAuthn is then simply unavailable (begin/finish refuse).
//
// baseDomain may be a bare host ("drive.example.com"), a host:port ("localhost:8443"), or a
// full origin URL ("https://localhost:8443"). The RP ID is the host alone (no scheme/port),
// while the allowed origin includes the port — WebAuthn origin matching is port-sensitive, so
// a non-443 port (typical in dev) must be carried through here.
func NewWebAuthn(baseDomain string) (*webauthn.WebAuthn, error) {
	if baseDomain == "" {
		return nil, nil
	}
	origin := baseDomain
	if !strings.Contains(origin, "://") {
		origin = "https://" + origin
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid BASE_DOMAIN %q: %w", baseDomain, err)
	}
	return webauthn.New(&webauthn.Config{
		RPDisplayName: "discodrive",
		RPID:          u.Hostname(),                    // host only, no scheme/port
		RPOrigins:     []string{u.Scheme + "://" + u.Host}, // host incl port
	})
}

// SetWebAuthn wires the relying party (called from main after NewService). Injected
// separately so the many test constructors of Service don't need a WebAuthn argument.
func (s *Service) SetWebAuthn(wa *webauthn.WebAuthn) { s.wa = wa }

// WebAuthnEnabled reports whether WebAuthn is configured (BASE_DOMAIN set), so the UI can
// hide the passkey option when it would only fail.
func (s *Service) WebAuthnEnabled() bool { return s.wa != nil }

// waUser adapts a db.User + its stored credentials to the webauthn.User interface.
type waUser struct {
	u     db.User
	creds []webauthn.Credential
}

func (w *waUser) WebAuthnID() []byte                        { return w.u.ID.Bytes[:] }
func (w *waUser) WebAuthnName() string                      { return w.u.Email }
func (w *waUser) WebAuthnDisplayName() string               { return w.u.Email }
func (w *waUser) WebAuthnCredentials() []webauthn.Credential { return w.creds }

// loadWAUser builds a waUser by loading the account and its decoded credentials.
func (s *Service) loadWAUser(ctx context.Context, uid pgtype.UUID) (*waUser, error) {
	u, err := s.q.GetUserByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListWebAuthnCredentials(ctx, uid)
	if err != nil {
		return nil, err
	}
	creds := make([]webauthn.Credential, 0, len(rows))
	for _, r := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal(r.Credential, &c); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return &waUser{u: u, creds: creds}, nil
}

// BeginWebAuthnRegistration starts enrolling a new authenticator for the (already
// authenticated) user. Returns the JSON creation options for navigator.credentials.create()
// and a signed session token that FinishWebAuthnRegistration must be given back.
func (s *Service) BeginWebAuthnRegistration(ctx context.Context, userID string) (options []byte, sessionToken string, err error) {
	if s.wa == nil {
		return nil, "", ErrWebAuthnNotConfigured
	}
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return nil, "", err
	}
	wu, err := s.loadWAUser(ctx, uid)
	if err != nil {
		return nil, "", err
	}
	// Request a discoverable credential (passkey): residentKey "preferred" lets password
	// managers / platforms offer to save & sync it, and enables usernameless login (A.5),
	// while "preferred" (not "required") still allows classic security keys like YubiKey.
	// BeginRegistration also excludes the user's existing credentials by default (no duplicates).
	sel := protocol.AuthenticatorSelection{
		ResidentKey:        protocol.ResidentKeyRequirementPreferred,
		RequireResidentKey: protocol.ResidentKeyNotRequired(),
		UserVerification:   protocol.VerificationPreferred,
	}
	creation, sessionData, err := s.wa.BeginRegistration(wu, webauthn.WithAuthenticatorSelection(sel))
	if err != nil {
		return nil, "", err
	}
	sd, err := json.Marshal(sessionData)
	if err != nil {
		return nil, "", err
	}
	tok, err := s.issuer.IssueWebAuthnSession(userID, base64.StdEncoding.EncodeToString(sd))
	if err != nil {
		return nil, "", err
	}
	options, err = json.Marshal(creation)
	if err != nil {
		return nil, "", err
	}
	return options, tok, nil
}

// FinishWebAuthnRegistration verifies the authenticator's attestation against the session
// token and stores the credential (full go-webauthn Credential as JSON) under the given name.
func (s *Service) FinishWebAuthnRegistration(ctx context.Context, userID, sessionToken string, attestation []byte, name string) error {
	if s.wa == nil {
		return ErrWebAuthnNotConfigured
	}
	subj, sdB64, err := s.issuer.ParseWebAuthnSession(sessionToken)
	if err != nil || subj != userID {
		return ErrWebAuthnSession
	}
	sdRaw, err := base64.StdEncoding.DecodeString(sdB64)
	if err != nil {
		return ErrWebAuthnSession
	}
	var sd webauthn.SessionData
	if err := json.Unmarshal(sdRaw, &sd); err != nil {
		return ErrWebAuthnSession
	}
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return err
	}
	wu, err := s.loadWAUser(ctx, uid)
	if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(attestation)
	if err != nil {
		return ErrWebAuthnFailed
	}
	cred, err := s.wa.CreateCredential(wu, sd, parsed)
	if err != nil {
		return ErrWebAuthnFailed
	}
	blob, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	if name == "" {
		name = "Security key"
	}
	_, err = s.q.InsertWebAuthnCredential(ctx, db.InsertWebAuthnCredentialParams{
		UserID:       uid,
		CredentialID: cred.ID,
		Credential:   blob,
		Name:         name,
	})
	return err
}

// BeginWebAuthnLogin starts a passwordless, usernameless (discoverable) sign-in: the
// authenticator's resident credential identifies the user at finish time. Returns the JSON
// request options for navigator.credentials.get() and a signed session token.
func (s *Service) BeginWebAuthnLogin(ctx context.Context) (options []byte, sessionToken string, err error) {
	if s.wa == nil {
		return nil, "", ErrWebAuthnNotConfigured
	}
	assertion, sessionData, err := s.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", err
	}
	sd, err := json.Marshal(sessionData)
	if err != nil {
		return nil, "", err
	}
	// No subject yet — the user is discovered from the authenticator response at finish.
	tok, err := s.issuer.IssueWebAuthnSession("", base64.StdEncoding.EncodeToString(sd))
	if err != nil {
		return nil, "", err
	}
	options, err = json.Marshal(assertion)
	if err != nil {
		return nil, "", err
	}
	return options, tok, nil
}

// FinishWebAuthnLogin verifies a discoverable-login assertion, bumps the stored credential
// (sign count / last used), and issues a full session for the identified user.
func (s *Service) FinishWebAuthnLogin(ctx context.Context, sessionToken string, assertion []byte) (LoginResult, error) {
	if s.wa == nil {
		return LoginResult{}, ErrWebAuthnNotConfigured
	}
	_, sdB64, err := s.issuer.ParseWebAuthnSession(sessionToken)
	if err != nil {
		return LoginResult{}, ErrWebAuthnSession
	}
	sdRaw, err := base64.StdEncoding.DecodeString(sdB64)
	if err != nil {
		return LoginResult{}, ErrWebAuthnSession
	}
	var sd webauthn.SessionData
	if err := json.Unmarshal(sdRaw, &sd); err != nil {
		return LoginResult{}, ErrWebAuthnSession
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(assertion)
	if err != nil {
		return LoginResult{}, ErrWebAuthnFailed
	}

	// The user handle (= our user UUID bytes) identifies the account during discoverable login.
	var loggedIn db.User
	handler := func(_, userHandle []byte) (webauthn.User, error) {
		if len(userHandle) != 16 {
			return nil, ErrWebAuthnFailed
		}
		var uid pgtype.UUID
		copy(uid.Bytes[:], userHandle)
		uid.Valid = true
		wu, err := s.loadWAUser(ctx, uid)
		if err != nil {
			return nil, err
		}
		loggedIn = wu.u
		return wu, nil
	}
	cred, err := s.wa.ValidateDiscoverableLogin(handler, sd, parsed)
	if err != nil {
		return LoginResult{}, ErrWebAuthnFailed
	}

	// Persist the bumped sign count + last-used (clone detection lives in the stored blob).
	blob, err := json.Marshal(cred)
	if err != nil {
		return LoginResult{}, err
	}
	if err := s.q.UpdateWebAuthnCredential(ctx, db.UpdateWebAuthnCredentialParams{CredentialID: cred.ID, Credential: blob}); err != nil {
		return LoginResult{}, err
	}
	token, err := s.issueFor(loggedIn)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, User: loggedIn}, nil
}
