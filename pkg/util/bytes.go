package util

import (
	"fmt"
)

const (
	KiB uint64 = 1024
	MiB uint64 = KiB * 1024
	GiB uint64 = MiB * 1024
	TiB uint64 = GiB * 1024
	PiB uint64 = TiB * 1024
	EiB uint64 = PiB * 1024
)

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
