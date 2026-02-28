package merry

// The merry package augments standard golang errors with stacktraces
// and other context information.
//
// You can add any context information to an error with `e = merry.WithValue(e, "code", 12345)`
// You can retrieve that value with `v, _ := merry.Value(e, "code").(int)`
//
// Any error augmented like this will automatically get a stacktrace attached, if it doesn't have one
// already.  If you just want to add the stacktrace, use `Wrap(e)`
//
// It also providers a way to override an error's message:
//
//     var InvalidInputs = errors.New("Bad inputs")
//
// `Here()` captures a new stacktrace, and WithMessagef() sets a new error message:
//
//     return merry.Here(InvalidInputs).WithMessagef("Bad inputs: %v", inputs)
//
// Errors are immutable.  All functions and methods which add context return new errors.
// But errors can still be compared to the originals with `Is()`
//
//     if merry.Is(err, InvalidInputs) {
//
// Functions which add context to errors have equivalent methods on *Error, to allow
// convenient chaining:
//
//     return merry.New("Invalid body").WithHTTPCode(400)
//
// merry.Errors also implement fmt.Formatter, similar to github.com/pkg/errors.
//
//     fmt.Sprintf("%+v", e) == merry.Details(e)
//
// pkg/errors Cause() interface is not implemented (yet).
import (
	"errors"
	"fmt"
	v2 "github.com/ansel1/merry/v2"
)

// MaxStackDepth is no longer used.  It remains here for backward compatibility.
// deprecated: See Set/GetMaxStackDepth.
var MaxStackDepth = 50

// StackCaptureEnabled returns whether stack capturing is enabled
func StackCaptureEnabled() bool {
	return v2.StackCaptureEnabled()
}

// SetStackCaptureEnabled sets stack capturing globally.  Disabling stack capture can increase performance
func SetStackCaptureEnabled(enabled bool) {
	v2.SetStackCaptureEnabled(enabled)
}

// VerboseDefault no longer has any effect.
// deprecated: see SetVerboseDefault
func VerboseDefault() bool {
	return false
}

// SetVerboseDefault used to control the behavior of the Error() function on errors
// processed by this package.  Error() now always just returns the error's message.
// This setting no longer has any effect.
// deprecated: To print the details of an error, use Details(err), or format the
// error with the verbose flag: fmt.Sprintf("%+v", err)
func SetVerboseDefault(bool) {
}

// GetMaxStackDepth returns the number of frames captured in stacks.
func GetMaxStackDepth() int {
	return v2.MaxStackDepth()
}

// SetMaxStackDepth sets the MaxStackDepth.
func SetMaxStackDepth(depth int) {
	v2.SetMaxStackDepth(depth)
}

// New creates a new error, with a stack attached.  The equivalent of golang's errors.New().
// Accepts v2 wrappers to apply to the error.
func New(msg string, wrappers ...v2.Wrapper) Error {
	return WrapSkipping(errors.New(msg), 1, wrappers...)
}

// Errorf creates a new error with a formatted message and a stack.  The equivalent of golang's fmt.Errorf().
// args can be format args, or v2 wrappers which will be applied to the error.
func Errorf(format string, args ...interface{}) Error {
	var wrappers []v2.Wrapper

	// pull out the args which are wrappers
	n := 0
	for _, arg := range args {
		if w, ok := arg.(v2.Wrapper); ok {
			wrappers = append(wrappers, w)
		} else {
			args[n] = arg
			n++
		}
	}
	args = args[:n]

	return WrapSkipping(fmt.Errorf(format, args...), 1, wrappers...)
}

// UserError creates a new error with a message intended for display to an
// end user.
func UserError(msg string) Error {
	return WrapSkipping(errors.New(msg), 1, v2.WithUserMessage(msg))
}

// UserErrorf is like UserError, but uses fmt.Sprintf()
func UserErrorf(format string, args ...interface{}) Error {
	msg := fmt.Sprintf(format, args...)
	return WrapSkipping(errors.New(msg), 1, v2.WithUserMessagef(msg))
}

// Wrap turns the argument into a merry.Error.  If the argument already is a
// merry.Error, this is a no-op.
// If e == nil, return nil
func Wrap(err error, wrappers ...v2.Wrapper) Error {
	return coerce(v2.WrapSkipping(err, 1, wrappers...))
}

// WrapSkipping turns the error arg into a merry.Error if the arg is not
// already a merry.Error.
// If e is nil, return nil.
// If a merry.Error is created by this call, the stack captured will skip
// `skip` frames (0 is the call site of `WrapSkipping()`)
func WrapSkipping(err error, skip int, wrappers ...v2.Wrapper) Error {
	return coerce(v2.WrapSkipping(err, skip+1, wrappers...))
}

// WithValue adds a context an error.  If the key was already set on e,
// the new value will take precedence.
// If e is nil, returns nil.
func WithValue(err error, key, value interface{}) Error {
	return WrapSkipping(err, 1, v2.WithValue(key, value))
}

// Value returns the value for key, or nil if not set.
// If e is nil, returns nil.
func Value(err error, key interface{}) interface{} {
	return v2.Value(err, key)
}

// Values returns a map of all values attached to the error
// If a key has been attached multiple times, the map will
// contain the last value mapped
// If e is nil, returns nil.
func Values(err error) map[interface{}]interface{} {
	return v2.Values(err)
}

// RegisteredDetails extracts details registered with RegisterDetailFunc from an error, and
// returns them as a map.  Values may be nil.
//
// If err is nil or there are no registered details, nil is returned.
func RegisteredDetails(err error) map[string]interface{} {
	return v2.RegisteredDetails(err)
}

// Here returns an error with a new stacktrace, at the call site of Here().
// Useful when returning copies of exported package errors.
// If e is nil, returns nil.
func Here(err error) Error {
	return WrapSkipping(err, 1, v2.CaptureStack(false))
}

// HereSkipping returns an error with a new stacktrace, at the call site
// of HereSkipping() - skip frames.
func HereSkipping(err error, skip int) Error {
	return WrapSkipping(err, skip+1, v2.CaptureStack(false))
}

// Message returns just returns err.Error().  It is here for
// historical reasons.
func Message(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Stack returns the stack attached to an error, or nil if one is not attached
// If e is nil, returns nil.
func Stack(err error) []uintptr {
	return v2.Stack(err)
}

// WithHTTPCode returns an error with an http code attached.
// If e is nil, returns nil.
func WithHTTPCode(e error, code int) Error {
	return WrapSkipping(e, 1, v2.WithHTTPCode(code))
}

// HTTPCode converts an error to an http status code.  All errors
// map to 500, unless the error has an http code attached.
// If e is nil, returns 200.
func HTTPCode(err error) int {
	return v2.HTTPCode(err)
}

// UserMessage returns the end-user safe message.  Returns empty if not set.
// If e is nil, returns "".
func UserMessage(err error) string {
	return v2.UserMessage(err)
}

// Cause returns the cause of the argument.  If e is nil, or has no cause,
// nil is returned.
func Cause(err error) error {
	return v2.Cause(err)
}

// RootCause returns the innermost cause of the argument (i.e. the last
// error in the cause chain)
func RootCause(err error) error {
	for {
		cause := Cause(err)
		if cause == nil {
			return err
		}
		err = cause
	}
}

// WithCause returns an error based on the first argument, with the cause
// set to the second argument.  If e is nil, returns nil.
func WithCause(err error, cause error) Error {
	return WrapSkipping(err, 1, v2.WithCause(cause))
}

// WithMessage returns an error with a new message.
// The resulting error's Error() method will return
// the new message.
// If e is nil, returns nil.
func WithMessage(err error, msg string) Error {
	return WrapSkipping(err, 1, v2.WithMessage(msg))
}

// WithMessagef is the same as WithMessage(), using fmt.Sprintf().
func WithMessagef(err error, format string, args ...interface{}) Error {
	return WrapSkipping(err, 1, v2.WithMessagef(format, args...))
}

// WithUserMessage adds a message which is suitable for end users to see.
// If e is nil, returns nil.
func WithUserMessage(err error, msg string) Error {
	return WrapSkipping(err, 1, v2.WithUserMessage(msg))
}

// WithUserMessagef is the same as WithMessage(), using fmt.Sprintf()
func WithUserMessagef(err error, format string, args ...interface{}) Error {
	return WrapSkipping(err, 1, v2.WithUserMessagef(format, args...))
}

// Append a message after the current error message, in the format "original: new".
// If e == nil, return nil.
func Append(err error, msg string) Error {
	return WrapSkipping(err, 1, v2.AppendMessage(msg))
}

// Appendf is the same as Append, but uses fmt.Sprintf().
func Appendf(err error, format string, args ...interface{}) Error {
	return WrapSkipping(err, 1, v2.AppendMessagef(format, args...))
}

// Prepend a message before the current error message, in the format "new: original".
// If e == nil, return nil.
func Prepend(err error, msg string) Error {
	return WrapSkipping(err, 1, v2.PrependMessage(msg))
}

// Prependf is the same as Prepend, but uses fmt.Sprintf()
func Prependf(err error, format string, args ...interface{}) Error {
	return WrapSkipping(err, 1, v2.PrependMessagef(format, args...))
}

// Is is equivalent to errors.Is, but tests against multiple targets.
//
// merry.Is(err1, err2, err3) == errors.Is(err1, err2) || errors.Is(err1, err3)
func Is(e error, originals ...error) bool {
	for _, o := range originals {
		if errors.Is(e, o) {
			return true
		}
	}
	return false
}

// Unwrap returns the innermost underlying error.
// This just calls errors.Unwrap() until if finds the deepest error.
// It isn't very useful, and only remains for historical purposes
//
// deprecated: use errors.Is() or errors.As() instead.
func Unwrap(e error) error {
	for {
		next := errors.Unwrap(e)
		if next == nil {
			return e
		}
		e = next
	}
}
