// Package workflow loads, validates, and catalogs workflow definitions.
package workflow

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrWorkflowNotFound = errors.New("workflow not found")

type Catalog struct {
	definitions map[string]Definition
}

func newCatalog(definitions map[string]Definition) *Catalog {
	out := make(map[string]Definition, len(definitions))
	for id, definition := range definitions {
		out[id] = definition
	}
	return &Catalog{definitions: out}
}

func (c *Catalog) Get(id string) (Definition, error) {
	if c == nil {
		return Definition{}, ErrWorkflowNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Definition{}, ErrWorkflowNotFound
	}
	definition, ok := c.definitions[id]
	if !ok {
		return Definition{}, fmt.Errorf("%w: %s", ErrWorkflowNotFound, id)
	}
	return definition, nil
}

func (c *Catalog) List() []Definition {
	if c == nil || len(c.definitions) == 0 {
		return nil
	}
	ids := make([]string, 0, len(c.definitions))
	for id := range c.definitions {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]Definition, 0, len(ids))
	for _, id := range ids {
		out = append(out, c.definitions[id])
	}
	return out
}
