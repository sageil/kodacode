package provider

import "embed"

//go:embed prompts/*.txt
var promptFS embed.FS

func builtinPrompt(name string) string {
	data, err := promptFS.ReadFile("prompts/" + name + ".txt")
	if err != nil {
		panic("provider: missing prompt " + name + ": " + err.Error())
	}
	return string(data)
}

func BuiltinPromptFS() embed.FS { return promptFS }
