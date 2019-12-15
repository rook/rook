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
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodSpecs(t *testing.T) {
	store := simpleStore()
	store.Spec.Gateway.Resources = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(500.0, resource.BinarySI),
		},
	}
	store.Spec.Gateway.PriorityClassName = "my-priority-class"
	info := testop.CreateConfigDir(1)
	info.CephVersion = cephver.Mimic
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")

	c := &clusterConfig{
		clusterInfo: info,
		store:       store,
		rookVersion: "rook/rook:myversion",
		clusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "ceph/ceph:v13.2.1"},
			Network: cephv1.NetworkSpec{
				HostNetwork: true,
			},
		},
		DataPathMap: data,
	}

	resourceName := fmt.Sprintf("%s-%s", AppName, c.store.Name)
	rgwConfig := &rgwConfig{
		ResourceName: resourceName,
	}

	s := c.makeRGWPodSpec(rgwConfig)

	// Check pod anti affinity is well added to be compliant with HostNetwork setting
	assert.Equal(t,
		1,
		len(s.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution))
	assert.Equal(t,
		c.getLabels(),
		s.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].LabelSelector.MatchLabels)

	podTemplate := cephtest.NewPodTemplateSpecTester(t, &s)
	podTemplate.RunFullSuite(cephconfig.RgwType, "default", "rook-ceph-rgw", "mycluster", "ceph/ceph:myversion",
		"200", "100", "1337", "500", /* resources */
		"my-priority-class")
}

func TestSSLPodSpec(t *testing.T) {
	store := simpleStore()
	store.Spec.Gateway.Resources = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(500.0, resource.BinarySI),
		},
	}
	store.Spec.Gateway.PriorityClassName = "my-priority-class"
	info := testop.CreateConfigDir(1)
	info.CephVersion = cephver.Mimic
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")
	store.Spec.Gateway.SSLCertificateRef = "mycert"
	store.Spec.Gateway.SecurePort = 443

	c := &clusterConfig{
		clusterInfo: info,
		store:       store,
		rookVersion: "rook/rook:myversion",
		clusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "ceph/ceph:v13.2.1"},
			Network: cephv1.NetworkSpec{
				HostNetwork: true,
			},
		},
		DataPathMap: data,
	}

	resourceName := fmt.Sprintf("%s-%s", AppName, c.store.Name)
	rgwConfig := &rgwConfig{
		ResourceName: resourceName,
	}
	s := c.makeRGWPodSpec(rgwConfig)

	podTemplate := cephtest.NewPodTemplateSpecTester(t, &s)
	podTemplate.RunFullSuite(cephconfig.RgwType, "default", "rook-ceph-rgw", "mycluster", "ceph/ceph:myversion",
		"200", "100", "1337", "500", /* resources */
		"my-priority-class")

	assert.True(t, s.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, s.Spec.DNSPolicy)

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

	// no replication or EC is valid
	s.Spec.MetadataPool.Replicated.Size = 0
	err = validateStore(context, s)
	assert.Nil(t, err)
	s.Spec.MetadataPool.Replicated.Size = 1
	err = validateStore(context, s)
	assert.Nil(t, err)
}

func testPodSpecPlacement(t *testing.T, hostNet bool, req int, placement *rook.Placement) {
	c := newConfig()
	c.clusterSpec = &cephv1.ClusterSpec{
		Network: cephv1.NetworkSpec{HostNetwork: hostNet},
	}

	d := v1.PodSpec{
		InitContainers:    []v1.Container{},
		Containers:        []v1.Container{},
		RestartPolicy:     v1.RestartPolicyAlways,
		HostNetwork:       hostNet,
		PriorityClassName: c.store.Spec.Gateway.PriorityClassName,
	}

	if placement != nil {
		c.store.Spec.Gateway.Placement = *placement
	}

	c.setPodPlacement(&d, c.store.Spec.Gateway.Placement)

	// should have a required anti-affnity
	assert.Equal(t,
		req,
		len(d.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution))
}

func makePlacement() rook.Placement {
	return rook.Placement{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					TopologyKey: v1.LabelZoneFailureDomain,
				},
			},
		},
	}
}

func TestPodSpecPlacement(t *testing.T) {
	// no placement settings in the crd
	testPodSpecPlacement(t, true, 1, nil)
	testPodSpecPlacement(t, false, 0, nil)

	// crd has other preferred and required anti-affinity setting
	p := makePlacement()
	testPodSpecPlacement(t, true, 2, &p)
	p = makePlacement()
	testPodSpecPlacement(t, false, 1, &p)
}
