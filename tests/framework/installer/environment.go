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

// testHelmPath gets the helm path
func testHelmPath() string {
	return getEnvVarWithDefault("TEST_HELM_PATH", "/tmp/rook-tests-scripts-helm/helm")
}

// TestLogCollectionLevel gets whether to collect all logs
func TestLogCollectionLevel() string {
	return getEnvVarWithDefault("TEST_LOG_COLLECTION_LEVEL", "")
}

func StorageClassName() string {
	return getEnvVarWithDefault("TEST_STORAGE_CLASS", "")
}

func UsePVC() bool {
	return StorageClassName() != ""
}

// baseTestDir gets the base test directory
func baseTestDir() (string, error) {
	// If the base test directory is actively set to WORKING_DIR (as in CI),
	// we use the current working directory.
	val := getEnvVarWithDefault("TEST_BASE_DIR", "/data")
	if val == "WORKING_DIR" {
		var err error
		val, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	return val, nil
}

// TestScratchDevice get the scratch device to be used for OSD
func TestScratchDevice() string {
	return getEnvVarWithDefault("TEST_SCRATCH_DEVICE", "/dev/nvme0n1")
}

// getDeviceFilter get the device name used for OSD
func getDeviceFilter() string {
	return getEnvVarWithDefault("DEVICE_FILTER", `""`)
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
