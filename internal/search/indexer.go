package search

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

type IndexerConfig struct {
	CtagsBinary     string
	ExcludePatterns []string
	MaxFileSize     int64
	IgnorePatterns  []string
}

type fileStat struct {
	mtime time.Time
	size  int64
	hash  string
}

type Indexer struct {
	db         *sql.DB
	projectDir string
	cfg        IndexerConfig
	ctagsBin   string
	fileStats  map[string]fileStat
	analyzers  *AnalyzerRegistry
}

func NewIndexer(db *sql.DB, projectDir string, cfg IndexerConfig) *Indexer {
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = 512 * 1024
	}
	return &Indexer{
		db:         db,
		projectDir: projectDir,
		cfg:        cfg,
		fileStats:  make(map[string]fileStat),
		analyzers:  NewAnalyzerRegistry(),
	}
}

func (ix *Indexer) Index(ctx context.Context) (int, error) {
	ix.resolveCtagsBinary()

	files, err := ix.discoverFiles(ctx)
	if err != nil {
		return 0, fmt.Errorf("discover files: %w", err)
	}

	changed, err := ix.filterChanged(ctx, files)
	if err != nil {
		return 0, fmt.Errorf("filter changed: %w", err)
	}

	if err := ix.removeDeleted(ctx, files); err != nil {
		return 0, fmt.Errorf("remove deleted: %w", err)
	}

	if len(changed) == 0 {
		return 0, nil
	}

	indexed, err := ix.indexChanged(ctx, changed)
	if err != nil {
		return indexed, err
	}
	return indexed, nil
}

func (ix *Indexer) SyncPaths(ctx context.Context, paths []string) (int, error) {
	ix.resolveCtagsBinary()

	normalized := ix.normalizePaths(paths)
	if len(normalized) == 0 {
		return 0, nil
	}

	hashes, err := ix.loadFileHashes(ctx)
	if err != nil {
		return 0, err
	}

	var changed []fileEntry
	var toDelete []string
	for _, abs := range normalized {
		rel, err := filepath.Rel(ix.projectDir, abs)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)

		info, statErr := os.Stat(abs)
		switch {
		case statErr != nil:
			delete(ix.fileStats, abs)
			if hashes[abs] != "" {
				toDelete = append(toDelete, abs)
			}
		case !ix.shouldIndex(abs, rel):
			delete(ix.fileStats, abs)
			if hashes[abs] != "" {
				toDelete = append(toDelete, abs)
			}
		default:
			h, err := hashFile(abs)
			if err != nil {
				continue
			}
			ix.fileStats[abs] = fileStat{mtime: info.ModTime(), size: info.Size(), hash: h}
			if hashes[abs] != h {
				changed = append(changed, fileEntry{path: abs, hash: h})
			}
		}
	}

	if len(toDelete) > 0 {
		if err := ix.removePaths(ctx, toDelete); err != nil {
			return 0, err
		}
	}

	if len(changed) == 0 {
		return 0, nil
	}
	return ix.indexChanged(ctx, changed)
}

func (ix *Indexer) resolveCtagsBinary() {
	if ix.ctagsBin != "" {
		return
	}
	bin, err := ResolveCtagsBinary(ix.cfg.CtagsBinary)
	if err != nil {
		log.Printf("search index: ctags not available, using built-in fallback extractors where available: %v", err)
		return
	}
	ix.ctagsBin = bin
}

func (ix *Indexer) normalizePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) {
			path = filepath.Join(ix.projectDir, path)
		}
		rel, err := filepath.Rel(ix.projectDir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func (ix *Indexer) indexChanged(ctx context.Context, changed []fileEntry) (int, error) {
	const batchSize = 100
	indexed := 0
	for i := 0; i < len(changed); i += batchSize {
		end := min(i+batchSize, len(changed))
		batch := changed[i:end]
		n, err := ix.indexBatch(ctx, batch)
		if err != nil {
			return indexed, err
		}
		indexed += n
	}

	if indexed > 0 {
		ix.optimizeFTS(ctx)
	}

	return indexed, nil
}

func (ix *Indexer) Reindex(ctx context.Context) (int, error) {
	if _, err := ix.db.ExecContext(ctx, "DELETE FROM symbols"); err != nil {
		return 0, err
	}
	if _, err := ix.db.ExecContext(ctx, "DELETE FROM files"); err != nil {
		return 0, err
	}
	return ix.Index(ctx)
}

func (ix *Indexer) discoverFiles(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = ix.projectDir
	out, err := cmd.Output()
	if err != nil {
		return ix.walkFiles()
	}

	var files []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		abs := filepath.Join(ix.projectDir, line)
		if ix.shouldIndex(abs, line) {
			files = append(files, abs)
		}
	}
	return files, nil
}

func (ix *Indexer) walkFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(ix.projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(ix.projectDir, path)
		if ix.shouldIndex(path, rel) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func (ix *Indexer) shouldIndex(absPath, relPath string) bool {
	for _, pat := range ix.cfg.IgnorePatterns {
		if matched, _ := doublestar.Match(pat, relPath); matched {
			return false
		}
	}
	for _, pat := range ix.cfg.ExcludePatterns {
		if matched, _ := doublestar.Match(pat, relPath); matched {
			return false
		}
	}
	info, err := os.Stat(absPath)
	if err != nil || info.Size() > ix.cfg.MaxFileSize {
		return false
	}
	return true
}

func (ix *Indexer) filterChanged(ctx context.Context, files []string) ([]fileEntry, error) {
	hashes, err := ix.loadFileHashes(ctx)
	if err != nil {
		return nil, err
	}

	var changed []fileEntry
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		// Skip expensive hash if mtime+size match cached entry and the
		// cached hash still matches the database.
		if cached, ok := ix.fileStats[f]; ok {
			if cached.mtime.Equal(info.ModTime()) && cached.size == info.Size() && hashes[f] == cached.hash {
				continue
			}
		}
		h, err := hashFile(f)
		if err != nil {
			continue
		}
		ix.fileStats[f] = fileStat{mtime: info.ModTime(), size: info.Size(), hash: h}
		if hashes[f] == h {
			continue
		}
		changed = append(changed, fileEntry{path: f, hash: h})
	}
	return changed, nil
}

type fileEntry struct {
	path string
	hash string
}

func (ix *Indexer) loadFileHashes(ctx context.Context) (map[string]string, error) {
	rows, err := ix.db.QueryContext(ctx, "SELECT path, hash FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	hashes := make(map[string]string)
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, err
		}
		hashes[path] = hash
	}
	return hashes, rows.Err()
}

func (ix *Indexer) removeDeleted(ctx context.Context, currentFiles []string) error {
	current := make(map[string]bool, len(currentFiles))
	for _, f := range currentFiles {
		current[f] = true
	}

	indexed, err := ix.loadFileHashes(ctx)
	if err != nil {
		return err
	}

	var deleted []string
	for path := range indexed {
		if !current[path] {
			deleted = append(deleted, path)
		}
	}
	return ix.removePaths(ctx, deleted)
}

func (ix *Indexer) removePaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, "DELETE FROM files WHERE path = ?")
	if err != nil {
		return err
	}
	defer stmt.Close() //nolint:errcheck

	for _, path := range paths {
		if _, err := stmt.ExecContext(ctx, path); err != nil {
			return err
		}
		delete(ix.fileStats, path)
	}
	return tx.Commit()
}

func (ix *Indexer) optimizeFTS(ctx context.Context) {
	_, _ = ix.db.ExecContext(ctx, "INSERT INTO symbols_fts(symbols_fts) VALUES ('optimize')")
}

func (ix *Indexer) indexBatch(ctx context.Context, batch []fileEntry) (int, error) {
	paths := make([]string, len(batch))
	for i, f := range batch {
		paths[i] = f.path
	}

	var symbols []Symbol
	if ix.ctagsBin != "" {
		var err error
		symbols, err = ExtractSymbols(ctx, ix.ctagsBin, paths)
		if err != nil {
			log.Printf("search index: ctags error: %v", err)
			if len(symbols) == 0 {
				return 0, nil
			}
		}
		symbols = ix.enrichSymbols(symbols)
	} else {
		for _, f := range batch {
			symbols = append(symbols, ix.analyzers.FallbackExtract(ctx, f.path)...)
		}
	}

	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, f := range batch {
		if _, err := tx.ExecContext(ctx, "DELETE FROM symbols WHERE file_path = ?", f.path); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT OR REPLACE INTO files (path, hash, indexed_at) VALUES (?, ?, ?)",
			f.path, f.hash, time.Now().Unix(),
		); err != nil {
			return 0, err
		}
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO symbols (file_path, name, kind, language, signature, doc, line, parent, tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close() //nolint:errcheck

	for _, sym := range symbols {
		if _, err := stmt.ExecContext(ctx,
			sym.FilePath, sym.Name, sym.Kind, sym.Language,
			sym.Signature, sym.Doc, sym.Line, sym.Parent, sym.Tokens,
		); err != nil {
			return 0, err
		}
	}

	return len(batch), tx.Commit()
}

func (ix *Indexer) enrichSymbols(symbols []Symbol) []Symbol {
	if len(symbols) == 0 {
		return nil
	}

	byFile := make(map[string][]Symbol)
	for i := range symbols {
		symbols[i].Language = DetectLanguage(symbols[i].FilePath, symbols[i].Language)
		byFile[symbols[i].FilePath] = append(byFile[symbols[i].FilePath], symbols[i])
	}

	symIndex := make(map[string]int, len(symbols))
	for i, sym := range symbols {
		symIndex[symbolKey(sym)] = i
	}

	for filePath, fileSyms := range byFile {
		language := fileSyms[0].Language
		enriched := ix.analyzers.Enrich(filePath, language, fileSyms)
		for _, es := range enriched {
			if i, ok := symIndex[symbolKey(es)]; ok {
				symbols[i] = mergeSymbolMetadata(symbols[i], es)
			}
		}
	}

	return symbols
}

func symbolKey(sym Symbol) string {
	return sym.FilePath + ":" + strconv.Itoa(sym.Line) + ":" + sym.Name + ":" + sym.Kind
}

func mergeSymbolMetadata(base, update Symbol) Symbol {
	if update.Language != "" {
		base.Language = update.Language
	}
	if update.Signature != "" {
		base.Signature = update.Signature
	}
	if update.Doc != "" {
		base.Doc = update.Doc
	}
	if update.Parent != "" {
		base.Parent = update.Parent
	}
	if update.Tokens != "" {
		base.Tokens = update.Tokens
	}
	return base
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
