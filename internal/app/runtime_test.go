package app

import (
	"testing"
)

func TestNewRuntimeAllowsUnsetModelRoute(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if runtime == nil || runtime.Provider == nil || runtime.Runner == nil {
		t.Fatalf("runtime = %#v", runtime)
	}
}
