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
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rbd-mirror")

const (
	// AppName is the ceph rbd mirror  application name
	AppName = "rook-ceph-rbd-mirror"
	// minimum amount of memory in MB to run the pod
	cephRbdMirrorPodMinimumMemory uint64 = 512
)

// Mirroring represents the Rook and environment configuration settings needed to set up rbd mirroring.
type Mirroring struct {
	ClusterInfo       *cephconfig.ClusterInfo
	Namespace         string
	placement         rookalpha.Placement
	annotations       rookalpha.Annotations
	context           *clusterd.Context
	resources         v1.ResourceRequirements
	priorityClassName string
	ownerRef          metav1.OwnerReference
	spec              cephv1.RBDMirroringSpec
	cephVersion       cephv1.CephVersionSpec
	rookVersion       string
	Network           cephv1.NetworkSpec
	dataDirHostPath   string
	isUpgrade         bool
	skipUpgradeChecks bool
}

// New creates an instance of the rbd mirroring
func New(
	cluster *cephconfig.ClusterInfo,
	context *clusterd.Context,
	namespace, rookVersion string,
	cephVersion cephv1.CephVersionSpec,
	placement rookalpha.Placement,
	annotations rookalpha.Annotations,
	network cephv1.NetworkSpec,
	spec cephv1.RBDMirroringSpec,
	resources v1.ResourceRequirements,
	priorityClassName string,
	ownerRef metav1.OwnerReference,
	dataDirHostPath string,
	isUpgrade bool,
	skipUpgradeChecks bool,
) *Mirroring {
	return &Mirroring{
		ClusterInfo:       cluster,
		context:           context,
		Namespace:         namespace,
		placement:         placement,
		annotations:       annotations,
		rookVersion:       rookVersion,
		cephVersion:       cephVersion,
		spec:              spec,
		Network:           network,
		resources:         resources,
		priorityClassName: priorityClassName,
		ownerRef:          ownerRef,
		dataDirHostPath:   dataDirHostPath,
		isUpgrade:         isUpgrade,
		skipUpgradeChecks: skipUpgradeChecks,
	}
}

var updateDeploymentAndWait = mon.UpdateCephDeploymentAndWait

// Start begins the process of running rbd mirroring daemons.
func (m *Mirroring) Start() error {
	// Validate pod's memory if specified
	err := opspec.CheckPodMemory(m.resources, cephRbdMirrorPodMinimumMemory)
	if err != nil {
		return errors.Wrap(err, "error checking pod memory")
	}

	logger.Infof("configure rbd-mirroring with %d workers", m.spec.Workers)

	for i := 0; i < m.spec.Workers; i++ {
		daemonID := k8sutil.IndexToName(i)
		resourceName := fmt.Sprintf("%s-%s", AppName, daemonID)
		daemonConf := &daemonConfig{
			DaemonID:     daemonID,
			ResourceName: resourceName,
			DataPathMap:  config.NewDatalessDaemonDataPathMap(m.Namespace, m.dataDirHostPath),
		}

		keyring, err := m.generateKeyring(daemonConf)
		if err != nil {
			return errors.Wrapf(err, "failed to generate keyring for %q", resourceName)
		}

		// Start the deployment
		d := m.makeDeployment(daemonConf)
		if _, err := m.context.Clientset.AppsV1().Deployments(m.Namespace).Create(d); err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create %s deployment", resourceName)
			}
			logger.Infof("deployment for rbd-mirror %s already exists. updating if needed", resourceName)

			// Always invoke ceph version before an upgrade so we are sure to be up-to-date
			daemon := string(config.RbdMirrorType)
			var cephVersionToUse cephver.CephVersion

			// If this is not a Ceph upgrade there is no need to check the ceph version
			if m.isUpgrade {
				currentCephVersion, err := client.LeastUptodateDaemonVersion(m.context, m.ClusterInfo.Name, daemon)
				if err != nil {
					logger.Warningf("failed to retrieve current ceph %q version. %v", daemon, err)
					logger.Debug("could not detect ceph version during update, this is likely an initial bootstrap, proceeding with m.ClusterInfo.CephVersion")
					cephVersionToUse = m.ClusterInfo.CephVersion

				} else {
					logger.Debugf("current cluster version for rbd mirrors before upgrading is: %+v", currentCephVersion)
					cephVersionToUse = currentCephVersion
				}
			}

			if err := updateDeploymentAndWait(m.context, d, m.Namespace, daemon, daemonConf.DaemonID, cephVersionToUse, m.isUpgrade, m.skipUpgradeChecks, false); err != nil {
				// fail could be an issue updating label selector (immutable), so try del and recreate
				logger.Debugf("updateDeploymentAndWait failed for rbd-mirror %q. Attempting del-and-recreate. %v", resourceName, err)
				err = m.context.Clientset.AppsV1().Deployments(m.Namespace).Delete(d.Name, &metav1.DeleteOptions{})
				if err != nil {
					return errors.Wrapf(err, "failed to delete rbd-mirror %s during del-and-recreate update attempt", resourceName)
				}
				if _, err := m.context.Clientset.AppsV1().Deployments(m.Namespace).Create(d); err != nil {
					return errors.Wrapf(err, "failed to recreate rbd-mirror deployment %s during del-and-recreate update attempt", resourceName)
				}
			}
		}

		if existingDeployment, err := m.context.Clientset.AppsV1().Deployments(m.Namespace).Get(d.GetName(), metav1.GetOptions{}); err != nil {
			logger.Warningf("failed to find rbd-mirror deployment %q for keyring association. %v", resourceName, err)
		} else {
			if err = m.associateKeyring(keyring, existingDeployment); err != nil {
				logger.Warningf("failed to associate keyring with rbd-mirror deployment %q. %v", resourceName, err)
			}
		}
		logger.Infof("%s deployment started", resourceName)
	}

	// Remove extra rbd-mirror deployments if necessary
	err = m.removeExtraMirrors()
	if err != nil {
		logger.Errorf("failed to remove extra mirrors. %v", err)
	}

	return nil
}

func (m *Mirroring) removeExtraMirrors() error {
	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", AppName)}
	d, err := m.context.Clientset.AppsV1().Deployments(m.Namespace).List(opts)
	if err != nil {
		return errors.Wrapf(err, "failed to get mirrors")
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
				logger.Warningf("failed to delete rbd-mirror %q. %v", daemonName, err)
			}

			logger.Infof("removed rbd-mirror %s", daemonName)
		}
	}
	return nil
}
