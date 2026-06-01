package app

import "github.com/sageil/kodacode/internal/codeintel"

type CodeIntelService = codeintel.CodeIntelService
type WorkspaceLSPStatus = codeintel.WorkspaceLSPStatus

func NewCodeIntelService(config LSPConfig) *CodeIntelService {
	return codeintel.NewCodeIntelService(config)
}
