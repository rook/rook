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

package newosd

import (
	"fmt"
	"time"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	osdconfig "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-osd")

const (
	serviceAccountName = "rook-ceph-osd"

	osdAppName    = "rook-ceph-osd"
	osdAppNameFmt = "rook-ceph-osd-%d"

	inventoryAppName    = "rook-ceph-osd-inventory"
	inventoryAppNameFmt = "rook-ceph-osd-inventory-%s"
	osdInventoryTimeout = 10 * time.Minute // TODO: is this a good value?

	clusterAvailableSpaceReserve = 0.05
	unknownID                    = -1
)

// Controller keeps track of the OSDs
type Controller struct {
	context     *clusterd.Context
	namespace   string
	cephCluster *cephCluster
	rookImage   string
	ownerRef    metav1.OwnerReference
}

type cephCluster struct {
	spec        *cephv1.ClusterSpec
	storeConfig *osdconfig.StoreConfig
	info        *cephconfig.ClusterInfo
}

// NewController creates a new controller for configuring and running Ceph OSDs.
func NewController(
	context *clusterd.Context,
	namespace string,
	cephSpec *cephv1.ClusterSpec,
	cephInfo *cephconfig.ClusterInfo,
	rookImage string,
	ownerRef metav1.OwnerReference,
) *Controller {
	storeConfig := osdconfig.ToStoreConfig(cephSpec.Storage.Config)
	return &Controller{
		context:   context,
		namespace: namespace,
		cephCluster: &cephCluster{
			spec:        cephSpec,
			storeConfig: &storeConfig,
			info:        cephInfo,
		},
		rookImage: rookImage,
		ownerRef:  ownerRef,
	}
}

// Start the osd management
func (c *Controller) Start() error {
	logger.Infof("OSD controller started for namespace %s", c.namespace)

	// Validate pod's memory if specified
	r := cephv1.GetOSDResources(c.cephCluster.spec.Resources)
	err := opspec.CheckPodMemory(r, cephOsdPodMinimumMemory)
	if err != nil {
		return fmt.Errorf("user-requested OSD pod resource requirements are insufficient. %+v", err)
	}

	desiredStorage, err := c.getDesiredStorage()
	if err != nil {
		return fmt.Errorf("OSD controller failed. %+v", err)
	}
	logger.Debugf("desiredStorage: %+v", desiredStorage)

	provisionableNodes := c.getProvisionableNodes(desiredStorage)

	provisionableNodeCh := make(chan rookalpha.Node, len(provisionableNodes))
	go func() {
		defer close(provisionableNodeCh)
		for _, n := range provisionableNodes {
			provisionableNodeCh <- n
		}
	}()

	inventoriedNodeCh := make(chan inventoriedNode, len(provisionableNodes))
	go c.inventoryNodes(provisionableNodeCh, inventoriedNodeCh)

	for in := range inventoriedNodeCh {
		logger.Debugf("inventoried node: %+v", in)
	}

	// TODO: because ceph-volume only reports disks in /dev/ (i.e., not /dev/disk/by-X/... paths),
	// inventory is invalidated if the node reboots after the inventory is taken, because those
	// paths aren't guaranteed to be consistent across reboots. How do we resolve this?

	// TODO: provision new OSDs on provisionable nodes with provisionable disks

	// TODO: should we whitelist or blacklist disks? I get some `/dev/dm-N` disks reported back as
	// available, but that is not true, as they are associated with Ceph disks

	// TODO: find nodes already provisioned with OSDs ???

	// TODO: get osd inventory from nodes that are newly provisionable and nodes which are already
	// provisioned with osds

	// TODO: start OSDs on the OSD inventory from above

	// TODO: what does automatic osd removal look like now?

	// if len(config.errorMessages) > 0 {
	// 	return fmt.Errorf("%d failures encountered while running osds in namespace %s: %+v",
	// 		len(config.errorMessages), c.Namespace, strings.Join(config.errorMessages, "\n"))
	// }

	logger.Infof("finished running OSDs in namespace %s", c.namespace)
	return nil
}
