package cmd

import "github.com/spf13/cobra"

var poolCmd = &cobra.Command{
	Use:   "pool",
	Short: "Performs commands and operations on storage pools in the cluster",
}

func init() {
	poolCmd.AddCommand(poolListCmd)
	poolCmd.AddCommand(poolCreateCmd)
}
