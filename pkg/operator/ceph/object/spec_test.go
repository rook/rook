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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

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

	c := &config{store: store, rookVersion: "rook/rook:myversion", cephVersion: cephv1.CephVersionSpec{Image: "ceph/ceph:v13.2.1"}, hostNetwork: true}
	s := c.makeRGWPodSpec()
	assert.NotNil(t, s)
	//assert.Equal(t, instanceName(store), s.Name)
	assert.Equal(t, v1.RestartPolicyAlways, s.Spec.RestartPolicy)
	assert.Equal(t, 3, len(s.Spec.Volumes))
	assert.Equal(t, "rook-data", s.Spec.Volumes[0].Name)
	assert.Equal(t, cephconfig.DefaultConfigMountName, s.Spec.Volumes[1].Name)

	assert.Equal(t, c.instanceName(), s.ObjectMeta.Name)
	assert.Equal(t, appName, s.ObjectMeta.Labels["app"])
	assert.Equal(t, store.Namespace, s.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, store.Name, s.ObjectMeta.Labels["rook_object_store"])
	assert.Equal(t, 0, len(s.ObjectMeta.Annotations))

	cont := s.Spec.InitContainers[0]
	assert.Equal(t, 1, len(s.Spec.InitContainers))
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 3, len(cont.VolumeMounts))
	assert.Equal(t, 0, len(cont.Command))
	assert.Equal(t, 6, len(cont.Args))
	assert.Equal(t, "ceph", cont.Args[0])
	assert.Equal(t, "rgw", cont.Args[1])
	assert.Equal(t, "--config-dir=/var/lib/rook", cont.Args[2])
	assert.Equal(t, "--rgw-name=default", cont.Args[3])
	assert.Equal(t, "--rgw-port=123", cont.Args[4])
	assert.Equal(t, "--rgw-secure-port=0", cont.Args[5])

	cont = s.Spec.Containers[0]
	assert.Equal(t, "ceph/ceph:v13.2.1", cont.Image)
	assert.Equal(t, 2, len(cont.VolumeMounts))

	assert.Equal(t, 1, len(cont.Command))
	assert.Equal(t, "radosgw", cont.Command[0])
	assert.Equal(t, 3, len(cont.Args))
	assert.Equal(t, "--foreground", cont.Args[0])
	assert.Equal(t, "--name=client.radosgw.gateway", cont.Args[1])
	assert.Equal(t, "--rgw-mime-types-file=/var/lib/rook/rgw/mime.types", cont.Args[2])

	assert.Equal(t, len(k8sutil.ClusterDaemonEnvVars()), len(cont.Env))

	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())
}

func TestSSLPodSpec(t *testing.T) {
	store := simpleStore()
	store.Spec.Gateway.SSLCertificateRef = "mycert"
	store.Spec.Gateway.SecurePort = 443

	c := &config{store: store, rookVersion: "v1.0", hostNetwork: true}
	s := c.makeRGWPodSpec()

	assert.NotNil(t, s)
	assert.Equal(t, c.instanceName(), s.Name)
	assert.Equal(t, 4, len(s.Spec.Volumes))
	assert.Equal(t, certVolumeName, s.Spec.Volumes[3].Name)
	assert.True(t, s.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, s.Spec.DNSPolicy)

	initCont := s.Spec.InitContainers[0]

	assert.Equal(t, 3, len(initCont.VolumeMounts))
	assert.Equal(t, 7, len(initCont.Args))
	assert.Equal(t, fmt.Sprintf("--rgw-secure-port=%d", 443), initCont.Args[5])
	assert.Equal(t, fmt.Sprintf("--rgw-cert=%s/%s", certMountPath, certFilename), initCont.Args[6])

	rgwCont := s.Spec.Containers[0]

	assert.Equal(t, 3, len(rgwCont.VolumeMounts))
	assert.Equal(t, certVolumeName, rgwCont.VolumeMounts[2].Name)
	assert.Equal(t, certMountPath, rgwCont.VolumeMounts[2].MountPath)
	assert.Equal(t, 3, len(rgwCont.Args))
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
