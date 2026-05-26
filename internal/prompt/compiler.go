package prompt

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceGlobal  Source = "global"
	SourceProject Source = "project"
	SourceRuntime Source = "runtime"
	SourceSession Source = "session"
	SourceUser    Source = "user"
)

type Stability string

const (
	StabilityStable  Stability = "stable"
	StabilityDynamic Stability = "dynamic"
)

type Kind string

const (
	KindPolicy   Kind = "policy"
	KindRole     Kind = "role"
	KindTooling  Kind = "tooling"
	KindRepo     Kind = "repo"
	KindMemory   Kind = "memory"
	KindRuntime  Kind = "runtime"
	KindMetadata Kind = "metadata"
)

type Fragment struct {
	Kind            Kind
	Source          Source
	Stability       Stability
	Key             string
	Label           string
	Content         string
	ProviderContent string
}

func (f Fragment) Validate() error {
	if f.Kind == "" {
		return errors.New("kind is required")
	}
	if f.Source == "" {
		return errors.New("source is required")
	}
	if f.Stability == "" {
		return errors.New("stability is required")
	}
	if strings.TrimSpace(f.Content) == "" {
		return errors.New("content is required")
	}
	return nil
}

type Request struct {
	Fragments []Fragment
}

type Compiled struct {
	Fragments             []Fragment
	Document              string
	StablePrefix          string
	DynamicSuffix         string
	ProviderDocument      string
	ProviderStablePrefix  string
	ProviderDynamicSuffix string
}

type Compiler interface {
	Compile(context.Context, Request) (Compiled, error)
}

type StaticCompiler struct{}

func NewStaticCompiler() StaticCompiler {
	return StaticCompiler{}
}

func (StaticCompiler) Compile(_ context.Context, req Request) (Compiled, error) {
	if len(req.Fragments) == 0 {
		return Compiled{}, errors.New("at least one fragment is required")
	}

	var (
		stable  []Fragment
		dynamic []Fragment
	)

	for i, fragment := range req.Fragments {
		if err := fragment.Validate(); err != nil {
			return Compiled{}, fmt.Errorf("fragment %d: %w", i, err)
		}
		switch fragment.Stability {
		case StabilityStable:
			stable = append(stable, fragment)
		case StabilityDynamic:
			dynamic = append(dynamic, fragment)
		default:
			return Compiled{}, fmt.Errorf("fragment %d: unsupported stability %q", i, fragment.Stability)
		}
	}

	out := make([]Fragment, len(req.Fragments))
	copy(out, req.Fragments)

	stableText := renderDocument(stable)
	dynamicText := renderDocument(dynamic)
	providerStableText := renderProviderDocument(stable)
	providerDynamicText := renderProviderDocument(dynamic)
	document := strings.TrimSpace(joinNonEmpty(stableText, dynamicText))
	providerDocument := strings.TrimSpace(joinNonEmpty(providerStableText, providerDynamicText))
	if providerDocument == "" {
		providerDocument = document
	}
	if providerStableText == "" {
		providerStableText = stableText
	}
	if providerDynamicText == "" {
		providerDynamicText = dynamicText
	}

	return Compiled{
		Fragments:             out,
		Document:              document,
		StablePrefix:          stableText,
		DynamicSuffix:         dynamicText,
		ProviderDocument:      providerDocument,
		ProviderStablePrefix:  providerStableText,
		ProviderDynamicSuffix: providerDynamicText,
	}, nil
}

func renderDocument(fragments []Fragment) string {
	return renderFragmentDocument(fragments, false)
}

func renderProviderDocument(fragments []Fragment) string {
	return renderFragmentDocument(fragments, true)
}

func renderFragmentDocument(fragments []Fragment, providerFacing bool) string {
	if len(fragments) == 0 {
		return ""
	}

	parts := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		content := strings.TrimSpace(fragment.Content)
		if providerFacing {
			content = strings.TrimSpace(fragment.providerContent())
		}
		if content == "" {
			continue
		}
		parts = append(parts, content)
	}
	return joinNonEmpty(parts...)
}

func (f Fragment) providerContent() string {
	if trimmed := strings.TrimSpace(f.ProviderContent); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(f.Content)
}

func joinNonEmpty(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, strings.TrimSpace(part))
		}
	}
	return strings.Join(out, "\n\n")
}
