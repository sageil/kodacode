package agent

import "embed"

//go:embed prompt/*.txt
var promptFS embed.FS

func builtinPrompt(name string) string {
	data, err := promptFS.ReadFile("prompt/" + name + ".txt")
	if err != nil {
		panic("agent: missing prompt " + name + ": " + err.Error())
	}
	return string(data)
}

func CompactionPrompt() string {
	return builtinPrompt("compaction")
}
