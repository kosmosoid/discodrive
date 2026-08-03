-- Quota enforcement sums a user's occupied space on every write: file nodes (live AND
-- trashed — the trash keeps files on disk for TRASH_DAYS) plus their version snapshots.
-- The existing nodes_children index is partial on deleted_at IS NULL, so it cannot serve
-- that sum; this one covers it, with size in the index so the sum never touches the heap.
CREATE INDEX nodes_user_files ON nodes USING btree (user_id) INCLUDE (size) WHERE (is_dir = false);

-- The versions half of the sum walks file_versions by node; file_versions_node already
-- indexes node_id, this adds the size payload so that half is index-only too.
CREATE INDEX file_versions_node_size ON file_versions USING btree (node_id) INCLUDE (size);
