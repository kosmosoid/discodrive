-- Bookmarks: server-authoritative browser bookmark tree with a per-user
-- monotonic change cursor (users.bookmark_seq, bumped in the same statement as
-- every mutation) and tombstones, so browser extensions can two-way merge-sync.
-- bookmark_gc_seq is the GC watermark: clients whose cursor is older than it
-- must do a full resync (tombstones they missed are physically gone).

ALTER TABLE users ADD COLUMN bookmark_seq bigint DEFAULT 0 NOT NULL;
ALTER TABLE users ADD COLUMN bookmark_gc_seq bigint DEFAULT 0 NOT NULL;

CREATE TABLE browser_bookmarks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    parent_id uuid,
    is_folder boolean DEFAULT false NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    url text DEFAULT ''::text NOT NULL,
    position integer DEFAULT 0 NOT NULL,
    deleted boolean DEFAULT false NOT NULL,
    seq bigint NOT NULL,
    favicon_ext text DEFAULT ''::text NOT NULL,
    favicon_tried_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT browser_bookmarks_folder_url_check CHECK ((NOT is_folder OR url = ''::text))
);

ALTER TABLE ONLY browser_bookmarks
    ADD CONSTRAINT browser_bookmarks_pkey PRIMARY KEY (id);

-- No self-FK on parent_id on purpose: a physical GC delete of a tombstoned
-- folder must not cascade into (or be blocked by) children that a concurrent
-- LWW move just re-parented. Integrity is enforced by the queries.

CREATE INDEX browser_bookmarks_user_parent ON browser_bookmarks USING btree (user_id, parent_id) WHERE (NOT deleted);

CREATE INDEX browser_bookmarks_user_seq ON browser_bookmarks USING btree (user_id, seq);

CREATE INDEX browser_bookmarks_favicon_pending ON browser_bookmarks USING btree (favicon_tried_at) WHERE ((favicon_tried_at IS NULL) AND (NOT deleted) AND (NOT is_folder));

ALTER TABLE ONLY browser_bookmarks
    ADD CONSTRAINT browser_bookmarks_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
