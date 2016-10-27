package rook

import "github.com/spf13/cobra"

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Performs commands and operations on nodes in the cluster",
}

func init() {
	nodeCmd.AddCommand(nodeListCmd)
}
