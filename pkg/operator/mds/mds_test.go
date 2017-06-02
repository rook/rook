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
package mds

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"
)

func TestStartMDS(t *testing.T) {
	info := testop.CreateClusterInfo(1)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
	}

	c := New(&clusterd.Context{Executor: executor}, "myname", "ns", "myversion", k8sutil.Placement{})
	c.dataDir = "/tmp/mdstest"
	defer os.RemoveAll(c.dataDir)

	clientset := testop.New(3)

	// start a basic cluster
	err := c.Start(clientset, info)
	assert.Nil(t, err)

	validateStart(t, c, clientset)

	// starting again should be a no-op
	err = c.Start(clientset, info)
	assert.Nil(t, err)

	validateStart(t, c, clientset)
}

func validateStart(t *testing.T, c *Cluster, clientset *fake.Clientset) {

	r, err := clientset.ExtensionsV1beta1().Deployments(c.Namespace).Get("mds", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mds", r.Name)
}

func TestPodSpecs(t *testing.T) {
	c := New(nil, "myname", "ns", "myversion", k8sutil.Placement{})
	mdsID := "mds1"

	d := c.makeDeployment(mdsID)
	assert.NotNil(t, d)
	assert.Equal(t, "mds", d.Name)
	assert.Equal(t, v1.RestartPolicyAlways, d.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, 2, len(d.Spec.Template.Spec.Volumes))
	assert.Equal(t, "rook-data", d.Spec.Template.Spec.Volumes[0].Name)

	assert.Equal(t, "mds", d.ObjectMeta.Name)
	assert.Equal(t, "mds", d.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, d.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))

	cont := d.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "quay.io/rook/rookd:myversion", cont.Image)
	assert.Equal(t, 2, len(cont.VolumeMounts))
	assert.Equal(t, 6, len(cont.Env))

	expectedCommand := fmt.Sprintf("/usr/local/bin/rookd mds --config-dir=/var/lib/rook --mds-id=%s ",
		mdsID)

	assert.NotEqual(t, -1, strings.Index(cont.Command[2], expectedCommand), cont.Command[2])
}
