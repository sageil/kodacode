package codeintel

import (
	"errors"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/lsp"
	"github.com/sageil/kodacode/internal/tool"
)

func TestSelectCodeIntelCodeActionRequiresSelectorWhenMultipleMatch(t *testing.T) {
	_, err := selectCodeIntelCodeAction([]lsp.CodeAction{
		{Title: "Fix A", Kind: "quickfix"},
		{Title: "Fix B", Kind: "quickfix"},
	}, "", "", false)
	if err == nil || !strings.Contains(err.Error(), "multiple code actions matched") {
		t.Fatalf("error = %v", err)
	}
}

func TestSelectCodeIntelCodeActionPrefersPreferredAction(t *testing.T) {
	action, err := selectCodeIntelCodeAction([]lsp.CodeAction{
		{Title: "Organize Imports", Kind: "source.organizeImports"},
		{Title: "Organize Imports", Kind: "source.organizeImports", IsPreferred: true},
	}, "", "source.organizeImports", false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if action == nil || !action.IsPreferred {
		t.Fatalf("action = %#v", action)
	}
}

func TestCodeIntelCodeActionSelectionNoticeIsRecoverableNotice(t *testing.T) {
	err := codeIntelCodeActionSelectionNotice(errors.New("no code actions available"))
	notice, ok := tool.AsCodeIntelNotice(err)
	if !ok {
		t.Fatalf("AsCodeIntelNotice() = false for %T %[1]v", err)
	}
	if notice.Kind != tool.CodeIntelNoticeKindUnsupported ||
		!strings.Contains(notice.Message, "no code actions available") ||
		!strings.Contains(notice.Message, "use apply_patch") {
		t.Fatalf("notice = %#v", notice)
	}
}
