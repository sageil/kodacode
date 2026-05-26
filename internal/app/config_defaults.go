package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/observability"
)

func LoadConfigFromEnv(getenv func(string) string) Config {
	config := Config{
		UtilityModelRetryAttempts:        defaultUtilityRetryAttempts,
		UtilityModelRetryAfterMaxSeconds: int(defaultUtilityRetryAfterMaxDelay / time.Second),
		OutputBudgets:                    defaultOutputBudgets(),
		Sessions: SessionConfig{
			MaxProviderRequestsPerTurn: defaultMaxProviderRequestsPerTurn,
			MaxOutputContinuations:     defaultMaxOutputContinuations,
			MaxRetries:                 defaultProviderRetryAttempts,
			CompactionThreshold:        sessionHistoryDefaultCompactionThreshold,
			CompactionTargetThreshold:  sessionHistoryDefaultTargetThreshold,
		},
		ModelCache: ModelCacheConfig{
			ExpiryDays: 7,
		},
		Retention: RetentionConfig{},
		Execution: defaultExecutionConfig(),
		Workflow:  defaultWorkflowConfig(),
		LSP:       defaultLSPConfig(),
		Logging:   loadLoggingConfigFromEnv(getenv),
	}
	return config
}

func defaultExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		PermissionMode:  PermissionModeAuto,
		Network:         ExecutionNetworkDisabled,
		AllowLoginShell: false,
	}
}

func loadLoggingConfigFromEnv(getenv func(string) string) observability.Config {
	return observability.Config{
		Dir: defaultLogDir(getenv),
	}
}

func defaultLogDir(getenv func(string) string) string {
	if xdg := strings.TrimSpace(getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "kodacode")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "kodacode")
	}
	return filepath.Join(home, ".local", "share", "kodacode")
}
