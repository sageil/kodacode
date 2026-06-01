package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/configdir"
	"github.com/sageil/kodacode/internal/prompt"
)

const promptInstructionsFilename = "AGENTS.md"

func loadPromptSourceFragments(workspaceRoot string) ([]prompt.Fragment, error) {
	fragments := make([]prompt.Fragment, 0, 2)

	globalPath, err := globalPromptSourcePath()
	if err != nil {
		return nil, err
	}
	if fragment, ok, err := loadPromptSourceFragment(globalPath, prompt.KindPolicy, prompt.SourceGlobal, "global-instructions"); err != nil {
		return nil, err
	} else if ok {
		fragments = append(fragments, fragment)
	}

	projectPath := filepath.Join(strings.TrimSpace(workspaceRoot), promptInstructionsFilename)
	if fragment, ok, err := loadPromptSourceFragment(projectPath, prompt.KindRepo, prompt.SourceProject, "project-instructions"); err != nil {
		return nil, err
	} else if ok {
		fragments = append(fragments, fragment)
	}

	return fragments, nil
}

func globalPromptSourcePath() (string, error) {
	return filepath.Join(configdir.Root(), promptInstructionsFilename), nil
}

func loadPromptSourceFragment(path string, kind prompt.Kind, source prompt.Source, label string) (prompt.Fragment, bool, error) {
	content, ok, err := readPromptSource(path)
	if err != nil {
		return prompt.Fragment{}, false, err
	}
	if !ok {
		return prompt.Fragment{}, false, nil
	}

	return prompt.Fragment{
		Kind:      kind,
		Source:    source,
		Stability: prompt.StabilityStable,
		Layer:     label,
		Key:       label,
		Label:     label,
		Content:   content,
	}, true, nil
}

func readPromptSource(path string) (string, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}

	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return "", false, nil
	}
	return trimmed, true, nil
}
