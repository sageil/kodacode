package textutil

import "strings"

// StringAccumulator appends to a seeded string without repeatedly copying the
// full previously-built value on every delta.
type StringAccumulator struct {
	builder     strings.Builder
	initialized bool
}

func (a *StringAccumulator) Append(current, chunk string) string {
	if chunk == "" {
		return current
	}
	if !a.initialized {
		a.initialized = true
		a.builder.Grow(len(current) + len(chunk))
		a.builder.WriteString(current)
	}
	a.builder.WriteString(chunk)
	return a.builder.String()
}

func (a *StringAccumulator) Reset() {
	*a = StringAccumulator{}
}
