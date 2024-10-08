/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package csi

import (
	"context"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
)

func Test_getCSIDriverNamePrefixFromDeployment(t *testing.T) {
	namespace := "test"
	deployment := func(name, containerName, drivernameSuffix string) *apps.Deployment {
		return &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: apps.DeploymentSpec{
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name: containerName,
								Args: []string{
									"--drivername=test-prefix." + drivernameSuffix,
								},
							},
						},
					},
				},
			},
		}
	}
	clientset := kfake.NewSimpleClientset()

	ctx := context.TODO()
	csidrivers := []struct {
		testCaseName     string
		deploymentName   string
		containerName    string
		driverNameSuffix string
		expectedPrefix   string
	}{
		{
			"get csi driver name prefix for rbd when deployment exists",
			csiRBDProvisioner,
			csiRBDContainerName,
			rbdDriverSuffix,
			"test-prefix",
		},
		{
			"get csi driver name prefix for rbd when deployment does not exist",
			"",
			"csi-rbdplugin",
			"",
			"",
		},
		{
			"get csi driver name prefix for cephfs when deployment exists",
			csiCephFSProvisioner,
			csiCephFSContainerName,
			cephFSDriverSuffix,
			"test-prefix",
		},
		{
			"get csi driver name prefix for cephfs when deployment does not exist",
			"",
			"csi-cephfsplugin",
			"",
			"",
		},
		{
			"get csi driver name prefix for nfs when deployment exists",
			csiNFSProvisioner,
			csiNFSContainerName,
			nfsDriverSuffix,
			"test-prefix",
		},
		{
			"get csi driver name prefix for nfs when deployment does not exist",
			"",
			"csi-nfsplugin",
			"",
			"",
		},
	}

	for _, c := range csidrivers {
		t.Run(c.testCaseName, func(t *testing.T) {
			if c.deploymentName != "" {
				_, err := clientset.AppsV1().Deployments(namespace).Create(ctx, deployment(c.deploymentName, c.containerName, c.driverNameSuffix), metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			prefix, err := getCSIDriverNamePrefixFromDeployment(ctx, clientset, namespace, c.deploymentName, c.containerName)
			assert.NoError(t, err)
			assert.Equal(t, c.expectedPrefix, prefix)
		})
	}
}
