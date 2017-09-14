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
package rgw

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	cephrgw "github.com/rook/rook/pkg/ceph/rgw"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/pool"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStartRGW(t *testing.T) {
	clientset := testop.New(3)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return `{"key":"mysecurekey"}`, nil
		},
		MockExecuteCommandWithCombinedOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			return `{"id":"test-id"}`, nil
		},
	}

	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{Clientset: clientset, Executor: executor, ConfigDir: configDir}
	store := simpleStore()
	version := "v1.1.0"

	// start a basic cluster
	err := store.Create(context, version, false)
	assert.Nil(t, err)

	validateStart(t, store, clientset)

	// starting again should be a no-op
	err = store.Create(context, version, false)
	assert.Nil(t, err)

	validateStart(t, store, clientset)
}

func validateStart(t *testing.T, store *ObjectStore, clientset *fake.Clientset) {

	r, err := clientset.ExtensionsV1beta1().Deployments(store.Namespace).Get(store.instanceName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, store.instanceName(), r.Name)

	s, err := clientset.CoreV1().Services(store.Namespace).Get(store.instanceName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, store.instanceName(), s.Name)

	secret, err := clientset.CoreV1().Secrets(store.Namespace).Get(store.instanceName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, store.instanceName(), secret.Name)
	assert.Equal(t, 1, len(secret.StringData))
}

func TestPodSpecs(t *testing.T) {
	store := simpleStore()

	s := store.makeRGWPodSpec("myversion", true)
	assert.NotNil(t, s)
	//assert.Equal(t, store.instanceName(), s.Name)
	assert.Equal(t, v1.RestartPolicyAlways, s.Spec.RestartPolicy)
	assert.Equal(t, 2, len(s.Spec.Volumes))
	assert.Equal(t, "rook-data", s.Spec.Volumes[0].Name)
	assert.Equal(t, k8sutil.ConfigOverrideName, s.Spec.Volumes[1].Name)

	assert.Equal(t, store.instanceName(), s.ObjectMeta.Name)
	assert.Equal(t, appName, s.ObjectMeta.Labels["app"])
	assert.Equal(t, store.Namespace, s.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, store.Name, s.ObjectMeta.Labels["rook_object_store"])
	assert.Equal(t, 0, len(s.ObjectMeta.Annotations))

	cont := s.Spec.Containers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 2, len(cont.VolumeMounts))

	assert.Equal(t, 6, len(cont.Args))
	assert.Equal(t, "rgw", cont.Args[0])
	assert.Equal(t, "--config-dir=/var/lib/rook", cont.Args[1])
	assert.Equal(t, fmt.Sprintf("--rgw-name=%s", "default"), cont.Args[2])
	assert.Equal(t, fmt.Sprintf("--rgw-host=%s", store.instanceName()), cont.Args[3])
	assert.Equal(t, fmt.Sprintf("--rgw-port=%d", 123), cont.Args[4])
	assert.Equal(t, fmt.Sprintf("--rgw-secure-port=%d", 0), cont.Args[5])
}

func TestSSLPodSpec(t *testing.T) {
	store := simpleStore()
	store.Spec.Gateway.SSLCertificateRef = "mycert"
	store.Spec.Gateway.SecurePort = 443

	s := store.makeRGWPodSpec("v1.0", true)
	assert.NotNil(t, s)
	assert.Equal(t, store.instanceName(), s.Name)
	assert.Equal(t, 3, len(s.Spec.Volumes))
	assert.Equal(t, certVolumeName, s.Spec.Volumes[2].Name)
	assert.True(t, s.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, s.Spec.DNSPolicy)

	cont := s.Spec.Containers[0]
	assert.Equal(t, 3, len(cont.VolumeMounts))
	assert.Equal(t, certVolumeName, cont.VolumeMounts[2].Name)
	assert.Equal(t, certMountPath, cont.VolumeMounts[2].MountPath)

	assert.Equal(t, 7, len(cont.Args))
	assert.Equal(t, fmt.Sprintf("--rgw-secure-port=%d", 443), cont.Args[5])
	assert.Equal(t, fmt.Sprintf("--rgw-cert=%s/%s", certMountPath, certFilename), cont.Args[6])
}

func TestCreateObjectStore(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: func(debug bool, actionName, command string, args ...string) (string, error) {
			return `{"realms": []}`, nil
		},
		MockExecuteCommandWithOutputFile: func(debug bool, actionName, command, outfile string, args ...string) (string, error) {
			if command == "ceph" && args[1] == "erasure-code-profile" {
				return `{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`, nil
			}
			return "", nil
		},
	}

	store := simpleStore()
	context := cephrgw.NewContext(&clusterd.Context{Executor: executor}, store.Name, store.Namespace)

	// create the pools
	err := store.createObjectStore(context, "1.2.3.4")
	assert.Nil(t, err)
}

func TestValidateSpec(t *testing.T) {
	// valid store
	s := simpleStore()
	err := s.validate()
	assert.Nil(t, err)

	// no name
	s.Name = ""
	err = s.validate()
	assert.NotNil(t, err)
	s.Name = "default"
	err = s.validate()
	assert.Nil(t, err)

	// no namespace
	s.Namespace = ""
	err = s.validate()
	assert.NotNil(t, err)
	s.Namespace = "mycluster"
	err = s.validate()
	assert.Nil(t, err)

	// no replication or EC
	s.Spec.MetadataPool.Replicated.Size = 0
	err = s.validate()
	assert.NotNil(t, err)
	s.Spec.MetadataPool.Replicated.Size = 1
	err = s.validate()
	assert.Nil(t, err)
}

func simpleStore() *ObjectStore {
	return &ObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "mycluster"},
		Spec: ObjectStoreSpec{
			MetadataPool: pool.PoolSpec{Replicated: pool.ReplicatedSpec{Size: 1}},
			DataPool:     pool.PoolSpec{ErasureCoded: pool.ErasureCodedSpec{CodingChunks: 1, DataChunks: 2}},
			Gateway: GatewaySpec{
				Port: 123,
			},
		},
	}
}
