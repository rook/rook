package util

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
			return fmt.Errorf("%s is required for %s", reqFlag, cmd.Name())
		}
	}

	return nil
}
