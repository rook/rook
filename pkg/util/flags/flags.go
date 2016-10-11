package flags

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func SplitList(list string) []string {
	if list == "" {
		return nil
	}

	return strings.Split(list, ",")
}

func VerifyRequiredFlags(cmd *cobra.Command, requiredFlags []string) error {
	for _, reqFlag := range requiredFlags {
		val, err := cmd.Flags().GetString(reqFlag)
		if err != nil || val == "" {
			return createRequiredFlagError(cmd, reqFlag)
		}
	}

	return nil
}

func VerifyRequiredUint64Flags(cmd *cobra.Command, requiredFlags []string) error {
	for _, reqFlag := range requiredFlags {
		val, err := cmd.Flags().GetUint64(reqFlag)
		if err != nil || val == 0 {
			return createRequiredFlagError(cmd, reqFlag)
		}
	}

	return nil
}

func createRequiredFlagError(cmd *cobra.Command, flagName string) error {
	return fmt.Errorf("%s is required for %s", flagName, cmd.Name())
}
