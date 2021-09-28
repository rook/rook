/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package utils

import (
	"fmt"
	"os"
	"strconv"
)

// TestEnvName gets the name of the test environment. In the CI it is "aws_1.18.x" or similar.
func TestEnvName() string {
	return GetEnvVarWithDefault("TEST_ENV_NAME", "localhost")
}

// TestRetryNumber  get the max retry. Example, for OpenShift it's 40.
func TestRetryNumber() int {
	count := GetEnvVarWithDefault("RETRY_MAX", "45")
	number, err := strconv.Atoi(count)
	if err != nil {
		panic(fmt.Errorf("Error when converting to numeric value %v", err))
	}
	return number
}

// IsPlatformOpenShift check if the platform is openshift or not
func IsPlatformOpenShift() bool {
	return TestEnvName() == "openshift"
}

// GetEnvVarWithDefault get environment variable by key.
func GetEnvVarWithDefault(env, defaultValue string) string {
	val := os.Getenv(env)
	if val == "" {
		return defaultValue
	}
	return val
}
