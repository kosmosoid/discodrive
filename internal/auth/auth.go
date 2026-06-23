package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"discodrive/internal/db"
	"discodrive/internal/secret"
)

const setupTokenKey = "admin.setup_token"

var (
	ErrEmailTaken      = errors.New("email already taken")
	ErrInvalidCreds    = errors.New("invalid email or password")
	ErrAdminExists     = errors.New("admin already exists")
	ErrInvalidMFAToken = errors.New("invalid or expired sign-in session")
)

// Service holds the domain logic for authentication and multi-tenancy.
type Service struct {
	pool   *pgxpool.Pool
	q      *db.Queries
	issuer *TokenIssuer
	cipher *secret.Cipher       // encrypts TOTP secrets (A.3); nil/disabled → 2FA setup refused
	wa     *webauthn.WebAuthn   // WebAuthn relying party (A.4); nil → WebAuthn unavailable
	// lookupUser verifies a user against the DB in the middleware (role/existence check).
	// Extracted as a field so the seam is testable without a live DB.
	lookupUser func(context.Context, pgtype.UUID) (db.User, error)
	// getUserByEmail / availableFactors are seams so Login is testable without a live DB.
	getUserByEmail   func(context.Context, string) (db.User, error)
	availableFactors func(context.Context, pgtype.UUID) ([]string, error)
}

func NewService(pool *pgxpool.Pool, issuer *TokenIssuer, cipher *secret.Cipher) *Service {
	q := db.New(pool)
	s := &Service{pool: pool, q: q, issuer: issuer, cipher: cipher, lookupUser: q.GetUserByID}
	s.getUserByEmail = q.GetUserByEmail
	// Post-password second factor is TOTP only. WebAuthn is a passwordless ALTERNATIVE
	// login method (its own route), not a factor forced after a password — so a registered
	// passkey must not gate password login.
	s.availableFactors = func(ctx context.Context, uid pgtype.UUID) ([]string, error) {
		f, err := q.AvailableMFAFactors(ctx, uid)
		if err != nil {
			return nil, err
		}
		var methods []string
		if f.HasTotp {
			methods = append(methods, "totp")
		}
		return methods, nil
	}
	return s
}

// Register creates a personal tenant + user (role: user) and issues a token.
func (s *Service) Register(ctx context.Context, email, password string) (string, db.User, error) {
	switch _, err := s.q.GetUserByEmail(ctx, email); {
	case err == nil:
		return "", db.User{}, ErrEmailTaken
	case !errors.Is(err, pgx.ErrNoRows):
		return "", db.User{}, err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return "", db.User{}, err
	}

	user, err := s.createUserTx(ctx, email, hash, "user", email)
	if err != nil {
		return "", db.User{}, err
	}
	token, err := s.issueFor(user)
	return token, user, err
}

// LoginResult is the outcome of a password login. Exactly one of Token / MFAToken is set:
// Token = full session (no second factor), MFAToken = intermediate (second factor pending).
type LoginResult struct {
	Token    string
	MFAToken string
	Methods  []string // available second factors: "totp", "webauthn"
	User     db.User
	// Method records how the session was obtained, for the audit log: "totp", "backup",
	// "passkey" (set by the MFA/passkey completions). Empty for a plain password login.
	Method string
}

// Login verifies the password. With no enabled second factor it issues a full session token.
// With a second factor it issues a short-lived MFA-pending token and the list of methods;
// the caller completes the login via /auth/mfa/* (A.3/A.5).
func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	u, err := s.getUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginResult{}, ErrInvalidCreds
	}
	if err != nil {
		return LoginResult{}, err
	}
	ok, err := VerifyPassword(password, u.PasswordHash)
	if err != nil {
		return LoginResult{}, err
	}
	if !ok {
		return LoginResult{}, ErrInvalidCreds
	}

	methods, err := s.availableFactors(ctx, u.ID)
	if err != nil {
		return LoginResult{}, err
	}
	if len(methods) > 0 {
		mfaTok, err := s.issuer.IssueMFA(db.UUIDString(u.ID), db.UUIDString(u.TenantID))
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{MFAToken: mfaTok, Methods: methods, User: u}, nil
	}

	token, err := s.issueFor(u)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, User: u}, nil
}

// SetupNeeded returns true while no admin exists in the system.
// Also cleans up any stale setup token left over from a previous scheme.
func (s *Service) SetupNeeded(ctx context.Context) (bool, error) {
	admins, err := s.q.CountAdmins(ctx)
	if err != nil {
		return false, err
	}
	if admins > 0 {
		_ = s.q.DeleteSetting(ctx, setupTokenKey)
		return false, nil
	}
	return true, nil
}

// SetupAdmin creates the first administrator (token-less first-run onboarding).
// Only available while no admin exists; returns ErrAdminExists afterward (takeover guard).
func (s *Service) SetupAdmin(ctx context.Context, email, password string) (db.User, error) {
	admins, err := s.q.CountAdmins(ctx)
	if err != nil {
		return db.User{}, err
	}
	if admins > 0 {
		return db.User{}, ErrAdminExists
	}
	hash, err := HashPassword(password)
	if err != nil {
		return db.User{}, err
	}
	return s.createUserTx(ctx, email, hash, "admin", "admin")
}

// createUserTx creates a tenant and a user in a single transaction; for admin
// it also deletes the setup token.
func (s *Service) createUserTx(ctx context.Context, email, hash, role, tenantName string) (db.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.User{}, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	tenant, err := qtx.CreateTenant(ctx, tenantName)
	if err != nil {
		return db.User{}, err
	}
	user, err := qtx.CreateUser(ctx, db.CreateUserParams{
		TenantID:           tenant.ID,
		Email:              email,
		PasswordHash:       hash,
		StorageQuota:       pgtype.Int8{}, // NULL = no explicit quota
		Role:               role,
		MustChangePassword: false,
	})
	if err != nil {
		return db.User{}, err
	}
	if role == "admin" {
		if err := qtx.DeleteSetting(ctx, setupTokenKey); err != nil {
			return db.User{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return db.User{}, err
	}
	return user, nil
}

// AdminCreateUser creates a user from the admin panel (with role and quota).
func (s *Service) AdminCreateUser(ctx context.Context, email, password, role string, quota *int64) (db.User, error) {
	if role != "admin" && role != "user" {
		role = "user"
	}
	switch _, err := s.q.GetUserByEmail(ctx, email); {
	case err == nil:
		return db.User{}, ErrEmailTaken
	case !errors.Is(err, pgx.ErrNoRows):
		return db.User{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return db.User{}, err
	}
	q := pgtype.Int8{}
	if quota != nil {
		q = pgtype.Int8{Int64: *quota, Valid: true}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.User{}, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	tenant, err := qtx.CreateTenant(ctx, email)
	if err != nil {
		return db.User{}, err
	}
	user, err := qtx.CreateUser(ctx, db.CreateUserParams{
		TenantID: tenant.ID, Email: email, PasswordHash: hash, StorageQuota: q, Role: role,
		MustChangePassword: true,
	})
	if err != nil {
		return db.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.User{}, err
	}
	return user, nil
}

func (s *Service) issueFor(u db.User) (string, error) {
	return s.issuer.Issue(db.UUIDString(u.ID), db.UUIDString(u.TenantID), u.Role, u.TokenVersion, "")
}

// issueForDevice issues a token bound to a specific device (sync daemon): it carries
// device_id so the middleware can verify the device is still alive (instant revocation).
func (s *Service) issueForDevice(u db.User, deviceID string) (string, error) {
	return s.issuer.Issue(db.UUIDString(u.ID), db.UUIDString(u.TenantID), u.Role, u.TokenVersion, deviceID)
}

// ChangePassword changes the password after verifying the current one and increments
// token_version (invalidating all active sessions). Returns a fresh token so the
// initiator stays logged in.
func (s *Service) ChangePassword(ctx context.Context, userID, current, newPassword string) (string, error) {
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return "", ErrInvalidCreds
	}
	u, err := s.q.GetUserByID(ctx, uid)
	if err != nil {
		return "", ErrInvalidCreds
	}
	ok, err := VerifyPassword(current, u.PasswordHash)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrInvalidCreds
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return "", err
	}
	updated, err := s.q.UpdatePassword(ctx, db.UpdatePasswordParams{ID: uid, PasswordHash: hash})
	if err != nil {
		return "", err
	}
	return s.issueFor(updated)
}

// newWebdavSecret generates an app-specific password (plain, shown once)
// and its argon2id hash for storage in devices.secret_hash.
func newWebdavSecret() (plain, hash string, err error) {
	b := make([]byte, 20) // 160 bits → 32 base32 characters
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plain = strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
	hash, err = HashPassword(plain)
	return plain, hash, err
}

// CreateWebdavPassword creates a device (kind=webdav) with an app-specific password.
// The plain-text password is returned once; only the argon2 hash is stored in the DB.
func (s *Service) CreateWebdavPassword(ctx context.Context, userID, name string) (db.Device, string, error) {
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return db.Device{}, "", err
	}
	plain, hash, err := newWebdavSecret()
	if err != nil {
		return db.Device{}, "", err
	}
	dev, err := s.q.CreateWebdavDevice(ctx, db.CreateWebdavDeviceParams{
		UserID: uid, Name: name, SecretHash: pgtype.Text{String: hash, Valid: true},
	})
	if err != nil {
		return db.Device{}, "", err
	}
	return dev, plain, nil
}

// VerifyWebdavPassword looks up a user by email and checks the app-specific
// password against any of their WebDAV devices. Returns userID and deviceID.
func (s *Service) VerifyWebdavPassword(ctx context.Context, email, password string) (userID, deviceID string, ok bool) {
	devs, err := s.q.ListWebdavDevicesByEmail(ctx, email)
	if err != nil {
		return "", "", false
	}
	for _, d := range devs {
		if !d.SecretHash.Valid {
			continue
		}
		if good, _ := VerifyPassword(password, d.SecretHash.String); good {
			return db.UUIDString(d.UserID), db.UUIDString(d.ID), true
		}
	}
	return "", "", false
}
