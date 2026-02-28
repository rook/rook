package errors

import (
	"go.uber.org/multierr"
)

// Combine combines the passed errors into a single error.
//
// If zero arguments were passed or if all items are nil, a nil error is
// returned.
//
// If only a single error was passed, it is returned as-is.
//
// Combine omits nil errors so this function may be used to combine
// together errors from operations that fail independently of each other.
//
// 	errors.Combine(
// 		reader.Close(),
// 		writer.Close(),
// 		pipe.Close(),
// 	)
//
// If any of the passed errors is already an aggregated error, it will be flattened along
// with the other errors.
//
//		errors.Combine(errors.Combine(err1, err2), err3)
//		// is the same as
//		errors.Combine(err1, err2, err3)
//
// The returned error formats into a readable multi-line error message if
// formatted with %+v.
//
//		fmt.Sprintf("%+v", errors.Combine(err1, err2))
func Combine(errors ...error) error {
	return multierr.Combine(errors...)
}

// Append appends the given errors together. Either value may be nil.
//
// This function is a specialization of Combine for the common case where
// there are only two errors.
//
//		err = errors.Append(reader.Close(), writer.Close())
//
// The following pattern may also be used to record failure of deferred
// operations without losing information about the original error.
//
//		func doSomething(..) (err error) {
//			f := acquireResource()
//			defer func() {
//				err = errors.Append(err, f.Close())
//			}()
func Append(left error, right error) error {
	return multierr.Append(left, right)
}

// GetErrors returns a slice containing zero or more errors that the supplied
// error is composed of. If the error is nil, the returned slice is empty.
//
//		err := errors.Append(r.Close(), w.Close())
//		errors := errors.GetErrors(err)
//
// If the error is not composed of other errors, the returned slice contains
// just the error that was passed in.
//
// Callers of this function are free to modify the returned slice.
func GetErrors(err error) []error {
	return multierr.Errors(err)
}
