-- Cookie pass-through for downloads from login-protected sites: the extension
-- reads the link host's cookies from the browser session and sends them along;
-- the worker attaches them to the download request. Transport, not storage:
-- cleared as soon as the item is done (kept on error so retry still works).

ALTER TABLE saved_items ADD COLUMN cookie_header text;
