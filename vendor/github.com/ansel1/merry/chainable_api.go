package merry

import (
	"fmt"
	v2 "github.com/ansel1/merry/v2"
)

// Error extends the standard golang `error` interface with functions
// for attachment additional data to the error
type Error interface {
	error
	Appendf(format string, args ...interface{}) Error
	Append(msg string) Error
	Prepend(msg string) Error
	Prependf(format string, args ...interface{}) Error
	WithMessage(msg string) Error
	WithMessagef(format string, args ...interface{}) Error
	WithUserMessage(msg string) Error
	WithUserMessagef(format string, args ...interface{}) Error
	WithValue(key, value interface{}) Error
	Here() Error
	WithStackSkipping(skip int) Error
	WithHTTPCode(code int) Error
	WithCause(err error) Error
	Cause() error
	fmt.Formatter
}

// make sure errImpl implements Error
var _ Error = (*errImpl)(nil)

// WithValue is equivalent to WithValue(e, key, value).
func (e *errImpl) WithValue(key, value interface{}) Error {
	return WrapSkipping(e, 1, v2.WithValue(key, value))
}

// Here is equivalent to Here(e).
func (e *errImpl) Here() Error {
	return HereSkipping(e, 1)
}

// WithStackSkipping is equivalent to HereSkipping(e, i).
func (e *errImpl) WithStackSkipping(skip int) Error {
	return HereSkipping(e, skip+1)
}

// WithHTTPCode is equivalent to WithHTTPCode(e, code).
func (e *errImpl) WithHTTPCode(code int) Error {
	return WrapSkipping(e, 1, v2.WithHTTPCode(code))
}

// WithMessage is equivalent to WithMessage(e, msg).
func (e *errImpl) WithMessage(msg string) Error {
	return WrapSkipping(e, 1, v2.WithMessage(msg))
}

// WithMessagef is equivalent to WithMessagef(e, format, args...).
func (e *errImpl) WithMessagef(format string, args ...interface{}) Error {
	return WrapSkipping(e, 1, v2.WithMessagef(format, args...))
}

// WithUserMessage is equivalent to WithUserMessage(e, msg).
func (e *errImpl) WithUserMessage(msg string) Error {
	return WrapSkipping(e, 1, v2.WithUserMessage(msg))
}

// WithUserMessagef is equivalent to WithUserMessagef(e, format, args...).
func (e *errImpl) WithUserMessagef(format string, args ...interface{}) Error {
	return WrapSkipping(e, 1, v2.WithUserMessagef(format, args...))
}

// Append is equivalent to Append(err, msg).
func (e *errImpl) Append(msg string) Error {
	return WrapSkipping(e, 1, v2.AppendMessage(msg))
}

// Appendf is equivalent to Appendf(err, format, msg).
func (e *errImpl) Appendf(format string, args ...interface{}) Error {
	return WrapSkipping(e, 1, v2.AppendMessagef(format, args...))
}

// Prepend is equivalent to Prepend(err, msg).
func (e *errImpl) Prepend(msg string) Error {
	return WrapSkipping(e, 1, v2.PrependMessage(msg))
}

// Prependf is equivalent to Prependf(err, format, args...).
func (e *errImpl) Prependf(format string, args ...interface{}) Error {
	return WrapSkipping(e, 1, v2.PrependMessagef(format, args...))
}

// WithCause is equivalent to WithCause(e, err).
func (e *errImpl) WithCause(err error) Error {
	return WrapSkipping(e, 1, v2.WithCause(err))
}

// errImpl coerces an error to an Error
type errImpl struct {
	err error
}

func coerce(err error) Error {
	if err == nil {
		return nil
	}

	if e, ok := err.(Error); ok {
		return e
	}

	return &errImpl{err}
}

// Format implements fmt.Formatter.
func (e *errImpl) Format(s fmt.State, verb rune) {
	// the inner err should always be an err produced
	// by v2
	if f, ok := e.err.(fmt.Formatter); ok {
		f.Format(s, verb)
		return
	}

	// should never happen, but fall back on something
	v2.Format(s, verb, e)
}

// Error implements the error interface.
func (e *errImpl) Error() string {
	return e.err.Error()
}

// Unwrap returns the next wrapped error.
func (e *errImpl) Unwrap() error {
	return e.err
}

// Cause implements Error.
func (e *errImpl) Cause() error {
	return Cause(e.err)
}
