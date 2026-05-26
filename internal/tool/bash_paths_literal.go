package tool

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func shellWordLiteral(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", false
	}
	var builder strings.Builder
	for _, part := range word.Parts {
		if !appendShellLiteralPart(&builder, part) {
			return "", false
		}
	}
	return builder.String(), true
}

func appendShellLiteralPart(builder *strings.Builder, part syntax.WordPart) bool {
	switch typed := part.(type) {
	case *syntax.Lit:
		builder.WriteString(typed.Value)
		return true
	case *syntax.SglQuoted:
		builder.WriteString(typed.Value)
		return true
	case *syntax.DblQuoted:
		for _, inner := range typed.Parts {
			if !appendShellLiteralPart(builder, inner) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func shellWordHasDynamicExpansion(word *syntax.Word) bool {
	if word == nil {
		return false
	}
	dynamic := false
	syntax.Walk(word, func(node syntax.Node) bool {
		switch node.(type) {
		case *syntax.Word:
			return true
		case *syntax.ParamExp, *syntax.CmdSubst, *syntax.ArithmExp, *syntax.ProcSubst, *syntax.ExtGlob:
			dynamic = true
			return false
		}
		return true
	})
	return dynamic
}
