-- name: GetSyncSettings :one
SELECT * FROM sync_settings WHERE user_id = $1;

-- name: UpsertSyncSettings :one
INSERT INTO sync_settings (user_id, enabled, folder_node_id, epoch, updated_at)
VALUES ($1, $2, $3, 1, now())
ON CONFLICT (user_id) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    folder_node_id = EXCLUDED.folder_node_id,
    epoch = CASE
        WHEN sync_settings.enabled IS DISTINCT FROM EXCLUDED.enabled
          OR sync_settings.folder_node_id IS DISTINCT FROM EXCLUDED.folder_node_id
        THEN sync_settings.epoch + 1
        ELSE sync_settings.epoch
    END,
    updated_at = now()
RETURNING *;
