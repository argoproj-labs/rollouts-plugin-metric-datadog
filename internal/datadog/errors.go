package datadog

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
)

// TransientError marks an error caused by Datadog itself being unavailable
// (5xx, 429 after SDK retries, timeout, dropped connection), as opposed to a
// setup error (bad query, key, permission, address). Error() returns the
// wrapped text unchanged.
type TransientError struct{ Err error }

func (e *TransientError) Error() string { return e.Err.Error() }
func (e *TransientError) Unwrap() error { return e.Err }

// MarkTransient wraps err in a TransientError when resp or err shows that
// Datadog is unavailable. Anything else, including DNS and TLS failures that
// usually mean a misconfigured address, is returned unchanged so it fails closed.
func MarkTransient(err error, resp *http.Response) error {
	if err == nil {
		return nil
	}
	if isTransient(err, resp) {
		return &TransientError{Err: err}
	}
	return err
}

func isTransient(err error, resp *http.Response) bool {
	if resp != nil && (resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests) {
		return true
	}
	var ne net.Error
	return errors.Is(err, context.DeadlineExceeded) ||
		(errors.As(err, &ne) && ne.Timeout()) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF)
}
