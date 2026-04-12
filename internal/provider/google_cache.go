package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/genai"
)

// googleCacheEntry holds a live context cache reference.
type googleCacheEntry struct {
	name      string    // server-assigned cache name
	hash      string    // hash of system+tools content for invalidation
	expiresAt time.Time // when the cache expires
}

// googleCacheTTL is how long a context cache lives before expiring.
const googleCacheTTL = 30 * time.Minute

// ensureCache creates or reuses a context cache for the given model + system/tools.
// Returns the cache name to use in GenerateContentConfig.CachedContent,
// or empty string if caching is not possible.
func (p *GoogleProvider) ensureCache(ctx context.Context, model string, opts ChatOptions) string {
	hash := hashCacheContent(opts)
	if hash == "" {
		return "" // nothing to cache
	}

	p.mu.Lock()
	entry := p.cacheByModel[model]
	p.mu.Unlock()

	// Reuse existing cache if hash matches and not expired.
	if entry != nil && entry.hash == hash && time.Now().Before(entry.expiresAt) {
		return entry.name
	}

	if entry != nil {
		_, _ = p.client.Caches.Delete(ctx, entry.name, nil)
	}

	cacheConfig := &genai.CreateCachedContentConfig{
		TTL: googleCacheTTL,
	}

	// System instruction.
	if len(opts.SystemParts) > 0 {
		var sb strings.Builder
		for i, part := range opts.SystemParts {
			if i > 0 && part != "" {
				sb.WriteString("\n\n")
			}
			sb.WriteString(part)
		}
		if text := sb.String(); text != "" {
			cacheConfig.SystemInstruction = &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(text)},
			}
		}
	}

	// Tools.
	if len(opts.Tools) > 0 {
		decls := make([]*genai.FunctionDeclaration, 0, len(opts.Tools))
		for _, t := range opts.Tools {
			var schema any
			if len(t.Parameters) > 0 {
				if err := json.Unmarshal(t.Parameters, &schema); err != nil {
					log.Printf("google: invalid tool schema for %q: %v", t.Name, err)
				}
			}
			decls = append(decls, &genai.FunctionDeclaration{
				Name:                 t.Name,
				Description:          t.Description,
				ParametersJsonSchema: schema,
			})
		}
		cacheConfig.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
	}

	cached, err := p.client.Caches.Create(ctx, model, cacheConfig)
	if err != nil {
		log.Printf("google: cache create warning: %v", err)
		return ""
	}

	p.mu.Lock()
	p.cacheByModel[model] = &googleCacheEntry{
		name:      cached.Name,
		hash:      hash,
		expiresAt: time.Now().Add(googleCacheTTL - time.Minute), // 1 min buffer
	}
	p.mu.Unlock()

	return cached.Name
}

// hashCacheContent builds a deterministic hash of system parts + tool definitions.
func hashCacheContent(opts ChatOptions) string {
	h := sha256.New()
	for _, p := range opts.SystemParts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	for _, t := range opts.Tools {
		h.Write([]byte(t.Name))
		h.Write([]byte{0})
		h.Write(t.Parameters)
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	if sum[0] == 0 && sum[1] == 0 { // all zeros = nothing hashed
		return ""
	}
	return fmt.Sprintf("%x", sum)
}
