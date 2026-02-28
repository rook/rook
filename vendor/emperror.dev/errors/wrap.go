package errors

// Unwrap returns the result of calling the Unwrap method on err, if err implements
// Unwrap. Otherwise, Unwrap returns nil.
//
// It supports both Go 1.13 Unwrap and github.com/pkg/errors.Causer interfaces
// (the former takes precedence).
func Unwrap(err error) error {
	switch e := err.(type) {
	case interface{ Unwrap() error }:
		return e.Unwrap()

	case interface{ Cause() error }:
		return e.Cause()
	}

	return nil
}

// UnwrapEach loops through an error chain and calls a function for each of them.
//
// The provided function can return false to break the loop before it reaches the end of the chain.
//
// It supports both Go 1.13 errors.Wrapper and github.com/pkg/errors.Causer interfaces
// (the former takes precedence).
func UnwrapEach(err error, fn func(err error) bool) {
	for err != nil {
		continueLoop := fn(err)
		if !continueLoop {
			break
		}

		err = Unwrap(err)
	}
}

// Cause returns the last error (root cause) in an err's chain.
// If err has no chain, it is returned directly.
//
// It supports both Go 1.13 errors.Wrapper and github.com/pkg/errors.Causer interfaces
// (the former takes precedence).
func Cause(err error) error {
	for {
		cause := Unwrap(err)
		if cause == nil {
			break
		}

		err = cause
	}

	return err
}
