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
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	testDSTemplate = []byte(`
kind: DaemonSet
apiVersion: apps/v1
metadata:
  name: test-label
  namespace: {{ .Namespace }}
spec:
  selector:
    matchLabels:
      app: test-label
  template:
    metadata:
      labels:
        app: test-label
    spec:
      serviceAccount: test-sa
      containers:
        - name: registrar
          image: {{ .RegistrarImage }}
        - name: rbdplugin
          image: {{ .CSIPluginImage }}
        - name: cephfsplugin
          image: {{ .CSIPluginImage }}
`)
	testDepTemplate = []byte(`
kind: Deployment
apiVersion: apps/v1
metadata:
  name: test-label
  namespace: {{ .Namespace }}
spec:
  replicas: {{ .ProvisionerReplicas }}
  selector:
    matchLabels:
      app: test-label
  template:
    metadata:
      labels:
        app: test-label
    spec:
      serviceAccount: test-sa
      containers:
        - name: csi-attacher
          image: {{ .AttacherImage }}
        - name: csi-snapshotter
          image: {{ .SnapshotterImage }}
        - name: csi-resizer
          image: {{ .ResizerImage }}
        - name: csi-provisioner
          image: {{ .ProvisionerImage }}
        - name: csi-cephfsplugin
          image: {{ .CSIPluginImage }}
`)
)

func TestDaemonSetTemplate(t *testing.T) {
	tmp, err := ioutil.TempFile("", "yaml")
	assert.Nil(t, err)

	defer os.Remove(tmp.Name())

	_, err = tmp.Write(testDSTemplate)
	assert.Nil(t, err)
	err = tmp.Close()
	assert.Nil(t, err)

	tp := templateParam{
		Param:     CSIParam,
		Namespace: "foo",
	}
	_, err = templateToDaemonSet("test-ds", tmp.Name(), tp)
	assert.Nil(t, err)
}

func TestDeploymentTemplate(t *testing.T) {
	tmp, err := ioutil.TempFile("", "yaml")
	assert.Nil(t, err)

	defer os.Remove(tmp.Name())

	_, err = tmp.Write(testDepTemplate)
	assert.Nil(t, err)
	err = tmp.Close()
	assert.Nil(t, err)

	tp := templateParam{
		Param:     CSIParam,
		Namespace: "foo",
	}
	_, err = templateToDeployment("test-dep", tmp.Name(), tp)
	assert.Nil(t, err)
}

func Test_getPortFromConfig(t *testing.T) {
	k8s := fake.NewSimpleClientset()

	var key = "TEST_CSI_PORT_ENV"
	var defaultPort uint16 = 8000

	// empty env variable
	port, err := getPortFromConfig(k8s, key, defaultPort)
	assert.Nil(t, err)
	assert.Equal(t, port, defaultPort)

	// valid port is set in env
	err = os.Setenv(key, "9000")
	assert.Nil(t, err)
	port, err = getPortFromConfig(k8s, key, defaultPort)
	assert.Nil(t, err)
	assert.Equal(t, port, uint16(9000))

	err = os.Unsetenv(key)
	assert.Nil(t, err)
	// higher port value is set in env
	err = os.Setenv(key, "65536")
	assert.Nil(t, err)
	port, err = getPortFromConfig(k8s, key, defaultPort)
	assert.Error(t, err)
	assert.Equal(t, port, defaultPort)

	err = os.Unsetenv(key)
	assert.Nil(t, err)
	// negative port is set in env
	err = os.Setenv(key, "-1")
	assert.Nil(t, err)
	port, err = getPortFromConfig(k8s, key, defaultPort)
	assert.Error(t, err)
	assert.Equal(t, port, defaultPort)

	err = os.Unsetenv(key)
	assert.Nil(t, err)
}
