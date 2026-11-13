/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package osd

import (
	"context"
	"strings"
	"syscall"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestStoreHostOSDKeyInKMS(t *testing.T) {
	osd := OSDInfo{ID: 3, UUID: "9d1b4c0e-6f7a-4f53-9a8e-1f2b3c4d5e6f"}

	// no KMS_PROVIDER selects the Kubernetes Secrets provider
	kmsEnabled := cephv1.ClusterSecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{"KMS_SERVICE_NAME": "test"}}}
	run := func(security cephv1.ClusterSecuritySpec, keyInKMS string, getErr error) (*fake.Clientset, []string, error) {
		var ran []string
		clientset := fake.NewSimpleClientset()
		// the API server moves StringData to Data, the fake clientset does not
		clientset.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			secret := action.(k8stesting.CreateAction).GetObject().(*v1.Secret)
			for k, v := range secret.StringData {
				if secret.Data == nil {
					secret.Data = map[string][]byte{}
				}
				secret.Data[k] = []byte(v)
			}
			return false, nil, nil
		})
		if keyInKMS != "" {
			// the default KMS keeps keys in Kubernetes Secrets
			_, err := clientset.CoreV1().Secrets("ns").Create(context.TODO(), &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: kms.GenerateOSDEncryptionSecretName(osd.UUID), Namespace: "ns"},
				Data:       map[string][]byte{kms.OsdEncryptionSecretNameKeyName: []byte(keyInKMS)},
			}, metav1.CreateOptions{})
			assert.NoError(t, err)
		}
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				ran = append(ran, strings.Join(args[:3], " "))
				return "mon-key", getErr
			},
		}
		c := &Cluster{
			context:     &clusterd.Context{Executor: executor, Clientset: clientset},
			clusterInfo: cephclient.AdminTestClusterInfo("ns"),
			spec:        cephv1.ClusterSpec{Security: security},
		}
		return clientset, ran, c.storeHostOSDKeyInKMS(osd)
	}
	kmsKey := func(clientset *fake.Clientset) string {
		s, err := clientset.CoreV1().Secrets("ns").Get(context.TODO(), kms.GenerateOSDEncryptionSecretName(osd.UUID), metav1.GetOptions{})
		if err != nil {
			return ""
		}
		return string(s.Data[kms.OsdEncryptionSecretNameKeyName])
	}

	t.Run("key in the mons is copied to the KMS", func(t *testing.T) {
		clientset, ran, err := run(kmsEnabled, "", nil)
		assert.NoError(t, err)
		assert.Equal(t, []string{"config-key get dm-crypt/osd/" + osd.UUID + "/luks"}, ran)
		assert.Equal(t, "mon-key", kmsKey(clientset))
	})

	t.Run("key already in the KMS reads nothing from the mons", func(t *testing.T) {
		clientset, ran, err := run(kmsEnabled, "kms-key", nil)
		assert.NoError(t, err)
		assert.Empty(t, ran)
		assert.Equal(t, "kms-key", kmsKey(clientset))
	})

	t.Run("failed mon read stores nothing", func(t *testing.T) {
		clientset, _, err := run(kmsEnabled, "", syscall.ENOENT)
		assert.Error(t, err)
		assert.Equal(t, "", kmsKey(clientset))
	})

	t.Run("no KMS configured fails without copying the key", func(t *testing.T) {
		clientset, ran, err := run(cephv1.ClusterSecuritySpec{}, "", nil)
		assert.ErrorContains(t, err, "security.kms is not configured")
		assert.Empty(t, ran)
		assert.Equal(t, "", kmsKey(clientset))
	})
}

func TestHostOSDKeyNames(t *testing.T) {
	deployment := func(name, uuid, cvMode string, onPVC bool) *appsv1.Deployment {
		env := []v1.EnvVar{{Name: "ROOK_OSD_UUID", Value: uuid}, {Name: "ROOK_CV_MODE", Value: cvMode}, {Name: "ROOK_BLOCK_PATH", Value: "/dev/x"}}
		if onPVC {
			env = append(env, v1.EnvVar{Name: "ROOK_PVC_BACKED_OSD", Value: "true"})
		}
		labels := map[string]string{"app": AppName, encrypted: "true", OsdIdLabelKey: name[len(name)-1:]}
		if onPVC {
			labels[OSDOverPVCLabelKey] = "pvc"
		}
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", Labels: labels},
			Spec: appsv1.DeploymentSpec{Template: v1.PodTemplateSpec{Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "osd", Env: env}},
			}}},
		}
	}
	clientset := fake.NewSimpleClientset(
		deployment("rook-ceph-osd-0", "uuid-host-raw", "raw", false),
		deployment("rook-ceph-osd-1", "uuid-host-lvm", "lvm", false),
		deployment("rook-ceph-osd-2", "uuid-pvc-raw", "raw", true),
	)

	names, err := HostOSDKeyNames(context.TODO(), &clusterd.Context{Clientset: clientset}, "ns")
	assert.NoError(t, err)
	assert.Equal(t, []string{"uuid-host-raw"}, names)
}

func TestHostOSDsUseKMS(t *testing.T) {
	vault := cephv1.ClusterSecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault"}}}
	c := &Cluster{clusterInfo: &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 19, Minor: 2, Extra: 1}}, spec: cephv1.ClusterSpec{Security: vault}}
	encrypted := osdProperties{storeConfig: config.StoreConfig{EncryptedDevice: true}}
	deviceEncrypted := osdProperties{devices: []cephv1.Device{{Name: "sdb", Config: map[string]string{config.EncryptedDeviceKey: "true"}}}}

	assert.True(t, c.hostOSDsUseKMS(encrypted))
	assert.True(t, c.hostOSDsUseKMS(deviceEncrypted))
	assert.False(t, c.hostOSDsUseKMS(osdProperties{}))
	assert.False(t, c.hostOSDsUseKMS(osdProperties{storeConfig: encrypted.storeConfig, pvc: v1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc"}}))

	// v19.2.0 does not write the OSD fsid to the LUKS2 header
	c.clusterInfo.CephVersion = cephver.CephVersion{Major: 19, Minor: 2, Extra: 0}
	assert.False(t, c.hostOSDsUseKMS(encrypted))
	c.clusterInfo.CephVersion = cephver.CephVersion{Major: 20, Minor: 2, Extra: 0}
	assert.True(t, c.hostOSDsUseKMS(encrypted))

	c.spec.Security = cephv1.ClusterSecuritySpec{}
	assert.False(t, c.hostOSDsUseKMS(encrypted))
}

func TestHostEncryptionOpenInitContainer(t *testing.T) {
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns", CephVersion: cephver.Tentacle}
	clusterInfo.SetName("test")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	spec := cephv1.ClusterSpec{
		CephVersion:     cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v20"},
		Storage:         cephv1.StorageScopeSpec{Nodes: []cephv1.Node{{Name: "node1"}}},
		DataDirHostPath: "/var/lib/rook/",
		Security: cephv1.ClusterSecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{
			ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault", "VAULT_ADDR": "http://vault:8200"},
			TokenSecretName:   "vault-token",
		}},
	}
	c := New(&clusterd.Context{Clientset: fake.NewClientset(), ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, clusterInfo, spec, "rook/rook:myversion")
	osdProps := osdProperties{crushHostname: "node1", storeConfig: config.StoreConfig{EncryptedDevice: true}}
	provision := &provisionConfig{DataPathMap: opconfig.NewDatalessDaemonDataPathMap("ns", "/var/lib/rook")}
	mapper := "/dev/mapper/ceph-9d1b4c0e-6f7a-4f53-9a8e-1f2b3c4d5e6f-sdb-block-dmcrypt"
	osd := &OSDInfo{ID: 0, UUID: "9d1b4c0e-6f7a-4f53-9a8e-1f2b3c4d5e6f", CVMode: "raw", Encrypted: true, BlockPath: mapper}

	deployment, err := c.makeDeployment(osdProps, osd, provision)
	assert.NoError(t, err)
	initContainers := deployment.Spec.Template.Spec.InitContainers
	open := initContainers[0]
	assert.Equal(t, hostEncryptionOpenInitContainer, open.Name)
	assert.Equal(t, "activate", initContainers[1].Name)
	verifyEnvVar(t, open.Env, "ROOK_OSD_UUID", osd.UUID, true)
	verifyEnvVar(t, open.Env, "ROOK_BLOCK_PATH", mapper, true)
	verifyEnvVar(t, open.Env, "KMS_PROVIDER", "vault", true)
	assert.Contains(t, open.VolumeMounts, v1.VolumeMount{Name: "devices", MountPath: "/dev"})
	assert.Contains(t, open.VolumeMounts, v1.VolumeMount{Name: hostEncryptionKeyVolumeName, MountPath: hostEncryptionKeyDir})
	// the key is fetched at runtime, never put in the pod spec
	for _, env := range open.Env {
		assert.NotEqual(t, CephVolumeEncryptedKeyEnvVarName, env.Name)
	}
	// Vault without TLS gets no TLS volume
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		assert.NotEqual(t, "vault", volume.Name)
	}

	// lvm mode OSDs keep their keys in the mons and are opened by ceph-volume
	osd = &OSDInfo{ID: 1, CVMode: "lvm", Encrypted: true}
	deployment, err = c.makeDeployment(osdProps, osd, provision)
	assert.NoError(t, err)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[0].Name)
}
