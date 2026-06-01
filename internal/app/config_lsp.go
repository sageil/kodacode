package app

import "github.com/sageil/kodacode/internal/codeintel"

type LSPConfig = codeintel.Config

func defaultLSPConfig() LSPConfig {
	return codeintel.DefaultConfig()
}
