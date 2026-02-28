package errors

import (
	"fmt"
)

// WithDetails annotates err with with arbitrary key-value pairs.
func WithDetails(err error, details ...interface{}) error {
	if err == nil {
		return nil
	}

	if len(details) == 0 {
		return err
	}

	if len(details)%2 != 0 {
		details = append(details, nil)
	}

	var w *withDetails
	if !As(err, &w) {
		w = &withDetails{
			error: err,
		}

		err = w
	}

	// Limiting the capacity of the stored keyvals ensures that a new
	// backing array is created if the slice must grow in With.
	// Using the extra capacity without copying risks a data race.
	d := append(w.details, details...)
	w.details = d[:len(d):len(d)]

	return err
}

// GetDetails extracts the key-value pairs from err's chain.
func GetDetails(err error) []interface{} {
	var details []interface{}

	// Usually there is only one error with details (when using the WithDetails API),
	// but errors themselves can also implement the details interface exposing their attributes.
	UnwrapEach(err, func(err error) bool {
		if derr, ok := err.(interface{ Details() []interface{} }); ok {
			details = append(derr.Details(), details...)
		}

		return true
	})

	return details
}

// withDetails annotates an error with arbitrary key-value pairs.
type withDetails struct {
	error   error
	details []interface{}
}

func (w *withDetails) Error() string { return w.error.Error() }
func (w *withDetails) Cause() error  { return w.error }
func (w *withDetails) Unwrap() error { return w.error }

// Details returns the appended details.
func (w *withDetails) Details() []interface{} {
	return w.details
}

func (w *withDetails) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = fmt.Fprintf(s, "%+v", w.error)

			return
		}

		_, _ = fmt.Fprintf(s, "%v", w.error)

	case 's':
		_, _ = fmt.Fprintf(s, "%s", w.error)

	case 'q':
		_, _ = fmt.Fprintf(s, "%q", w.error)
	}
}
