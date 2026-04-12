package agent

import "embed"

//go:embed agents/*.md
var builtinFS embed.FS

func BuiltinAgentFS() embed.FS { return builtinFS }

func BuiltinPromptFS() embed.FS { return promptFS }

func BuiltinAgents() ([]Agent, error) {
	return LoadFS(builtinFS, true)
}
