package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload. Subject = user_id.
type Claims struct {
	TenantID string `json:"tid"`
	Role     string `json:"role"`
	// Ver is the token version at issue time (users.token_version). A password change
	// increments the counter in the DB, causing old tokens to stop matching → 401.
	Ver int64 `json:"ver"`
	// DeviceID is set for tokens issued to a device (sync daemon): the devices row ID.
	// Empty for web sessions. Middleware checks whether the device is still alive → instant revocation.
	DeviceID string `json:"did,omitempty"`
	// Pur is the token purpose. Empty for full session tokens (back-compatible).
	// "mfa" marks a short-lived intermediate token: password proven, second factor pending.
	// The main middleware rejects any token with a non-empty purpose.
	Pur string `json:"pur,omitempty"`
	jwt.RegisteredClaims
}

// TokenIssuer issues and validates JWTs (HS256, short TTL).
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenIssuer(secret string, ttl time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: []byte(secret), ttl: ttl}
}

// Issue issues a JWT. deviceID is non-empty only for sync-device tokens (daemon);
// pass "" for web sessions.
func (t *TokenIssuer) Issue(userID, tenantID, role string, ver int64, deviceID string) (string, error) {
	now := time.Now()
	claims := Claims{
		TenantID: tenantID,
		Role:     role,
		Ver:      ver,
		DeviceID: deviceID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// mfaTokenTTL bounds the window to complete a second factor after the password step.
const mfaTokenTTL = 5 * time.Minute

// IssueMFA issues a short-lived intermediate token (purpose=mfa) for the
// password-proven-but-second-factor-pending state. It grants no access on its own:
// the main middleware rejects it; only the /auth/mfa/* completion handlers accept it (A.3/A.5).
func (t *TokenIssuer) IssueMFA(userID, tenantID string) (string, error) {
	now := time.Now()
	claims := Claims{
		TenantID: tenantID,
		Pur:      "mfa",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(mfaTokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// waSessionClaims carries a marshaled webauthn.SessionData between the begin and finish
// steps of a WebAuthn ceremony, signed so the client cannot tamper with the challenge.
type waSessionClaims struct {
	Data string `json:"was"`
	jwt.RegisteredClaims
}

// IssueWebAuthnSession signs a short-lived token (5 min) carrying base64 SessionData for
// the given user. Used to keep WebAuthn registration/login stateless across two requests.
func (t *TokenIssuer) IssueWebAuthnSession(userID, data string) (string, error) {
	now := time.Now()
	claims := waSessionClaims{
		Data: data,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(mfaTokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// ParseWebAuthnSession validates the token and returns the subject and the base64 SessionData.
func (t *TokenIssuer) ParseWebAuthnSession(tokenStr string) (userID, data string, err error) {
	claims := &waSessionClaims{}
	if _, err = jwt.ParseWithClaims(tokenStr, claims, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected JWT signing method")
		}
		return t.secret, nil
	}); err != nil {
		return "", "", err
	}
	return claims.Subject, claims.Data, nil
}

func (t *TokenIssuer) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected JWT signing method")
		}
		return t.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}
