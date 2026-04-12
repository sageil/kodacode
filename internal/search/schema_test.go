package search

import "testing"

func TestEmbeddingsTableCreated(t *testing.T) {
	db, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='embeddings'").Scan(&name)
	if err != nil {
		t.Fatalf("embeddings table not found: %v", err)
	}
	if name != "embeddings" {
		t.Errorf("got table name %q, want %q", name, "embeddings")
	}
}

func TestEmbeddingsTableCascadeDelete(t *testing.T) {
	db, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	// Insert a file, symbol, and embedding.
	if _, err := db.Exec("INSERT INTO files (path, hash, indexed_at) VALUES ('a.go', 'h1', 0)"); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec("INSERT INTO symbols (file_path, name, kind, language, line) VALUES ('a.go', 'Foo', 'function', 'go', 1)")
	if err != nil {
		t.Fatal(err)
	}
	symID, _ := res.LastInsertId()
	if _, err := db.Exec("INSERT INTO embeddings (symbol_id, vector, model, updated_at) VALUES (?, X'00000000', 'test', 0)", symID); err != nil {
		t.Fatal(err)
	}

	// Delete the symbol — embedding should cascade.
	if _, err := db.Exec("DELETE FROM symbols WHERE id = ?", symID); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM embeddings WHERE symbol_id = ?", symID).Scan(&count); err != nil {
		t.Fatalf("count query error: %v", err)
	}
	if count != 0 {
		t.Errorf("embedding row not cascade-deleted, count = %d", count)
	}
}
