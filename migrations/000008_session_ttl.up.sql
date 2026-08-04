-- How long a web session may stay idle before the user has to sign in again, per user.
-- Sessions slide (every authorized request re-issues the token), so this is an idle
-- timeout, not a hard session length. 0 means the token is issued without an expiry:
-- the user asked never to be logged out. A password change still invalidates it, since
-- that bumps users.token_version.
ALTER TABLE users ADD COLUMN session_ttl_minutes integer DEFAULT 60 NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_session_ttl_check CHECK (session_ttl_minutes >= 0);
