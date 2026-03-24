-- Migration 001: Initial schema

CREATE TABLE IF NOT EXISTS book (
    book_id     INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT    NOT NULL,
    author      TEXT    NOT NULL DEFAULT '',
    format      TEXT    NOT NULL CHECK (format IN ('pdf', 'epub')),
    source_path TEXT    NOT NULL UNIQUE,
    file_hash   TEXT    NOT NULL,
    tags        TEXT    NOT NULL DEFAULT '[]',
    ingested_at TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS chapter (
    chapter_id    INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id       INTEGER NOT NULL REFERENCES book(book_id) ON DELETE CASCADE,
    title         TEXT    NOT NULL,
    chapter_order INTEGER NOT NULL,
    page_start    INTEGER,
    page_end      INTEGER,
    UNIQUE(book_id, chapter_order)
);

CREATE TABLE IF NOT EXISTS chunk (
    chunk_id          INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id           INTEGER NOT NULL REFERENCES book(book_id) ON DELETE CASCADE,
    chapter_id        INTEGER NOT NULL REFERENCES chapter(chapter_id) ON DELETE CASCADE,
    heading           TEXT    NOT NULL DEFAULT '',
    body              TEXT    NOT NULL,
    char_count        INTEGER NOT NULL,
    page_start        INTEGER,
    page_end          INTEGER,
    chunk_order       INTEGER NOT NULL,
    prev_chunk_id     INTEGER REFERENCES chunk(chunk_id),
    next_chunk_id     INTEGER REFERENCES chunk(chunk_id),
    embedding_version TEXT    NOT NULL DEFAULT '',
    created_at        TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_chunk_book    ON chunk(book_id);
CREATE INDEX IF NOT EXISTS idx_chunk_chapter ON chunk(chapter_id);

CREATE VIRTUAL TABLE IF NOT EXISTS chunk_fts USING fts5(
    heading,
    body,
    content = 'chunk',
    content_rowid = 'chunk_id',
    tokenize = 'trigram'
);

CREATE TRIGGER IF NOT EXISTS chunk_ai AFTER INSERT ON chunk BEGIN
    INSERT INTO chunk_fts(rowid, heading, body)
    VALUES (new.chunk_id, new.heading, new.body);
END;

CREATE TRIGGER IF NOT EXISTS chunk_ad AFTER DELETE ON chunk BEGIN
    INSERT INTO chunk_fts(chunk_fts, rowid, heading, body)
    VALUES ('delete', old.chunk_id, old.heading, old.body);
END;

CREATE TRIGGER IF NOT EXISTS chunk_au AFTER UPDATE ON chunk BEGIN
    INSERT INTO chunk_fts(chunk_fts, rowid, heading, body)
    VALUES ('delete', old.chunk_id, old.heading, old.body);
    INSERT INTO chunk_fts(rowid, heading, body)
    VALUES (new.chunk_id, new.heading, new.body);
END;

CREATE VIRTUAL TABLE IF NOT EXISTS chunk_vec USING vec0(
    chunk_id INTEGER PRIMARY KEY,
    embedding float[768]
);

CREATE TABLE IF NOT EXISTS ingest_log (
    log_id     INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id    INTEGER REFERENCES book(book_id),
    status     TEXT    NOT NULL CHECK (status IN ('started', 'extracted', 'chunked', 'embedded', 'completed', 'failed')),
    message    TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);
