/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package provisioner

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	cephtest "github.com/rook/rook/pkg/daemon/ceph/test"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	storagebeta "k8s.io/api/storage/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/controller"
)

func TestProvisionImage(t *testing.T) {
	clientset := test.New(3)
	namespace := "ns"
	configDir, _ := ioutil.TempDir("", "")
	os.Setenv("POD_NAMESPACE", "rook-ceph")
	defer os.Setenv("POD_NAMESPACE", "")
	defer os.RemoveAll(configDir)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(configDir, namespace))
			}

			if command == "rbd" && args[0] == "create" {
				return `[{"image":"pvc-uid-1-1","size":1048576,"format":2}]`, nil
			}

			if command == "rbd" && args[0] == "info" {
				assert.Equal(t, "testpool/pvc-uid-1-1", args[1])
				return `{"name":"pvc-uid-1-1","size":1048576,"objects":1,"order":20,"object_size":1048576,"block_name_prefix":"testpool_data.229226b8b4567",` +
					`"format":2,"features":["layering"],"op_features":[],"flags":[],"create_timestamp":"Fri Oct  5 19:46:20 2018"}`, nil
			}
			return "", nil
		},
	}

	context := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
		ConfigDir: configDir,
	}

	provisioner := New(context, "foo.io")
	volume := newVolumeOptions(newStorageClass("class-1", "foo.io/block", map[string]string{"pool": "testpool", "clusterNamespace": "testCluster", "fsType": "ext3", "dataBlockPool": ""}, v1.PersistentVolumeReclaimRetain), newClaim("claim-1", "uid-1-1", "class-1", "", "class-1", nil), v1.PersistentVolumeReclaimRetain)

	pv, err := provisioner.Provision(volume)
	assert.Nil(t, err)

	assert.Equal(t, "pvc-uid-1-1", pv.Name)
	assert.NotNil(t, pv.Spec.PersistentVolumeSource.FlexVolume)
	assert.Equal(t, v1.PersistentVolumeReclaimRetain, pv.Spec.PersistentVolumeReclaimPolicy)
	assert.Equal(t, "foo.io/rook-ceph", pv.Spec.PersistentVolumeSource.FlexVolume.Driver)
	assert.Equal(t, "ext3", pv.Spec.PersistentVolumeSource.FlexVolume.FSType)
	assert.Equal(t, "testCluster", pv.Spec.PersistentVolumeSource.FlexVolume.Options["clusterNamespace"])
	assert.Equal(t, "class-1", pv.Spec.PersistentVolumeSource.FlexVolume.Options["storageClass"])
	assert.Equal(t, "testpool", pv.Spec.PersistentVolumeSource.FlexVolume.Options["pool"])
	assert.Equal(t, "pvc-uid-1-1", pv.Spec.PersistentVolumeSource.FlexVolume.Options["image"])
	assert.Equal(t, "", pv.Spec.PersistentVolumeSource.FlexVolume.Options["dataBlockPool"])

	volume = newVolumeOptions(newStorageClass("class-1", "foo.io/block", map[string]string{"pool": "testpool", "clusterNamespace": "testCluster", "fsType": "ext3", "dataBlockPool": "iamdatapool"}, v1.PersistentVolumeReclaimRecycle), newClaim("claim-1", "uid-1-1", "class-1", "", "class-1", nil), v1.PersistentVolumeReclaimRecycle)

	pv, err = provisioner.Provision(volume)
	assert.Nil(t, err)

	assert.Equal(t, "pvc-uid-1-1", pv.Name)
	assert.NotNil(t, pv.Spec.PersistentVolumeSource.FlexVolume)
	assert.Equal(t, v1.PersistentVolumeReclaimRecycle, pv.Spec.PersistentVolumeReclaimPolicy)
	assert.Equal(t, "foo.io/rook-ceph", pv.Spec.PersistentVolumeSource.FlexVolume.Driver)
	assert.Equal(t, "ext3", pv.Spec.PersistentVolumeSource.FlexVolume.FSType)
	assert.Equal(t, "testCluster", pv.Spec.PersistentVolumeSource.FlexVolume.Options["clusterNamespace"])
	assert.Equal(t, "class-1", pv.Spec.PersistentVolumeSource.FlexVolume.Options["storageClass"])
	assert.Equal(t, "testpool", pv.Spec.PersistentVolumeSource.FlexVolume.Options["pool"])
	assert.Equal(t, "pvc-uid-1-1", pv.Spec.PersistentVolumeSource.FlexVolume.Options["image"])
	assert.Equal(t, "iamdatapool", pv.Spec.PersistentVolumeSource.FlexVolume.Options["dataBlockPool"])
}

func TestReclaimPolicyForProvisionedImages(t *testing.T) {
	clientset := test.New(3)
	namespace := "ns"
	configDir, _ := ioutil.TempDir("", "")
	os.Setenv("POD_NAMESPACE", "rook-system")
	defer os.Setenv("POD_NAMESPACE", "")
	defer os.RemoveAll(configDir)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(configDir, namespace))
			}

			if command == "rbd" && args[0] == "create" {
				return `[{"image":"pvc-uid-1-1","size":1048576,"format":2}]`, nil
			}

			if command == "rbd" && args[0] == "info" {
				assert.Equal(t, "testpool/pvc-uid-1-1", args[1])
				return `{"name":"pvc-uid-1-1","size":1048576,"objects":1,"order":20,"object_size":1048576,"block_name_prefix":"testpool_data.229226b8b4567",` +
					`"format":2,"features":["layering"],"op_features":[],"flags":[],"create_timestamp":"Fri Oct  5 19:46:20 2018"}`, nil
			}
			return "", nil
		},
	}

	context := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
		ConfigDir: configDir,
	}

	provisioner := New(context, "foo.io")
	for _, reclaimPolicy := range []v1.PersistentVolumeReclaimPolicy{v1.PersistentVolumeReclaimDelete, v1.PersistentVolumeReclaimRetain, v1.PersistentVolumeReclaimRecycle} {
		volume := newVolumeOptions(newStorageClass("class-1", "foo.io/block", map[string]string{"pool": "testpool", "clusterNamespace": "testCluster", "fsType": "ext3", "dataBlockPool": "iamdatapool"}, reclaimPolicy), newClaim("claim-1", "uid-1-1", "class-1", "", "class-1", nil), reclaimPolicy)
		pv, err := provisioner.Provision(volume)
		assert.Nil(t, err)

		assert.Equal(t, reclaimPolicy, pv.Spec.PersistentVolumeReclaimPolicy)
	}
}

func TestParseClassParameters(t *testing.T) {
	cfg := make(map[string]string)
	cfg["pool"] = "testPool"
	cfg["clustername"] = "myname"
	cfg["fstype"] = "ext4"

	provConfig, err := parseClassParameters(cfg)
	assert.Nil(t, err)

	assert.Equal(t, "testPool", provConfig.blockPool)
	assert.Equal(t, "myname", provConfig.clusterNamespace)
	assert.Equal(t, "ext4", provConfig.fstype)
}

func TestParseClassParametersDefault(t *testing.T) {
	cfg := make(map[string]string)
	cfg["blockPool"] = "testPool"

	provConfig, err := parseClassParameters(cfg)
	assert.Nil(t, err)

	assert.Equal(t, "testPool", provConfig.blockPool)
	assert.Equal(t, "rook-ceph", provConfig.clusterNamespace)
	assert.Equal(t, "", provConfig.fstype)
}

func TestParseClassParametersNoPool(t *testing.T) {
	cfg := make(map[string]string)
	cfg["clustername"] = "myname"

	_, err := parseClassParameters(cfg)
	assert.EqualError(t, err, "StorageClass for provisioner rookVolumeProvisioner must contain 'blockPool' parameter")

}

func TestParseClassParametersInvalidOption(t *testing.T) {
	cfg := make(map[string]string)
	cfg["pool"] = "testPool"
	cfg["foo"] = "bar"

	_, err := parseClassParameters(cfg)
	assert.EqualError(t, err, "invalid option \"foo\" for volume plugin rookVolumeProvisioner")
}

func newVolumeOptions(storageClass *storagebeta.StorageClass, claim *v1.PersistentVolumeClaim, reclaimPolicy v1.PersistentVolumeReclaimPolicy) controller.VolumeOptions {
	return controller.VolumeOptions{
		PersistentVolumeReclaimPolicy: reclaimPolicy,
		PVName:                        "pvc-" + string(claim.ObjectMeta.UID),
		PVC:                           claim,
		Parameters:                    storageClass.Parameters,
	}
}

func newStorageClass(name, provisioner string, parameters map[string]string, reclaimPolicy v1.PersistentVolumeReclaimPolicy) *storagebeta.StorageClass {
	return &storagebeta.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner:   provisioner,
		Parameters:    parameters,
		ReclaimPolicy: &reclaimPolicy,
	}
}

func newClaim(name, claimUID, provisioner, volumeName, storageclassName string, annotations map[string]string) *v1.PersistentVolumeClaim {
	claim := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       v1.NamespaceDefault,
			UID:             types.UID(claimUID),
			ResourceVersion: "0",
			SelfLink:        "/api/v1/namespaces/" + v1.NamespaceDefault + "/persistentvolumeclaims/" + name,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce, v1.ReadOnlyMany},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceName(v1.ResourceStorage): resource.MustParse("1Mi"),
				},
			},
			VolumeName:       volumeName,
			StorageClassName: &storageclassName,
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimPending,
		},
	}
	for k, v := range annotations {
		claim.Annotations[k] = v
	}
	return claim
}
