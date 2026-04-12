package search

import (
	"context"
	"log"
)

type SymbolExtractor interface {
	Name() string
	Supports(language, path string) bool
	Extract(ctx context.Context, path string) ([]Symbol, error)
}

type SymbolEnricher interface {
	Name() string
	Supports(language, path string) bool
	Enrich(path string, symbols []Symbol) []Symbol
}

type AnalyzerRegistry struct {
	extractors []SymbolExtractor
	enrichers  []SymbolEnricher
}

func NewAnalyzerRegistry() *AnalyzerRegistry {
	r := &AnalyzerRegistry{}
	r.RegisterExtractor(goSymbolExtractor{})
	r.RegisterEnricher(goSymbolEnricher{})
	r.RegisterEnricher(jsTSSymbolEnricher{})
	r.RegisterEnricher(pythonSymbolEnricher{})
	r.RegisterEnricher(rustSymbolEnricher{})
	r.RegisterEnricher(javaSymbolEnricher{})
	r.RegisterEnricher(rubySymbolEnricher{})
	r.RegisterEnricher(zigSymbolEnricher{})
	r.RegisterEnricher(csharpSymbolEnricher{})
	r.RegisterEnricher(luaSymbolEnricher{})
	r.RegisterEnricher(phpSymbolEnricher{})
	r.RegisterEnricher(swiftSymbolEnricher{})
	r.RegisterEnricher(kotlinSymbolEnricher{})
	r.RegisterEnricher(cSymbolEnricher{})
	r.RegisterEnricher(cppSymbolEnricher{})
	r.RegisterEnricher(commentDocEnricher{})
	return r
}

func (r *AnalyzerRegistry) RegisterExtractor(extractor SymbolExtractor) {
	if r == nil || extractor == nil {
		return
	}
	r.extractors = append(r.extractors, extractor)
}

func (r *AnalyzerRegistry) RegisterEnricher(enricher SymbolEnricher) {
	if r == nil || enricher == nil {
		return
	}
	r.enrichers = append(r.enrichers, enricher)
}

func (r *AnalyzerRegistry) FallbackExtract(ctx context.Context, path string) []Symbol {
	if r == nil {
		return nil
	}
	language := DetectLanguage(path, "")
	for _, extractor := range r.extractors {
		if !extractor.Supports(language, path) {
			continue
		}
		symbols, err := extractor.Extract(ctx, path)
		if err != nil {
			log.Printf("search index: fallback extractor %s failed for %s: %v", extractor.Name(), path, err)
			return nil
		}
		for i := range symbols {
			symbols[i].Language = DetectLanguage(symbols[i].FilePath, symbols[i].Language)
		}
		return symbols
	}
	return nil
}

func (r *AnalyzerRegistry) Enrich(path string, language string, symbols []Symbol) []Symbol {
	if r == nil || len(symbols) == 0 {
		return symbols
	}
	current := cloneSymbols(symbols)
	language = DetectLanguage(path, language)
	for _, enricher := range r.enrichers {
		if !enricher.Supports(language, path) {
			continue
		}
		current = enricher.Enrich(path, current)
	}
	return current
}

func cloneSymbols(in []Symbol) []Symbol {
	out := make([]Symbol, len(in))
	copy(out, in)
	return out
}
