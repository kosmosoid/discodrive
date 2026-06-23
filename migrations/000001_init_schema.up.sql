-- Initial DiscoDrive schema (squashed from the pre-release migration history).

CREATE FUNCTION notify_change_log() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    PERFORM pg_notify('change_log', NEW.user_id::text || ':' || NEW.seq::text);
    RETURN NEW;
END;
$$;

CREATE TABLE addressbook_objects (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    addressbook_id uuid NOT NULL,
    uid text NOT NULL,
    data text NOT NULL,
    etag text NOT NULL,
    parsed jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE addressbooks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    uri text NOT NULL,
    name text NOT NULL,
    ctag bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE albums (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    artist_id uuid,
    name text NOT NULL,
    year integer,
    genre text,
    cover_art text,
    song_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    musicbrainz_id text
);

CREATE TABLE artists (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    sort_name text DEFAULT ''::text NOT NULL,
    cover_art text,
    musicbrainz_id text
);

CREATE TABLE audit_log (
    id bigint NOT NULL,
    user_id uuid NOT NULL,
    event text NOT NULL,
    ip text DEFAULT ''::text NOT NULL,
    user_agent text DEFAULT ''::text NOT NULL,
    detail jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE SEQUENCE audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE audit_log_id_seq OWNED BY audit_log.id;

CREATE TABLE backup_codes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    code_hash text NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE book_authors (
    book_id uuid NOT NULL,
    name text NOT NULL,
    sort_name text DEFAULT ''::text NOT NULL
);

CREATE TABLE book_tags (
    book_id uuid NOT NULL,
    tag text NOT NULL
);

CREATE TABLE bookmarks (
    user_id uuid NOT NULL,
    item_id uuid NOT NULL,
    item_type text NOT NULL,
    position_ms bigint DEFAULT 0 NOT NULL,
    comment text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    changed_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT bookmarks_item_type_check CHECK ((item_type = ANY (ARRAY['song'::text, 'episode'::text])))
);

CREATE TABLE books (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    node_id uuid NOT NULL,
    title text NOT NULL,
    sort_title text DEFAULT ''::text NOT NULL,
    language text,
    isbn text,
    description text,
    publisher text,
    published_date text,
    series text,
    series_index real,
    format text NOT NULL,
    content_type text NOT NULL,
    size bigint,
    cover_path text,
    added_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    metadata_edited boolean DEFAULT false NOT NULL
);

CREATE TABLE calendar_objects (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    calendar_id uuid NOT NULL,
    uid text NOT NULL,
    data text NOT NULL,
    etag text NOT NULL,
    parsed jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE calendars (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    uri text NOT NULL,
    name text NOT NULL,
    color text DEFAULT ''::text NOT NULL,
    ctag bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    components text DEFAULT 'VEVENT'::text NOT NULL
);

CREATE TABLE change_log (
    id bigint NOT NULL,
    user_id uuid NOT NULL,
    node_id uuid NOT NULL,
    seq bigint NOT NULL,
    op text NOT NULL,
    version bigint NOT NULL,
    device_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT change_log_op_check CHECK ((op = ANY (ARRAY['create'::text, 'update'::text, 'move'::text, 'delete'::text])))
);

CREATE SEQUENCE change_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE change_log_id_seq OWNED BY change_log.id;

CREATE TABLE device_pairings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    device_code_hash text NOT NULL,
    user_code text NOT NULL,
    proposed_name text NOT NULL,
    kind text DEFAULT 'desktop'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    user_id uuid,
    device_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT device_pairings_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'consumed'::text])))
);

CREATE TABLE devices (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    last_seen_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    secret_hash text,
    token_hash text,
    CONSTRAINT devices_kind_check CHECK ((kind = ANY (ARRAY['macos'::text, 'ios'::text, 'webdav'::text, 'web'::text, 'desktop'::text])))
);

CREATE TABLE ebook_settings (
    user_id uuid NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    folder_node_id uuid,
    password_cipher text,
    api_key text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE file_versions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    node_id uuid NOT NULL,
    version bigint NOT NULL,
    content_hash text,
    disk_path text,
    size bigint,
    device_id uuid,
    is_conflict_loser boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE internet_radio_stations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    stream_url text NOT NULL,
    homepage_url text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE known_logins (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    fingerprint text NOT NULL,
    user_agent text NOT NULL,
    ip text NOT NULL,
    first_seen timestamp with time zone DEFAULT now() NOT NULL,
    last_seen timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE music_settings (
    user_id uuid NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    folder_node_id uuid,
    password_cipher text,
    api_key text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    tag_edit_versioning boolean DEFAULT true NOT NULL
);

CREATE TABLE nodes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    parent_id uuid,
    name text NOT NULL,
    is_dir boolean DEFAULT false NOT NULL,
    size bigint,
    content_hash text,
    disk_path text,
    mime text,
    is_vault boolean DEFAULT false NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    modified_at timestamp with time zone DEFAULT now() NOT NULL,
    modified_by uuid,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    is_conflict_loser boolean DEFAULT false NOT NULL,
    conflict_of uuid
);

CREATE TABLE notification_prefs (
    user_id uuid NOT NULL,
    event_key text NOT NULL,
    channel text NOT NULL,
    enabled boolean NOT NULL
);

CREATE TABLE play_history (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    song_id uuid NOT NULL,
    played_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE play_queue_entries (
    user_id uuid NOT NULL,
    idx integer NOT NULL,
    song_id uuid NOT NULL
);

CREATE TABLE play_queues (
    user_id uuid NOT NULL,
    current_id uuid,
    position_ms bigint DEFAULT 0 NOT NULL,
    changed_by text DEFAULT ''::text NOT NULL,
    changed_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE playlist_songs (
    playlist_id uuid NOT NULL,
    song_id uuid NOT NULL,
    "position" integer NOT NULL
);

CREATE TABLE playlists (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    comment text DEFAULT ''::text NOT NULL,
    public boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    changed_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE podcast_channels (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    feed_url text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    cover_url text DEFAULT ''::text NOT NULL,
    cover_path text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_fetched_at timestamp with time zone
);

CREATE TABLE podcast_episodes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    channel_id uuid NOT NULL,
    user_id uuid NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    pub_date timestamp with time zone,
    audio_url text NOT NULL,
    duration integer,
    suffix text DEFAULT ''::text NOT NULL,
    content_type text DEFAULT ''::text NOT NULL,
    size bigint,
    status text DEFAULT 'new'::text NOT NULL,
    disk_path text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT podcast_episodes_status_check CHECK ((status = ANY (ARRAY['new'::text, 'downloading'::text, 'completed'::text, 'error'::text, 'skipped'::text])))
);

CREATE TABLE ratings (
    user_id uuid NOT NULL,
    item_id uuid NOT NULL,
    item_type text NOT NULL,
    rating integer NOT NULL,
    CONSTRAINT ratings_item_type_check CHECK ((item_type = ANY (ARRAY['song'::text, 'album'::text, 'artist'::text]))),
    CONSTRAINT ratings_rating_check CHECK (((rating >= 1) AND (rating <= 5)))
);

CREATE TABLE reading_progress (
    user_id uuid NOT NULL,
    document text NOT NULL,
    progress text DEFAULT ''::text NOT NULL,
    percentage real DEFAULT 0 NOT NULL,
    device text DEFAULT ''::text NOT NULL,
    device_id text DEFAULT ''::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE resource_shares (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid NOT NULL,
    owner_id uuid NOT NULL,
    shared_with_user uuid,
    share_link_token text,
    access text NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    share_password_hash text,
    CONSTRAINT resource_shares_access_check CHECK ((access = ANY (ARRAY['read'::text, 'read_write'::text]))),
    CONSTRAINT resource_shares_one_subject CHECK (((shared_with_user IS NOT NULL) <> (share_link_token IS NOT NULL))),
    CONSTRAINT resource_shares_resource_type_check CHECK ((resource_type = ANY (ARRAY['file_node'::text, 'calendar'::text, 'addressbook'::text])))
);

CREATE TABLE settings (
    key text NOT NULL,
    value text NOT NULL,
    is_secret boolean DEFAULT false NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_by uuid
);

CREATE TABLE songs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    album_id uuid,
    artist_id uuid,
    node_id uuid NOT NULL,
    title text NOT NULL,
    track integer,
    disc integer,
    duration integer,
    bitrate integer,
    suffix text,
    content_type text,
    size bigint,
    genre text,
    musicbrainz_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE stars (
    user_id uuid NOT NULL,
    item_id uuid NOT NULL,
    item_type text NOT NULL,
    starred_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT stars_item_type_check CHECK ((item_type = ANY (ARRAY['song'::text, 'album'::text, 'artist'::text])))
);

CREATE TABLE sync_settings (
    user_id uuid NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    folder_node_id uuid,
    epoch bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE tenants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE user_totp (
    user_id uuid NOT NULL,
    secret text NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    confirmed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    email text NOT NULL,
    password_hash text NOT NULL,
    storage_quota bigint,
    storage_used bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    role text DEFAULT 'user'::text NOT NULL,
    change_seq bigint DEFAULT 0 NOT NULL,
    quota_notified_at timestamp with time zone,
    token_version bigint DEFAULT 0 NOT NULL,
    language text DEFAULT 'en'::text NOT NULL,
    must_change_password boolean DEFAULT false NOT NULL,
    CONSTRAINT users_role_check CHECK ((role = ANY (ARRAY['admin'::text, 'user'::text])))
);

CREATE TABLE webauthn_credentials (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    credential_id bytea NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone,
    credential jsonb NOT NULL
);

ALTER TABLE ONLY audit_log ALTER COLUMN id SET DEFAULT nextval('audit_log_id_seq'::regclass);

ALTER TABLE ONLY change_log ALTER COLUMN id SET DEFAULT nextval('change_log_id_seq'::regclass);

ALTER TABLE ONLY addressbook_objects
    ADD CONSTRAINT addressbook_objects_addressbook_id_uid_key UNIQUE (addressbook_id, uid);

ALTER TABLE ONLY addressbook_objects
    ADD CONSTRAINT addressbook_objects_pkey PRIMARY KEY (id);

ALTER TABLE ONLY addressbooks
    ADD CONSTRAINT addressbooks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY addressbooks
    ADD CONSTRAINT addressbooks_uri_key UNIQUE (uri);

ALTER TABLE ONLY albums
    ADD CONSTRAINT albums_pkey PRIMARY KEY (id);

ALTER TABLE ONLY albums
    ADD CONSTRAINT albums_user_id_artist_id_name_key UNIQUE (user_id, artist_id, name);

ALTER TABLE ONLY artists
    ADD CONSTRAINT artists_pkey PRIMARY KEY (id);

ALTER TABLE ONLY artists
    ADD CONSTRAINT artists_user_id_name_key UNIQUE (user_id, name);

ALTER TABLE ONLY audit_log
    ADD CONSTRAINT audit_log_pkey PRIMARY KEY (id);

ALTER TABLE ONLY backup_codes
    ADD CONSTRAINT backup_codes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY book_authors
    ADD CONSTRAINT book_authors_pkey PRIMARY KEY (book_id, name);

ALTER TABLE ONLY book_tags
    ADD CONSTRAINT book_tags_pkey PRIMARY KEY (book_id, tag);

ALTER TABLE ONLY bookmarks
    ADD CONSTRAINT bookmarks_pkey PRIMARY KEY (user_id, item_id, item_type);

ALTER TABLE ONLY books
    ADD CONSTRAINT books_node_id_key UNIQUE (node_id);

ALTER TABLE ONLY books
    ADD CONSTRAINT books_pkey PRIMARY KEY (id);

ALTER TABLE ONLY calendar_objects
    ADD CONSTRAINT calendar_objects_calendar_id_uid_key UNIQUE (calendar_id, uid);

ALTER TABLE ONLY calendar_objects
    ADD CONSTRAINT calendar_objects_pkey PRIMARY KEY (id);

ALTER TABLE ONLY calendars
    ADD CONSTRAINT calendars_pkey PRIMARY KEY (id);

ALTER TABLE ONLY calendars
    ADD CONSTRAINT calendars_uri_key UNIQUE (uri);

ALTER TABLE ONLY change_log
    ADD CONSTRAINT change_log_pkey PRIMARY KEY (id);

ALTER TABLE ONLY device_pairings
    ADD CONSTRAINT device_pairings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY device_pairings
    ADD CONSTRAINT device_pairings_user_code_key UNIQUE (user_code);

ALTER TABLE ONLY devices
    ADD CONSTRAINT devices_pkey PRIMARY KEY (id);

ALTER TABLE ONLY ebook_settings
    ADD CONSTRAINT ebook_settings_api_key_key UNIQUE (api_key);

ALTER TABLE ONLY ebook_settings
    ADD CONSTRAINT ebook_settings_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY file_versions
    ADD CONSTRAINT file_versions_node_id_version_key UNIQUE (node_id, version);

ALTER TABLE ONLY file_versions
    ADD CONSTRAINT file_versions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY internet_radio_stations
    ADD CONSTRAINT internet_radio_stations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY known_logins
    ADD CONSTRAINT known_logins_pkey PRIMARY KEY (id);

ALTER TABLE ONLY known_logins
    ADD CONSTRAINT known_logins_user_id_fingerprint_key UNIQUE (user_id, fingerprint);

ALTER TABLE ONLY music_settings
    ADD CONSTRAINT music_settings_api_key_key UNIQUE (api_key);

ALTER TABLE ONLY music_settings
    ADD CONSTRAINT music_settings_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY nodes
    ADD CONSTRAINT nodes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY notification_prefs
    ADD CONSTRAINT notification_prefs_pkey PRIMARY KEY (user_id, event_key, channel);

ALTER TABLE ONLY play_history
    ADD CONSTRAINT play_history_pkey PRIMARY KEY (id);

ALTER TABLE ONLY play_queue_entries
    ADD CONSTRAINT play_queue_entries_pkey PRIMARY KEY (user_id, idx);

ALTER TABLE ONLY play_queues
    ADD CONSTRAINT play_queues_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY playlist_songs
    ADD CONSTRAINT playlist_songs_pkey PRIMARY KEY (playlist_id, "position");

ALTER TABLE ONLY playlists
    ADD CONSTRAINT playlists_pkey PRIMARY KEY (id);

ALTER TABLE ONLY podcast_channels
    ADD CONSTRAINT podcast_channels_pkey PRIMARY KEY (id);

ALTER TABLE ONLY podcast_channels
    ADD CONSTRAINT podcast_channels_user_id_feed_url_key UNIQUE (user_id, feed_url);

ALTER TABLE ONLY podcast_episodes
    ADD CONSTRAINT podcast_episodes_channel_id_audio_url_key UNIQUE (channel_id, audio_url);

ALTER TABLE ONLY podcast_episodes
    ADD CONSTRAINT podcast_episodes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY ratings
    ADD CONSTRAINT ratings_pkey PRIMARY KEY (user_id, item_id, item_type);

ALTER TABLE ONLY reading_progress
    ADD CONSTRAINT reading_progress_pkey PRIMARY KEY (user_id, document);

ALTER TABLE ONLY resource_shares
    ADD CONSTRAINT resource_shares_pkey PRIMARY KEY (id);

ALTER TABLE ONLY resource_shares
    ADD CONSTRAINT resource_shares_share_link_token_key UNIQUE (share_link_token);

ALTER TABLE ONLY settings
    ADD CONSTRAINT settings_pkey PRIMARY KEY (key);

ALTER TABLE ONLY songs
    ADD CONSTRAINT songs_node_id_key UNIQUE (node_id);

ALTER TABLE ONLY songs
    ADD CONSTRAINT songs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY stars
    ADD CONSTRAINT stars_pkey PRIMARY KEY (user_id, item_id, item_type);

ALTER TABLE ONLY sync_settings
    ADD CONSTRAINT sync_settings_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY tenants
    ADD CONSTRAINT tenants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY user_totp
    ADD CONSTRAINT user_totp_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_email_key UNIQUE (email);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY webauthn_credentials
    ADD CONSTRAINT webauthn_credentials_credential_id_key UNIQUE (credential_id);

ALTER TABLE ONLY webauthn_credentials
    ADD CONSTRAINT webauthn_credentials_pkey PRIMARY KEY (id);

CREATE INDEX addressbook_objects_parsed_gin ON addressbook_objects USING gin (parsed);

CREATE INDEX albums_user_artist ON albums USING btree (user_id, artist_id);

CREATE INDEX artists_user_sort ON artists USING btree (user_id, sort_name);

CREATE INDEX audit_log_user ON audit_log USING btree (user_id, id DESC);

CREATE INDEX backup_codes_user ON backup_codes USING btree (user_id);

CREATE INDEX book_authors_name ON book_authors USING btree (name);

CREATE INDEX book_tags_tag ON book_tags USING btree (tag);

CREATE INDEX books_node ON books USING btree (node_id);

CREATE INDEX books_user_series ON books USING btree (user_id, series, series_index);

CREATE INDEX books_user_sort ON books USING btree (user_id, sort_title);

CREATE INDEX calendar_objects_parsed_gin ON calendar_objects USING gin (parsed);

CREATE UNIQUE INDEX change_log_user_seq ON change_log USING btree (user_id, seq);

CREATE INDEX device_pairings_code_hash ON device_pairings USING btree (device_code_hash);

CREATE INDEX devices_token_hash ON devices USING btree (token_hash) WHERE (token_hash IS NOT NULL);

CREATE INDEX file_versions_node ON file_versions USING btree (node_id);

CREATE INDEX internet_radio_user ON internet_radio_stations USING btree (user_id);

CREATE INDEX nodes_children ON nodes USING btree (user_id, parent_id) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX nodes_uniq_name_in_parent ON nodes USING btree (user_id, parent_id, name) WHERE ((deleted_at IS NULL) AND (parent_id IS NOT NULL));

CREATE UNIQUE INDEX nodes_uniq_name_in_root ON nodes USING btree (user_id, name) WHERE ((deleted_at IS NULL) AND (parent_id IS NULL));

CREATE INDEX play_history_user_time ON play_history USING btree (user_id, played_at DESC);

CREATE INDEX playlists_user ON playlists USING btree (user_id);

CREATE INDEX podcast_channels_user ON podcast_channels USING btree (user_id);

CREATE INDEX podcast_episodes_channel ON podcast_episodes USING btree (channel_id);

CREATE INDEX podcast_episodes_user_created ON podcast_episodes USING btree (user_id, created_at DESC);

CREATE INDEX resource_shares_resource ON resource_shares USING btree (resource_type, resource_id);

CREATE INDEX resource_shares_subject ON resource_shares USING btree (shared_with_user);

CREATE INDEX songs_node ON songs USING btree (node_id);

CREATE INDEX songs_user_album ON songs USING btree (user_id, album_id);

CREATE INDEX webauthn_credentials_user ON webauthn_credentials USING btree (user_id);

CREATE TRIGGER change_log_notify AFTER INSERT ON change_log FOR EACH ROW EXECUTE FUNCTION notify_change_log();

ALTER TABLE ONLY addressbook_objects
    ADD CONSTRAINT addressbook_objects_addressbook_id_fkey FOREIGN KEY (addressbook_id) REFERENCES addressbooks(id) ON DELETE CASCADE;

ALTER TABLE ONLY addressbooks
    ADD CONSTRAINT addressbooks_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY albums
    ADD CONSTRAINT albums_artist_id_fkey FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE SET NULL;

ALTER TABLE ONLY albums
    ADD CONSTRAINT albums_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY artists
    ADD CONSTRAINT artists_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY audit_log
    ADD CONSTRAINT audit_log_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY backup_codes
    ADD CONSTRAINT backup_codes_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY book_authors
    ADD CONSTRAINT book_authors_book_id_fkey FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE;

ALTER TABLE ONLY book_tags
    ADD CONSTRAINT book_tags_book_id_fkey FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE;

ALTER TABLE ONLY bookmarks
    ADD CONSTRAINT bookmarks_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY books
    ADD CONSTRAINT books_node_id_fkey FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE;

ALTER TABLE ONLY books
    ADD CONSTRAINT books_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY calendar_objects
    ADD CONSTRAINT calendar_objects_calendar_id_fkey FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE;

ALTER TABLE ONLY calendars
    ADD CONSTRAINT calendars_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY change_log
    ADD CONSTRAINT change_log_device_id_fkey FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE SET NULL;

ALTER TABLE ONLY change_log
    ADD CONSTRAINT change_log_node_id_fkey FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE;

ALTER TABLE ONLY change_log
    ADD CONSTRAINT change_log_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY device_pairings
    ADD CONSTRAINT device_pairings_device_id_fkey FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE SET NULL;

ALTER TABLE ONLY device_pairings
    ADD CONSTRAINT device_pairings_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY devices
    ADD CONSTRAINT devices_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY ebook_settings
    ADD CONSTRAINT ebook_settings_folder_node_id_fkey FOREIGN KEY (folder_node_id) REFERENCES nodes(id) ON DELETE SET NULL;

ALTER TABLE ONLY ebook_settings
    ADD CONSTRAINT ebook_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY file_versions
    ADD CONSTRAINT file_versions_device_id_fkey FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE SET NULL;

ALTER TABLE ONLY file_versions
    ADD CONSTRAINT file_versions_node_id_fkey FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE;

ALTER TABLE ONLY internet_radio_stations
    ADD CONSTRAINT internet_radio_stations_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY known_logins
    ADD CONSTRAINT known_logins_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY music_settings
    ADD CONSTRAINT music_settings_folder_node_id_fkey FOREIGN KEY (folder_node_id) REFERENCES nodes(id) ON DELETE SET NULL;

ALTER TABLE ONLY music_settings
    ADD CONSTRAINT music_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY nodes
    ADD CONSTRAINT nodes_conflict_of_fkey FOREIGN KEY (conflict_of) REFERENCES nodes(id) ON DELETE SET NULL;

ALTER TABLE ONLY nodes
    ADD CONSTRAINT nodes_modified_by_fkey FOREIGN KEY (modified_by) REFERENCES devices(id) ON DELETE SET NULL;

ALTER TABLE ONLY nodes
    ADD CONSTRAINT nodes_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES nodes(id) ON DELETE CASCADE;

ALTER TABLE ONLY nodes
    ADD CONSTRAINT nodes_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY notification_prefs
    ADD CONSTRAINT notification_prefs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY play_history
    ADD CONSTRAINT play_history_song_id_fkey FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE;

ALTER TABLE ONLY play_history
    ADD CONSTRAINT play_history_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY play_queue_entries
    ADD CONSTRAINT play_queue_entries_song_id_fkey FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE;

ALTER TABLE ONLY play_queue_entries
    ADD CONSTRAINT play_queue_entries_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY play_queues
    ADD CONSTRAINT play_queues_current_id_fkey FOREIGN KEY (current_id) REFERENCES songs(id) ON DELETE SET NULL;

ALTER TABLE ONLY play_queues
    ADD CONSTRAINT play_queues_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY playlist_songs
    ADD CONSTRAINT playlist_songs_playlist_id_fkey FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE CASCADE;

ALTER TABLE ONLY playlist_songs
    ADD CONSTRAINT playlist_songs_song_id_fkey FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE;

ALTER TABLE ONLY playlists
    ADD CONSTRAINT playlists_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY podcast_channels
    ADD CONSTRAINT podcast_channels_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY podcast_episodes
    ADD CONSTRAINT podcast_episodes_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES podcast_channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY podcast_episodes
    ADD CONSTRAINT podcast_episodes_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY ratings
    ADD CONSTRAINT ratings_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY reading_progress
    ADD CONSTRAINT reading_progress_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY resource_shares
    ADD CONSTRAINT resource_shares_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY resource_shares
    ADD CONSTRAINT resource_shares_shared_with_user_fkey FOREIGN KEY (shared_with_user) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY settings
    ADD CONSTRAINT settings_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY songs
    ADD CONSTRAINT songs_album_id_fkey FOREIGN KEY (album_id) REFERENCES albums(id) ON DELETE SET NULL;

ALTER TABLE ONLY songs
    ADD CONSTRAINT songs_artist_id_fkey FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE SET NULL;

ALTER TABLE ONLY songs
    ADD CONSTRAINT songs_node_id_fkey FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE;

ALTER TABLE ONLY songs
    ADD CONSTRAINT songs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY stars
    ADD CONSTRAINT stars_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY sync_settings
    ADD CONSTRAINT sync_settings_folder_node_id_fkey FOREIGN KEY (folder_node_id) REFERENCES nodes(id) ON DELETE SET NULL;

ALTER TABLE ONLY sync_settings
    ADD CONSTRAINT sync_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY user_totp
    ADD CONSTRAINT user_totp_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY users
    ADD CONSTRAINT users_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY webauthn_credentials
    ADD CONSTRAINT webauthn_credentials_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

