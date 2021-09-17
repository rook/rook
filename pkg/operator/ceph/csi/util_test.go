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
