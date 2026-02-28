// Copyright 2015 Dave Cheney <dave@cheney.net>. All rights reserved.
// Use of this source code (or at least parts of it) is governed by a BSD-style
// license that can be found in the LICENSE_THIRD_PARTY file.

package errors

import (
	"fmt"
	"runtime"

	"github.com/pkg/errors"
)

// StackTrace is stack of Frames from innermost (newest) to outermost (oldest).
//
// It is an alias of the same type in github.com/pkg/errors.
type StackTrace = errors.StackTrace

// Frame represents a program counter inside a stack frame.
// For historical reasons if Frame is interpreted as a uintptr
// its value represents the program counter + 1.
//
// It is an alias of the same type in github.com/pkg/errors.
type Frame = errors.Frame

// stack represents a stack of program counters.
//
// It is a duplicate of the same (sadly unexported) type in github.com/pkg/errors.
type stack []uintptr

// nolint: gocritic
func (s *stack) Format(st fmt.State, verb rune) {
	switch verb {
	case 'v':
		switch {
		case st.Flag('+'):
			for _, pc := range *s {
				f := Frame(pc)
				fmt.Fprintf(st, "\n%+v", f)
			}
		}
	}
}

func (s *stack) StackTrace() StackTrace {
	f := make([]Frame, len(*s))
	for i := 0; i < len(f); i++ {
		f[i] = Frame((*s)[i])
	}

	return f
}

// callers is based on the function with the same name in github.com/pkg/errors,
// but accepts a custom depth (useful to customize the error constructor caller depth).
func callers(depth int) *stack {
	const maxDepth = 32

	var pcs [maxDepth]uintptr

	n := runtime.Callers(2+depth, pcs[:])
	st := make(stack, n)
	copy(st, pcs[:n])

	return &st
}
