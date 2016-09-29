// +build linux,amd64 linux,arm64

package main

import (
	"fmt"
	"os"

	"github.com/quantum/castle/pkg/cephmgr/cephd"
	"github.com/quantum/castle/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	daemonType string
)

var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Runs a castled daemon",
	Hidden: true,
}

func init() {
	daemonCmd.Flags().StringVar(&daemonType, "type", "", "type of daemon [mon|osd]")
	daemonCmd.MarkFlagRequired("type")

	daemonCmd.RunE = runDaemon
}

func runDaemon(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(daemonCmd, []string{"type"}); err != nil {
		return err
	}
	if daemonType != "mon" && daemonType != "osd" {
		return fmt.Errorf("unknown daemon type: %s", daemonType)
	}

	// daemon command passes through args to the child daemon process.  Look for the
	// terminator arg, and pass through all args after that (without a terminator arg,
	// FlagSet.Parse prints errors for args it doesn't recognize)
	passthruIndex := 3
	for i := range os.Args {
		if os.Args[i] == "--" {
			passthruIndex = i + 1
			break
		}
	}

	// run the specified daemon
	return cephd.New().RunDaemon(daemonType, os.Args[passthruIndex:]...)
}
