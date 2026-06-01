package codeintel

import (
	"errors"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/tool"
)

func codeIntelUnavailableNotice(err error) *tool.CodeIntelNotice {
	message := strings.TrimSpace(errorMessage(err))
	if message == "" {
		return nil
	}
	return &tool.CodeIntelNotice{
		Kind:    tool.CodeIntelNoticeKindUnavailable,
		Message: message,
	}
}

func codeIntelUnsupportedNotice(message string) *tool.CodeIntelNotice {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	return &tool.CodeIntelNotice{
		Kind:    tool.CodeIntelNoticeKindUnsupported,
		Message: message,
	}
}

func codeIntelNoticeError(notice *tool.CodeIntelNotice) error {
	if notice == nil {
		return nil
	}
	return tool.NewCodeIntelNoticeError(notice.Kind, notice.Message)
}

func codeIntelUnavailableError(err error) error {
	return codeIntelNoticeError(codeIntelUnavailableNotice(err))
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	var messages []string
	collectErrorMessages(err, &messages)
	if len(messages) == 0 {
		return ""
	}
	sort.Strings(messages)
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1] == message {
			continue
		}
		out = append(out, message)
	}
	return strings.Join(out, "; ")
}

func collectErrorMessages(err error, out *[]string) {
	if err == nil {
		return
	}
	type joined interface {
		Unwrap() []error
	}
	if multi, ok := err.(joined); ok {
		for _, child := range multi.Unwrap() {
			collectErrorMessages(child, out)
		}
		return
	}
	*out = append(*out, err.Error())
	for _, child := range []error{errors.Unwrap(err)} {
		if child != nil {
			collectErrorMessages(child, out)
		}
	}
}
