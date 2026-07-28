-- Saved items: read-later articles and server-side downloads created from the
-- browser extension (and the web UI). Downloads are an internal queue: rows are
-- hidden from listings and auto-removed once finished.

CREATE TABLE saved_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    url text NOT NULL,
    kind text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    error_msg text DEFAULT ''::text NOT NULL,
    content_path text,
    size_bytes bigint,
    bytes_done bigint DEFAULT 0 NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT saved_items_kind_check CHECK ((kind = ANY (ARRAY['article'::text, 'download'::text]))),
    CONSTRAINT saved_items_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'done'::text, 'error'::text])))
);

ALTER TABLE ONLY saved_items
    ADD CONSTRAINT saved_items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY saved_items
    ADD CONSTRAINT saved_items_user_id_url_kind_key UNIQUE (user_id, url, kind);

CREATE INDEX saved_items_user_created ON saved_items USING btree (user_id, created_at DESC);

CREATE INDEX saved_items_active ON saved_items USING btree (status) WHERE (status = ANY (ARRAY['pending'::text, 'processing'::text]));

ALTER TABLE ONLY saved_items
    ADD CONSTRAINT saved_items_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
