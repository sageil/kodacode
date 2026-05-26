package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/tool"
)

func handleStepQuestionToolPreview(toolName, arguments string, commitAssistant func() error, discardAssistantPreview func() error) error {
	if strings.TrimSpace(toolName) != tool.QuestionToolName {
		return nil
	}
	if isPlannerSavePlanQuestion(toolName, []byte(arguments)) {
		if commitAssistant == nil {
			return nil
		}
		return commitAssistant()
	}
	if discardAssistantPreview == nil {
		return nil
	}
	return discardAssistantPreview()
}
