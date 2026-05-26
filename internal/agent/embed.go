package agent

import "embed"

//go:embed *.md
var builtinAgentFS embed.FS

func BuiltinAgentFS() embed.FS { return builtinAgentFS }
