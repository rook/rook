/*
	Package errors is a drop-in replacement for the standard errors package and github.com/pkg/errors.


	Overview

	This is a single, lightweight library merging the features of standard library `errors` package
	and https://github.com/pkg/errors. It also backports a few features
	(like Go 1.13 error handling related features).


	Printing errors

	If not stated otherwise, errors can be formatted with the following specifiers:
		%s	error message
		%q	double-quoted error message
		%v	error message in default format
		%+v	error message and stack trace
*/
package errors // import "emperror.dev/errors"

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
)

// NewPlain returns a simple error without any annotated context, like stack trace.
// Useful for creating sentinel errors and in testing.
//
//		var ErrSomething = errors.NewPlain("something went wrong")
func NewPlain(message string) error {
	return &plainError{message}
}

// plainError is a trivial implementation of error.
type plainError struct {
	msg string
}

func (e *plainError) Error() string {
	return e.msg
}

// Sentinel is a simple error without any annotated context, like stack trace.
// Useful for creating sentinel errors.
//
// 	const ErrSomething = errors.Sentinel("something went wrong")
//
// See https://dave.cheney.net/2016/04/07/constant-errors
type Sentinel string

func (e Sentinel) Error() string {
	return string(e)
}

// New returns a new error annotated with stack trace at the point New is called.
//
// New is a shorthand for:
//		WithStack(NewPlain(message))
func New(message string) error {
	return WithStackDepth(NewPlain(message), 1)
}

// NewWithDetails returns a new error annotated with stack trace at the point NewWithDetails is called,
// and the supplied details.
func NewWithDetails(message string, details ...interface{}) error {
	return WithDetails(WithStackDepth(NewPlain(message), 1), details...)
}

// Errorf returns a new error with a formatted message and annotated with stack trace at the point Errorf is called.
//
//		err := errors.Errorf("something went %s", "wrong")
func Errorf(format string, a ...interface{}) error {
	return WithStackDepth(NewPlain(fmt.Sprintf(format, a...)), 1)
}

// WithStack annotates err with a stack trace at the point WithStack was called.
// If err is nil, WithStack returns nil.
//
// WithStack is commonly used with sentinel errors and errors returned from libraries
// not annotating errors with stack trace:
//
//		var ErrSomething = errors.NewPlain("something went wrong")
//
//		func doSomething() error {
//			return errors.WithStack(ErrSomething)
//		}
func WithStack(err error) error {
	return WithStackDepth(err, 1)
}

// WithStackDepth annotates err with a stack trace at the given call depth.
// Zero identifies the caller of WithStackDepth itself.
// If err is nil, WithStackDepth returns nil.
//
// WithStackDepth is generally used in other error constructors:
//
//		func MyWrapper(err error) error {
//			return WithStackDepth(err, 1)
//		}
func WithStackDepth(err error, depth int) error {
	if err == nil {
		return nil
	}

	return &withStack{
		error: err,
		stack: callers(depth + 1),
	}
}

// WithStackIf behaves the same way as WithStack except it does not annotate the error with a stack trace
// if there is already one in err's chain.
func WithStackIf(err error) error {
	return WithStackDepthIf(err, 1)
}

// WithStackDepthIf behaves the same way as WithStackDepth except it does not annotate the error with a stack trace
// if there is already one in err's chain.
func WithStackDepthIf(err error, depth int) error {
	if err == nil {
		return nil
	}

	var st interface{ StackTrace() errors.StackTrace }
	if !As(err, &st) {
		return &withStack{
			error: err,
			stack: callers(depth + 1),
		}
	}

	return err
}

type withStack struct {
	error
	*stack
}

func (w *withStack) Cause() error  { return w.error }
func (w *withStack) Unwrap() error { return w.error }

// nolint: errcheck
func (w *withStack) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			fmt.Fprintf(s, "%+v", w.error)
			w.stack.Format(s, verb)

			return
		}

		fallthrough
	case 's':
		io.WriteString(s, w.Error())
	case 'q':
		fmt.Fprintf(s, "%q", w.Error())
	}
}

// WithMessage annotates err with a new message.
// If err is nil, WithMessage returns nil.
//
// WithMessage is useful when the error already contains a stack trace, but adding additional info to the message
// helps in debugging.
//
// Errors returned by WithMessage are formatted slightly differently:
//		%s	error messages separated by a colon and a space (": ")
//		%q	double-quoted error messages separated by a colon and a space (": ")
//		%v	one error message per line
//		%+v	one error message per line and stack trace (if any)
func WithMessage(err error, message string) error {
	return errors.WithMessage(err, message)
}

// WithMessagef annotates err with the format specifier.
// If err is nil, WithMessagef returns nil.
//
// WithMessagef is useful when the error already contains a stack trace, but adding additional info to the message
// helps in debugging.
//
// The same formatting rules apply as in case of WithMessage.
func WithMessagef(err error, format string, a ...interface{}) error {
	return errors.WithMessagef(err, format, a...)
}

// Wrap returns an error annotating err with a stack trace
// at the point Wrap is called, and the supplied message.
// If err is nil, Wrap returns nil.
//
// Wrap is a shorthand for:
//		WithStack(WithMessage(err, message))
func Wrap(err error, message string) error {
	return WithStackDepth(WithMessage(err, message), 1)
}

// Wrapf returns an error annotating err with a stack trace
// at the point Wrapf is called, and the format specifier.
// If err is nil, Wrapf returns nil.
//
// Wrapf is a shorthand for:
//		WithStack(WithMessagef(err, format, a...))
func Wrapf(err error, format string, a ...interface{}) error {
	return WithStackDepth(WithMessagef(err, format, a...), 1)
}

// WrapIf behaves the same way as Wrap except it does not annotate the error with a stack trace
// if there is already one in err's chain.
//
// If err is nil, WrapIf returns nil.
func WrapIf(err error, message string) error {
	return WithStackDepthIf(WithMessage(err, message), 1)
}

// WrapIff behaves the same way as Wrapf except it does not annotate the error with a stack trace
// if there is already one in err's chain.
//
// If err is nil, WrapIff returns nil.
func WrapIff(err error, format string, a ...interface{}) error {
	return WithStackDepthIf(WithMessagef(err, format, a...), 1)
}

// WrapWithDetails returns an error annotating err with a stack trace
// at the point WrapWithDetails is called, and the supplied message and details.
// If err is nil, WrapWithDetails returns nil.
//
// WrapWithDetails is a shorthand for:
//		WithDetails(WithStack(WithMessage(err, message, details...))
func WrapWithDetails(err error, message string, details ...interface{}) error {
	return WithDetails(WithStackDepth(WithMessage(err, message), 1), details...)
}

// WrapIfWithDetails returns an error annotating err with a stack trace
// at the point WrapIfWithDetails is called, and the supplied message and details.
// If err is nil, WrapIfWithDetails returns nil.
//
// WrapIfWithDetails is a shorthand for:
//		WithDetails(WithStackIf(WithMessage(err, message, details...))
func WrapIfWithDetails(err error, message string, details ...interface{}) error {
	return WithDetails(WithStackDepthIf(WithMessage(err, message), 1), details...)
}
