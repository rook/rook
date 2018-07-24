/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"fmt"
	"reflect"
	"sort"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type cluster struct {
	context   *clusterd.Context
	Namespace string
	Spec      *cephv1beta1.ClusterSpec
	mons      *mon.Cluster
	mgrs      *mgr.Cluster
	osds      *osd.Cluster
	stopCh    chan struct{}
	ownerRef  metav1.OwnerReference
}

func newCluster(c *cephv1beta1.Cluster, context *clusterd.Context) *cluster {
	return &cluster{Namespace: c.Namespace, Spec: &c.Spec, context: context,
		stopCh:   make(chan struct{}),
		ownerRef: ClusterOwnerRef(c.Namespace, string(c.UID))}
}

func (c *cluster) createInstance(rookImage string) error {

	// Create a configmap for overriding ceph config settings
	// These settings should only be modified by a user after they are initialized
	placeholderConfig := map[string]string{
		k8sutil.ConfigOverrideVal: "",
	}
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8sutil.ConfigOverrideName,
		},
		Data: placeholderConfig,
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &cm.ObjectMeta, &c.ownerRef)

	_, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create override configmap %s. %+v", c.Namespace, err)
	}

	// Start the mon pods
	c.mons = mon.New(c.context, c.Namespace, c.Spec.DataDirHostPath, rookImage, c.Spec.Mon, cephv1beta1.GetMonPlacement(c.Spec.Placement),
		c.Spec.Network.HostNetwork, cephv1beta1.GetMonResources(c.Spec.Resources), c.ownerRef)
	err = c.mons.Start()
	if err != nil {
		return fmt.Errorf("failed to start the mons. %+v", err)
	}

	err = c.createInitialCrushMap()
	if err != nil {
		return fmt.Errorf("failed to create initial crushmap: %+v", err)
	}

	c.mgrs = mgr.New(c.context, c.Namespace, rookImage, cephv1beta1.GetMgrPlacement(c.Spec.Placement),
		c.Spec.Network.HostNetwork, c.Spec.Dashboard, cephv1beta1.GetMgrResources(c.Spec.Resources), c.ownerRef)
	err = c.mgrs.Start()
	if err != nil {
		return fmt.Errorf("failed to start the ceph mgr. %+v", err)
	}

	// Start the OSDs
	c.osds = osd.New(c.context, c.Namespace, rookImage, c.Spec.ServiceAccount, c.Spec.Storage, c.Spec.DataDirHostPath,
		cephv1beta1.GetOSDPlacement(c.Spec.Placement), c.Spec.Network.HostNetwork, cephv1beta1.GetOSDResources(c.Spec.Resources), c.ownerRef)
	err = c.osds.Start()
	if err != nil {
		return fmt.Errorf("failed to start the osds. %+v", err)
	}

	logger.Infof("Done creating rook instance in namespace %s", c.Namespace)
	return nil
}

func (c *cluster) createInitialCrushMap() error {
	configMapExists := false
	createCrushMap := false

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(crushConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// crush config map was not found, meaning we haven't created the initial crush map
		createCrushMap = true
	} else {
		// crush config map was found, look in it to verify we've created the initial crush map
		configMapExists = true
		val, ok := cm.Data[crushmapCreatedKey]
		if !ok {
			createCrushMap = true
		} else if val != "1" {
			createCrushMap = true
		}
	}

	if !createCrushMap {
		// no need to create the crushmap, bail out
		return nil
	}

	logger.Info("creating initial crushmap")
	out, err := client.CreateDefaultCrushMap(c.context, c.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create initial crushmap: %+v. output: %s", err, out)
	}

	logger.Info("created initial crushmap")

	// save the fact that we've created the initial crushmap to a configmap
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crushConfigMapName,
			Namespace: c.Namespace,
		},
		Data: map[string]string{crushmapCreatedKey: "1"},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &configMap.ObjectMeta, &c.ownerRef)

	if !configMapExists {
		if _, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(configMap); err != nil {
			return fmt.Errorf("failed to create configmap %s: %+v", crushConfigMapName, err)
		}
	} else {
		if _, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Update(configMap); err != nil {
			return fmt.Errorf("failed to update configmap %s: %+v", crushConfigMapName, err)
		}
	}

	return nil
}

func clusterChanged(oldCluster, newCluster cephv1beta1.ClusterSpec) bool {
	changeFound := false
	oldStorage := oldCluster.Storage
	newStorage := newCluster.Storage

	// sort the nodes by name then compare to see if there are changes
	sort.Sort(rookv1alpha2.NodesByName(oldStorage.Nodes))
	sort.Sort(rookv1alpha2.NodesByName(newStorage.Nodes))
	if !reflect.DeepEqual(oldStorage.Nodes, newStorage.Nodes) {
		logger.Infof("The list of nodes has changed")
		changeFound = true
	}

	if oldCluster.Dashboard.Enabled != newCluster.Dashboard.Enabled {
		logger.Infof("dashboard enabled has changed from %t to %t", oldCluster.Dashboard.Enabled, newCluster.Dashboard.Enabled)
		changeFound = true
	}

	return changeFound
}
