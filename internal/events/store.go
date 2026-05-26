package events

import (
	"context"
	"errors"
	"time"
)

type Query struct {
	SessionID     string
	AfterSequence int64
	ExcludeTypes  []Type
}

func (q Query) Validate() error {
	if q.SessionID == "" {
		return ErrSessionRequired
	}
	if q.AfterSequence < -1 {
		return ErrAfterSequenceInvalid
	}
	for _, typ := range q.ExcludeTypes {
		if typ == "" {
			return errors.New("exclude_types must not contain empty values")
		}
	}
	return nil
}

type LatestQuery struct {
	SessionID string
	Types     []Type
}

func (q LatestQuery) Validate() error {
	if q.SessionID == "" {
		return ErrSessionRequired
	}
	if len(q.Types) == 0 {
		return errors.New("types are required")
	}
	for _, typ := range q.Types {
		if typ == "" {
			return errors.New("types must not contain empty values")
		}
	}
	return nil
}

type Appender interface {
	Append(context.Context, Draft) (Event, error)
}

type WatchStore interface {
	Watch(context.Context, Query) (<-chan Event, error)
}

type ReplayStore interface {
	Appender
	WatchStore
	Replay(context.Context, Query) ([]Event, error)
	Latest(context.Context, LatestQuery) (Event, bool, error)
}

type SessionIndexEntry struct {
	SessionID     string
	WorkspaceRoot string
	UpdatedAt     time.Time
}
