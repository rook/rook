package merry

import (
	"fmt"
	"io"
	"path"
	"runtime"
	"sort"
	"strings"
)

// Location returns zero values if e has no stacktrace
func Location(err error) (file string, line int) {
	s := Stack(err)
	if len(s) > 0 {
		fnc, _ := runtime.CallersFrames(s[:1]).Next()
		return fnc.File, fnc.Line
	}
	return "", 0
}

// SourceLine returns the string representation of
// Location's result or an empty string if there's
// no stracktrace.
func SourceLine(err error) string {
	s := Stack(err)
	if len(s) > 0 {
		fnc, _ := runtime.CallersFrames(s[:1]).Next()
		_, f := path.Split(fnc.File)
		return fmt.Sprintf("%s (%s:%d)", fnc.Function, f, fnc.Line)
	}
	return ""
}

// FormattedStack returns the stack attached to an error, formatted as a slice of strings.
// Each string represents a frame in the stack, newest first.  The strings may
// have internal newlines.
//
// Returns nil if no formatted stack and no stack is associated, or err is nil.
func FormattedStack(err error) []string {
	formattedStack, _ := Value(err, errKeyStack).([]string)
	if len(formattedStack) > 0 {
		return formattedStack
	}

	s := Stack(err)
	if len(s) > 0 {
		lines := make([]string, 0, len(s))

		frames := runtime.CallersFrames(s)
		for {
			frame, more := frames.Next()
			lines = append(lines, fmt.Sprintf("%s\n\t%s:%d", frame.Function, frame.File, frame.Line))
			if !more {
				break
			}

		}
		return lines
	}
	return nil
}

// Stacktrace returns the error's stacktrace as a string formatted.
// If e has no stacktrace, returns an empty string.
func Stacktrace(err error) string {
	return strings.Join(FormattedStack(err), "\n")
}

// Details returns e.Error(), e's stacktrace, and any additional details which have
// be registered with RegisterDetail.  User message and HTTP code are already registered.
//
// The details of each error in e's cause chain will also be printed.
func Details(e error) string {
	if e == nil {
		return ""
	}

	msg := e.Error()
	var dets []string

	detailsLock.Lock()

	for label, f := range detailFields {
		v := f(e)
		if v != nil {
			dets = append(dets, fmt.Sprintf("%s: %v", label, v))
		}
	}

	detailsLock.Unlock()

	if len(dets) > 0 {
		// sort so output is predictable
		sort.Strings(dets)
		msg += "\n" + strings.Join(dets, "\n")
	}

	s := Stacktrace(e)
	if s != "" {
		msg += "\n\n" + s
	}

	if c := Cause(e); c != nil {
		msg += "\n\nCaused By: " + Details(c)
	}

	return msg
}

// Format adapts errors to fmt.Formatter interface.  It's intended to be used
// help error impls implement fmt.Formatter, e.g.:
//
//	    func (e *myErr) Format(f fmt.State, verb rune) {
//		     Format(f, verb, e)
//	    }
func Format(s fmt.State, verb rune, err error) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			io.WriteString(s, Details(err))
			return
		}
		fallthrough
	case 's':
		io.WriteString(s, msgWithCauses(err))
	case 'q':
		fmt.Fprintf(s, "%q", err.Error())
	}
}

func msgWithCauses(err error) string {
	messages := make([]string, 0, 5)

	for err != nil {
		if ce := err.Error(); ce != "" {
			messages = append(messages, ce)
		}
		err = Cause(err)
	}

	return strings.Join(messages, ": ")
}
