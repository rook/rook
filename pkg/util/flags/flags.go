/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package flags

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

func createRequiredFlagError(name string, flags []string) error {
	if len(flags) == 0 {
		return nil
	}

	if len(flags) == 1 {
		return fmt.Errorf("%s is required for %s", flags[0], name)
	}

	return fmt.Errorf("%s are required for %s", strings.Join(flags, ","), name)
}

func SetLoggingFlags(flags *pflag.FlagSet) error {
	//Add commandline flags to the flagset. We will always write to stderr
	//and not to a file by default
	flags.AddGoFlagSet(flag.CommandLine)
	if err := flags.Set("logtostderr", "true"); err != nil {
		return fmt.Errorf("failed to set flag %q. %v", "logtostderr", err)
	}
	if err := flags.Parse(nil); err != nil {
		return fmt.Errorf("failed to parse logging flag. %v", err)
	}
	return nil
}

func SetFlagsFromEnv(flags *pflag.FlagSet, prefix string) error {
	var errorFlag bool
	var err error
	flags.VisitAll(func(f *pflag.Flag) {
		envVar := prefix + "_" + strings.Replace(strings.ToUpper(f.Name), "-", "_", -1)
		value := os.Getenv(envVar)
		if value != "" {
			// Set the environment variable. Will override default values, but be overridden by command line parameters.
			if err = flags.Set(f.Name, value); err != nil {
				errorFlag = true
			}
		}
	})
	if errorFlag != false {
		return fmt.Errorf("error while setting CLI flags from environment variables. %v", err)
	}
	return nil
}

// GetFlagsAndValues returns all flags and their values as a slice with elements in the format of
// "--<flag>=<value>"
func GetFlagsAndValues(flags *pflag.FlagSet, excludeFilter string) []string {
	var flagValues []string

	flags.VisitAll(func(f *pflag.Flag) {
		val := f.Value.String()
		if excludeFilter != "" {
			if matched, _ := regexp.Match(excludeFilter, []byte(f.Name)); matched {
				val = "*****"
			}
		}

		flagValues = append(flagValues, fmt.Sprintf("--%s=%s", f.Name, val))
	})

	return flagValues
}
