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

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/rbac/v1beta1"
)

func TestStartAPI(t *testing.T) {
	clientset := testop.New(3)
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "myname", "ns", "myversion", k8sutil.Placement{})

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
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "myname", "ns", "myversion", k8sutil.Placement{})

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

	expectedCommand := fmt.Sprintf("/usr/local/bin/rookd api --config-dir=/var/lib/rook --port=%d", model.Port)

	assert.NotEqual(t, -1, strings.Index(cont.Command[2], expectedCommand), cont.Command[2])
}

func TestClusterRole(t *testing.T) {
	clientset := testop.New(1)
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "myname", "ns", "myversion", k8sutil.Placement{})

	// the role is create
	err := c.makeClusterRole()
	assert.Nil(t, err)
	role, err := c.context.Clientset.RbacV1beta1().ClusterRoles().Get(DeploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, DeploymentName, role.Name)
	assert.Equal(t, 3, len(role.Rules))
	account, err := c.context.Clientset.CoreV1().ServiceAccounts(c.Namespace).Get(DeploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, c.Namespace, account.Namespace)
	binding, err := c.context.Clientset.RbacV1beta1().ClusterRoleBindings().Get(DeploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, DeploymentName, binding.RoleRef.Name)
	assert.Equal(t, "ClusterRole", binding.RoleRef.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io", binding.RoleRef.APIGroup)
	assert.Equal(t, DeploymentName, binding.Subjects[0].Name)
	assert.Equal(t, "ServiceAccount", binding.Subjects[0].Kind)

	// update the rules
	clusterAccessRules = []v1beta1.PolicyRule{
		v1beta1.PolicyRule{
			APIGroups: []string{""},
			Resources: []string{"namespaces"},
			Verbs:     []string{"get", "list"},
		},
	}
	err = c.makeClusterRole()
	assert.Nil(t, err)
	role, err = c.context.Clientset.RbacV1beta1().ClusterRoles().Get(DeploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(role.Rules))
	assert.Equal(t, "", role.Rules[0].APIGroups[0])
	assert.Equal(t, 1, len(role.Rules[0].Resources))
	assert.Equal(t, 2, len(role.Rules[0].Verbs))
}
