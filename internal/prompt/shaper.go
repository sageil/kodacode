package prompt

import (
	"context"
	"errors"
)

const ViewShapeGeneric = "generic"

type View struct {
	Shape           string
	Instructions    string
	CacheablePrefix string
	DynamicSuffix   string
}

type Shaper interface {
	Shape(context.Context, Compiled) (View, error)
}

type StaticShaper struct{}

func NewShaper() StaticShaper {
	return StaticShaper{}
}

func (StaticShaper) Shape(_ context.Context, compiled Compiled) (View, error) {
	instructions := compiled.ProviderDocument
	cacheablePrefix := compiled.ProviderStablePrefix
	dynamicSuffix := compiled.ProviderDynamicSuffix
	if instructions == "" {
		instructions = compiled.Document
	}
	if cacheablePrefix == "" {
		cacheablePrefix = compiled.StablePrefix
	}
	if dynamicSuffix == "" {
		dynamicSuffix = compiled.DynamicSuffix
	}
	if instructions == "" {
		return View{}, errors.New("compiled document is required")
	}
	return View{
		Shape:           ViewShapeGeneric,
		Instructions:    instructions,
		CacheablePrefix: cacheablePrefix,
		DynamicSuffix:   dynamicSuffix,
	}, nil
}
