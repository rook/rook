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
	"sort"
	"testing"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/api/rbac/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStartAPI(t *testing.T) {
	clientset := testop.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "myversion", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

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

	r, err := c.context.Clientset.ExtensionsV1beta1().Deployments(c.Namespace).Get(deploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, deploymentName, r.Name)

	s, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get(deploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, deploymentName, s.Name)
}

func TestPodSpecs(t *testing.T) {
	clientset := testop.New(1)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "rook/rook:myversion", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	d := c.makeDeployment()
	assert.NotNil(t, d)
	assert.Equal(t, deploymentName, d.Name)
	assert.Equal(t, v1.RestartPolicyAlways, d.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, 1, len(d.Spec.Template.Spec.Volumes))
	assert.Equal(t, "rook-data", d.Spec.Template.Spec.Volumes[0].Name)

	assert.Equal(t, deploymentName, d.ObjectMeta.Name)
	assert.Equal(t, deploymentName, d.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, d.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))

	cont := d.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 1, len(cont.VolumeMounts))
	assert.Equal(t, 7, len(cont.Env))

	var envs []string
	for _, v := range cont.Env {
		envs = append(envs, v.Name)
	}
	sort.Strings(envs[:])

	assert.Equal(t, "POD_NAME", envs[0])
	assert.Equal(t, "POD_NAMESPACE", envs[1])
	assert.Equal(t, "ROOK_ADMIN_SECRET", envs[2])
	assert.Equal(t, "ROOK_CLUSTER_NAME", envs[3])
	assert.Equal(t, "ROOK_MON_ENDPOINTS", envs[4])
	assert.Equal(t, "ROOK_MON_SECRET", envs[5])
	assert.Equal(t, "ROOK_NAMESPACE", envs[6])

	assert.Equal(t, "api", cont.Args[0])
	assert.Equal(t, "--config-dir=/var/lib/rook", cont.Args[1])
	assert.Equal(t, "--port=8124", cont.Args[2])
}

func TestClusterRole(t *testing.T) {
	clientset := testop.New(1)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "myversion", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	// the role is create
	err := c.makeRole()
	assert.Nil(t, err)
	role, err := c.context.Clientset.RbacV1beta1().Roles(c.Namespace).Get(deploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, deploymentName, role.Name)
	assert.Equal(t, 4, len(role.Rules))
	account, err := c.context.Clientset.CoreV1().ServiceAccounts(c.Namespace).Get(deploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, c.Namespace, account.Namespace)
	binding, err := c.context.Clientset.RbacV1beta1().RoleBindings(c.Namespace).Get(deploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, deploymentName, binding.RoleRef.Name)
	assert.Equal(t, "Role", binding.RoleRef.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io", binding.RoleRef.APIGroup)
	assert.Equal(t, deploymentName, binding.Subjects[0].Name)
	assert.Equal(t, "ServiceAccount", binding.Subjects[0].Kind)

	// update the rules
	clusterAccessRules = []v1beta1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces"},
			Verbs:     []string{"get", "list"},
		},
	}
	err = c.makeRole()
	assert.Nil(t, err)
	role, err = c.context.Clientset.RbacV1beta1().Roles(c.Namespace).Get(deploymentName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(role.Rules))
	assert.Equal(t, "", role.Rules[0].APIGroups[0])
	assert.Equal(t, 1, len(role.Rules[0].Resources))
	assert.Equal(t, 2, len(role.Rules[0].Verbs))
}
