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
	"strings"
	"time"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	detectVersionName = "rook-ceph-detect-version"
)

var (
	// supportedVersions are production-ready versions that rook supports
	supportedVersions = []string{cephv1beta1.Luminous, cephv1beta1.Mimic}
	// allVersions includes all supportedVersions as well as unreleased versions that are being tested with rook
	allVersions = append(supportedVersions, cephv1beta1.Nautilus)
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

func (c *cluster) setCephMajorVersion(timeout time.Duration) error {
	version, err := c.detectCephMajorVersion(timeout)
	if err != nil {
		// Don't return the err yet here so we can override the failure with a setting in the crd below.
		logger.Errorf("failed to detect ceph version. %+v", err)
	} else {
		logger.Infof("detected ceph version %s for image %s", version, c.Spec.CephVersion.Image)
	}

	if c.Spec.CephVersion.Name != "" {
		// if the cephVersion.name is already set, override the detected version
		logger.Warningf("overriding ceph version to %s specified in the CRD", c.Spec.CephVersion.Name)
		return nil
	}

	c.Spec.CephVersion.Name = version
	return err
}

func (c *cluster) detectCephMajorVersion(timeout time.Duration) (string, error) {
	// get the major ceph version by running "ceph --version" in the ceph image
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      detectVersionName,
			Namespace: c.Namespace,
		},
		Spec: batch.JobSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"job": detectVersionName,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Command: []string{"ceph"},
							Args:    []string{"--version"},
							Name:    "version",
							Image:   c.Spec.CephVersion.Image,
						},
					},
					RestartPolicy: v1.RestartPolicyOnFailure,
				},
			},
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &job.ObjectMeta, &c.ownerRef)

	// run the job to detect the version
	if err := k8sutil.RunReplaceableJob(c.context.Clientset, job); err != nil {
		return "", fmt.Errorf("failed to start version job. %+v", err)
	}

	if err := k8sutil.WaitForJobCompletion(c.context.Clientset, job, timeout); err != nil {
		return "", fmt.Errorf("failed to complete version job. %+v", err)
	}

	log, err := k8sutil.GetPodLog(c.context.Clientset, c.Namespace, "job="+detectVersionName)
	if err != nil {
		return "", fmt.Errorf("failed to get version job log to detect version. %+v", err)
	}

	version, err := extractCephVersion(log)
	if err != nil {
		return "", fmt.Errorf("failed to extract ceph version. %+v", err)
	}

	// delete the job since we're done with it
	k8sutil.DeleteBatchJob(c.context.Clientset, c.Namespace, job.Name, false)
	return version, nil
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
	c.mons = mon.New(c.context, c.Namespace, c.Spec.DataDirHostPath, rookImage, c.Spec.CephVersion, c.Spec.Mon, cephv1beta1.GetMonPlacement(c.Spec.Placement),
		c.Spec.Network.HostNetwork, cephv1beta1.GetMonResources(c.Spec.Resources), c.ownerRef)
	err = c.mons.Start()
	if err != nil {
		return fmt.Errorf("failed to start the mons. %+v", err)
	}

	err = c.createInitialCrushMap()
	if err != nil {
		return fmt.Errorf("failed to create initial crushmap: %+v", err)
	}

	c.mgrs = mgr.New(c.context, c.Namespace, rookImage, c.Spec.CephVersion, cephv1beta1.GetMgrPlacement(c.Spec.Placement),
		c.Spec.Network.HostNetwork, c.Spec.Dashboard, cephv1beta1.GetMgrResources(c.Spec.Resources), c.ownerRef)
	err = c.mgrs.Start()
	if err != nil {
		return fmt.Errorf("failed to start the ceph mgr. %+v", err)
	}

	// Start the OSDs
	c.osds = osd.New(c.context, c.Namespace, rookImage, c.Spec.CephVersion, c.Spec.ServiceAccount, c.Spec.Storage, c.Spec.DataDirHostPath,
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

func clusterChanged(oldCluster, newCluster cephv1beta1.ClusterSpec, clusterRef *cluster) bool {
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
	if oldCluster.Dashboard.UrlPrefix != newCluster.Dashboard.UrlPrefix {
		logger.Infof("dashboard url prefix has changed from \"%s\" to \"%s\"", oldCluster.Dashboard.UrlPrefix, newCluster.Dashboard.UrlPrefix)
		changeFound = true
	}

	if oldCluster.Mon.Count != newCluster.Mon.Count {
		logger.Infof("number of mons have changed from %d to %d. The health check will update the mons...", oldCluster.Mon.Count, newCluster.Mon.Count)
		clusterRef.mons.MonCountMutex.Lock()
		clusterRef.mons.Count = newCluster.Mon.Count
		clusterRef.mons.MonCountMutex.Unlock()
	}

	if oldCluster.Mon.AllowMultiplePerNode != newCluster.Mon.AllowMultiplePerNode {
		logger.Infof("allow multiple mons per node changed from %t to %t. The health check will update the mons...", oldCluster.Mon.AllowMultiplePerNode, newCluster.Mon.AllowMultiplePerNode)
		clusterRef.mons.MonCountMutex.Lock()
		clusterRef.mons.AllowMultiplePerNode = newCluster.Mon.AllowMultiplePerNode
		clusterRef.mons.MonCountMutex.Unlock()
	}

	if oldCluster.CephVersion.AllowUnsupported != newCluster.CephVersion.AllowUnsupported {
		logger.Infof("ceph version allowUnsupported has changed from %t to %t", oldCluster.CephVersion.AllowUnsupported, newCluster.CephVersion.AllowUnsupported)
		changeFound = true
	}

	return changeFound
}

func extractCephVersion(version string) (string, error) {
	for _, v := range allVersions {
		if strings.Contains(version, v) {
			return v, nil
		}
	}
	return "", fmt.Errorf("failed to parse version from: %s", version)
}

func versionSupported(version string) bool {
	for _, v := range supportedVersions {
		if v == version {
			return true
		}
	}
	return false
}
