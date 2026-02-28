// Package merry provides enriched golang errors, with stacktraces
//
// merry creates errors with stacktraces, and can augment those errors with additional
// information.
//
// When you create a new merry error, or wrap an existing error in a merry error, merry attaches
// a stacktrace to the error:
//
//	err := merry.New("an error occurred")
//
// err has a stacktrace attached.  Alternately, you can wrap existing errors.  merry will
// attach a stacktrace at the point of wrapping:
//
//	_, err := ioutil.ReadAll(r)
//	if err != nil {
//	    return merry.Wrap(err)
//	}
//
// Capturing the stack can be globally disabled with `SetStackCaptureEnabled(false)`.  Wrapping
// is idempotent: Wrap will only attach a stacktrace if the error doesn't already have one.
//
// Wrap() is the simplest way to attach a stacktrace to an error, but other functions can be
// used instead, with both add a stacktrace, and augment or modify the error.  For example,
// Prepend() modifies the error's message (and also attaches a stacktrace):
//
//	_, err := ioutil.ReadAll(r)
//	if err != nil {
//	    return merry.Prepend(err, "reading from conn failed")
//	    // err.Error() would read something like "reading from conn failed: timeout"
//	}
//
// See the other package functions for other ways to augment or modify errors, such as Append,
// WithUserMessage, WithHTTPCode, WithValue, etc.  These functions all return a merry.Error interface, which
// has methods which mirror the package level functions, to allow simple chaining:
//
//	return merry.New("object not found").WithHTTPCode(404)
//
// # Here
//
// Wrap will not take a new stacktrace if an error already has one attached.  Here will create
// a new error which replaces the stacktrace with a new one:
//
//	var ErrOverflow = merry.New("overflowed")
//
//	func Read() error {
//	    // ...
//	    return merry.Here(ErrOverflow)
//	}
//
// # Is
//
// The go idiom of exporting package-level error variables for comparison to errors returned
// by the package is broken by merry.  For example:
//
//	_, err := io.ReadAll(r)
//	if err == io.EOF {
//	    // ...
//	}
//
// If the error returned was a merry error, the equality comparison would always fail, because merry
// augments errors by wrapping them in layers.  To compensate for this, merry has the Is() function.
//
//	if merry.Is(err, io.EOF) {
//
// Is() will unwrap the err and compare each layer to the second argument.
//
// # Cause
//
// You can add a cause to an error:
//
//	if err == io.EOF {
//	    err = merry.New("reading failed"), err)
//	    fmt.Println(err.Error()) // reading failed: EOF
//	}
//
// Cause(error) will return the cause of the argument.  RootCause(error) returns the innermost cause.
// Is(err1, err2) is cause aware, and will return true if err2 is a cause (anywhere in the causal change)
// of err1.
//
// # Formatting and printing
//
// To obtain an error's stacktrace, call Stack().  To get other information about the site
// of the error, or print the error's stacktrace, see Location(), SourceLine(), Stacktrace(), and Details().
//
// merry errors also implement the fmt.Formatter interface.  errors support the following fmt flags:
//
//	%+v   print the equivalent of Details(err), which includes the user message, full stacktrace,
//	      and recursively prints the details of the cause chain.
package merry
