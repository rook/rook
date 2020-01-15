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
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
)

const (
	// VersionMaster tag for the latest manifests
	VersionMaster = "master"

	// test suite names
	CassandraTestSuite   = "cassandra"
	CephTestSuite        = "ceph"
	CockroachDBTestSuite = "cockroachdb"
	EdgeFSTestSuite      = "edgefs"
	NFSTestSuite         = "nfs"
	YugabyteDBTestSuite  = "yugabytedb"
)

var (
	// ** Variables that might need to be changed depending on the dev environment. The init function below will modify some of them automatically. **
	baseTestDir       string
	createBaseTestDir = true
	// ** end of Variables to modify
	logger              = capnslog.NewPackageLogger("github.com/rook/rook", "installer")
	createArgs          = []string{"create", "-f"}
	createFromStdinArgs = append(createArgs, "-")
	deleteArgs          = []string{"delete", "-f"}
	deleteFromStdinArgs = append(deleteArgs, "-")
)

type TestSuite interface {
	Setup()
	Teardown()
}

func SkipTestSuite(name string) bool {
	testsToRun := os.Getenv("STORAGE_PROVIDER_TESTS")
	// jenkins passes "null" if the env var is not set.
	if testsToRun == "" || testsToRun == "null" {
		// run all test suites
		return false
	}
	if strings.EqualFold(testsToRun, name) {
		// this suite was requested
		return false
	}

	logger.Infof("skipping test suite since only %s should be tested rather than %s", testsToRun, name)
	return true
}

func init() {
	// If the base test directory is actively set to empty (as in CI), we use the current working directory.
	baseTestDir = Env.BaseTestDir
	if baseTestDir == "" {
		baseTestDir, _ = os.Getwd()
	}
	if baseTestDir == "/data" {
		createBaseTestDir = false
	}
}

func SystemNamespace(namespace string) string {
	return fmt.Sprintf("%s-system", namespace)
}

func checkError(t *testing.T, err error, message string) {
	// During cleanup the resource might not be found because the test might have failed before the test was done and never created the resource
	if err == nil || errors.IsNotFound(err) {
		return
	}
	assert.NoError(t, err, "%s. %+v", message, err)
}

func concatYaml(first, second string) string {
	return first + `
---
` + second
}
