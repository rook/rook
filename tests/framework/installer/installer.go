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
	"runtime"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
)

const (
	// Version tag for the latest manifests
	VersionMaster = "master"
	// Version tag for Rook v0.9
	Version0_9 = "v0.9.3"
)

var (
	// ** Variables that might need to be changed depending on the dev environment. The init function below will modify some of them automatically. **
	baseTestDir       string
	forceUseDevices   = false
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

func init() {
	// this default will only work if running kubernetes on the local machine
	baseTestDir, _ = os.Getwd()

	// The following settings could apply to any environment when the kube context is running on the host and the tests are running inside a
	// VM such as minikube. This is a cheap test for this condition, we need to find a better way to automate these settings.
	if runtime.GOOS == "darwin" {
		createBaseTestDir = false
		baseTestDir = "/data"
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

// GatherCRDObjectDebuggingInfo gathers all the descriptions for pods, pvs and pvcs
func GatherCRDObjectDebuggingInfo(k8shelper *utils.K8sHelper, namespace string) {
	k8shelper.PrintPodDescribeForNamespace(namespace)
	k8shelper.PrintPVs(true /*detailed*/)
	k8shelper.PrintPVCs(namespace, true /*detailed*/)
	k8shelper.PrintStorageClasses(true /*detailed*/)
}
