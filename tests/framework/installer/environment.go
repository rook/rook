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

package installer

import (
	"os"
)

// testEnvName gets the name of the test environment. In the CI it is "aws_1.18.x" or similar.
func testEnvName() string {
	return getEnvVarWithDefault("TEST_ENV_NAME", "localhost")
}

// testHelmPath gets the helm path
func testHelmPath() string {
	return getEnvVarWithDefault("TEST_HELM_PATH", "helm")
}

// TestLogCollectionLevel gets whether to collect all logs
func TestLogCollectionLevel() string {
	return getEnvVarWithDefault("TEST_LOG_COLLECTION_LEVEL", "")
}

// testStorageProvider gets the storage provider for which tests should be run
func testStorageProvider() string {
	return getEnvVarWithDefault("STORAGE_PROVIDER_TESTS", "")
}

// TestIsOfficialBuild gets the storage provider for which tests should be run
func TestIsOfficialBuild() bool {
	// PRs will set this to "false", but the official build will not set it, so we compare against "false"
	return getEnvVarWithDefault("TEST_IS_OFFICIAL_BUILD", "") != "false"
}

// baseTestDir gets the base test directory
func baseTestDir() string {
	// If the base test directory is actively set to empty (as in CI), we use the current working directory.
	val := getEnvVarWithDefault("TEST_BASE_DIR", "/data")
	if val == "WORKING_DIR" {
		val, _ = os.Getwd()
	}
	return val
}

// TestScratchDevice get the scratch device to be used for OSD
func TestScratchDevice() string {
	return getEnvVarWithDefault("TEST_SCRATCH_DEVICE", "/dev/nvme0n1")
}

func getEnvVarWithDefault(env, defaultValue string) string {
	val := os.Getenv(env)
	if val == "" {
		logger.Infof("test environment variable (default) %q=%q", env, defaultValue)
		return defaultValue
	}
	logger.Infof("test environment variable %q=%q", env, val)
	return val
}
