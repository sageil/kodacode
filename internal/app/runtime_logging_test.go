package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
)

func TestRuntimeWritesOperationAndDebugLogs(t *testing.T) {
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	runtime := newRuntimeWithClient(t, &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
			}),
		},
	})
	runtime.SetLogger(logger)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("turn status = %q", result.Status)
	}

	operationsLog := readAppLogFile(t, filepath.Join(logDir, observability.OperationsLogName))
	if !strings.Contains(operationsLog, "session created") {
		t.Fatalf("operations log missing session creation entry: %q", operationsLog)
	}
	if !strings.Contains(operationsLog, "session turn completed") {
		t.Fatalf("operations log missing turn completion entry: %q", operationsLog)
	}
	if !strings.Contains(operationsLog, "component=runtime") {
		t.Fatalf("operations log missing runtime component: %q", operationsLog)
	}

	debugLog := readAppLogFile(t, filepath.Join(logDir, observability.DebugLogName))
	if !strings.Contains(debugLog, "provider request started") || !strings.Contains(debugLog, "provider_request_index=1") {
		t.Fatalf("debug log missing provider-request entry: %q", debugLog)
	}
	if !strings.Contains(debugLog, "component=turn_runner") {
		t.Fatalf("debug log missing turn-runner component: %q", debugLog)
	}
}

func TestTurnRunnerRawSSELoggingDisabledByDefault(t *testing.T) {
	t.Setenv("KODACODE_LOG_RAW_SSE", "")
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	runner := &TurnRunner{logger: logger}
	observer := runner.providerRawSSEObserver(provider.Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "engineer",
		Model:     provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"},
	}, 3)
	if observer != nil {
		t.Fatal("providerRawSSEObserver() != nil")
	}
}

func TestTurnRunnerRawSSELoggingMetadataWhenEnabled(t *testing.T) {
	t.Setenv("KODACODE_LOG_RAW_SSE", "metadata")
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	runner := &TurnRunner{logger: logger}
	observer := runner.providerRawSSEObserver(provider.Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "engineer",
		Model:     provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"},
	}, 3)
	if observer == nil {
		t.Fatal("providerRawSSEObserver() = nil")
	}
	observer(provider.RawSSEFrame{
		APIMode:  "chat_completions",
		Sequence: 41,
		Event:    "delta",
		Data:     []byte(`{"sensitive":"payload"}`),
	})

	debugLog := readAppLogFile(t, filepath.Join(logDir, observability.DebugLogName))
	if !strings.Contains(debugLog, "provider raw sse frame") ||
		!strings.Contains(debugLog, "sse_sequence=41") ||
		!strings.Contains(debugLog, "sse_data_bytes=23") {
		t.Fatalf("debug log missing raw SSE summary fields: %q", debugLog)
	}
	if strings.Contains(debugLog, "sensitive") || strings.Contains(debugLog, "sse_data=") {
		t.Fatalf("debug log included raw SSE frame data by default: %q", debugLog)
	}
}

func TestTurnRunnerRawSSELoggingIncludesFrameDataWhenEnabled(t *testing.T) {
	t.Setenv("KODACODE_LOG_RAW_SSE", "1")
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	runner := &TurnRunner{logger: logger}
	observer := runner.providerRawSSEObserver(provider.Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "engineer",
		Model:     provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"},
	}, 3)
	if observer == nil {
		t.Fatal("providerRawSSEObserver() = nil")
	}
	observer(provider.RawSSEFrame{
		APIMode:  "chat_completions",
		Sequence: 41,
		Event:    "delta",
		Data:     []byte(`{"sensitive":"payload"}`),
	})

	debugLog := readAppLogFile(t, filepath.Join(logDir, observability.DebugLogName))
	if !strings.Contains(debugLog, "sse_data=") || !strings.Contains(debugLog, "sensitive") {
		t.Fatalf("debug log missing enabled raw SSE frame data: %q", debugLog)
	}
}

func TestRuntimeDebugLogIncludesProviderRouteSelection(t *testing.T) {
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	router, err := provider.NewRoutedClient(map[string]provider.Client{
		"proxy": &fakeProvider{streams: []provider.Stream{provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"}})}},
	})
	if err != nil {
		t.Fatalf("provider.NewRoutedClient() error = %v", err)
	}
	wrapped, err := provider.WrapClient(router, func(next provider.ProviderHandler) provider.ProviderHandler {
		return next
	})
	if err != nil {
		t.Fatalf("provider.WrapClient() error = %v", err)
	}

	runtime := newRuntimeWithClient(t, wrapped)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "proxy", ModelID: "primary"},
	}
	runtime.Config.OpenAICompatible = OpenAICompatibleProviderConfig{
		ProviderID: "proxy",
		BaseURL:    "http://proxy.invalid/responses",
		APIKey:     "test-key",
	}
	runtime.SetLogger(logger)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("turn status = %q", result.Status)
	}

	debugLog := readAppLogFile(t, filepath.Join(logDir, observability.DebugLogName))
	if !strings.Contains(debugLog, "provider route selected") || !strings.Contains(debugLog, "model=proxy/primary") {
		t.Fatalf("debug log missing selected route entry: %q", debugLog)
	}
}

func TestRuntimeLogsProviderStatusForRouteAndStepFailures(t *testing.T) {
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	providerErr := &provider.ProviderError{
		Message:    "openai compatible chat completions api: Provider returned error",
		StatusCode: 502,
		Retryable:  true,
	}
	router, err := provider.NewRoutedClient(map[string]provider.Client{
		"proxy": &fakeProvider{err: providerErr},
	})
	if err != nil {
		t.Fatalf("provider.NewRoutedClient() error = %v", err)
	}

	runtime := newRuntimeWithClient(t, router)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "proxy", ModelID: "primary"},
	}
	runtime.Config.OpenAICompatible = OpenAICompatibleProviderConfig{
		ProviderID: "proxy",
		BaseURL:    "http://proxy.invalid/chat/completions",
		APIKey:     "test-key",
	}
	runtime.SetLogger(logger)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("turn status = %q, want failed", result.Status)
	}

	debugLog := readAppLogFile(t, filepath.Join(logDir, observability.DebugLogName))
	if !strings.Contains(debugLog, "provider route attempt failed") || !strings.Contains(debugLog, "provider_status=502") || !strings.Contains(debugLog, "provider_retryable=true") {
		t.Fatalf("debug log missing structured provider failure fields: %q", debugLog)
	}

	operationsLog := readAppLogFile(t, filepath.Join(logDir, observability.OperationsLogName))
	if !strings.Contains(operationsLog, "provider request failed") || !strings.Contains(operationsLog, "provider_status=502") || !strings.Contains(operationsLog, "provider_retryable=true") {
		t.Fatalf("operations log missing structured provider failure fields: %q", operationsLog)
	}
}

func TestRuntimeReconfigureReplacesLoggerFromConfig(t *testing.T) {
	initialLogDir := t.TempDir()
	updatedLogDir := t.TempDir()
	runtime, err := NewRuntime(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: OpenAIProviderConfig{
			APIKey:  "test-key",
			BaseURL: "http://example.invalid/v1/responses",
		},
		Logging: observability.Config{
			Dir: initialLogDir,
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := runtime.Close(); closeErr != nil {
			t.Fatalf("runtime.Close() error = %v", closeErr)
		}
	})

	err = runtime.Reconfigure(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: OpenAIProviderConfig{
			APIKey:  "test-key",
			BaseURL: "http://example.invalid/v1/responses",
		},
		Logging: observability.Config{
			Dir:          updatedLogDir,
			DebugEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("Reconfigure() error = %v", err)
	}

	runtime.log("runtime").Op("reconfigured ops")
	runtime.log("runtime").Debug("reconfigured debug")

	operationsLog := readAppLogFile(t, filepath.Join(updatedLogDir, observability.OperationsLogName))
	if !strings.Contains(operationsLog, "reconfigured ops") {
		t.Fatalf("operations log missing reconfigured entry: %q", operationsLog)
	}

	debugLog := readAppLogFile(t, filepath.Join(updatedLogDir, observability.DebugLogName))
	if !strings.Contains(debugLog, "reconfigured debug") {
		t.Fatalf("debug log missing reconfigured entry: %q", debugLog)
	}

	initialLog := readAppLogFile(t, filepath.Join(initialLogDir, observability.OperationsLogName))
	if strings.Contains(initialLog, "reconfigured ops") {
		t.Fatalf("initial operations log unexpectedly received reconfigured entry: %q", initialLog)
	}
}

func TestRuntimeDebugLogIncludesSnapshotAndCancelNotRunning(t *testing.T) {
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.SetLogger(logger)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.SnapshotSession(context.Background(), sessionID); err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	if err := runtime.CancelSessionTurn(context.Background(), CancelSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-missing",
	}); !errors.Is(err, ErrTurnNotRunning) {
		t.Fatalf("CancelSessionTurn() error = %v, want ErrTurnNotRunning", err)
	}

	debugLog := readAppLogFile(t, filepath.Join(logDir, observability.DebugLogName))
	for _, needle := range []string{
		"session snapshot requested",
		"session snapshot loaded",
		"session turn cancellation ignored; turn not running",
	} {
		if !strings.Contains(debugLog, needle) {
			t.Fatalf("debug log missing %q: %q", needle, debugLog)
		}
	}
}

func readAppLogFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}
