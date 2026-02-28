package merry

import (
	"errors"
	"fmt"
	"runtime"
)

// New creates a new error, with a stack attached.  The equivalent of golang's errors.New()
func New(msg string, wrappers ...Wrapper) error {
	return WrapSkipping(errors.New(msg), 1, wrappers...)
}

// Errorf creates a new error with a formatted message and a stack.  The equivalent of golang's fmt.Errorf().
// args may contain either arguments to format, or Wrapper options, which will be applied to the error.
func Errorf(format string, args ...interface{}) error {
	fmtArgs, wrappers := splitWrappers(args)

	return WrapSkipping(fmt.Errorf(format, fmtArgs...), 1, wrappers...)
}

// Sentinel creates an error without running hooks or capturing a stack.  It is intended
// to create sentinel errors, which will be wrapped with a stack later from where the
// error is returned.  At that time, a stack will be captured and hooks will be run.
//
//	var ErrNotFound = merry.Sentinel("not found", merry.WithHTTPCode(404))
//
//	func FindUser(name string) (*User, error) {
//	  // some db code which fails to find a user
//	  return nil, merry.Wrap(ErrNotFound)
//	}
//
//	func main() {
//	  _, err := FindUser("bob")
//	  fmt.Println(errors.Is(err, ErrNotFound) // "true"
//	  fmt.Println(merry.Details(err))         // stacktrace will start at the return statement
//	                                          // in FindUser()
//	}
func Sentinel(msg string, wrappers ...Wrapper) error {
	return ApplySkipping(errors.New(msg), 1, wrappers...)
}

// Sentinelf is like Sentinel, but takes a formatted message.  args can be a mix of
// format arguments and Wrappers.
func Sentinelf(format string, args ...interface{}) error {
	fmtArgs, wrappers := splitWrappers(args)

	return ApplySkipping(fmt.Errorf(format, fmtArgs...), 1, wrappers...)
}

func splitWrappers(args []interface{}) ([]interface{}, []Wrapper) {
	var wrappers []Wrapper

	// pull out the args which are wrappers
	n := 0
	for _, arg := range args {
		if w, ok := arg.(Wrapper); ok {
			wrappers = append(wrappers, w)
		} else {
			args[n] = arg
			n++
		}
	}
	args = args[:n]

	return args, wrappers
}

// Wrap adds context to errors by applying Wrappers.  See WithXXX() functions for Wrappers supplied
// by this package.
//
// If StackCaptureEnabled is true, a stack starting at the caller will be automatically captured
// and attached to the error.  This behavior can be overridden with wrappers which either capture
// their own stacks, or suppress auto capture.
//
// If err is nil, returns nil.
func Wrap(err error, wrappers ...Wrapper) error {
	return WrapSkipping(err, 1, wrappers...)
}

// WrapSkipping is like Wrap, but the captured stacks will start `skip` frames
// further up the call stack.  If skip is 0, it behaves the same as Wrap.
func WrapSkipping(err error, skip int, wrappers ...Wrapper) error {
	if err == nil {
		return nil
	}

	if len(onceHooks) > 0 {
		if _, ok := Lookup(err, errKeyHooked); !ok {
			err = ApplySkipping(err, skip+1, onceHooks...)
			err = ApplySkipping(err, skip+1, WithValue(errKeyHooked, err))
		}
	}
	err = ApplySkipping(err, skip+1, hooks...)
	err = ApplySkipping(err, skip+1, wrappers...)
	err = captureStack(err, skip+1, false)

	// ensure the resulting error implements Formatter
	// https://github.com/ansel1/merry/issues/26
	if _, ok := err.(fmt.Formatter); !ok {
		err = &formatError{err}
	}

	return err
}

// Apply is like Wrap, but does not execute hooks or do automatic stack capture.  It just
// applies the wrappers to the error.
func Apply(err error, wrappers ...Wrapper) error {
	return ApplySkipping(err, 1, wrappers...)
}

// ApplySkipping is like WrapSkipping, but does not execute hooks or do automatic stack capture.  It just
// applies the wrappers to the error.  It is useful in Wrapper implementations which
// // want to apply other Wrappers without starting an infinite recursion.
func ApplySkipping(err error, skip int, wrappers ...Wrapper) error {
	if err == nil {
		return nil
	}

	for _, w := range wrappers {
		err = w.Wrap(err, skip+1)
	}

	return err
}

// Prepend is a convenience function for the PrependMessage wrapper.  It eases migration
// from merry v1.  It accepts a varargs of additional Wrappers.
func Prepend(err error, msg string, wrappers ...Wrapper) error {
	return WrapSkipping(err, 1, append(wrappers, PrependMessage(msg))...)
}

// Prependf is a convenience function for the PrependMessagef wrapper.  It eases migration
// from merry v1.  The args can be format arguments mixed with Wrappers.
func Prependf(err error, format string, args ...interface{}) error {
	fmtArgs, wrappers := splitWrappers(args)

	return WrapSkipping(err, 1, append(wrappers, PrependMessagef(format, fmtArgs...))...)
}

// Append is a convenience function for the AppendMessage wrapper.  It eases migration
// from merry v1.  It accepts a varargs of additional Wrappers.
func Append(err error, msg string, wrappers ...Wrapper) error {
	return WrapSkipping(err, 1, append(wrappers, AppendMessage(msg))...)
}

// Appendf is a convenience function for the AppendMessagef wrapper.  It eases migration
// from merry v1.  The args can be format arguments mixed with Wrappers.
func Appendf(err error, format string, args ...interface{}) error {
	fmtArgs, wrappers := splitWrappers(args)

	return WrapSkipping(err, 1, append(wrappers, AppendMessagef(format, fmtArgs...))...)
}

// Value returns the value for key, or nil if not set.
// If e is nil, returns nil.  Will not search causes.
func Value(err error, key interface{}) interface{} {
	v, _ := Lookup(err, key)
	return v
}

// Lookup returns the value for the key, and a boolean indicating
// whether the value was set.  Will not search causes.
//
// if err is nil, returns nil and false.
func Lookup(err error, key interface{}) (interface{}, bool) {
	var merr interface {
		error
		isMerryError()
	}

	// I've tried implementing this logic a few different ways.  It's tricky:
	//
	// - Lookup should only search the current error, but not causes.  errWithCause's
	//   Unwrap() will eventually unwrap to the cause, so we don't want to just
	//   search the entire stream of errors returned by Unwrap.
	// - We need to handle cases where error implementations created outside
	//   this package are in the middle of the chain.  We need to use Unwrap
	//   in these cases to traverse those errors and dig down to the next
	//   merry error.
	// - Some error packages, including our own, do funky stuff with Unwrap(),
	//   returning shim types to control the unwrapping order, rather than
	//   the actual, raw wrapped error.  Typically, these shims implement
	//   Is/As to delegate to the raw error they encapsulate, but implement
	//   Unwrap by encapsulating the raw error in another shim.  So if we're looking
	//   for a raw error type, we can't just use Unwrap() and do type assertions
	//   against the result.  We have to use errors.As(), to allow the shims to delegate
	//   the type assertion to the raw error correctly.
	//
	// Based on all these constraints, we use errors.As() with an internal interface
	// that can only be implemented by our internal error types.  When one is found,
	// we handle each of our internal types as a special case.  For errWithCause, we
	// traverse to the wrapped error, ignoring the cause and the funky Unwrap logic.
	// We could have just used errors.As(err, *errWithValue), but that would have
	// traversed into the causes.

	for {
		switch t := err.(type) {
		case *errWithValue:
			if t.key == key {
				return t.value, true
			}
			err = t.err
		case *errWithCause:
			err = t.err
		default:
			if errors.As(err, &merr) {
				err = merr
			} else {
				return nil, false
			}
		}
	}
}

// Values returns a map of all values attached to the error
// If a key has been attached multiple times, the map will
// contain the last value mapped
// If e is nil, returns nil.
func Values(err error) map[interface{}]interface{} {
	var values map[interface{}]interface{}

	for err != nil {
		if e, ok := err.(*errWithValue); ok {
			if _, ok := values[e.key]; !ok {
				if values == nil {
					values = map[interface{}]interface{}{}
				}
				values[e.key] = e.value
			}
		}
		err = errors.Unwrap(err)
	}

	return values
}

// Stack returns the stack attached to an error, or nil if one is not attached
// If e is nil, returns nil.
func Stack(err error) []uintptr {
	stack, _ := Value(err, errKeyStack).([]uintptr)
	return stack
}

// HTTPCode converts an error to an http status code.  All errors
// map to 500, unless the error has an http code attached.
// If e is nil, returns 200.
func HTTPCode(err error) int {
	if err == nil {
		return 200
	}

	code, _ := Value(err, errKeyHTTPCode).(int)
	if code == 0 {
		return 500
	}

	return code
}

// UserMessage returns the end-user safe message.  Returns empty if not set.
// If e is nil, returns "".
func UserMessage(err error) string {
	msg, _ := Value(err, errKeyUserMessage).(string)
	return msg
}

// Cause returns the cause of the argument.  If e is nil, or has no cause,
// nil is returned.
func Cause(err error) error {
	var causer *errWithCause
	if errors.As(err, &causer) {
		return causer.cause
	}
	return nil
}

// RegisteredDetails extracts details registered with RegisterDetailFunc from an error, and
// returns them as a map.  Values may be nil.
//
// If err is nil or there are no registered details, nil is returned.
func RegisteredDetails(err error) map[string]interface{} {
	detailsLock.Lock()
	defer detailsLock.Unlock()

	if len(detailFields) == 0 || err == nil {
		return nil
	}

	dets := map[string]interface{}{}

	for label, f := range detailFields {
		dets[label] = f(err)
	}

	return dets
}

// captureStack: return an error with a stack attached.  Stack will skip
// specified frames.  skip = 0 will start at caller.
// If the err already has a stack, to auto-stack-capture is disabled globally,
// this is a no-op.  Use force to override and force a stack capture
// in all cases.
func captureStack(err error, skip int, force bool) error {
	if err == nil {
		return nil
	}

	var c interface {
		Callers() []uintptr
	}

	switch {
	case force:
		// always capture
	case HasStack(err):
		return err
	case errors.As(err, &c):
		// if the go-errors already captured a stack
		// reuse it
		if stack := c.Callers(); len(stack) > 0 {
			return Set(err, errKeyStack, stack)
		}
	case !captureStacks:
		return err
	}

	s := make([]uintptr, MaxStackDepth())
	length := runtime.Callers(2+skip, s[:])
	return Set(err, errKeyStack, s[:length])
}

// HasStack returns true if a stack is already attached to the err.
// If err == nil, returns false.
//
// If a stack capture was suppressed with NoCaptureStack(), this will
// still return true, indicating that stack capture processing has already
// occurred on this error.
func HasStack(err error) bool {
	_, ok := Lookup(err, errKeyStack)
	return ok
}
