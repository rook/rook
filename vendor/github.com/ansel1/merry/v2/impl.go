package merry

import (
	"errors"
	"fmt"
	"reflect"
)

type errKey int

const (
	errKeyNone errKey = iota
	errKeyStack
	errKeyMessage
	errKeyHTTPCode
	errKeyUserMessage
	errKeyForceCapture
	errKeyHooked
)

func (e errKey) String() string {
	switch e {
	case errKeyNone:
		return "none"
	case errKeyStack:
		return "stack"
	case errKeyMessage:
		return "message"
	case errKeyHTTPCode:
		return "http status code"
	case errKeyUserMessage:
		return "user message"
	case errKeyForceCapture:
		return "force stack capture"
	default:
		return ""
	}
}

// formatError adds a Format implementation to an error.
type formatError struct {
	error
}

// Format implements fmt.Formatter
func (e *formatError) Format(s fmt.State, verb rune) {
	Format(s, verb, e)
}

// String implements fmt.Stringer
func (e *formatError) String() string {
	return e.Error()
}

// Unwrap returns the next wrapped error.
func (e *formatError) Unwrap() error {
	return e.error
}

type errWithValue struct {
	err        error
	key, value interface{}
}

// Format implements fmt.Formatter
func (e *errWithValue) Format(s fmt.State, verb rune) {
	Format(s, verb, e)
}

// Error implements golang's error interface
// returns the message value if set, otherwise
// delegates to inner error
func (e *errWithValue) Error() string {
	if e.key == errKeyMessage {
		if s, ok := e.value.(string); ok {
			return s
		}
	}
	return e.err.Error()
}

// String implements fmt.Stringer
func (e *errWithValue) String() string {
	return e.Error()
}

// Unwrap returns the next wrapped error.
func (e *errWithValue) Unwrap() error {
	return e.err
}

// isMerryError is a marker method for identifying error types implemented by this package.
func (e *errWithValue) isMerryError() {}

type errWithCause struct {
	err   error
	cause error
}

func (e *errWithCause) Unwrap() error {
	// skip through any directly nested errWithCauses.
	// our implementation of Is/As already recursed through them,
	// so we want to dig down to the first non-errWithCause.

	nextErr := e.err
	for {
		if e, ok := nextErr.(*errWithCause); ok {
			nextErr = e.err
		} else {
			break
		}
	}

	// errWithCause.Is/As() also already checked nextErr, so we want to
	// unwrap it and get to the next error down.
	nextErr = errors.Unwrap(nextErr)

	// we've reached the end of this wrapper chain.  Return the cause.
	if nextErr == nil {
		return e.cause
	}

	// return a new errWithCause wrapper, wrapping next error, but bundling
	// it will our cause, ignoring the causes of the errWithCauses we skip
	// over above.  This is how we carry the latest cause along as we unwrap
	// the chain.  When we get to the end of the chain, we'll return this latest
	// cause.
	return &errWithCause{err: nextErr, cause: e.cause}
}

func (e *errWithCause) String() string {
	return e.Error()
}

func (e *errWithCause) Error() string {
	return e.err.Error()
}

func (e *errWithCause) Format(f fmt.State, verb rune) {
	Format(f, verb, e)
}

// errWithCause needs to provide custom implementations of Is and As.
// errors.Is() doesn't work on errWithCause because error.Is() uses errors.Unwrap() to traverse the error
// chain.  But errWithCause.Unwrap() doesn't return the next error in the chain.  Instead,
// it wraps the next error in a shim.  The standard Is/As tests would compare the shim to the target.
// We need to override Is/As to compare the target to the error inside the shim.

func (e *errWithCause) Is(target error) bool {
	// This does most of what errors.Is() does, by delegating
	// to the nested error.  But it does not use Unwrap to recurse
	// any further.  This just compares target with next error in the stack.
	isComparable := reflect.TypeOf(target).Comparable()
	if isComparable && e.err == target {
		return true
	}

	// since errWithCause implements Is(), this will effectively recurse through
	// any directly nested errWithCauses.
	if x, ok := e.err.(interface{ Is(error) bool }); ok && x.Is(target) {
		return true
	}
	return false
}

func (e *errWithCause) As(target interface{}) bool {
	// This does most of what errors.As() does, by delegating
	// to the nested error.  But it does not use Unwrap to recurse
	// any further. This just compares target with next error in the stack.
	val := reflect.ValueOf(target)
	typ := val.Type()
	targetType := typ.Elem()
	if reflect.TypeOf(e.err).AssignableTo(targetType) {
		val.Elem().Set(reflect.ValueOf(e.err))
		return true
	}

	// since errWithCause implements As(), this will effectively recurse through
	// any directly nested errWithCauses.
	if x, ok := e.err.(interface{ As(interface{}) bool }); ok && x.As(target) {
		return true
	}
	return false
}

// isMerryError is a marker method for identifying error types implemented by this package.
func (e *errWithCause) isMerryError() {}
