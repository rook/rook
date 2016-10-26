package flags

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func VerifyRequiredFlags(cmd *cobra.Command, requiredFlags []string) error {
	var missingFlags []string
	for _, reqFlag := range requiredFlags {
		val, err := cmd.Flags().GetString(reqFlag)
		if err != nil || val == "" {
			missingFlags = append(missingFlags, reqFlag)
		}
	}

	return createRequiredFlagError(cmd.Name(), missingFlags)
}

func VerifyRequiredUint64Flags(cmd *cobra.Command, requiredFlags []string) error {
	var missingFlags []string
	for _, reqFlag := range requiredFlags {
		val, err := cmd.Flags().GetUint64(reqFlag)
		if err != nil || val == 0 {
			missingFlags = append(missingFlags, reqFlag)
		}
	}

	return createRequiredFlagError(cmd.Name(), missingFlags)
}

func createRequiredFlagError(name string, flags []string) error {
	if len(flags) == 0 {
		return nil
	}

	if len(flags) == 1 {
		return fmt.Errorf("%s is required for %s", flags[0], name)
	}

	return fmt.Errorf("%s are required for %s", strings.Join(flags, ","), name)
}
