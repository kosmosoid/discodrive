-- Stage 0 base access layer. Queries are extended in steps 0.2–0.5.

-- name: CreateTenant :one
INSERT INTO tenants (name) VALUES ($1) RETURNING *;

-- name: CreateUser :one
INSERT INTO users (tenant_id, email, password_hash, storage_quota, role, must_change_password)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserLanguage :one
SELECT language FROM users WHERE id = $1;

-- name: SetUserLanguage :exec
UPDATE users SET language = $2 WHERE id = $1;

-- name: ListUserIDs :many
SELECT id FROM users;

-- Users with used space (sum of live file sizes) — for the admin dashboard.
-- name: ListUsersWithUsage :many
SELECT u.id, u.email, u.role, u.storage_quota, u.created_at,
       COALESCE((
           SELECT SUM(n.size) FROM nodes n
           WHERE n.user_id = u.id AND n.deleted_at IS NULL AND n.is_dir = false
       ), 0)::bigint AS used
FROM users u
ORDER BY u.created_at;

-- name: UpdateUser :one
UPDATE users SET storage_quota = $2, role = $3 WHERE id = $1 RETURNING *;

-- Password change: new hash + bump token_version (invalidates all active sessions)
-- + clear the forced-change flag (A.2).
-- name: UpdatePassword :one
UPDATE users SET password_hash = $2, token_version = token_version + 1, must_change_password = false
WHERE id = $1 RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: CountAdmins :one
SELECT count(*) FROM users WHERE role = 'admin';

-- name: CreateDevice :one
INSERT INTO devices (user_id, name, kind)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListDevicesForUser :many
SELECT * FROM devices WHERE user_id = $1 ORDER BY created_at;

-- Device by id (middleware verifies the device token is still live → instant revocation).
-- name: GetDevice :one
SELECT * FROM devices WHERE id = $1;

-- name: DeleteDevice :exec
DELETE FROM devices WHERE id = $1 AND user_id = $2;

-- name: CreateWebdavDevice :one
INSERT INTO devices (user_id, name, kind, secret_hash)
VALUES ($1, $2, 'webdav', $3)
RETURNING *;

-- name: ListWebdavDevicesByEmail :many
SELECT d.* FROM devices d
JOIN users u ON u.id = d.user_id
WHERE u.email = $1 AND d.kind = 'webdav' AND d.secret_hash IS NOT NULL;

-- name: CreateNode :one
INSERT INTO nodes (
    user_id, parent_id, name, is_dir, size,
    content_hash, disk_path, mime, is_vault, modified_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- A conflict copy is a separate file node.
-- name: CreateConflictNode :one
INSERT INTO nodes (
    user_id, parent_id, name, is_dir, size, content_hash, disk_path, mime,
    is_conflict_loser, conflict_of
) VALUES ($1, $2, $3, false, $4, $5, $6, $7, true, $8)
RETURNING *;

-- name: GetNode :one
SELECT * FROM nodes WHERE id = $1 AND deleted_at IS NULL;

-- name: GetNodeForUser :one
SELECT * FROM nodes WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- name: ListChildren :many
SELECT * FROM nodes
WHERE user_id = $1 AND parent_id = $2 AND deleted_at IS NULL
ORDER BY is_dir DESC, name;

-- name: ListRootNodes :many
SELECT * FROM nodes
WHERE user_id = $1 AND parent_id IS NULL AND deleted_at IS NULL
ORDER BY is_dir DESC, name;

-- Children of a node without owner scoping (for listing a shared folder after canAccess).
-- name: ListNodeChildren :many
SELECT * FROM nodes
WHERE parent_id = $1 AND deleted_at IS NULL
ORDER BY is_dir DESC, name;

-- name: ListLiveNodes :many
SELECT * FROM nodes WHERE user_id = $1 AND deleted_at IS NULL;

-- Paths of trashed (soft-deleted) nodes — so a rescan doesn't re-import their files.
-- name: ListTombstonedNodePaths :many
SELECT disk_path FROM nodes
WHERE user_id = $1 AND deleted_at IS NOT NULL AND disk_path IS NOT NULL;

-- name: ListExpiredTombstones :many
SELECT id, user_id, disk_path, is_dir FROM nodes
WHERE deleted_at IS NOT NULL AND deleted_at < $1;

-- name: HardDeleteNode :exec
DELETE FROM nodes WHERE id = $1;

-- name: SoftDeleteNode :exec
UPDATE nodes SET deleted_at = now() WHERE id = $1;

-- name: UpdateNodeName :one
UPDATE nodes
SET name = $2, version = version + 1, modified_at = now(), modified_by = $3
WHERE id = $1
RETURNING *;

-- name: UpdateNodeParent :one
UPDATE nodes
SET parent_id = $2, version = version + 1, modified_at = now(), modified_by = $3
WHERE id = $1
RETURNING *;

-- name: UpdateNodeContent :one
UPDATE nodes
SET size = $2, content_hash = $3, mime = $4, version = version + 1, modified_at = now(), modified_by = $5
WHERE id = $1
RETURNING *;

-- name: BumpNodeVersion :one
UPDATE nodes SET version = version + 1, modified_at = now() WHERE id = $1 RETURNING version;

-- name: GetLiveNodeByPath :one
SELECT * FROM nodes
WHERE user_id = sqlc.arg(user_id) AND disk_path = sqlc.arg(path)::text AND deleted_at IS NULL;

-- Rewrite disk_path of a node and its whole subtree on rename/move (mirrors the tree).
-- name: RewriteSubtreePaths :exec
UPDATE nodes
SET disk_path = sqlc.arg(new_prefix)::text || substring(disk_path FROM char_length(sqlc.arg(old_prefix)::text) + 1)
WHERE user_id = sqlc.arg(user_id)
  AND (disk_path = sqlc.arg(old_prefix)::text OR disk_path LIKE sqlc.arg(old_prefix)::text || '/%');

-- Like RewriteSubtreePaths, but only for trashed nodes — so restoring doesn't
-- touch a LIVE node sharing the same disk_path (the name was reused after deletion).
-- name: RewriteTombstonedSubtreePaths :exec
UPDATE nodes
SET disk_path = sqlc.arg(new_prefix)::text || substring(disk_path FROM char_length(sqlc.arg(old_prefix)::text) + 1)
WHERE user_id = sqlc.arg(user_id)
  AND deleted_at IS NOT NULL
  AND (disk_path = sqlc.arg(old_prefix)::text OR disk_path LIKE sqlc.arg(old_prefix)::text || '/%');

-- Soft-delete a node and its whole subtree (disk is cleaned up by GC, step 0.6).
-- name: SoftDeleteSubtree :exec
UPDATE nodes
SET deleted_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND (disk_path = sqlc.arg(prefix)::text OR disk_path LIKE sqlc.arg(prefix)::text || '/%')
  AND deleted_at IS NULL;

-- name: NextChangeSeq :one
UPDATE users SET change_seq = change_seq + 1 WHERE id = $1 RETURNING change_seq;

-- name: AppendChange :one
INSERT INTO change_log (user_id, node_id, seq, op, version, device_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- Delta sync: changes after seq, with the node's current state.
-- LIMIT — pagination: large deltas aren't returned in a single chunk (3.1).
-- content_hash — lets the client tell a real change from a touch.
-- name: ListChangesSince :many
SELECT cl.seq, cl.op, cl.version, cl.created_at,
       n.id AS node_id, n.name, n.parent_id, n.is_dir, n.size, n.content_hash, n.disk_path,
       (n.deleted_at IS NOT NULL)::bool AS deleted
FROM change_log cl
JOIN nodes n ON n.id = cl.node_id
WHERE cl.user_id = $1 AND cl.seq > $2
ORDER BY cl.seq
LIMIT sqlc.arg(lim);

-- name: ListChangesSinceUnderPrefix :many
SELECT cl.seq, cl.op, cl.version, cl.created_at,
       n.id AS node_id, n.name, n.parent_id, n.is_dir, n.size, n.content_hash, n.disk_path,
       (n.deleted_at IS NOT NULL)::bool AS deleted
FROM change_log cl
JOIN nodes n ON n.id = cl.node_id
WHERE cl.user_id = $1 AND cl.seq > $2 AND n.disk_path LIKE sqlc.arg(prefix)::text ESCAPE '\'
ORDER BY cl.seq
LIMIT sqlc.arg(lim);

-- name: InsertFileVersion :one
INSERT INTO file_versions (
    node_id, version, content_hash, disk_path, size, device_id, is_conflict_loser
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListFileVersions :many
SELECT * FROM file_versions WHERE node_id = $1 ORDER BY version DESC;

-- name: GetFileVersion :one
SELECT * FROM file_versions WHERE node_id = $1 AND version = $2;

-- Nodes that have more than keep versions (trimming candidates).
-- name: ListNodesWithExcessVersions :many
SELECT node_id FROM file_versions GROUP BY node_id HAVING count(*) > sqlc.arg(keep);

-- Delete versions beyond the keep newest; return snapshot paths for disk cleanup.
-- name: TrimNodeVersions :many
DELETE FROM file_versions
WHERE id IN (
    SELECT fv.id FROM file_versions fv
    WHERE fv.node_id = sqlc.arg(nid)
    ORDER BY fv.version DESC OFFSET sqlc.arg(keep)
)
RETURNING disk_path;

-- name: SetSharePasswordHash :exec
UPDATE resource_shares SET share_password_hash = $2 WHERE id = $1;

-- name: CreateShare :one
INSERT INTO resource_shares (
    resource_type, resource_id, owner_id, shared_with_user, share_link_token, access, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListSharesForResource :many
SELECT * FROM resource_shares WHERE resource_type = $1 AND resource_id = $2;

-- name: GetShare :one
SELECT * FROM resource_shares WHERE id = $1;

-- name: DeleteShare :exec
DELETE FROM resource_shares WHERE id = $1;

-- name: GetActiveShareByToken :one
SELECT * FROM resource_shares
WHERE share_link_token = sqlc.arg(token)::text
  AND (expires_at IS NULL OR expires_at > now());

-- name: ListSharesForUser :many
SELECT * FROM resource_shares
WHERE shared_with_user = $1 AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC;

-- A user's access level to a file_node, accounting for inheritance: we check
-- the node itself AND all its ancestors (recursive CTE), taking active shares for the user.
-- name: CalendarShareForUser :one
SELECT id FROM resource_shares
WHERE resource_type = 'calendar' AND resource_id = $1
  AND shared_with_user = $2 AND (expires_at IS NULL OR expires_at > now())
LIMIT 1;

-- name: AddressbookShareForUser :one
SELECT id FROM resource_shares
WHERE resource_type = 'addressbook' AND resource_id = $1
  AND shared_with_user = $2 AND (expires_at IS NULL OR expires_at > now())
LIMIT 1;

-- name: SharedAccessForUser :one
WITH RECURSIVE chain(node_id, parent_id) AS (
    SELECT n.id, n.parent_id FROM nodes n WHERE n.id = sqlc.arg(start_id)
    UNION ALL
    SELECT n.id, n.parent_id FROM nodes n JOIN chain c ON n.id = c.parent_id
)
SELECT
    COALESCE(bool_or(rs.access = 'read_write'), false)::bool AS can_write,
    (count(rs.id) > 0)::bool AS can_read
FROM resource_shares rs
JOIN chain c ON rs.resource_id = c.node_id
WHERE rs.resource_type = 'file_node'
  AND rs.shared_with_user = sqlc.arg(user_id)
  AND (rs.expires_at IS NULL OR rs.expires_at > now());

-- name: UpsertSetting :exec
INSERT INTO settings (key, value, is_secret, updated_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    is_secret = EXCLUDED.is_secret,
    updated_by = EXCLUDED.updated_by,
    updated_at = now();

-- name: GetSetting :one
SELECT * FROM settings WHERE key = $1;

-- name: ListPublicSettings :many
SELECT * FROM settings WHERE is_secret = false ORDER BY key;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = $1;

-- name: GetTrashedNodeForUser :one
SELECT * FROM nodes WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL;

-- name: ListTrashNodes :many
SELECT n.* FROM nodes n
WHERE n.user_id = $1 AND n.deleted_at IS NOT NULL
  AND (n.parent_id IS NULL OR EXISTS (
        SELECT 1 FROM nodes p WHERE p.id = n.parent_id AND p.user_id = n.user_id AND p.deleted_at IS NULL))
ORDER BY n.deleted_at DESC;

-- name: UndeleteSubtree :exec
UPDATE nodes SET deleted_at = NULL
WHERE user_id = sqlc.arg(user_id)
  AND (disk_path = sqlc.arg(prefix)::text OR disk_path LIKE sqlc.arg(prefix)::text || '/%')
  AND deleted_at IS NOT NULL;

-- name: ListTrashedSubtree :many
SELECT id, disk_path, is_dir FROM nodes
WHERE user_id = sqlc.arg(user_id)
  AND (disk_path = sqlc.arg(prefix)::text OR disk_path LIKE sqlc.arg(prefix)::text || '/%')
  AND deleted_at IS NOT NULL;

-- name: HardDeleteSubtree :exec
DELETE FROM nodes
WHERE user_id = sqlc.arg(user_id)
  AND (disk_path = sqlc.arg(prefix)::text OR disk_path LIKE sqlc.arg(prefix)::text || '/%')
  AND deleted_at IS NOT NULL;

-- name: ListSecretKeys :many
SELECT key FROM settings WHERE is_secret = true ORDER BY key;

-- name: ListNotificationPrefs :many
SELECT event_key, channel, enabled FROM notification_prefs WHERE user_id = $1;

-- name: NotificationPrefsForEvent :many
SELECT channel, enabled FROM notification_prefs WHERE user_id = $1 AND event_key = $2;

-- name: UpsertNotificationPref :exec
INSERT INTO notification_prefs (user_id, event_key, channel, enabled)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, event_key, channel) DO UPDATE SET enabled = EXCLUDED.enabled;

-- name: CountUserLogins :one
SELECT count(*) FROM known_logins WHERE user_id = $1;

-- name: UpsertKnownLogin :one
INSERT INTO known_logins (user_id, fingerprint, user_agent, ip)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, fingerprint) DO UPDATE SET last_seen = now(), ip = EXCLUDED.ip
RETURNING (xmax = 0)::bool AS inserted;

-- name: ListQuotaCandidates :many
SELECT id, email, storage_used, storage_quota FROM users
WHERE storage_quota IS NOT NULL
  AND storage_used * 10 >= storage_quota * 9
  AND quota_notified_at IS NULL;

-- name: MarkQuotaNotified :exec
UPDATE users SET quota_notified_at = now() WHERE id = $1;

-- name: ClearQuotaNotified :exec
UPDATE users SET quota_notified_at = NULL
WHERE storage_quota IS NOT NULL
  AND storage_used * 10 < storage_quota * 9
  AND quota_notified_at IS NOT NULL;

-- == calendars ==
-- name: CreateCalendar :one
INSERT INTO calendars (user_id, uri, name, color) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: CreateCalendarWithComponents :one
INSERT INTO calendars (user_id, uri, name, color, components) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ListCalendars :many
SELECT * FROM calendars WHERE user_id = $1 ORDER BY created_at;

-- name: GetCalendar :one
SELECT * FROM calendars WHERE id = $1;

-- name: GetCalendarByURI :one
SELECT * FROM calendars WHERE uri = $1;

-- name: CountCalendars :one
SELECT count(*) FROM calendars WHERE user_id = $1;

-- name: SetCalendarName :exec
UPDATE calendars SET name = $2 WHERE id = $1 AND user_id = $3;

-- name: SetCalendarColor :exec
UPDATE calendars SET color = $2 WHERE id = $1 AND user_id = $3;

-- name: BumpCalendarCtag :exec
UPDATE calendars SET ctag = ctag + 1 WHERE id = $1;

-- name: DeleteCalendar :exec
DELETE FROM calendars WHERE id = $1 AND user_id = $2;

-- == calendar_objects ==
-- name: UpsertCalendarObject :one
INSERT INTO calendar_objects (calendar_id, uid, data, etag, parsed)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (calendar_id, uid) DO UPDATE
SET data = EXCLUDED.data, etag = EXCLUDED.etag, parsed = EXCLUDED.parsed, updated_at = now()
RETURNING *;

-- name: GetCalendarObject :one
SELECT * FROM calendar_objects WHERE calendar_id = $1 AND uid = $2;

-- name: ListCalendarObjects :many
SELECT * FROM calendar_objects WHERE calendar_id = $1 ORDER BY uid;

-- name: DeleteCalendarObject :execrows
DELETE FROM calendar_objects WHERE calendar_id = $1 AND uid = $2;

-- == addressbooks ==
-- name: CreateAddressbook :one
INSERT INTO addressbooks (user_id, uri, name) VALUES ($1, $2, $3) RETURNING *;

-- name: ListAddressbooks :many
SELECT * FROM addressbooks WHERE user_id = $1 ORDER BY created_at;

-- name: GetAddressbook :one
SELECT * FROM addressbooks WHERE id = $1;

-- name: GetAddressbookByURI :one
SELECT * FROM addressbooks WHERE uri = $1;

-- name: CountAddressbooks :one
SELECT count(*) FROM addressbooks WHERE user_id = $1;

-- name: SetAddressbookName :exec
UPDATE addressbooks SET name = $2 WHERE id = $1 AND user_id = $3;

-- name: BumpAddressbookCtag :exec
UPDATE addressbooks SET ctag = ctag + 1 WHERE id = $1;

-- name: DeleteAddressbook :exec
DELETE FROM addressbooks WHERE id = $1 AND user_id = $2;

-- == addressbook_objects ==
-- name: UpsertAddressbookObject :one
INSERT INTO addressbook_objects (addressbook_id, uid, data, etag, parsed)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (addressbook_id, uid) DO UPDATE
SET data = EXCLUDED.data, etag = EXCLUDED.etag, parsed = EXCLUDED.parsed, updated_at = now()
RETURNING *;

-- name: GetAddressbookObject :one
SELECT * FROM addressbook_objects WHERE addressbook_id = $1 AND uid = $2;

-- name: ListAddressbookObjects :many
SELECT * FROM addressbook_objects WHERE addressbook_id = $1 ORDER BY uid;

-- name: DeleteAddressbookObject :execrows
DELETE FROM addressbook_objects WHERE addressbook_id = $1 AND uid = $2;

-- Device pairing (device authorization flow, step 3.1.2).

-- name: CreatePairing :one
INSERT INTO device_pairings (device_code_hash, user_code, proposed_name, kind, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPairingByCodeHash :one
SELECT * FROM device_pairings WHERE device_code_hash = $1;

-- name: GetPairingByUserCode :one
SELECT * FROM device_pairings WHERE user_code = $1;

-- name: ApprovePairing :one
UPDATE device_pairings SET status = 'approved', user_id = $2, device_id = $3
WHERE id = $1 AND status = 'pending'
RETURNING id;

-- Atomically "claim" an approved pairing: a race between two polls → token exactly once.
-- name: ConsumePairingIfApproved :one
UPDATE device_pairings SET status = 'consumed'
WHERE id = $1 AND status = 'approved'
RETURNING id;

-- name: DeleteExpiredPairings :exec
DELETE FROM device_pairings WHERE expires_at < now();

-- name: CreateDesktopDevice :one
INSERT INTO devices (user_id, name, kind) VALUES ($1, $2, 'desktop')
RETURNING *;

-- name: SetDeviceTokenHash :exec
UPDATE devices SET token_hash = $2 WHERE id = $1;

-- name: GetDeviceByTokenHash :one
SELECT * FROM devices WHERE token_hash = $1;

-- name: TouchDevice :exec
UPDATE devices SET last_seen_at = now() WHERE id = $1;

-- name: AvailableMFAFactors :one
-- Which interactive second factors the user has. Used by Login to branch.
SELECT
    EXISTS(SELECT 1 FROM user_totp ut WHERE ut.user_id = $1 AND ut.enabled) AS has_totp,
    EXISTS(SELECT 1 FROM webauthn_credentials wc WHERE wc.user_id = $1) AS has_webauthn;

-- TOTP 2FA (A.3). Secret is AES-GCM ciphertext.

-- name: UpsertUserTOTP :exec
-- Begin (or restart) TOTP setup: store an encrypted secret, not yet confirmed.
INSERT INTO user_totp (user_id, secret, enabled, confirmed_at)
VALUES ($1, $2, false, NULL)
ON CONFLICT (user_id) DO UPDATE SET secret = EXCLUDED.secret, enabled = false, confirmed_at = NULL, created_at = now();

-- name: GetUserTOTP :one
SELECT * FROM user_totp WHERE user_id = $1;

-- name: ConfirmUserTOTP :exec
UPDATE user_totp SET enabled = true, confirmed_at = now() WHERE user_id = $1;

-- name: DeleteUserTOTP :exec
DELETE FROM user_totp WHERE user_id = $1;

-- Backup codes (A.3). One-time, argon2id-hashed.

-- name: InsertBackupCode :exec
INSERT INTO backup_codes (user_id, code_hash) VALUES ($1, $2);

-- name: DeleteBackupCodes :exec
DELETE FROM backup_codes WHERE user_id = $1;

-- name: ListUnusedBackupCodes :many
SELECT * FROM backup_codes WHERE user_id = $1 AND used_at IS NULL;

-- name: MarkBackupCodeUsed :exec
UPDATE backup_codes SET used_at = now() WHERE id = $1;

-- WebAuthn credentials (A.4). The full go-webauthn Credential is stored as JSON.

-- name: InsertWebAuthnCredential :one
INSERT INTO webauthn_credentials (user_id, credential_id, credential, name)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ListWebAuthnCredentials :many
SELECT * FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at;

-- name: RenameWebAuthnCredential :exec
UPDATE webauthn_credentials SET name = $3 WHERE id = $1 AND user_id = $2;

-- name: DeleteWebAuthnCredential :exec
DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2;

-- name: UpdateWebAuthnCredential :exec
-- A.5: persist the credential after a login (sign_count bumps; clone detection) + last used.
UPDATE webauthn_credentials SET credential = $2, last_used_at = now() WHERE credential_id = $1;

-- Audit log (A.7).

-- name: InsertAuditLog :exec
INSERT INTO audit_log (user_id, event, ip, user_agent, detail) VALUES ($1, $2, $3, $4, $5);

-- name: ListAuditLog :many
SELECT * FROM audit_log WHERE user_id = $1 ORDER BY id DESC LIMIT $2;
