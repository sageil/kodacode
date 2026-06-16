package search

import (
	"context"
	"math"
	"os"
	"path/filepath"
)

const (
	semanticChunkLimit = 800
)

type cachedFile struct {
	ModTime int64
	Size    int64
	Chunks  []cachedChunk
}

type cachedChunk struct {
	Path      string
	StartLine int
	EndLine   int
	Line      int
	Snippet   string
	Embedding []float32
}

type cachedChunkFile struct {
	AbsolutePath string
	Chunks       []cachedChunk
}

func semanticPaths(req Request) ([]string, error) {
	return semanticPathsWithSkipDirs(req, defaultSkipDirMatcher())
}

func (s *Service) semanticPaths(req Request) ([]string, error) {
	return semanticPathsWithSkipDirs(req, s.skipDirs)
}

func semanticPathsWithSkipDirs(req Request, skipDirs skipDirMatcher) ([]string, error) {
	info, err := os.Stat(req.RootPath)
	if err != nil {
		return nil, err
	}
	ignores := newRequestGitignoreMatcher(req)
	if !info.IsDir() {
		if err := ignores.loadDir(filepath.Dir(req.RootPath)); err != nil {
			return nil, err
		}
		if ignores.ignored(req.RootPath, false) {
			return nil, nil
		}
		return []string{req.RootPath}, nil
	}

	paths := make([]string, 0, 64)
	err = filepath.WalkDir(req.RootPath, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if skipDirs.shouldSkip(entry.Name()) {
				return filepath.SkipDir
			}
			if current != req.RootPath && ignores.ignored(current, true) {
				return filepath.SkipDir
			}
			if err := ignores.loadDir(current); err != nil {
				return err
			}
			return nil
		}
		if ignores.ignored(current, false) {
			return nil
		}
		relative := relPathFromRoot(req.RootPath, current)
		if !matchesGlob(req.Glob, relative) {
			return nil
		}
		paths = append(paths, current)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

func estimatedChunkCount(size int64) int {
	if size <= 0 {
		return 1
	}
	return max(1, int(size/(2*1024)))
}

func (s *Service) cachedChunkFiles(ctx context.Context, req Request, paths []string) ([]cachedChunkFile, error) {
	type pendingPath struct {
		path string
		info os.FileInfo
	}
	out := make([]cachedChunkFile, 0, len(paths))
	pending := make([]pendingPath, 0, len(paths))
	uncachedEstimated := 0
	for _, path := range paths {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		entry, ok, err := s.cachedFileEntry(ctx, req.WorkspaceRoot, path, info)
		if err != nil {
			return nil, err
		}
		if ok {
			if len(entry.Chunks) > 0 {
				out = append(out, cachedChunkFile{AbsolutePath: path, Chunks: entry.Chunks})
			}
			continue
		}
		uncachedEstimated += estimatedChunkCount(info.Size())
		if uncachedEstimated > semanticChunkLimit {
			return nil, ErrSemanticScopeTooLarge
		}
		pending = append(pending, pendingPath{path: path, info: info})
	}
	for _, item := range pending {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		entry, _, err := s.ensureFileCache(ctx, req.WorkspaceRoot, item.path, item.info)
		if err != nil {
			return nil, err
		}
		if len(entry.Chunks) > 0 {
			out = append(out, cachedChunkFile{AbsolutePath: item.path, Chunks: entry.Chunks})
		}
	}
	return out, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot float64
	var normA float64
	var normB float64
	for idx := range a {
		dot += float64(a[idx] * b[idx])
		normA += float64(a[idx] * a[idx])
		normB += float64(b[idx] * b[idx])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
