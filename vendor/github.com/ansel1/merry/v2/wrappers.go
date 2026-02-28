package merry

import "fmt"

// Wrapper knows how to wrap errors with context information.
type Wrapper interface {
	// Wrap returns a new error, wrapping the argument, and typically adding some context information.
	// skipCallers is how many callers to skip when capturing a stack to skip to the caller of the merry
	// API surface.  It's intended to make it possible to write wrappers which capture stacktraces.  e.g.
	//
	//     func CaptureStack() Wrapper {
	//         return WrapperFunc(func(err error, skipCallers int) error {
	//             s := make([]uintptr, 50)
	//             // Callers
	//             l := runtime.Callers(2+skipCallers, s[:])
	//             return WithStack(s[:l]).Wrap(err, skipCallers + 1)
	//         })
	//    }
	Wrap(err error, skipCallers int) error
}

// WrapperFunc implements Wrapper.
type WrapperFunc func(error, int) error

// Wrap implements the Wrapper interface.
func (w WrapperFunc) Wrap(err error, callerDepth int) error {
	return w(err, callerDepth+1)
}

// WithValue associates a key/value pair with an error.
func WithValue(key, value interface{}) Wrapper {
	return WrapperFunc(func(err error, _ int) error {
		return Set(err, key, value)
	})
}

// WithMessage overrides the value returned by err.Error().
func WithMessage(msg string) Wrapper {
	return WithValue(errKeyMessage, msg)
}

// WithMessagef overrides the value returned by err.Error().
func WithMessagef(format string, args ...interface{}) Wrapper {
	return WrapperFunc(func(err error, _ int) error {
		if err == nil {
			return nil
		}
		return Set(err, errKeyMessage, fmt.Sprintf(format, args...))
	})
}

// WithUserMessage associates an end-user message with an error.
func WithUserMessage(msg string) Wrapper {
	return WithValue(errKeyUserMessage, msg)
}

// WithUserMessagef associates a formatted end-user message with an error.
func WithUserMessagef(format string, args ...interface{}) Wrapper {
	return WrapperFunc(func(err error, _ int) error {
		if err == nil {
			return nil
		}
		return Set(err, errKeyUserMessage, fmt.Sprintf(format, args...))
	})
}

// AppendMessage a message after the current error message, in the format "original: new".
func AppendMessage(msg string) Wrapper {
	return WrapperFunc(func(err error, _ int) error {
		if err == nil {
			return nil
		}
		return Set(err, errKeyMessage, err.Error()+": "+msg)
	})
}

// AppendMessagef is the same as AppendMessage, but with a formatted message.
func AppendMessagef(format string, args ...interface{}) Wrapper {
	return WrapperFunc(func(err error, _ int) error {
		if err == nil {
			return nil
		}
		return Set(err, errKeyMessage, err.Error()+": "+fmt.Sprintf(format, args...))
	})
}

// PrependMessage a message before the current error message, in the format "new: original".
func PrependMessage(msg string) Wrapper {
	return WrapperFunc(func(err error, _ int) error {
		if err == nil {
			return nil
		}
		return Set(err, errKeyMessage, msg+": "+err.Error())
	})
}

// PrependMessagef is the same as PrependMessage, but with a formatted message.
func PrependMessagef(format string, args ...interface{}) Wrapper {
	return WrapperFunc(func(err error, _ int) error {
		if err == nil {
			return nil
		}
		return Set(err, errKeyMessage, fmt.Sprintf(format, args...)+": "+err.Error())
	})
}

// WithHTTPCode associates an HTTP status code with an error.
func WithHTTPCode(statusCode int) Wrapper {
	return WithValue(errKeyHTTPCode, statusCode)
}

// WithStack associates a stack of caller frames with an error.  Generally, this package
// will automatically capture and associate a stack with errors which are created or
// wrapped by this package.  But this allows the caller to associate an externally
// generated stack.
func WithStack(stack []uintptr) Wrapper {
	return WithValue(errKeyStack, stack)
}

// WithFormattedStack associates a stack of pre-formatted strings describing frames of a
// stacktrace.  Generally, a formatted stack is generated from the raw []uintptr stack
// associated with the error, but a pre-formatted stack can be associated with the error
// instead, and takes precedence over the raw stack.  This is useful if pre-formatted
// stack information is coming from some other source.
func WithFormattedStack(stack []string) Wrapper {
	return WithValue(errKeyStack, stack)
}

// NoCaptureStack will suppress capturing a stack, even if StackCaptureEnabled() == true.
func NoCaptureStack() Wrapper {
	return WrapperFunc(func(err error, _ int) error {
		// if this err already has a stack set, there is no need to set the
		// stack property again, and we don't want to override the prior the stack
		if HasStack(err) {
			return err
		}
		return Set(err, errKeyStack, nil)
	})
}

// CaptureStack will override an earlier stack with a stack captured from the current
// call site.  If StackCaptureEnabled() == false, this is a no-op.
//
// If force is set, StackCaptureEnabled() will be ignored: a stack will always be captured.
func CaptureStack(force bool) Wrapper {
	return WrapperFunc(func(err error, callerDepth int) error {
		return captureStack(err, callerDepth+1, force || StackCaptureEnabled())
	})
}

// WithCause sets one error as the cause of another error.  This is useful for associating errors
// from lower API levels with sentinel errors in higher API levels.  errors.Is() and errors.As()
// will traverse both the main chain of error wrappers, and down the chain of causes.
//
// If err is nil, this is a no-op
func WithCause(err error) Wrapper {
	return WrapperFunc(func(nerr error, _ int) error {
		if nerr == nil || err == nil {
			return nerr
		}
		return &errWithCause{err: nerr, cause: err}
	})
}

// Set wraps an error with a key/value pair.  This is the simplest form of associating
// a value with an error.  It does not capture a stacktrace, invoke hooks, or do any
// other processing.  It is mainly intended as a primitive for writing Wrapper implementations.
//
// if err is nil, returns nil.
func Set(err error, key, value interface{}) error {
	if err == nil {
		return nil
	}
	return &errWithValue{
		err:   err,
		key:   key,
		value: value,
	}
}
