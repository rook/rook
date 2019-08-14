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

package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/stretchr/testify/assert"
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
	logger.Infof("testing that actual args: %s\nmatch expected args:%v", fullArgString, actualArgs)
	for _, arg := range expectedArgs {
		validArgMatcher := strings.Join(arg, " ")
		// We join each individual argument together the same was as the big string
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

// AssertLabelsContainRookRequirements asserts that the the labels under test contain the labels
// which all Rook pods should have. This can be used with labels for Kubernetes Deployments,
// DaemonSets, etc.
func AssertLabelsContainRookRequirements(t *testing.T, labels map[string]string, appName string) {
	resourceLabels := []string{}
	for k, v := range labels {
		resourceLabels = append(resourceLabels, fmt.Sprintf("%s=%s", k, v))
	}
	expectedLabels := []string{
		"app=" + appName,
	}
	assert.Subset(t, resourceLabels, expectedLabels,
		"labels on resource do not match Rook requirements", labels)
}
