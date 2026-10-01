-- One row per page of a curriculum PDF. The file name is the SHA-256 of
-- the page bytes, so the lesson view can open a page without opening the book.
-- Lessons, curriculum items, and the original PDF are left alone.

CREATE TABLE pdf_pages (
    id            INTEGER PRIMARY KEY,
    attachment_id INTEGER NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
    page_number   INTEGER NOT NULL,
    content_hash  TEXT NOT NULL,
    stored_path   TEXT NOT NULL,
    content_type  TEXT NOT NULL DEFAULT 'application/pdf',
    UNIQUE (attachment_id, page_number)
);

CREATE INDEX pdf_pages_by_hash ON pdf_pages (content_hash);
