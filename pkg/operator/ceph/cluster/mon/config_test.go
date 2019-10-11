/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package mon

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateClusterSecrets(t *testing.T) {
	clientset := test.New(1)
	configDir := "ns"
	os.MkdirAll(configDir, 0755)
	defer os.RemoveAll(configDir)
	adminSecret := "AQDkLIBd9vLGJxAAnXsIKPrwvUXAmY+D1g0X1Q=="
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			logger.Infof("COMMAND: %s %v", command, args)
			if command == "ceph-authtool" && args[0] == "--create-keyring" {
				filename := args[1]
				assert.NoError(t, ioutil.WriteFile(filename, []byte(fmt.Sprintf("key = %s", adminSecret)), 0644))
			}
			return "", nil
		},
	}
	context := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
	}
	namespace := "ns"
	ownerRef := &metav1.OwnerReference{}
	info, maxID, mapping, err := CreateOrLoadClusterInfo(context, namespace, ownerRef)
	assert.NoError(t, err)
	assert.Equal(t, -1, maxID)
	require.NotNil(t, info)
	assert.Equal(t, adminSecret, info.AdminSecret)
	assert.NotEqual(t, "", info.FSID)
	assert.NotNil(t, mapping)

	// check for the cluster secret
	secret, err := clientset.CoreV1().Secrets(namespace).Get("rook-ceph-mon", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, adminSecret, string(secret.Data["admin-secret"]))
}
