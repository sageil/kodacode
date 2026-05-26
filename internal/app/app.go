package app

import (
	"context"
	"io"
	"os"
)

// Run executes the plain command-line path.
func Run(ctx context.Context, out io.Writer) error {
	return RunCLI(ctx, os.Stdin, out, os.Args[1:], os.Getenv, os.Getwd)
}

func RunCLI(ctx context.Context, in io.Reader, out io.Writer, args []string, getenv func(string) string, getwd func() (string, error)) error {
	return runCommand(ctx, in, out, args, getenv, getwd, NewRuntime)
}
