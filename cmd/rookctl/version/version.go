package version

import (
	"fmt"

	"github.com/rook/rook/pkg/version"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of rookd",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("rook: %v\n", version.Version)
		return nil
	},
}
