-- Version snapshots used to be taken of the content a push had just written, so the
-- newest version existed twice: once as the live file, once under .versions. Snapshots
-- are now taken of the content a write REPLACES, which means the newest version is the
-- file itself and a file written once has no snapshot at all.
--
-- Two things follow for data written under the old scheme:

-- 1. A node's own current version is now redundant with the live file. Dropping the row
--    here would leak its file on disk, so the rows stay and the "prune-live-snapshots"
--    worker job removes row and file together (it skips recently modified nodes, so it
--    cannot race a push that is mid-write).

-- 2. The next overwrite of such a node snapshots a version that already has a row.
--    Make that a no-op instead of a duplicate: dedupe what exists, then enforce
--    uniqueness (InsertFileVersion does ON CONFLICT DO NOTHING).
DELETE FROM file_versions a
USING file_versions b
WHERE a.node_id = b.node_id AND a.version = b.version AND a.ctid > b.ctid;

CREATE UNIQUE INDEX file_versions_node_version ON file_versions USING btree (node_id, version);
