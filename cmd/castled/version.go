// +build linux,amd64 linux,arm64

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/quantum/castle/pkg/cephmgr/cephd"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of castled",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("cephd: %v\n", cephd.Version())
		rmajor, rminor, rpatch := cephd.RadosVersion()
		fmt.Printf("rados: %v.%v.%v\n", rmajor, rminor, rpatch)
		return nil
	},
}
