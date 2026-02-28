package merry

import (
	v2 "github.com/ansel1/merry/v2"
)

// RegisterDetail registers an error property key in a global registry, with a label.
// The registry is used by the Details() function.  Registered error properties will
// be included in Details() output, if the value of that error property is not nil.
// For example:
//
//	err := New("boom")
//	err = err.WithValue(colorKey, "red")
//	fmt.Println(Details(err))
//
//	// Output:
//	// boom
//	//
//	// <stacktrace>
//
//	RegisterDetail("Color", colorKey)
//	fmt.Println(Details(err))
//
//	// Output:
//	// boom
//	// Color: red
//	//
//	// <stacktrace>
//
// Error property keys are typically not exported by the packages which define them.
// Packages instead export functions which let callers access that property.
// It's therefore up to the package
// to register those properties which would make sense to include in the Details() output.
// In other words, it's up to the author of the package which generates the errors
// to publish printable error details, not the callers of the package.
func RegisterDetail(label string, key interface{}) {
	v2.RegisterDetail(label, key)
}

// Location returns zero values if e has no stacktrace
func Location(err error) (file string, line int) {
	return v2.Location(err)
}

// SourceLine returns the string representation of
// Location's result or an empty string if there's
// no stracktrace.
func SourceLine(err error) string {
	return v2.SourceLine(err)
}

// Stacktrace returns the error's stacktrace as a string formatted
// the same way as golangs runtime package.
// If e has no stacktrace, returns an empty string.
func Stacktrace(err error) string {
	return v2.Stacktrace(err)
}

// Details returns e.Error(), e's stacktrace, and any additional details which have
// be registered with RegisterDetail.  User message and HTTP code are already registered.
//
// The details of each error in e's cause chain will also be printed.
func Details(err error) string {
	return v2.Details(err)
}
