package tui

import (
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
)

func writeMutationDiffPreview(call *events.ToolCallState) (*textdiff.Preview, bool) {
	if call == nil || call.WriteMutation == nil || call.WriteMutation.DiffPreview == nil {
		return nil, false
	}
	return call.WriteMutation.DiffPreview, true
}

func mutationDiffOpsFromPreview(preview *textdiff.Preview) []mutationDiffOp {
	if preview == nil || len(preview.Ops) == 0 {
		return nil
	}
	ops := make([]mutationDiffOp, 0, len(preview.Ops))
	for _, op := range preview.Ops {
		switch op.Kind {
		case textdiff.OpContext:
			ops = append(ops, mutationDiffOp{kind: mutationDiffEqual, text: op.Text})
		case textdiff.OpDelete:
			ops = append(ops, mutationDiffOp{kind: mutationDiffDelete, text: op.Text})
		case textdiff.OpInsert:
			ops = append(ops, mutationDiffOp{kind: mutationDiffInsert, text: op.Text})
		case textdiff.OpSkip:
			ops = append(ops, mutationDiffOp{kind: mutationDiffEqual, text: ""})
		}
	}
	return ops
}
