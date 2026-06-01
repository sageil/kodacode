package provider

import (
	"errors"
	"strings"
)

type Tool struct {
	Name         string
	Description  string
	Kind         ToolKind
	InputSchema  string
	InputFormat  *ToolInputFormat
	ParallelSafe bool `json:"-"`
}

type ToolKind string

const (
	ToolKindFunction ToolKind = "function"
	ToolKindCustom   ToolKind = "custom"
)

type ToolInputFormat struct {
	Type       string
	Syntax     string
	Definition string
}

func (t Tool) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("tool name is required")
	}
	if strings.TrimSpace(t.Description) == "" {
		return errors.New("tool description is required")
	}
	switch t.KindOrDefault() {
	case ToolKindFunction:
		if strings.TrimSpace(t.InputSchema) == "" {
			return errors.New("tool input_schema is required")
		}
	case ToolKindCustom:
		if t.InputFormat != nil {
			if strings.TrimSpace(t.InputFormat.Type) == "" {
				return errors.New("tool input_format.type is required")
			}
			if strings.TrimSpace(t.InputFormat.Type) == "grammar" {
				if strings.TrimSpace(t.InputFormat.Syntax) == "" {
					return errors.New("tool input_format.syntax is required")
				}
				if strings.TrimSpace(t.InputFormat.Definition) == "" {
					return errors.New("tool input_format.definition is required")
				}
			}
		}
	default:
		return errors.New("tool kind must be function or custom")
	}
	return nil
}

func (t Tool) KindOrDefault() ToolKind {
	if strings.TrimSpace(string(t.Kind)) == "" {
		return ToolKindFunction
	}
	return t.Kind
}

func validateInputToolKind(kind ToolKind) error {
	switch kind {
	case "", ToolKindFunction, ToolKindCustom:
		return nil
	default:
		return errors.New("tool_kind must be function or custom")
	}
}
