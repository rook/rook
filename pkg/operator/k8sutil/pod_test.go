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
package k8sutil

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
)

func TestMakeRookImage(t *testing.T) {
	assert.Equal(t, "rook/rook:v1", MakeRookImage("v1"))
}

func TestMakeRookImageWithEnv(t *testing.T) {
	os.Setenv(repoPrefixEnvVar, "myrepo.io/rook")
	assert.Equal(t, "myrepo.io/rook/rook:v1", MakeRookImage("v1"))
	os.Setenv(repoPrefixEnvVar, "")
}

func TestDefaultVersion(t *testing.T) {
	assert.Equal(t, fmt.Sprintf("rook/rook:%s", defaultVersion), MakeRookImage(""))
}

func TestBuildSecurityContext(t *testing.T) {
	privileged := true
	runAsUser := int64(0)
	runAsNonRoot := false
	readOnlyRootFilesystem := false

	expected := v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	assert.Equal(t, expected, BuildSecurityContext(true, 0, false, false))

	privileged = false
	runAsUser = int64(1000)
	runAsNonRoot = true
	readOnlyRootFilesystem = true

	expected = v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		RunAsNonRoot:           &runAsNonRoot,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	assert.Equal(t, expected, BuildSecurityContext(false, 1000, true, true))
}
