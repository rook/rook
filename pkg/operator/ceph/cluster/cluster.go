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
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/rbd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	detectVersionName = "rook-ceph-detect-version"
)

type cluster struct {
	Info                 *cephconfig.ClusterInfo
	context              *clusterd.Context
	Namespace            string
	Spec                 *cephv1.ClusterSpec
	mons                 *mon.Cluster
	stopCh               chan struct{}
	ownerRef             metav1.OwnerReference
	orchestrationRunning bool
	orchestrationPending bool
	orchRunMux           sync.Mutex
	orchPenMux           sync.Mutex
}

func newCluster(c *cephv1.CephCluster, context *clusterd.Context) *cluster {
	ownerRef := ClusterOwnerRef(c.Namespace, string(c.UID))
	return &cluster{
		// at this phase of the cluster creation process, the identity components of the cluster are
		// not yet established. we reserve this struct which is filled in as soon as the cluster's
		// identity can be established.
		Info:      nil,
		Namespace: c.Namespace,
		Spec:      &c.Spec,
		context:   context,
		stopCh:    make(chan struct{}),
		ownerRef:  ownerRef,
		mons:      mon.New(context, c.Namespace, c.Spec.DataDirHostPath, c.Spec.Network.HostNetwork, ownerRef),
	}
}

func (c *cluster) detectCephVersion(image string, timeout time.Duration) (*cephver.CephVersion, error) {
	// get the major ceph version by running "ceph --version" in the ceph image
	podSpec := v1.PodSpec{
		Containers: []v1.Container{
			{
				Command: []string{"ceph"},
				Args:    []string{"--version"},
				Name:    "version",
				Image:   image,
			},
		},
		RestartPolicy: v1.RestartPolicyOnFailure,
	}

	// apply "mon" placement
	cephv1.GetMonPlacement(c.Spec.Placement).ApplyToPodSpec(&podSpec)

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
				Spec: podSpec,
			},
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &job.ObjectMeta, &c.ownerRef)

	// run the job to detect the version
	if err := k8sutil.RunReplaceableJob(c.context.Clientset, job); err != nil {
		return nil, fmt.Errorf("failed to start version job. %+v", err)
	}

	if err := k8sutil.WaitForJobCompletion(c.context.Clientset, job, timeout); err != nil {
		return nil, fmt.Errorf("failed to complete version job. %+v", err)
	}

	log, err := k8sutil.GetPodLog(c.context.Clientset, c.Namespace, "job="+detectVersionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get version job log to detect version. %+v", err)
	}

	version, err := cephver.ExtractCephVersion(log)
	if err != nil {
		return nil, fmt.Errorf("failed to extract ceph version. %+v", err)
	}

	// delete the job since we're done with it
	k8sutil.DeleteBatchJob(c.context.Clientset, c.Namespace, job.Name, false)

	logger.Infof("Detected ceph image version: %s", version)
	return version, nil
}

func (c *cluster) createInstance(rookImage string, cephVersion cephver.CephVersion) error {
	if c.checkSetOrchestrationRunning() == true {
		logger.Debugf("As createInstance is currently running added this request as pending.")
		c.setOrchestrationPending()
		return nil
	}

	defer c.unsetOrchestrationRunning()
	initRun := true
	for c.checkUnsetOrchestrationPending() == true || initRun == true {
		initRun = false
		// Use a DeepCopy of the spec to avoid using an inconsistent data-set
		spec := c.Spec.DeepCopy()

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
		clusterInfo, err := c.mons.Start(c.Info, rookImage, cephVersion, *c.Spec)
		if err != nil {
			return fmt.Errorf("failed to start the mons. %+v", err)
		}
		c.Info = clusterInfo // mons return the cluster's info

		// The cluster Identity must be established at this point
		if !c.Info.IsInitialized() {
			return fmt.Errorf("the cluster identity was not established: %+v", c.Info)
		}

		err = c.createInitialCrushMap()
		if err != nil {
			return fmt.Errorf("failed to create initial crushmap: %+v", err)
		}

		mgrs := mgr.New(c.Info, c.context, c.Namespace, rookImage,
			spec.CephVersion, cephv1.GetMgrPlacement(spec.Placement), spec.Network.HostNetwork,
			spec.Dashboard, cephv1.GetMgrResources(spec.Resources), c.ownerRef)
		err = mgrs.Start()
		if err != nil {
			return fmt.Errorf("failed to start the ceph mgr. %+v", err)
		}

		// Start the OSDs
		osds := osd.New(c.context, c.Namespace, rookImage, spec.CephVersion, spec.Storage, spec.DataDirHostPath,
			cephv1.GetOSDPlacement(spec.Placement), spec.Network.HostNetwork, cephv1.GetOSDResources(spec.Resources), c.ownerRef)
		err = osds.Start()
		if err != nil {
			return fmt.Errorf("failed to start the osds. %+v", err)
		}

		// Start the rbd mirroring daemon(s)
		rbdmirror := rbd.New(c.Info, c.context, c.Namespace, rookImage, spec.CephVersion, cephv1.GetRBDMirrorPlacement(spec.Placement),
			spec.Network.HostNetwork, spec.RBDMirroring, cephv1.GetRBDMirrorResources(spec.Resources), c.ownerRef)
		err = rbdmirror.Start()
		if err != nil {
			return fmt.Errorf("failed to start the rbd mirrors. %+v", err)
		}

		logger.Infof("Done creating rook instance in namespace %s", c.Namespace)
	}

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

func clusterChanged(oldCluster, newCluster cephv1.ClusterSpec, clusterRef *cluster) (bool, string) {

	// sort the nodes by name then compare to see if there are changes
	sort.Sort(rookv1alpha2.NodesByName(oldCluster.Storage.Nodes))
	sort.Sort(rookv1alpha2.NodesByName(newCluster.Storage.Nodes))

	// any change in the crd will trigger an orchestration
	if !reflect.DeepEqual(oldCluster, newCluster) {
		diff := cmp.Diff(oldCluster, newCluster)
		logger.Infof("The Cluster CRD has changed. diff=%s", diff)
		return true, diff
	}

	return false, ""
}

func (c *cluster) unsetOrchestrationRunning() {
	c.orchRunMux.Lock()
	c.orchestrationRunning = false
	c.orchRunMux.Unlock()
}

func (c *cluster) setOrchestrationPending() {
	c.orchPenMux.Lock()
	c.orchestrationPending = true
	c.orchPenMux.Unlock()
}

func (c *cluster) getOrchestrationRunning() bool {
	c.orchRunMux.Lock()
	defer c.orchRunMux.Unlock()
	return c.orchestrationRunning
}

func (c *cluster) getOrchestrationPending() bool {
	c.orchPenMux.Lock()
	defer c.orchPenMux.Unlock()
	return c.orchestrationPending
}

// make checking and setting orchestrationRunning "atomic"
func (c *cluster) checkSetOrchestrationRunning() bool {
	defer c.orchRunMux.Unlock()
	c.orchRunMux.Lock()
	if c.orchestrationRunning == false {
		c.orchestrationRunning = true
		return false
	}
	return true
}

// make checking and unsetting orchestrationPending "atomic"
func (c *cluster) checkUnsetOrchestrationPending() bool {
	defer c.orchPenMux.Unlock()
	c.orchPenMux.Lock()
	if c.orchestrationPending == true {
		c.orchestrationPending = false
		return true
	}
	return false
}
