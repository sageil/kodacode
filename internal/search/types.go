package search

type Symbol struct {
	FilePath  string
	Name      string
	Kind      string
	Language  string
	Signature string
	Doc       string
	Line      int
	Parent    string
	Tokens    string
}

type SearchResult struct {
	FilePath  string  `json:"file_path"`
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	Language  string  `json:"language"`
	Signature string  `json:"signature,omitempty"`
	Doc       string  `json:"doc,omitempty"`
	Line      int     `json:"line"`
	Score     float64 `json:"score"`
	Source    string  `json:"source,omitempty"` // "fts", "vector", or "both"
}
