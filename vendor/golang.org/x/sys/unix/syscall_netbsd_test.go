// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unix_test

import (
	"testing"

	"golang.org/x/sys/unix"
)

// stringsFromByteSlice converts a sequence of attributes to a []string.
// On NetBSD, each entry consists of a single byte containing the length
// of the attribute name, followed by the attribute name.
// The name is _not_ NULL-terminated.
func stringsFromByteSlice(buf []byte) []string {
	var result []string
	i := 0
	for i < len(buf) {
		next := i + 1 + int(buf[i])
		result = append(result, string(buf[i+1:next]))
		i = next
	}
	return result
}

func TestSysctlClockinfo(t *testing.T) {
	ci, err := unix.SysctlClockinfo("kern.clockrate")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tick = %v, tickadj = %v, hz = %v, profhz = %v, stathz = %v",
		ci.Tick, ci.Tickadj, ci.Hz, ci.Profhz, ci.Stathz)
}
