ALTER TABLE users DROP CONSTRAINT IF EXISTS users_session_ttl_check;
ALTER TABLE users DROP COLUMN IF EXISTS session_ttl_minutes;
