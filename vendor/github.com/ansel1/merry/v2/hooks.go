package merry

var hooks []Wrapper
var onceHooks []Wrapper

// AddHooks installs a global set of Wrappers which are applied to every error processed
// by this package.  They are applied before any other Wrappers or stack capturing are
// applied.  Hooks can add additional wrappers to errors, or translate annotations added
// by other error libraries into merry annotations.
//
// Note that these hooks will be applied each time an err is passed to Wrap/Apply.  If you
// only want your hook to run once per error, see AddOnceHooks.
//
// This function is not thread safe, and should only be called very early in program
// initialization.
func AddHooks(hook ...Wrapper) {
	hooks = append(hooks, hook...)
}

// AddOnceHooks is like AddHooks, but these hooks will only be applied once per error.
// Once hooks are applied to an error, the error is marked, and future Wrap/Apply calls
// on the error will not apply these hooks again.
//
// This function is not thread safe, and should only be called very early in program
// initialization.
func AddOnceHooks(hook ...Wrapper) {
	onceHooks = append(onceHooks, hook...)
}

// ClearHooks removes all installed hooks.
//
// This function is not thread safe, and should only be called very early in program
// initialization.
func ClearHooks() {
	hooks = nil
	onceHooks = nil
}
