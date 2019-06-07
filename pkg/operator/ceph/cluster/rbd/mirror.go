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

// Package rbd for mirroring
package rbd

import (
	"fmt"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rbd-mirror")

const (
	appName = "rook-ceph-rbd-mirror"
	// minimum amount of memory in MB to run the pod
	cephRbdMirrorPodMinimumMemory uint64 = 512
)

// Mirroring represents the Rook and environment configuration settings needed to set up rbd mirroring.
type Mirroring struct {
	ClusterInfo     *cephconfig.ClusterInfo
	Namespace       string
	placement       rookalpha.Placement
	annotations     rookalpha.Annotations
	context         *clusterd.Context
	resources       v1.ResourceRequirements
	ownerRef        metav1.OwnerReference
	spec            cephv1.RBDMirroringSpec
	cephVersion     cephv1.CephVersionSpec
	rookVersion     string
	hostNetwork     bool
	dataDirHostPath string
}

// New creates an instance of the rbd mirroring
func New(
	cluster *cephconfig.ClusterInfo,
	context *clusterd.Context,
	namespace, rookVersion string,
	cephVersion cephv1.CephVersionSpec,
	placement rookalpha.Placement,
	annotations rookalpha.Annotations,
	hostNetwork bool,
	spec cephv1.RBDMirroringSpec,
	resources v1.ResourceRequirements,
	ownerRef metav1.OwnerReference,
	dataDirHostPath string,
) *Mirroring {
	return &Mirroring{
		ClusterInfo:     cluster,
		context:         context,
		Namespace:       namespace,
		placement:       placement,
		annotations:     annotations,
		rookVersion:     rookVersion,
		cephVersion:     cephVersion,
		spec:            spec,
		hostNetwork:     hostNetwork,
		resources:       resources,
		ownerRef:        ownerRef,
		dataDirHostPath: dataDirHostPath,
	}
}

var updateDeploymentAndWait = k8sutil.UpdateDeploymentAndWait

// Start begins the process of running rbd mirroring daemons.
func (m *Mirroring) Start() error {
	// Validate pod's memory if specified
	err := opspec.CheckPodMemory(m.resources, cephRbdMirrorPodMinimumMemory)
	if err != nil {
		return fmt.Errorf("%+v", err)
	}

	logger.Infof("configure rbd-mirroring with %d workers", m.spec.Workers)

	for i := 0; i < m.spec.Workers; i++ {
		daemonID := k8sutil.IndexToName(i)
		resourceName := fmt.Sprintf("%s-%s", appName, daemonID)
		daemonConf := &daemonConfig{
			DaemonID:     daemonID,
			ResourceName: resourceName,
			DataPathMap:  config.NewDatalessDaemonDataPathMap(m.Namespace, m.dataDirHostPath),
		}

		if err := m.generateKeyring(daemonConf); err != nil {
			return fmt.Errorf("failed to generate keyring for %s. %+v", resourceName, err)
		}

		// Start the deployment
		d := m.makeDeployment(daemonConf)
		if _, err := m.context.Clientset.AppsV1().Deployments(m.Namespace).Create(d); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create %s deployment. %+v", resourceName, err)
			}
			logger.Infof("deployment for rbd-mirror %s already exists. updating if needed", resourceName)
			if _, err := updateDeploymentAndWait(m.context, d, m.Namespace); err != nil {
				// fail could be an issue updating label selector (immutable), so try del and recreate
				logger.Debugf("updateDeploymentAndWait failed for rbd-mirror %s. Attempting del-and-recreate. %+v", resourceName, err)
				err = m.context.Clientset.AppsV1().Deployments(m.Namespace).Delete(d.Name, &metav1.DeleteOptions{})
				if err != nil {
					return fmt.Errorf("failed to delete rbd-mirror %s during del-and-recreate update attempt. %+v", resourceName, err)
				}
				if _, err := m.context.Clientset.AppsV1().Deployments(m.Namespace).Create(d); err != nil {
					return fmt.Errorf("failed to recreate rbd-mirror deployment %s during del-and-recreate update attempt. %+v", resourceName, err)
				}
			}
		} else {
			logger.Infof("%s deployment started", resourceName)
		}
	}

	// Remove extra rbd-mirror deployments if necessary
	err = m.removeExtraMirrors()
	if err != nil {
		logger.Errorf("failed to remove extra mirrors. %+v", err)
	}

	return nil
}

func (m *Mirroring) removeExtraMirrors() error {
	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appName)}
	d, err := m.context.Clientset.AppsV1().Deployments(m.Namespace).List(opts)
	if err != nil {
		return fmt.Errorf("failed to get mirrors. %+v", err)
	}

	if len(d.Items) <= m.spec.Workers {
		logger.Infof("no extra daemons to remove")
		return nil
	}

	for _, deploy := range d.Items {
		daemonName, ok := deploy.Labels["rbdmirror"]
		if !ok {
			logger.Warningf("unrecognized rbdmirror %s", deploy.Name)
			continue
		}
		index, err := k8sutil.NameToIndex(daemonName)
		if err != nil {
			logger.Warningf("unrecognized rbdmirror %s with label %s", deploy.Name, daemonName)
			continue
		}
		if index >= m.spec.Workers {
			logger.Infof("removing extra rbd-mirror %s", daemonName)
			var gracePeriod int64
			propagation := metav1.DeletePropagationForeground
			deleteOpts := metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
			if err = m.context.Clientset.AppsV1().Deployments(m.Namespace).Delete(deploy.Name, &deleteOpts); err != nil {
				logger.Warningf("failed to delete rbd-mirror %s. %+v", daemonName, err)
			}

			logger.Infof("removed rbd-mirror %s", daemonName)
		}
	}
	return nil
}
