package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sageil/kodacode/v1/internal/config"
)

func (b *SystemPromptBuilder) loadInstructionFiles() ([]string, []string) {
	instructions, loadedPaths := b.loadKodacodeInstructionFiles()
	seen := make(map[string]bool)
	for _, path := range b.kodacodeFilePaths() {
		seen[path] = true
	}

	files := defaultInstructionFiles
	if b.cfg.Config != nil {
		if custom := b.cfg.Config.InstructionFiles; len(custom) > 0 {
			files = custom
		}
	}

	for _, filename := range files {
		paths := b.findAllInstructionFiles(filename)
		for _, path := range paths {
			if seen[path] {
				continue
			}
			seen[path] = true
			content, err := b.readFileWithLimit(path)
			if err == nil && content != "" {
				namespace := b.buildInstructionNamespace(path)
				header := "Instructions from: " + path
				if namespace != "" {
					header = fmt.Sprintf("Instructions from: %s %s", path, namespace)
				}
				instructions = append(instructions, header+"\n"+content)
				loadedPaths = append(loadedPaths, path)
			}
		}
	}

	return instructions, loadedPaths
}

func (b *SystemPromptBuilder) loadKodacodeInstructionFiles() ([]string, []string) {
	var instructions []string
	var loadedPaths []string
	for _, path := range b.kodacodeFilePaths() {
		content, err := b.readFileWithLimit(path)
		if err != nil || content == "" {
			continue
		}
		instructions = append(instructions, "Instructions from: "+path+"\n"+content)
		loadedPaths = append(loadedPaths, path)
	}
	return instructions, loadedPaths
}

func (b *SystemPromptBuilder) kodacodeFilePaths() []string {
	var paths []string
	globalPath := filepath.Join(config.ConfigDir(), "KODACODE.md")
	if _, err := os.Stat(globalPath); err == nil {
		paths = append(paths, globalPath)
	}
	projectPath := filepath.Join(b.cfg.ProjectDir, "KODACODE.md")
	if _, err := os.Stat(projectPath); err == nil {
		paths = append(paths, projectPath)
	} else {
		altPath := filepath.Join(b.cfg.ProjectDir, ".kodacode", "KODACODE.md")
		if _, err := os.Stat(altPath); err == nil {
			paths = append(paths, altPath)
		}
	}
	localPath := filepath.Join(b.cfg.ProjectDir, "KODACODE.local.md")
	if _, err := os.Stat(localPath); err == nil {
		paths = append(paths, localPath)
	}
	return paths
}

func (b *SystemPromptBuilder) findAllInstructionFiles(filename string) []string {
	var paths []string
	dir := b.cfg.ProjectDir

	for {
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
		paths[i], paths[j] = paths[j], paths[i]
	}

	return paths
}

func (b *SystemPromptBuilder) readFileWithLimit(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if info.Size() > maxInstructionSize {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer func() { _ = file.Close() }()

		buf := make([]byte, maxInstructionSize)
		n, err := file.Read(buf)
		if err != nil {
			return "", err
		}
		return string(buf[:n]) + "\n\n[truncated - file exceeds 32KB limit]", nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func (b *SystemPromptBuilder) buildInstructionNamespace(path string) string {
	rel, err := filepath.Rel(b.cfg.ProjectDir, filepath.Dir(path))
	if err != nil || rel == "." {
		return ""
	}
	return fmt.Sprintf("[%s]", rel)
}
