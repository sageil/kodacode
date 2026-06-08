package workflow

import "embed"

//go:embed workflows/*.yaml
var builtinWorkflowFS embed.FS

func BuiltinWorkflowFS() embed.FS { return builtinWorkflowFS }
