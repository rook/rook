package merry

import (
	"sync"
)

var maxStackDepth = 50
var captureStacks = true

// StackCaptureEnabled returns whether stack capturing is enabled.
func StackCaptureEnabled() bool {
	return captureStacks
}

// SetStackCaptureEnabled sets stack capturing globally.  Disabling stack capture can increase performance.
// Capture can be forced or suppressed to override this global setting on a particular error.
func SetStackCaptureEnabled(enabled bool) {
	captureStacks = enabled
}

// MaxStackDepth returns the number of frames captured in stacks.
func MaxStackDepth() int {
	return maxStackDepth
}

// SetMaxStackDepth sets the MaxStackDepth.
func SetMaxStackDepth(depth int) {
	maxStackDepth = depth
}

func init() {
	RegisterDetail("User Message", errKeyUserMessage)
	RegisterDetail("HTTP Code", errKeyHTTPCode)
}

var detailsLock sync.Mutex
var detailFields = map[string]func(err error) interface{}{}

// RegisterDetail registers an error property key in a global registry, with a label.
// See RegisterDetailFunc.  This function just wraps a call to Value(key) and passes
// it to RegisterDetailFunc.
func RegisterDetail(label string, key interface{}) {
	RegisterDetailFunc(label, func(err error) interface{} {
		return Value(err, key)
	})
}

// RegisterDetailFunc registers a label and a function for extracting a value from
// an error.  When formatting errors produced by this package using the
// `%+v` placeholder, or when using Details(), these functions will be called
// on the error, and any non-nil values will be added to the text.
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
//	func Color(err) string {
//	  s, _ := Value(err, colorKey)
//	  return s
//	}
//
//	RegisterDetailFunc("color", Color)
//	fmt.Println(Details(err))
//
//	// Output:
//	// boom
//	// color: red
//	//
//	// <stacktrace>
//
// Error property keys are typically not exported by the packages which define them.
// Packages instead export functions which let callers access that property.
// It's therefore up to the package
// to register those properties which would make sense to include in the Details() output.
// In other words, it's up to the author of the package which generates the errors
// to publish printable error details, not the callers of the package.
func RegisterDetailFunc(label string, f func(err error) interface{}) {
	detailsLock.Lock()
	defer detailsLock.Unlock()

	detailFields[label] = f
}
