// Package main is the entry point for the kodacode application.
// It starts the embedded Echo HTTP API server and then launches the Bubble Tea TUI.
// Both run in the same process; the TUI communicates with the API over localhost.
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"

	"github.com/sageil/kodacode/v1/internal/bootstrap"
	"github.com/sageil/kodacode/v1/internal/config"
)

// version, commit, and date are set by goreleaser via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Direct logs to a file so they're visible even when the TUI owns stderr.
	logDir := config.DataDir()
	_ = os.MkdirAll(logDir, 0o700)
	if f, err := os.OpenFile(filepath.Join(logDir, "kodacode.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(f)
	}

	// Force exit on second SIGINT/SIGTERM. The first is handled by Bubble Tea
	// as a key event, but if the TUI is blocked the key never processes.
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		<-sig
		os.Exit(0)
	}()

	// Recover from panics to ensure they're logged before crashing.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("kodacode: panic recovered: %v\n%s", r, debug.Stack())
			os.Exit(1)
		}
	}()

	resumeFlag := false
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("kodacode %s (commit %s, built %s)\n", version, commit, date)
			return
		case "login":
			if err := runLogin(); err != nil {
				log.Fatalf("kodacode login: %v", err)
			}
			return
		case "logout":
			if err := runLogout(); err != nil {
				log.Fatalf("kodacode logout: %v", err)
			}
			return
		case "--resume", "-r", "resume":
			resumeFlag = true
		}
	}

	bootstrap.EnsureDefaults()

	if err := run(resumeFlag); err != nil {
		log.Fatalf("kodacode: %v", err)
	}
}
