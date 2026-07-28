-- Client-supplied article HTML: the browser extension extracts the readable
-- content from the live DOM (paywalls, SPA pages, bot-walls) and sends it
-- along; the worker then skips the server-side fetch. Cleared once processed.

ALTER TABLE saved_items ADD COLUMN content_html text;
