// Package testlib provides common methods for testing code which applies to many Ceph daemons.
//
// Methods beginning with "TestSpec" can be used to test that Kubernetes resource specs (pods, etc.)
// are configured correctly.
package test

import (
	"fmt"
	"strings"

	"github.com/coreos/pkg/capnslog"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "ceph-op-testlib")

// ArgumentsMatchExpected returns a descriptive error if any of the expected arguments do not exist.
// This supports arguments in which flags appear multiple times with different values but does not
// support multiple instances of the same flag-value. This test is designed to fail if the list
// of actual arguments contains extra arguments not specified in the expected args.
// The expected arguments are given as an array of string arrays. This is to support flags which
// may have multiple values. Examples:
// expectedArgs := [][]string{
//     {"-h"},                              // test for a short flag
//     {"-vvv"},                            // test for a short flag with value(s) specified
//     {"-d", "3"},                         // test for a short flag with a value specified
//     {"--verbose"},                       // test for a --bool flag
//     {"--name=alex"},                     // test for a --flag=value flag
//     {"--name", "sam"},                   // test for a --flag with a value after a space
//     {"--full-name", "sam", "goodhuman"}, // test for a --flag with 2 values separated by spaces
// }
func ArgumentsMatchExpected(actualArgs []string, expectedArgs [][]string) error {
	// join all args into a big space-separated arg string so we can use string search on it
	// this is simpler than a bunch of nested for loops and if statements with continues in them
	fullArgString := strings.Join(actualArgs, " ")
	logger.Info("testing that actual args: %s\nmatch expected args:%v", fullArgString, actualArgs)
	for _, arg := range expectedArgs {
		// We join each individual argument together the same was as the big string
		validArgMatcher := strings.Join(arg, " ")
		if validArgMatcher == "" {
			return fmt.Errorf("Expected argument %v evaluated to empty string; ArgumentsMatchExpected() doesn't know what to do", arg)
		}
		matches := strings.Count(fullArgString, validArgMatcher)
		if matches > 1 {
			return fmt.Errorf("More than one instance of flag '%s' in: %s; ArgumentsMatchExpected() doesn't know what to do",
				validArgMatcher, fullArgString)
		} else if matches == 1 {
			// Remove the instance of the valid match so we can't match to it any more
			fullArgString = strings.Replace(fullArgString, validArgMatcher, "", 1)
		} else { // zero matches
			return fmt.Errorf("Expected argument '%s' missing in: %s\n(It's possible the same arg is in expectedArgs twice.)",
				validArgMatcher, strings.Join(actualArgs, " "))
		}
	}
	if remainingArgs := strings.Trim(fullArgString, " "); remainingArgs != "" {
		return fmt.Errorf("The actual arguments have additional args specified: %s", remainingArgs)
	}
	return nil
}
