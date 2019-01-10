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

package rbd

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRBDMirror(t *testing.T) {
	clientset := testop.New(1)
	keysCreated := map[string]bool{}
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		logger.Infof("%s %+v", command, args)
		if args[0] == "auth" && args[1] == "get-or-create-key" {
			daemonName := args[2]
			keysCreated[daemonName] = true
			return "{\"key\":\"mysecurekey\"}", nil
		}
		return "", nil
	}

	c := New(&clusterd.Context{Clientset: clientset, Executor: executor},
		"ns",
		"rook/rook:myversion",
		cephv1.CephVersionSpec{Image: "ceph/ceph:myceph"},
		rookalpha.Placement{},
		false,
		cephv1.RBDMirroringSpec{Workers: 2},
		v1.ResourceRequirements{},
		metav1.OwnerReference{},
	)

	err := c.Start()
	assert.Nil(t, err)
	assert.True(t, keysCreated[fullDaemonName("a")])
	assert.True(t, keysCreated[fullDaemonName("b")])
	assert.False(t, keysCreated[fullDaemonName("c")])

	opts := metav1.ListOptions{}
	d, err := clientset.ExtensionsV1beta1().Deployments(c.Namespace).List(opts)
	assert.Equal(t, 2, len(d.Items))
	for _, de := range d.Items {
		daemonName := de.Name[len(de.Name)-1:]
		assert.True(t, keysCreated[fullDaemonName(daemonName)])
	}
}
