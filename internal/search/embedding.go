package search

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"strings"
	"time"
)

// Embedder generates vector embeddings for text inputs.
type Embedder interface {
	Embed(ctx context.Context, model string, texts []string) ([][]float32, error)
}

// EmbeddingIndexerConfig holds the configuration for an EmbeddingIndexer.
type EmbeddingIndexerConfig struct {
	DB         *sql.DB
	Embedder   Embedder
	Model      string // embedding model ID passed to the Embedder
	BatchSize  int    // symbols per API call; 0 = default (100)
	Dimensions int    // expected vector size; 0 = accept any
	ProjectDir string // used for relative paths in chunk text
}

// EmbeddingIndexer generates and stores vector embeddings for symbols
// that have been indexed by the symbol Indexer.
type EmbeddingIndexer struct {
	db         *sql.DB
	embedder   Embedder
	model      string
	batchSize  int
	dimensions int
	projectDir string
}

// NewEmbeddingIndexer creates an indexer that generates embeddings for symbols.
func NewEmbeddingIndexer(cfg EmbeddingIndexerConfig) *EmbeddingIndexer {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	return &EmbeddingIndexer{
		db:         cfg.DB,
		embedder:   cfg.Embedder,
		model:      cfg.Model,
		batchSize:  batchSize,
		dimensions: cfg.Dimensions,
		projectDir: cfg.ProjectDir,
	}
}

// Index finds symbols without embeddings (or with a stale model) and
// generates embeddings in batches. Returns the number of symbols embedded.
func (ei *EmbeddingIndexer) Index(ctx context.Context) (int, error) {
	pending, err := ei.pendingSymbols(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("query pending symbols: %w", err)
	}
	return ei.indexPending(ctx, pending)
}

// SyncPaths generates embeddings only for symbols in the provided file paths.
// Paths may be absolute or relative to ProjectDir.
func (ei *EmbeddingIndexer) SyncPaths(ctx context.Context, paths []string) (int, error) {
	normalized := ei.normalizePaths(paths)
	if len(normalized) == 0 {
		return 0, nil
	}

	pending, err := ei.pendingSymbols(ctx, normalized)
	if err != nil {
		return 0, fmt.Errorf("query pending symbols: %w", err)
	}
	return ei.indexPending(ctx, pending)
}

func (ei *EmbeddingIndexer) indexPending(ctx context.Context, pending []pendingSymbol) (int, error) {
	if len(pending) == 0 {
		return 0, nil
	}

	total := 0
	for i := 0; i < len(pending); i += ei.batchSize {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
		end := min(i+ei.batchSize, len(pending))
		batch := pending[i:end]

		n, err := ei.indexBatch(ctx, batch)
		if err != nil {
			log.Printf("embedding indexer: batch error at offset %d: %v", i, err)
			return total, err
		}
		total += n
	}

	return total, nil
}

type pendingSymbol struct {
	id        int64
	filePath  string
	name      string
	kind      string
	signature string
	doc       string
	line      int
}

func (ei *EmbeddingIndexer) pendingSymbols(ctx context.Context, paths []string) ([]pendingSymbol, error) {
	args := []any{ei.model}
	query := `
		SELECT s.id, s.file_path, s.name, s.kind, s.signature, s.doc, s.line
		FROM symbols s
		LEFT JOIN embeddings e ON e.symbol_id = s.id AND e.model = ?
		WHERE e.symbol_id IS NULL`
	if len(paths) > 0 {
		query += ` AND s.file_path IN (` + placeholders(len(paths)) + `)`
		for _, path := range paths {
			args = append(args, path)
		}
	}
	query += ` ORDER BY s.file_path, s.line, s.id`

	rows, err := ei.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var symbols []pendingSymbol
	for rows.Next() {
		var s pendingSymbol
		if err := rows.Scan(&s.id, &s.filePath, &s.name, &s.kind, &s.signature, &s.doc, &s.line); err != nil {
			return nil, err
		}
		symbols = append(symbols, s)
	}
	return symbols, rows.Err()
}

func (ei *EmbeddingIndexer) normalizePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if ei.projectDir != "" && !filepath.IsAbs(path) {
			path = filepath.Join(ei.projectDir, path)
		}
		if ei.projectDir != "" {
			rel, err := filepath.Rel(ei.projectDir, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func (ei *EmbeddingIndexer) indexBatch(ctx context.Context, batch []pendingSymbol) (int, error) {
	texts := make([]string, len(batch))
	for i, s := range batch {
		texts[i] = ei.chunkText(s)
	}

	vectors, err := ei.embedder.Embed(ctx, ei.model, texts)
	if err != nil {
		return 0, fmt.Errorf("embed batch: %w", err)
	}
	if len(vectors) != len(batch) {
		return 0, fmt.Errorf("embed returned %d vectors for %d inputs", len(vectors), len(batch))
	}

	dim := len(vectors[0])
	if ei.dimensions == 0 {
		ei.dimensions = dim
	} else if dim != ei.dimensions {
		return 0, fmt.Errorf("dimension mismatch: got %d, expected %d", dim, ei.dimensions)
	}

	tx, err := ei.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		"INSERT OR REPLACE INTO embeddings (symbol_id, vector, model, updated_at) VALUES (?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close() //nolint:errcheck

	now := time.Now().Unix()
	for i, vec := range vectors {
		if _, err := stmt.ExecContext(ctx, batch[i].id, VectorToBlob(vec), ei.model, now); err != nil {
			return 0, err
		}
	}
	return len(batch), tx.Commit()
}

// chunkText builds the text representation of a symbol for embedding.
func (ei *EmbeddingIndexer) chunkText(s pendingSymbol) string {
	path := s.filePath
	if ei.projectDir != "" {
		if rel, err := filepath.Rel(ei.projectDir, s.filePath); err == nil {
			path = rel
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s:%d %s %s", path, s.line, s.kind, s.name)
	if s.signature != "" {
		b.WriteByte('\n')
		b.WriteString(s.signature)
	}
	if s.doc != "" {
		b.WriteByte('\n')
		b.WriteString(s.doc)
	}
	return b.String()
}

// VectorSearch queries the embedding index for symbols similar to the query text.
// Streams vectors from the database row-by-row to avoid holding all vectors in memory.
func (ei *EmbeddingIndexer) VectorSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	ids, err := ei.prefilterCandidateIDs(ctx, query, max(limit*25, 100))
	if err != nil {
		return nil, fmt.Errorf("prefilter candidates: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	queryVecs, err := ei.embedder.Embed(ctx, ei.model, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(queryVecs) == 0 {
		return nil, nil
	}
	queryVec := queryVecs[0]

	args := make([]any, 0, 1+len(ids))
	args = append(args, ei.model)
	querySQL := `
		SELECT e.vector, s.file_path, s.name, s.kind, s.language, s.signature, s.doc, s.line
		FROM embeddings e
		JOIN symbols s ON s.id = e.symbol_id
		WHERE e.model = ? AND s.id IN (` + placeholders(len(ids)) + `)`
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := ei.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query embeddings: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	// Maintain a fixed-size top-N sorted slice (descending by score).
	type scored struct {
		result SearchResult
		score  float32
	}
	top := make([]scored, 0, limit)

	// Reusable buffer for decoding vectors. This avoids allocating a new
	// []float32 per row (9K+ rows × 6KB each = 54 MB of GC pressure).
	var vecBuf []float32

	for rows.Next() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		var blob []byte
		var r SearchResult
		if err := rows.Scan(&blob, &r.FilePath, &r.Name, &r.Kind, &r.Language, &r.Signature, &r.Doc, &r.Line); err != nil {
			log.Printf("search: vector row scan error: %v", err)
			continue
		}
		if len(blob) == 0 || len(blob)%4 != 0 {
			continue
		}
		n := len(blob) / 4
		if cap(vecBuf) < n {
			vecBuf = make([]float32, n)
		}
		vecBuf = vecBuf[:n]
		for i := range n {
			vecBuf[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
		}

		sim := CosineSimilarity(queryVec, vecBuf)

		// Insert into top-N if it qualifies.
		if len(top) < limit {
			top = append(top, scored{result: r, score: sim})
			// Bubble up to maintain sorted order (descending).
			for i := len(top) - 1; i > 0 && top[i].score > top[i-1].score; i-- {
				top[i], top[i-1] = top[i-1], top[i]
			}
		} else if sim > top[len(top)-1].score {
			top[len(top)-1] = scored{result: r, score: sim}
			for i := len(top) - 1; i > 0 && top[i].score > top[i-1].score; i-- {
				top[i], top[i-1] = top[i-1], top[i]
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate embeddings: %w", err)
	}

	results := make([]SearchResult, len(top))
	for i := range top {
		results[i] = top[i].result
		results[i].Score = float64(top[i].score)
	}
	return results, nil
}

func (ei *EmbeddingIndexer) prefilterCandidateIDs(ctx context.Context, query string, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 100
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	seen := make(map[int64]struct{}, limit)
	ids := make([]int64, 0, limit)
	add := func(found []int64) {
		for _, id := range found {
			if len(ids) >= limit {
				return
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	ftsQuery := buildFTSQuery(query)
	if ftsQuery != "" {
		rows, err := ei.db.QueryContext(ctx, `
			SELECT s.id
			FROM symbols_fts
			JOIN symbols s ON symbols_fts.rowid = s.id
			WHERE symbols_fts MATCH ?
			ORDER BY bm25(symbols_fts, 10.0, 5.0, 2.0, 1.0, 3.0)
			LIMIT ?`,
			ftsQuery, limit,
		)
		if err == nil {
			defer rows.Close() //nolint:errcheck
			found, err := scanIDs(rows)
			if err != nil {
				return nil, err
			}
			add(found)
			if len(ids) >= limit {
				return ids, nil
			}
		}
	}

	loweredQuery := strings.ToLower(query)
	found, err := ei.queryCandidateIDs(ctx, `
		SELECT id
		FROM symbols
		WHERE lower(name) = ? OR lower(file_path) = ?
		ORDER BY CASE WHEN lower(name) = ? THEN 0 ELSE 1 END, length(name), length(file_path)
		LIMIT ?`,
		loweredQuery, loweredQuery, loweredQuery, limit-len(ids),
	)
	if err != nil {
		return nil, err
	}
	add(found)
	if len(ids) >= limit {
		return ids, nil
	}

	terms := dedupeTerms(tokenizeQuery(query))
	if len(terms) == 0 {
		return ids, nil
	}

	queryExactNames := `
		SELECT id
		FROM symbols
		WHERE lower(name) IN (` + placeholders(len(terms)) + `)
		ORDER BY length(name)
		LIMIT ?`
	exactNameArgs := make([]any, 0, len(terms)+1)
	for _, term := range terms {
		exactNameArgs = append(exactNameArgs, term)
	}
	exactNameArgs = append(exactNameArgs, limit-len(ids))
	found, err = ei.queryCandidateIDs(ctx, queryExactNames, exactNameArgs...)
	if err != nil {
		return nil, err
	}
	add(found)
	if len(ids) >= limit {
		return ids, nil
	}

	prefixQuery, prefixArgs := buildCandidateLikeQuery("prefix", terms, limit-len(ids))
	if prefixQuery != "" {
		found, err = ei.queryCandidateIDs(ctx, prefixQuery, prefixArgs...)
		if err != nil {
			return nil, err
		}
		add(found)
		if len(ids) >= limit {
			return ids, nil
		}
	}

	substringQuery, substringArgs := buildCandidateLikeQuery("substring", terms, limit-len(ids))
	if substringQuery != "" {
		found, err = ei.queryCandidateIDs(ctx, substringQuery, substringArgs...)
		if err != nil {
			return nil, err
		}
		add(found)
	}
	return ids, nil
}

func scanIDs(rows *sql.Rows) ([]int64, error) {
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (ei *EmbeddingIndexer) queryCandidateIDs(ctx context.Context, query string, args ...any) ([]int64, error) {
	rows, err := ei.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	return scanIDs(rows)
}

func dedupeTerms(terms []string) []string {
	if len(terms) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(terms))
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	return out
}

func buildCandidateLikeQuery(mode string, terms []string, limit int) (string, []any) {
	if limit <= 0 || len(terms) == 0 {
		return "", nil
	}
	var (
		clauses []string
		args    []any
		pattern string
	)
	switch mode {
	case "prefix":
		pattern = "%s%%"
	default:
		pattern = "%%%s%%"
	}
	for _, term := range terms {
		match := fmt.Sprintf(pattern, escapeLikeTerm(term))
		clauses = append(clauses,
			`lower(name) LIKE ? ESCAPE '\'`,
			`lower(file_path) LIKE ? ESCAPE '\'`,
			`lower(doc) LIKE ? ESCAPE '\'`,
		)
		args = append(args, match, match, match)
	}
	query := `
		SELECT id
		FROM symbols
		WHERE ` + strings.Join(clauses, " OR ") + `
		ORDER BY length(name), length(file_path)
		LIMIT ?`
	args = append(args, limit)
	return query, args
}

func escapeLikeTerm(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
	}
	return b.String()
}

// EmbeddingCount returns the number of embeddings stored for the current model.
func (ei *EmbeddingIndexer) EmbeddingCount(ctx context.Context) (int, error) {
	var count int
	err := ei.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM embeddings WHERE model = ?", ei.model).Scan(&count)
	return count, err
}
