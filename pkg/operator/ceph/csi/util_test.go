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

package csi

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func TestDaemonSetTemplate(t *testing.T) {
	tp := templateParam{
		Param:     CSIParam,
		Namespace: "foo",
	}
	ds, err := templateToDaemonSet("test-ds", RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	assert.Equal(t, "driver-registrar", ds.Spec.Template.Spec.Containers[0].Name)
}

func TestDeploymentTemplate(t *testing.T) {
	tp := templateParam{
		Param:     CSIParam,
		Namespace: "foo",
	}
	_, err := templateToDeployment("test-dep", RBDProvisionerDepTemplatePath, tp)
	assert.Nil(t, err)
}

func TestGetPortFromConfig(t *testing.T) {
	var key = "TEST_CSI_PORT_ENV"
	var defaultPort uint16 = 8000
	data := map[string]string{}

	// empty env variable
	port, err := getPortFromConfig(data, key, defaultPort)
	assert.Nil(t, err)
	assert.Equal(t, port, defaultPort)

	// valid port is set in env
	err = os.Setenv(key, "9000")
	assert.Nil(t, err)
	port, err = getPortFromConfig(data, key, defaultPort)
	assert.Nil(t, err)
	assert.Equal(t, port, uint16(9000))

	err = os.Unsetenv(key)
	assert.Nil(t, err)
	// higher port value is set in env
	err = os.Setenv(key, "65536")
	assert.Nil(t, err)
	port, err = getPortFromConfig(data, key, defaultPort)
	assert.Error(t, err)
	assert.Equal(t, port, defaultPort)

	err = os.Unsetenv(key)
	assert.Nil(t, err)
	// negative port is set in env
	err = os.Setenv(key, "-1")
	assert.Nil(t, err)
	port, err = getPortFromConfig(data, key, defaultPort)
	assert.Error(t, err)
	assert.Equal(t, port, defaultPort)

	err = os.Unsetenv(key)
	assert.Nil(t, err)
}

func TestApplyingResourcesToRBDPlugin(t *testing.T) {
	tp := templateParam{}
	rbdPlugin, err := templateToDaemonSet("rbdplugin", RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	params := make(map[string]string)

	// need to build using map[string]interface{} because the following resource
	// doesn't serialise nicely
	// https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity
	resource := []map[string]interface{}{
		{
			"name": "driver-registrar",
			"resource": map[string]interface{}{
				"limits": map[string]interface{}{
					"cpu":    "200m",
					"memory": "256Mi",
				},
				"requests": map[string]interface{}{
					"cpu":    "100m",
					"memory": "128Mi",
				},
			},
		},
	}

	resourceRaw, err := yaml.Marshal(resource)
	assert.Nil(t, err)
	params[rbdPluginResource] = string(resourceRaw)
	applyResourcesToContainers(params, rbdPluginResource, &rbdPlugin.Spec.Template.Spec)
	assert.Equal(t, rbdPlugin.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String(), "128Mi")
	assert.Equal(t, rbdPlugin.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String(), "256Mi")
	assert.Equal(t, rbdPlugin.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String(), "100m")
	assert.Equal(t, rbdPlugin.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String(), "200m")
}
