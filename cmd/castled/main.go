package main

// #cgo CFLAGS: -I${SRCDIR}/../../ceph/src/include
// #cgo LDFLAGS: -L${SRCDIR}/../../ceph/build/lib -lcephd -lstdc++
// #include "cephd/libcephd.h"
import "C"

import (
	"fmt"

	"github.com/quantum/castle/version"
	"github.com/spf13/cobra"
)

func cephVersion() string {
	var cMajor, cMinor, cPatch C.int
	return C.GoString(C.ceph_version(&cMajor, &cMinor, &cPatch))
}

func main() {

	var cmdVersion = &cobra.Command{
		Use:   "version",
		Short: "print castle version",
		Long:  `prints the version of castle.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("castled version %v (ceph version %v)\n", version.Version, cephVersion())
		},
	}

	var rootCmd = &cobra.Command{Use: "castled"}
	rootCmd.AddCommand(cmdVersion)
	rootCmd.Execute()
}
