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
package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

func TestStartAPI(t *testing.T) {
	clientset := testop.New(3)
	c := New(&k8sutil.Context{Clientset: clientset}, "myname", "ns", "myversion")

	// start a basic cluster
	err := c.Start()
	assert.Nil(t, err)

	validateStart(t, c)

	// starting again should be a no-op
	err = c.Start()
	assert.Nil(t, err)

	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {

	r, err := c.context.Clientset.ExtensionsV1beta1().Deployments(c.Namespace).Get(DeploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, DeploymentName, r.Name)

	s, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get(DeploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, DeploymentName, s.Name)
}

func TestPodSpecs(t *testing.T) {
	clientset := testop.New(1)
	c := New(&k8sutil.Context{Clientset: clientset}, "myname", "ns", "myversion")

	d := c.makeDeployment()
	assert.NotNil(t, d)
	assert.Equal(t, DeploymentName, d.Name)
	assert.Equal(t, v1.RestartPolicyAlways, d.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, 1, len(d.Spec.Template.Spec.Volumes))
	assert.Equal(t, "rook-data", d.Spec.Template.Spec.Volumes[0].Name)

	assert.Equal(t, DeploymentName, d.ObjectMeta.Name)
	assert.Equal(t, DeploymentName, d.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, d.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))

	cont := d.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "quay.io/rook/rookd:myversion", cont.Image)
	assert.Equal(t, 1, len(cont.VolumeMounts))
	assert.Equal(t, 7, len(cont.Env))
	for _, v := range cont.Env {
		assert.True(t, strings.HasPrefix(v.Name, "ROOKD_"))
	}

	expectedCommand := fmt.Sprintf("/usr/bin/rookd api --config-dir=/var/lib/rook --port=%d", model.Port)

	assert.NotEqual(t, -1, strings.Index(cont.Command[2], expectedCommand), cont.Command[2])
}
