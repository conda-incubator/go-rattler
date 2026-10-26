//go:build cgo

package gorattler

import (
	"errors"
	"fmt"
)

// ErrInvalidInput reports a request the native layer rejected as invalid
// (bad JSON, bad pointer, or a domain-level input error). Match with errors.Is.
var ErrInvalidInput = errors.New("invalid input")

// ErrNative reports an internal native failure, such as a panic or a
// serialization error. Match with errors.Is.
var ErrNative = errors.New("native failure")

// Error describes a failed native operation. Use errors.As to inspect it.
// errors.Is matches ErrInvalidInput or ErrNative according to Status.
type Error struct {
	// Operation is the numeric native operation ID that failed.
	Operation uint32
	// Status is the native status code (2 domain error, 3 invalid input, 4 internal).
	Status int
	// Message is the human-readable message from the native error envelope.
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("native operation %d failed (status %d): %s", e.Operation, e.Status, e.Message)
}

// Is reports whether target is the sentinel matching this error's status.
func (e *Error) Is(target error) bool {
	switch target {
	case ErrInvalidInput:
		return e.Status == 2 || e.Status == 3
	case ErrNative:
		return e.Status == 4
	}
	return false
}
