package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/bootstrap"
	"github.com/sageil/kodacode/internal/tui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	log.SetPrefix("kodacode " + buildInfo() + " ")

	ctx, stop := signal.NotifyContext(context.TODO(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	if bootstrapErr := bootstrap.EnsureDefaults(); bootstrapErr != nil {
		err = bootstrapErr
	} else if app.IsInteractiveTerminal(os.Stdin, os.Stdout) {
		err = tui.Run(ctx, os.Stdin, os.Stdout, os.Args[1:], os.Getenv, os.Getwd)
	} else {
		err = app.RunCLI(ctx, os.Stdin, os.Stdout, os.Args[1:], os.Getenv, os.Getwd)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func buildInfo() string {
	return "version=" + version + " commit=" + commit + " date=" + date
}
