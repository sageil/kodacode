// Package search provides hybrid semantic and keyword search over codebase symbols.
package search

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
)

type Searcher struct {
	db       *sql.DB
	embedder atomic.Pointer[EmbeddingIndexer]
}

func NewSearcher(db *sql.DB) *Searcher {
	return &Searcher{db: db}
}

// SetEmbeddingIndexer enables hybrid search (FTS5 + vector + RRF merge).
// When nil, Search falls back to FTS5 only. Safe for concurrent use.
func (s *Searcher) SetEmbeddingIndexer(ei *EmbeddingIndexer) {
	s.embedder.Store(ei)
}

// Search runs a full-text search. When an EmbeddingIndexer is configured,
// it also runs a vector search in parallel and merges results via RRF.
func (s *Searcher) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	ei := s.embedder.Load()
	if ei == nil {
		return s.ftsSearch(ctx, query, limit)
	}

	var (
		ftsResults, vectorResults []SearchResult
		ftsErr, vecErr           error
		wg                       sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		ftsResults, ftsErr = s.ftsSearch(ctx, query, limit)
	}()
	go func() {
		defer wg.Done()
		vectorResults, vecErr = ei.VectorSearch(ctx, query, limit)
	}()
	wg.Wait()

	// If both fail, return the FTS error (more actionable).
	if ftsErr != nil && vecErr != nil {
		return nil, ftsErr
	}

	// Graceful degradation: use whichever succeeded.
	if ftsErr != nil {
		for i := range vectorResults {
			vectorResults[i].Source = "vector"
		}
		return vectorResults, nil
	}
	if vecErr != nil {
		log.Printf("vector search failed, falling back to FTS: %v", vecErr)
		for i := range ftsResults {
			ftsResults[i].Source = "fts"
		}
		return ftsResults, nil
	}

	return MergeRRF(ftsResults, vectorResults, limit), nil
}

func (s *Searcher) ftsSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT s.file_path, s.name, s.kind, s.language, s.signature, s.doc, s.line,
		       bm25(symbols_fts, 10.0, 5.0, 2.0, 1.0, 3.0) AS score
		FROM symbols_fts
		JOIN symbols s ON symbols_fts.rowid = s.id
		WHERE symbols_fts MATCH ?
		ORDER BY score
		LIMIT ?`,
		ftsQuery, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.FilePath, &r.Name, &r.Kind, &r.Language, &r.Signature, &r.Doc, &r.Line, &r.Score); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Searcher) SymbolCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols").Scan(&count)
	return count, err
}

func buildFTSQuery(query string) string {
	terms := tokenizeQuery(query)
	if len(terms) == 0 {
		return ""
	}

	var parts []string
	for _, t := range terms {
		escaped := strings.ReplaceAll(t, `"`, `""`)
		parts = append(parts, `"`+escaped+`"`)
	}

	for _, t := range terms {
		split := SplitTokens(t)
		if split != strings.ToLower(t) {
			for _, st := range strings.Fields(split) {
				escaped := strings.ReplaceAll(st, `"`, `""`)
				parts = append(parts, `"`+escaped+`"`)
			}
		}
	}

	seen := make(map[string]bool)
	var unique []string
	for _, p := range parts {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}

	return strings.Join(unique, " OR ")
}

func tokenizeQuery(query string) []string {
	var terms []string
	for w := range strings.FieldsSeq(query) {
		w = strings.Trim(w, ".,;:!?()[]{}\"'`")
		if len(w) > 1 {
			terms = append(terms, w)
		}
	}
	return terms
}

func FormatResults(results []SearchResult, workDir string) string {
	if len(results) == 0 {
		return "No symbol matches found."
	}

	var sb strings.Builder
	for _, r := range results {
		path := r.FilePath
		if rel, err := relPath(path, workDir); err == nil {
			path = rel
		}

		fmt.Fprintf(&sb, "%s:%d  %s  %s", path, r.Line, r.Kind, r.Name)
		if r.Signature != "" {
			fmt.Fprintf(&sb, "\n  %s", r.Signature)
		}
		if r.Doc != "" {
			doc := r.Doc
			if len(doc) > 120 {
				doc = doc[:120] + "…"
			}
			fmt.Fprintf(&sb, "\n  %s", doc)
		}
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

func relPath(path, workDir string) (string, error) {
	if workDir == "" {
		return path, fmt.Errorf("no workdir")
	}
	if rel, ok := strings.CutPrefix(path, workDir); ok {
		return strings.TrimPrefix(rel, "/"), nil
	}
	return path, fmt.Errorf("not under workdir")
}
