package app

import "github.com/sageil/kodacode/internal/provider"

type ModelOverrideConfig struct {
	Ref                 provider.ModelRef
	Name                string
	ContextSize         *int
	MaxInputTokens      *int
	MaxOutputTokens     *int
	DefaultOutputTokens *int
	Reasoning           *bool
	ToolCalls           *bool
	Vision              *bool
	CostInput           *float64
	CostOutput          *float64
}
