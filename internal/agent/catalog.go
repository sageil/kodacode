package agent

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const DefaultID = "builder"

var ErrAgentNotFound = errors.New("agent not found")

type Catalog struct {
	definitions map[string]Definition
}

func newCatalog(definitions map[string]Definition) (*Catalog, error) {
	out := make(map[string]Definition, len(definitions))
	for id, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return nil, fmt.Errorf("agent %q: %w", id, err)
		}
		out[id] = definition
	}
	return &Catalog{definitions: out}, nil
}

func (c *Catalog) Get(id string) (Definition, error) {
	if c == nil {
		return Definition{}, ErrAgentNotFound
	}
	name := strings.TrimSpace(id)
	if name == "" {
		name = DefaultID
	}
	definition, ok := c.definitions[name]
	if !ok {
		return Definition{}, fmt.Errorf("%w: %s", ErrAgentNotFound, name)
	}
	return definition, nil
}

func (c *Catalog) List() []Definition {
	if c == nil || len(c.definitions) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.definitions))
	for name := range c.definitions {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Definition, 0, len(names))
	for _, name := range names {
		out = append(out, c.definitions[name])
	}
	return out
}

func (c *Catalog) cloneDefinitions() map[string]Definition {
	if c == nil || len(c.definitions) == 0 {
		return nil
	}
	out := make(map[string]Definition, len(c.definitions))
	for id, definition := range c.definitions {
		out[id] = definition
	}
	return out
}
