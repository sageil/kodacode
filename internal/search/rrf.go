package search

import (
	"sort"
	"strconv"
)

const rrfK = 60

// resultKey uniquely identifies a search result for deduplication.
func resultKey(r SearchResult) string {
	return r.FilePath + ":" + strconv.Itoa(r.Line) + ":" + r.Name
}

// MergeRRF combines two ranked result lists using Reciprocal Rank Fusion.
// Each result's final score is: sum(1 / (k + rank_i)) where k=60.
// The Source field is set to "fts", "vector", or "both".
func MergeRRF(ftsResults, vectorResults []SearchResult, limit int) []SearchResult {
	type entry struct {
		result SearchResult
		score  float64
		inFTS  bool
		inVec  bool
	}

	merged := make(map[string]*entry)

	for rank, r := range ftsResults {
		key := resultKey(r)
		e, ok := merged[key]
		if !ok {
			e = &entry{result: r}
			merged[key] = e
		}
		e.score += 1.0 / float64(rrfK+rank+1)
		e.inFTS = true
	}

	for rank, r := range vectorResults {
		key := resultKey(r)
		e, ok := merged[key]
		if !ok {
			e = &entry{result: r}
			merged[key] = e
		}
		e.score += 1.0 / float64(rrfK+rank+1)
		e.inVec = true
	}

	results := make([]SearchResult, 0, len(merged))
	for _, e := range merged {
		switch {
		case e.inFTS && e.inVec:
			e.result.Source = "both"
		case e.inVec:
			e.result.Source = "vector"
		default:
			e.result.Source = "fts"
		}
		e.result.Score = e.score
		results = append(results, e.result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}
