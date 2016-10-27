package rook

import "github.com/spf13/cobra"

var blockCmd = &cobra.Command{
	Use:   "block",
	Short: "Performs commands and operations on block devices and images in the cluster",
}

func init() {
	blockCmd.AddCommand(blockListCmd)
	blockCmd.AddCommand(blockCreateCmd)
	blockCmd.AddCommand(blockMountCmd)
	blockCmd.AddCommand(blockUnmountCmd)
}
