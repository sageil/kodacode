package search

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/sageil/kodacode/internal/provider"
)

const standardRRFK = 60

type hybridCandidate struct {
	Path          string
	StartLine     int
	EndLine       int
	DisplayLine   int
	Snippet       string
	LexicalRank   int
	SemanticRank  int
	LexicalScore  float64
	SemanticScore float64
}

func (s *Service) hybrid(ctx context.Context, req Request) ([]Result, string, error) {
	paths, err := s.semanticPaths(req)
	if err != nil {
		return nil, "", err
	}
	if len(paths) == 0 {
		return nil, "", nil
	}

	chunkFiles, err := s.cachedChunkFiles(ctx, req, paths)
	if err != nil {
		return nil, "", err
	}
	if len(chunkFiles) == 0 {
		return nil, "", nil
	}

	semantic, err := s.semanticCandidates(ctx, req, chunkFiles)
	if err != nil {
		return nil, "", err
	}
	lexical, err := lexicalChunkCandidates(req, chunkFiles)
	if err != nil {
		return nil, "", err
	}
	return s.mergeHybridCandidates(lexical, semantic, req.MaxResults), "", nil
}

func (s *Service) semanticCandidates(ctx context.Context, req Request, chunkFiles []cachedChunkFile) ([]hybridCandidate, error) {
	chunks := flattenChunkFiles(chunkFiles)
	if len(chunks) == 0 {
		return nil, nil
	}
	vectors, err := s.embedder.Embed(ctx, provider.EmbeddingRequest{
		Model:      s.model,
		Inputs:     []string{req.Query},
		Dimensions: s.dimensions,
	})
	if err != nil {
		return nil, err
	}
	s.logLifecycle("hybrid search query embedded",
		"workspace_root", req.WorkspaceRoot,
		"root_path", req.RootPath,
		"paths", len(chunkFiles),
		"chunks", len(chunks),
		"model", s.model.String(),
	)

	query := vectors[0]
	candidates := make([]hybridCandidate, 0, len(chunks))
	for _, chunk := range chunks {
		candidates = append(candidates, hybridCandidate{
			Path:          chunk.Path,
			StartLine:     chunk.StartLine,
			EndLine:       chunk.EndLine,
			DisplayLine:   chunk.Line,
			Snippet:       chunk.Snippet,
			SemanticScore: cosineSimilarity(query, chunk.Embedding),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].SemanticScore == candidates[j].SemanticScore {
			if candidates[i].Path == candidates[j].Path {
				return candidates[i].DisplayLine < candidates[j].DisplayLine
			}
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].SemanticScore > candidates[j].SemanticScore
	})
	for idx := range candidates {
		candidates[idx].SemanticRank = idx + 1
	}
	return candidates, nil
}

func lexicalChunkCandidates(req Request, chunkFiles []cachedChunkFile) ([]hybridCandidate, error) {
	matcher, err := newLineMatcher(req)
	if err != nil {
		return nil, err
	}

	candidates := make([]hybridCandidate, 0, len(chunkFiles))
	seen := map[string]bool{}
	rank := 0
	for _, file := range chunkFiles {
		if len(file.Chunks) == 0 {
			continue
		}
		next, err := lexicalChunkCandidatesForFile(file.AbsolutePath, file.Chunks, matcher, seen, rank)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, next...)
		rank += len(next)
	}
	return candidates, nil
}

func lexicalChunkCandidatesForFile(path string, chunks []cachedChunk, matcher lineMatcher, seen map[string]bool, rankOffset int) ([]hybridCandidate, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck

	out := make([]hybridCandidate, 0, len(chunks))
	reader := bufio.NewReader(file)
	lineNumber := 0
	chunkIndex := 0
	for {
		rawLine, readErr := reader.ReadBytes('\n')
		if len(rawLine) > 0 {
			lineNumber++
			line := trimSearchLineEnding(rawLine)
			if matcher.Match(line) {
				chunk, idx, ok := chunkContainingLine(chunks, lineNumber, chunkIndex)
				if ok {
					chunkIndex = idx
					key := hybridCandidateKey(chunk.Path, chunk.StartLine, chunk.EndLine)
					if !seen[key] {
						seen[key] = true
						out = append(out, hybridCandidate{
							Path:         chunk.Path,
							StartLine:    chunk.StartLine,
							EndLine:      chunk.EndLine,
							DisplayLine:  lineNumber,
							Snippet:      summarizeLine(line),
							LexicalRank:  rankOffset + len(out) + 1,
							LexicalScore: 1,
						})
						if len(out) == len(chunks) {
							return out, nil
						}
					}
				}
			}
		}
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			return out, nil
		}
		return nil, readErr
	}
}

func chunkContainingLine(chunks []cachedChunk, line int, startIdx int) (cachedChunk, int, bool) {
	idx := max(startIdx, 0)
	for idx < len(chunks) && line > chunks[idx].EndLine {
		idx++
	}
	if idx >= len(chunks) {
		return cachedChunk{}, idx, false
	}
	chunk := chunks[idx]
	if line < chunk.StartLine || line > chunk.EndLine {
		return cachedChunk{}, idx, false
	}
	return chunk, idx, true
}

func flattenChunkFiles(files []cachedChunkFile) []cachedChunk {
	chunks := make([]cachedChunk, 0, len(files))
	for _, file := range files {
		chunks = append(chunks, file.Chunks...)
	}
	return chunks
}

func (s *Service) mergeHybridCandidates(lexical, semantic []hybridCandidate, limit int) []Result {
	merged := map[string]hybridCandidate{}
	merge := func(candidate hybridCandidate) {
		key := hybridCandidateKey(candidate.Path, candidate.StartLine, candidate.EndLine)
		current, ok := merged[key]
		if !ok {
			merged[key] = candidate
			return
		}
		if candidate.LexicalRank > 0 {
			current.LexicalRank = candidate.LexicalRank
			current.LexicalScore = candidate.LexicalScore
			current.DisplayLine = candidate.DisplayLine
			current.Snippet = candidate.Snippet
		}
		if candidate.SemanticRank > 0 {
			current.SemanticRank = candidate.SemanticRank
			current.SemanticScore = candidate.SemanticScore
			if current.DisplayLine == 0 {
				current.DisplayLine = candidate.DisplayLine
				current.Snippet = candidate.Snippet
			}
		}
		merged[key] = current
	}
	for _, candidate := range lexical {
		merge(candidate)
	}
	for _, candidate := range semantic {
		merge(candidate)
	}

	ranked := make([]hybridCandidate, 0, len(merged))
	for _, candidate := range merged {
		ranked = append(ranked, candidate)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		scoreI := s.hybridCandidateScore(ranked[i])
		scoreJ := s.hybridCandidateScore(ranked[j])
		if scoreI == scoreJ {
			if ranked[i].Path == ranked[j].Path {
				if ranked[i].DisplayLine == ranked[j].DisplayLine {
					return ranked[i].StartLine < ranked[j].StartLine
				}
				return ranked[i].DisplayLine < ranked[j].DisplayLine
			}
			return ranked[i].Path < ranked[j].Path
		}
		return scoreI > scoreJ
	})
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}

	results := make([]Result, 0, len(ranked))
	for _, candidate := range ranked {
		source := SourceSemantic
		switch {
		case candidate.LexicalRank > 0 && candidate.SemanticRank > 0:
			source = SourceMerged
		case candidate.LexicalRank > 0:
			source = SourceLexical
		}
		results = append(results, Result{
			Path:    candidate.Path,
			Line:    candidate.DisplayLine,
			Snippet: candidate.Snippet,
			Source:  source,
			score:   s.hybridCandidateScore(candidate),
		})
	}
	return results
}

func (s *Service) hybridCandidateScore(candidate hybridCandidate) float64 {
	base := reciprocalRankFusionScore(s.rrfK, candidate.LexicalRank, candidate.SemanticRank)
	return base * s.pathBoostMultiplier(candidate.Path)
}

func (s *Service) pathBoostMultiplier(path string) float64 {
	if s == nil {
		return 1
	}
	return pathBoostMultiplier(path, s.pathBoosts)
}

func reciprocalRankFusionScore(k int, ranks ...int) float64 {
	score := 0.0
	for _, rank := range ranks {
		if rank <= 0 {
			continue
		}
		score += 1.0 / float64(k+rank)
	}
	return score
}

func hybridCandidateKey(path string, startLine, endLine int) string {
	return path + ":" + strconv.Itoa(startLine) + ":" + strconv.Itoa(endLine)
}
