package search

const embeddingSchema = `
CREATE TABLE IF NOT EXISTS embeddings (
    symbol_id  INTEGER NOT NULL REFERENCES symbols(id) ON DELETE CASCADE,
    model      TEXT NOT NULL,
    vector     BLOB NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (symbol_id, model)
);
`

const schema = `
CREATE TABLE IF NOT EXISTS files (
    path       TEXT PRIMARY KEY,
    hash       TEXT NOT NULL,
    indexed_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS symbols (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path TEXT NOT NULL REFERENCES files(path) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    kind      TEXT NOT NULL,
    language  TEXT NOT NULL,
    signature TEXT NOT NULL DEFAULT '',
    doc       TEXT NOT NULL DEFAULT '',
    line      INTEGER NOT NULL,
    parent    TEXT NOT NULL DEFAULT '',
    tokens    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file_path);

CREATE VIRTUAL TABLE IF NOT EXISTS symbols_fts USING fts5(
    name,
    signature,
    doc,
    file_path,
    tokens,
    content='symbols',
    content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS symbols_ai AFTER INSERT ON symbols BEGIN
    INSERT INTO symbols_fts(rowid, name, signature, doc, file_path, tokens)
    VALUES (new.id, new.name, new.signature, new.doc, new.file_path, new.tokens);
END;

CREATE TRIGGER IF NOT EXISTS symbols_ad AFTER DELETE ON symbols BEGIN
    INSERT INTO symbols_fts(symbols_fts, rowid, name, signature, doc, file_path, tokens)
    VALUES ('delete', old.id, old.name, old.signature, old.doc, old.file_path, old.tokens);
END;

CREATE TRIGGER IF NOT EXISTS symbols_au AFTER UPDATE ON symbols BEGIN
    INSERT INTO symbols_fts(symbols_fts, rowid, name, signature, doc, file_path, tokens)
    VALUES ('delete', old.id, old.name, old.signature, old.doc, old.file_path, old.tokens);
    INSERT INTO symbols_fts(rowid, name, signature, doc, file_path, tokens)
    VALUES (new.id, new.name, new.signature, new.doc, new.file_path, new.tokens);
END;
`
