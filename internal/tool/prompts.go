package tool

import "embed"

//go:embed prompts/*.txt
var promptFS embed.FS

func prompt(name string) string {
	data, err := promptFS.ReadFile("prompts/" + name + ".txt")
	if err != nil {
		panic("tool: missing prompt " + name + ": " + err.Error())
	}
	return string(data)
}
