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
package object

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	err := CreateStore(context, store, version, false, []metav1.OwnerReference{})
	assert.Nil(t, err)

	validateStart(t, store, clientset, false)

	// starting again should update the pods with the new settings
	store.Spec.Gateway.AllNodes = true
	err = UpdateStore(context, store, version, false, []metav1.OwnerReference{})
	assert.Nil(t, err)

	validateStart(t, store, clientset, true)
}

func validateStart(t *testing.T, store cephv1beta1.ObjectStore, clientset *fake.Clientset, allNodes bool) {
	if !allNodes {
		r, err := clientset.ExtensionsV1beta1().Deployments(store.Namespace).Get(instanceName(store), metav1.GetOptions{})
		assert.Nil(t, err)
		assert.Equal(t, instanceName(store), r.Name)

		_, err = clientset.ExtensionsV1beta1().DaemonSets(store.Namespace).Get(instanceName(store), metav1.GetOptions{})
		assert.True(t, errors.IsNotFound(err))
	} else {
		r, err := clientset.ExtensionsV1beta1().DaemonSets(store.Namespace).Get(instanceName(store), metav1.GetOptions{})
		assert.Nil(t, err)
		assert.Equal(t, instanceName(store), r.Name)

		_, err = clientset.ExtensionsV1beta1().Deployments(store.Namespace).Get(instanceName(store), metav1.GetOptions{})
		assert.True(t, errors.IsNotFound(err))
	}

	s, err := clientset.CoreV1().Services(store.Namespace).Get(instanceName(store), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, instanceName(store), s.Name)

	secret, err := clientset.CoreV1().Secrets(store.Namespace).Get(instanceName(store), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, instanceName(store), secret.Name)
	assert.Equal(t, 1, len(secret.StringData))
}

func TestPodSpecs(t *testing.T) {
	store := simpleStore()
	store.Spec.Gateway.Resources = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
		},
	}

	s := makeRGWPodSpec(store, "rook/rook:myversion", true)
	assert.NotNil(t, s)
	//assert.Equal(t, instanceName(store), s.Name)
	assert.Equal(t, v1.RestartPolicyAlways, s.Spec.RestartPolicy)
	assert.Equal(t, 2, len(s.Spec.Volumes))
	assert.Equal(t, "rook-data", s.Spec.Volumes[0].Name)
	assert.Equal(t, k8sutil.ConfigOverrideName, s.Spec.Volumes[1].Name)

	assert.Equal(t, instanceName(store), s.ObjectMeta.Name)
	assert.Equal(t, appName, s.ObjectMeta.Labels["app"])
	assert.Equal(t, store.Namespace, s.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, store.Name, s.ObjectMeta.Labels["rook_object_store"])
	assert.Equal(t, 0, len(s.ObjectMeta.Annotations))

	cont := s.Spec.Containers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 2, len(cont.VolumeMounts))

	assert.Equal(t, 6, len(cont.Args))
	assert.Equal(t, "ceph", cont.Args[0])
	assert.Equal(t, "rgw", cont.Args[1])
	assert.Equal(t, "--config-dir=/var/lib/rook", cont.Args[2])
	assert.Equal(t, fmt.Sprintf("--rgw-name=%s", "default"), cont.Args[3])
	assert.Equal(t, fmt.Sprintf("--rgw-port=%d", 123), cont.Args[4])
	assert.Equal(t, fmt.Sprintf("--rgw-secure-port=%d", 0), cont.Args[5])

	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())
}

func TestSSLPodSpec(t *testing.T) {
	store := simpleStore()
	store.Spec.Gateway.SSLCertificateRef = "mycert"
	store.Spec.Gateway.SecurePort = 443

	s := makeRGWPodSpec(store, "v1.0", true)
	assert.NotNil(t, s)
	assert.Equal(t, instanceName(store), s.Name)
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
			logger.Infof("Command: %s %v", command, args)
			if command == "ceph" {
				if args[1] == "erasure-code-profile" {
					return `{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`, nil
				}
				if args[0] == "auth" && args[1] == "get-or-create-key" {
					return `{"key":"mykey"}`, nil
				}
			}
			return "", nil
		},
	}

	store := simpleStore()
	clientset := testop.New(3)
	context := &clusterd.Context{Executor: executor, Clientset: clientset}

	// create the pools
	err := CreateStore(context, store, "1.2.3.4", false, []metav1.OwnerReference{})
	assert.Nil(t, err)
}

func TestValidateSpec(t *testing.T) {
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}

	// valid store
	s := simpleStore()
	err := validateStore(context, s)
	assert.Nil(t, err)

	// no name
	s.Name = ""
	err = validateStore(context, s)
	assert.NotNil(t, err)
	s.Name = "default"
	err = validateStore(context, s)
	assert.Nil(t, err)

	// no namespace
	s.Namespace = ""
	err = validateStore(context, s)
	assert.NotNil(t, err)
	s.Namespace = "mycluster"
	err = validateStore(context, s)
	assert.Nil(t, err)

	// no replication or EC
	s.Spec.MetadataPool.Replicated.Size = 0
	err = validateStore(context, s)
	assert.NotNil(t, err)
	s.Spec.MetadataPool.Replicated.Size = 1
	err = validateStore(context, s)
	assert.Nil(t, err)
}

func simpleStore() cephv1beta1.ObjectStore {
	return cephv1beta1.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "mycluster"},
		Spec: cephv1beta1.ObjectStoreSpec{
			MetadataPool: cephv1beta1.PoolSpec{Replicated: cephv1beta1.ReplicatedSpec{Size: 1}},
			DataPool:     cephv1beta1.PoolSpec{ErasureCoded: cephv1beta1.ErasureCodedSpec{CodingChunks: 1, DataChunks: 2}},
			Gateway:      cephv1beta1.GatewaySpec{Port: 123},
		},
	}
}
