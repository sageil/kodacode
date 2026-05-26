package app

import (
	"context"
	"io"
)

const (
	backgroundLogReadLimit = 16 * 1024
)

type BackgroundExecutionLogStore interface {
	Create(context.Context, BackgroundExecutionLogKey) (BackgroundExecutionLogHandle, error)
	ReadTail(context.Context, string, int) (string, int64, error)
	ReadPrefix(context.Context, string, int) (string, int64, error)
	ReadFrom(context.Context, string, int64, int) (string, int64, error)
}

type BackgroundExecutionLogKey struct {
	SessionID   string
	TurnID      string
	ExecutionID string
}

type BackgroundExecutionLogHandle struct {
	Ref    string
	Writer io.WriteCloser
}
