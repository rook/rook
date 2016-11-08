// +build linux,amd64 linux,arm64

package main

import (
	"fmt"

	etcdversion "github.com/coreos/etcd/version"
	"github.com/rook/rook/pkg/cephmgr/cephd"
	"github.com/rook/rook/pkg/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of rookd",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("rookd: %s\n", version.Version)
		fmt.Printf("cephd: %v\n", cephd.Version())
		fmt.Printf(" etcd: %s\n", etcdversion.Version)
		return nil
	},
}
