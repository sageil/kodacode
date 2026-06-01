package events

import (
	"errors"
	"strconv"
	"strings"
)

const TypePromptCompiled Type = "prompt_compiled"

type PromptCompiledPayload struct {
	Shape            string
	BaseInstructions string
	Instructions     string
	CacheablePrefix  string
	DynamicSuffix    string
	Layers           []PromptLayerPayload
	Fragments        []PromptFragmentPayload
}

func (PromptCompiledPayload) eventType() Type { return TypePromptCompiled }

func (p PromptCompiledPayload) validate() error {
	if strings.TrimSpace(p.Shape) == "" {
		return errors.New("shape is required")
	}
	if strings.TrimSpace(p.Instructions) == "" {
		return errors.New("instructions is required")
	}
	for i, fragment := range p.Fragments {
		if err := fragment.validate(); err != nil {
			return errors.New("fragment " + strconv.Itoa(i) + ": " + err.Error())
		}
	}
	for i, layer := range p.Layers {
		if err := layer.validate(); err != nil {
			return errors.New("layer " + strconv.Itoa(i) + ": " + err.Error())
		}
	}
	return nil
}

type PromptFragmentPayload struct {
	Kind      string
	Source    string
	Stability string
	Layer     string
	Key       string
	Label     string
	Bytes     int
	Tokens    int
}

func (p PromptFragmentPayload) validate() error {
	if strings.TrimSpace(p.Kind) == "" {
		return errors.New("kind is required")
	}
	if strings.TrimSpace(p.Source) == "" {
		return errors.New("source is required")
	}
	if strings.TrimSpace(p.Stability) == "" {
		return errors.New("stability is required")
	}
	if p.Bytes < 0 {
		return errors.New("bytes must be >= 0")
	}
	if p.Tokens < 0 {
		return errors.New("tokens must be >= 0")
	}
	return nil
}

type PromptLayerPayload struct {
	Name      string
	Kind      string
	Source    string
	Stability string
	Status    string
	Fragments int
	Bytes     int
	Tokens    int
}

func (p PromptLayerPayload) validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("name is required")
	}
	if p.Fragments < 0 {
		return errors.New("fragments must be >= 0")
	}
	if p.Bytes < 0 {
		return errors.New("bytes must be >= 0")
	}
	if p.Tokens < 0 {
		return errors.New("tokens must be >= 0")
	}
	return nil
}

func PromptLayersFromFragments(fragments []PromptFragmentPayload) []PromptLayerPayload {
	if len(fragments) == 0 {
		return nil
	}
	layers := make([]PromptLayerPayload, 0, len(fragments))
	indexByName := make(map[string]int, len(fragments))
	for _, fragment := range fragments {
		name := promptFragmentLayerName(fragment)
		if existing, ok := indexByName[name]; ok {
			layer := &layers[existing]
			layer.Kind = mergePromptLayerAttribute(layer.Kind, fragment.Kind)
			layer.Source = mergePromptLayerAttribute(layer.Source, fragment.Source)
			layer.Stability = mergePromptLayerAttribute(layer.Stability, fragment.Stability)
			layer.Fragments++
			layer.Bytes += max(fragment.Bytes, 0)
			layer.Tokens += max(fragment.Tokens, 0)
			continue
		}
		indexByName[name] = len(layers)
		layers = append(layers, PromptLayerPayload{
			Name:      name,
			Kind:      strings.TrimSpace(fragment.Kind),
			Source:    strings.TrimSpace(fragment.Source),
			Stability: strings.TrimSpace(fragment.Stability),
			Status:    "included",
			Fragments: 1,
			Bytes:     max(fragment.Bytes, 0),
			Tokens:    max(fragment.Tokens, 0),
		})
	}
	return layers
}

func promptFragmentLayerName(fragment PromptFragmentPayload) string {
	if layer := strings.TrimSpace(fragment.Layer); layer != "" {
		return layer
	}
	if key := strings.TrimSpace(fragment.Key); key != "" {
		return key
	}
	if label := strings.TrimSpace(fragment.Label); label != "" {
		return label
	}
	kind := strings.TrimSpace(fragment.Kind)
	source := strings.TrimSpace(fragment.Source)
	if kind != "" && source != "" {
		return kind + ":" + source
	}
	if kind != "" {
		return kind
	}
	return "prompt"
}

func mergePromptLayerAttribute(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	switch {
	case current == "":
		return next
	case next == "" || current == next:
		return current
	default:
		return "mixed"
	}
}
