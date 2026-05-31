/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package display

import (
	"fmt"
	"math"
)

const (
	// KiB kibibyte
	KiB uint64 = 1024
	// MiB mebibyte
	MiB uint64 = KiB * 1024
	// GiB gibibyte
	GiB uint64 = MiB * 1024
	// TiB tebibyte
	TiB uint64 = GiB * 1024
	// PiB pebibyte
	PiB uint64 = TiB * 1024
	// EiB exbibyte
	EiB uint64 = PiB * 1024
)

// BytesToString converts bytes to strings
func BytesToString(b uint64) string {
	if b < KiB {
		return fmt.Sprintf("%d B", b)
	} else if b < MiB {
		return formatStorageString(b, KiB, "KiB")
	} else if b < GiB {
		return formatStorageString(b, MiB, "MiB")
	} else if b < TiB {
		return formatStorageString(b, GiB, "GiB")
	} else if b < PiB {
		return formatStorageString(b, TiB, "TiB")
	} else if b < EiB {
		return formatStorageString(b, PiB, "PiB")
	}
	return formatStorageString(b, EiB, "EiB")
}

func formatStorageString(b, u uint64, unitLabel string) string {
	return fmt.Sprintf("%.2f %s", float64(b)/float64(u), unitLabel)
}

// BToMb converts bytes to megabytes
func BToMb(b uint64) uint64 {
	mb := float64(b) / 1024.0 / 1024.0
	return uint64(math.Round(mb))
}

// MbTob converts megabytes to bytes
func MbTob(b uint64) uint64 {
	return b * 1024 * 1024
}
