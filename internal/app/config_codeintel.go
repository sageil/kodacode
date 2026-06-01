package app

import "github.com/sageil/kodacode/internal/codeintel"

type CodeIntelConfig = codeintel.Config

func defaultCodeIntelConfig() CodeIntelConfig {
	return codeintel.DefaultConfig()
}
