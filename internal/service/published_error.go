package service

import "errors"

type publishedError struct {
	err error
}

func (e publishedError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e publishedError) Unwrap() error {
	return e.err
}

// markPublishedError wraps an error that has already been surfaced to the
// client over SSE so upstream async dispatchers do not emit a duplicate error.
func markPublishedError(err error) error {
	if err == nil {
		return nil
	}
	return publishedError{err: err}
}

// ErrorAlreadyPublished reports whether err has already been emitted to the
// client as an SSE error event.
func ErrorAlreadyPublished(err error) bool {
	var published publishedError
	return errors.As(err, &published)
}
