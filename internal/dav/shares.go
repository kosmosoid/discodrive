package dav

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// ErrNotOwner — operation is only available to the resource owner.
var ErrNotOwner = errors.New("dav: only the owner can manage access")

type ShareInfo struct {
	ID        string
	Email     string
	ExpiresAt string // RFC3339 or ""
}

type SharedCalendar struct {
	Calendar   db.Calendar
	OwnerEmail string
}

// ShareCalendar grants a user (by email) full read_write access to a calendar.
func (s *Service) ShareCalendar(ctx context.Context, ownerID, calID, withEmail string, expiresAt *time.Time) (db.ResourceShare, error) {
	cid, err := db.ParseUUID(calID)
	if err != nil {
		return db.ResourceShare{}, ErrNotFound
	}
	cal, err := s.q.GetCalendar(ctx, cid)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ResourceShare{}, ErrNotFound
	}
	if err != nil {
		return db.ResourceShare{}, err
	}
	if db.UUIDString(cal.UserID) != ownerID {
		return db.ResourceShare{}, ErrNotOwner
	}
	target, err := s.q.GetUserByEmail(ctx, withEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ResourceShare{}, ErrNotFound
	}
	if err != nil {
		return db.ResourceShare{}, err
	}
	var exp pgtype.Timestamptz
	if expiresAt != nil {
		exp = pgtype.Timestamptz{Time: *expiresAt, Valid: true}
	}
	return s.q.CreateShare(ctx, db.CreateShareParams{
		ResourceType:   "calendar",
		ResourceID:     cid,
		OwnerID:        cal.UserID,
		SharedWithUser: target.ID,
		Access:         "read_write",
		ExpiresAt:      exp,
	})
}

// ListCalendarShares returns the shares for a calendar (owner only).
func (s *Service) ListCalendarShares(ctx context.Context, ownerID, calID string) ([]ShareInfo, error) {
	cid, err := db.ParseUUID(calID)
	if err != nil {
		return nil, ErrNotFound
	}
	cal, err := s.q.GetCalendar(ctx, cid)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if db.UUIDString(cal.UserID) != ownerID {
		return nil, ErrNotOwner
	}
	shares, err := s.q.ListSharesForResource(ctx, db.ListSharesForResourceParams{ResourceType: "calendar", ResourceID: cid})
	if err != nil {
		return nil, err
	}
	out := make([]ShareInfo, 0, len(shares))
	for _, sh := range shares {
		if !sh.SharedWithUser.Valid {
			continue // skip link shares
		}
		email := ""
		if u, e := s.q.GetUserByID(ctx, sh.SharedWithUser); e == nil {
			email = u.Email
		}
		exp := ""
		if sh.ExpiresAt.Valid {
			exp = sh.ExpiresAt.Time.Format(time.RFC3339)
		}
		out = append(out, ShareInfo{ID: db.UUIDString(sh.ID), Email: email, ExpiresAt: exp})
	}
	return out, nil
}

// DeleteCalendarShare revokes a share (owner only).
func (s *Service) DeleteCalendarShare(ctx context.Context, ownerID, shareID string) error {
	sid, err := db.ParseUUID(shareID)
	if err != nil {
		return ErrNotFound
	}
	sh, err := s.q.GetShare(ctx, sid)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if db.UUIDString(sh.OwnerID) != ownerID {
		return ErrNotOwner
	}
	return s.q.DeleteShare(ctx, sid)
}

// SharedCalendarsForUser returns calendars shared with the user (active shares only).
func (s *Service) SharedCalendarsForUser(ctx context.Context, userID string) ([]SharedCalendar, error) {
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return nil, err
	}
	shares, err := s.q.ListSharesForUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]SharedCalendar, 0)
	for _, sh := range shares {
		if sh.ResourceType != "calendar" {
			continue
		}
		cal, err := s.q.GetCalendar(ctx, sh.ResourceID)
		if err != nil {
			continue
		}
		email := ""
		if ownerU, e := s.q.GetUserByID(ctx, cal.UserID); e == nil {
			email = ownerU.Email
		}
		out = append(out, SharedCalendar{Calendar: cal, OwnerEmail: email})
	}
	return out, nil
}

type FeedLink struct {
	ID          string
	Token       string
	HasPassword bool
	CreatedAt   string
}

// CreateCalendarFeedLink creates a public read-only subscription link for a calendar.
// passwordHash (if non-empty) is a pre-computed argon2 hash (see the API layer).
func (s *Service) CreateCalendarFeedLink(ctx context.Context, ownerID, calID, passwordHash string) (string, error) {
	cid, err := db.ParseUUID(calID)
	if err != nil {
		return "", ErrNotFound
	}
	cal, err := s.q.GetCalendar(ctx, cid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if db.UUIDString(cal.UserID) != ownerID {
		return "", ErrNotOwner
	}
	token := newFeedToken()
	share, err := s.q.CreateShare(ctx, db.CreateShareParams{
		ResourceType:   "calendar",
		ResourceID:     cid,
		OwnerID:        cal.UserID,
		ShareLinkToken: pgtype.Text{String: token, Valid: true},
		Access:         "read",
	})
	if err != nil {
		return "", err
	}
	if passwordHash != "" {
		if err := s.q.SetSharePasswordHash(ctx, db.SetSharePasswordHashParams{
			ID: share.ID, SharePasswordHash: pgtype.Text{String: passwordHash, Valid: true},
		}); err != nil {
			return "", err
		}
	}
	return token, nil
}

// ListCalendarFeedLinks returns public feed links for a calendar (owner only).
func (s *Service) ListCalendarFeedLinks(ctx context.Context, ownerID, calID string) ([]FeedLink, error) {
	cid, err := db.ParseUUID(calID)
	if err != nil {
		return nil, ErrNotFound
	}
	cal, err := s.q.GetCalendar(ctx, cid)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if db.UUIDString(cal.UserID) != ownerID {
		return nil, ErrNotOwner
	}
	shares, err := s.q.ListSharesForResource(ctx, db.ListSharesForResourceParams{ResourceType: "calendar", ResourceID: cid})
	if err != nil {
		return nil, err
	}
	out := make([]FeedLink, 0)
	for _, sh := range shares {
		if !sh.ShareLinkToken.Valid {
			continue // skip user shares
		}
		out = append(out, FeedLink{
			ID:          db.UUIDString(sh.ID),
			Token:       sh.ShareLinkToken.String,
			HasPassword: sh.SharePasswordHash.Valid && sh.SharePasswordHash.String != "",
			CreatedAt:   sh.CreatedAt.Time.Format(time.RFC3339),
		})
	}
	return out, nil
}

// CalendarByFeedToken resolves an active token to (calID, passwordHash, ok).
func (s *Service) CalendarByFeedToken(ctx context.Context, token string) (string, string, bool) {
	if token == "" {
		return "", "", false
	}
	sh, err := s.q.GetActiveShareByToken(ctx, token)
	if err != nil || sh.ResourceType != "calendar" {
		return "", "", false
	}
	hash := ""
	if sh.SharePasswordHash.Valid {
		hash = sh.SharePasswordHash.String
	}
	return db.UUIDString(sh.ResourceID), hash, true
}

type SharedAddressbook struct {
	Addressbook db.Addressbook
	OwnerEmail  string
}

// GetAddressbook wraps the lookup-by-id query (for ownership/access checks).
func (s *Service) GetAddressbook(ctx context.Context, abID string) (db.Addressbook, error) {
	aid, err := db.ParseUUID(abID)
	if err != nil {
		return db.Addressbook{}, err
	}
	a, err := s.q.GetAddressbook(ctx, aid)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Addressbook{}, ErrNotFound
	}
	return a, err
}

// ShareAddressbook grants a user (by email) full read_write access to an address book.
func (s *Service) ShareAddressbook(ctx context.Context, ownerID, abID, withEmail string) (db.ResourceShare, error) {
	aid, err := db.ParseUUID(abID)
	if err != nil {
		return db.ResourceShare{}, ErrNotFound
	}
	ab, err := s.q.GetAddressbook(ctx, aid)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ResourceShare{}, ErrNotFound
	}
	if err != nil {
		return db.ResourceShare{}, err
	}
	if db.UUIDString(ab.UserID) != ownerID {
		return db.ResourceShare{}, ErrNotOwner
	}
	target, err := s.q.GetUserByEmail(ctx, withEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ResourceShare{}, ErrNotFound
	}
	if err != nil {
		return db.ResourceShare{}, err
	}
	return s.q.CreateShare(ctx, db.CreateShareParams{
		ResourceType:   "addressbook",
		ResourceID:     aid,
		OwnerID:        ab.UserID,
		SharedWithUser: target.ID,
		Access:         "read_write",
	})
}

// ListAddressbookShares returns the shares for an address book (owner only).
func (s *Service) ListAddressbookShares(ctx context.Context, ownerID, abID string) ([]ShareInfo, error) {
	aid, err := db.ParseUUID(abID)
	if err != nil {
		return nil, ErrNotFound
	}
	ab, err := s.q.GetAddressbook(ctx, aid)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if db.UUIDString(ab.UserID) != ownerID {
		return nil, ErrNotOwner
	}
	shares, err := s.q.ListSharesForResource(ctx, db.ListSharesForResourceParams{ResourceType: "addressbook", ResourceID: aid})
	if err != nil {
		return nil, err
	}
	out := make([]ShareInfo, 0, len(shares))
	for _, sh := range shares {
		if !sh.SharedWithUser.Valid {
			continue
		}
		email := ""
		if u, e := s.q.GetUserByID(ctx, sh.SharedWithUser); e == nil {
			email = u.Email
		}
		exp := ""
		if sh.ExpiresAt.Valid {
			exp = sh.ExpiresAt.Time.Format(time.RFC3339)
		}
		out = append(out, ShareInfo{ID: db.UUIDString(sh.ID), Email: email, ExpiresAt: exp})
	}
	return out, nil
}

// DeleteAddressbookShare revokes an address book share (owner only).
func (s *Service) DeleteAddressbookShare(ctx context.Context, ownerID, shareID string) error {
	sid, err := db.ParseUUID(shareID)
	if err != nil {
		return ErrNotFound
	}
	sh, err := s.q.GetShare(ctx, sid)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if db.UUIDString(sh.OwnerID) != ownerID {
		return ErrNotOwner
	}
	return s.q.DeleteShare(ctx, sid)
}

// SharedAddressbooksForUser returns address books shared with the user (active shares only).
func (s *Service) SharedAddressbooksForUser(ctx context.Context, userID string) ([]SharedAddressbook, error) {
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return nil, err
	}
	shares, err := s.q.ListSharesForUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]SharedAddressbook, 0)
	for _, sh := range shares {
		if sh.ResourceType != "addressbook" {
			continue
		}
		ab, err := s.q.GetAddressbook(ctx, sh.ResourceID)
		if err != nil {
			continue
		}
		email := ""
		if ownerU, e := s.q.GetUserByID(ctx, ab.UserID); e == nil {
			email = ownerU.Email
		}
		out = append(out, SharedAddressbook{Addressbook: ab, OwnerEmail: email})
	}
	return out, nil
}

// CanAccessAddressbook reports whether the user can access the address book (owner or active share).
func (s *Service) CanAccessAddressbook(ctx context.Context, userID, abID string) (bool, error) {
	aid, err := db.ParseUUID(abID)
	if err != nil {
		return false, nil
	}
	ab, err := s.q.GetAddressbook(ctx, aid)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if db.UUIDString(ab.UserID) == userID {
		return true, nil
	}
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return false, nil
	}
	_, err = s.q.AddressbookShareForUser(ctx, db.AddressbookShareForUserParams{ResourceID: aid, SharedWithUser: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CanAccessCalendar reports whether the user can access the calendar (owner or active share).
func (s *Service) CanAccessCalendar(ctx context.Context, userID, calID string) (bool, error) {
	cid, err := db.ParseUUID(calID)
	if err != nil {
		return false, nil
	}
	cal, err := s.q.GetCalendar(ctx, cid)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if db.UUIDString(cal.UserID) == userID {
		return true, nil
	}
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return false, nil
	}
	_, err = s.q.CalendarShareForUser(ctx, db.CalendarShareForUserParams{ResourceID: cid, SharedWithUser: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
