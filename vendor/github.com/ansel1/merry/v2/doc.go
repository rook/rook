// Package merry adds context to errors, including automatic stack capture, cause chains, HTTP status code, user
// messages, and arbitrary values.
//
// Wrapped errors work a lot like google's golang.org/x/net/context package:
// each wrapper error contains the inner error, a key, and a value.
// Like contexts, errors are immutable: adding a key/value to an error
// always creates a new error which wraps the original.
//
// This package comes with built-in support for adding information to errors:
//
// * stacktraces
// * changing the error message
// * HTTP status codes
// * End user error messages
// * causes
//
// You can also add your own additional information.
//
// The stack capturing feature can be turned off for better performance, though it's pretty fast.  Benchmarks
// on an 2017 MacBook Pro, with go 1.10:
//
//	BenchmarkNew_withStackCapture-8      	 2000000	       749 ns/op
//	BenchmarkNew_withoutStackCapture-8   	20000000	        64.1 ns/op
//
// # Usage
//
// This package contains functions for creating errors, or wrapping existing errors.  To create:
//
//	err := New("boom!")
//	err := Errorf("error fetching %s", filename)
//
// Additional context information can be attached to errors using functional options, called Wrappers:
//
//	err := New("record not found", WithHTTPCode(404))
//
// Errorf() also accepts wrappers, mixed in with the format args:
//
//	err := Errorf("user %s not found", username, WithHTTPCode(404))
//
// Wrappers can be applied to existing errors with Wrap():
//
//	err = Wrap(err, WithHTTPCode(404))
//
// Wrap() will add a stacktrace to any error which doesn't already have one attached.  WrapSkipping()
// can be used to control where the stacktrace starts.
//
// This package contains wrappers for adding specific context information to errors, such as an
// HTTPCode.  You can create your own wrappers using the primitive Value(), WithValue(), and Set()
// functions.
//
// Errors produced by this package implement fmt.Formatter, to print additional information about the
// error:
//
//	fmt.Printf("%v", err)         // print error message and causes
//	fmt.Printf("%s", err)         // same as %s
//	fmt.Printf("%q", err)         // same as fmt.Printf("%q", err.Error())
//	fmt.Printf("%v+", err)        // print Details(err)
//
// Details() prints the error message, all causes, the stacktrace, and additional error
// values configured with RegisterDetailFunc().  By default, it will show the HTTP status
// code and user message.
//
// # Stacktraces
//
// By default, any error created by or wrapped by this package will automatically have
// a stacktrace captured and attached to the error.  This capture only happens if the
// error doesn't already have a stack attached to it, so wrapping the error with additional
// context won't capture additional stacks.
//
// When and how stacks are captured can be customized.  SetMaxStackDepth() can globally configure
// how many frames to capture.  SetStackCaptureEnabled() can globally configure whether
// stacks are captured by default.
//
// Wrap(err, NoStackCapture()) can be used to selectively suppress stack capture for a particular
// error.
//
// Wrap(err, CaptureStack(false)) will capture a new stack at the Wrap call site, even if the err
// already had an earlier stack attached.  The new stack overrides the older stack.
//
// Wrap(err, CaptureStack(true)) will force a stack capture at the call site even if stack
// capture is disabled globally.
//
// Finally, Wrappers are passed a depth argument so they know how deep they are in the call stack
// from the call site where this package's API was called.  This allows Wrappers to implement their
// own stack capturing logic.
//
// The package contains functions for creating new errors with stacks, or adding a stack to `error`
// instances.  Functions with add context (e.g. `WithValue()`) work on any `error`, and will
// automatically convert them to merry errors (with a stack) if necessary.
//
// # Hooks
//
// AddHooks() can install wrappers which are applied to all errors processed by this package.  Hooks
// are applied before any other wrappers or processing takes place.  They can be used to integrate
// with errors from other packages, normalizing errors (such as applying standard status codes to
// application errors), localizing user messages, or replacing the stack capturing mechanism.
package merry
