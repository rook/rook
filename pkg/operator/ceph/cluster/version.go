/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	daemonclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/util/log"
)

func (c *ClusterController) detectAndValidateCephVersion(cluster *cluster) (*cephver.CephVersion, bool, error) {
	version, err := controller.DetectCephVersion(
		c.OpManagerCtx,
		c.rookImage,
		cluster.Namespace,
		detectVersionName,
		cluster.ownerInfo,
		c.context.Clientset,
		cluster.Spec,
	)
	if err != nil {
		return nil, false, err
	}

	log.NamespacedInfo(cluster.Namespace, logger, "validating ceph version from provided image")
	if err := cluster.validateCephVersion(version); err != nil {
		return nil, cluster.isUpgrade, err
	}

	// Update ceph version field in cluster object status
	c.updateClusterCephVersion(cluster, *version)

	return version, cluster.isUpgrade, nil
}

func (c *cluster) printOverallCephVersion() {
	versions, err := daemonclient.GetAllCephDaemonVersions(c.context, c.ClusterInfo)
	if err != nil {
		log.NamespacedError(c.Namespace, logger, "failed to get ceph daemons versions. %v", err)
		return
	}

	if len(versions.Overall) == 1 {
		for v := range versions.Overall {
			version, err := cephver.ExtractCephVersion(v)
			if err != nil {
				log.NamespacedError(c.Namespace, logger, "failed to extract ceph version. %v", err)
				return
			}
			vv := *version
			log.NamespacedInfo(c.Namespace, logger, "successfully upgraded cluster to version: %q", vv.String())
		}
	} else {
		// This shouldn't happen, but let's log just in case
		log.NamespacedWarning(c.Namespace, logger, "upgrade orchestration completed but somehow we still have more than one Ceph version running. %v:", versions.Overall)
	}
}

// This function compare the Ceph spec image and the cluster running version
// It returns true if the image is different and false if identical
func (c *cluster) diffImageSpecAndClusterRunningVersion(imageSpecVersion cephver.CephVersion, runningVersions cephv1.CephDaemonsVersions) (bool, error) {
	numberOfCephVersions := len(runningVersions.Overall)
	if numberOfCephVersions == 0 {
		// let's return immediately
		return false, errors.Errorf("no 'overall' section in the ceph versions. %+v", runningVersions.Overall)
	}

	if numberOfCephVersions > 1 {
		// let's return immediately
		log.NamespacedWarning(c.Namespace, logger, "it looks like we have more than one ceph version running. triggering upgrade. %+v:", runningVersions.Overall)
		return true, nil
	}

	if numberOfCephVersions == 1 {
		for v := range runningVersions.Overall {
			version, err := cephver.ExtractCephVersion(v)
			if err != nil {
				log.NamespacedError(c.Namespace, logger, "failed to extract ceph version. %v", err)
				return false, err
			}
			clusterRunningVersion := *version

			// If this is the same version
			if cephver.IsIdentical(clusterRunningVersion, imageSpecVersion) {
				log.NamespacedDebug(c.Namespace, logger, "both cluster and image spec versions are identical, doing nothing %s", imageSpecVersion.String())
				return false, nil
			}

			if cephver.IsSuperior(imageSpecVersion, clusterRunningVersion) {
				log.NamespacedInfo(c.Namespace, logger, "image spec version %s is higher than the running cluster version %s, upgrading", imageSpecVersion.String(), clusterRunningVersion.String())
				return true, nil
			}

			if cephver.IsInferior(imageSpecVersion, clusterRunningVersion) {
				log.NamespacedWarning(c.Namespace, logger, "image spec version %s is lower than the running cluster version %s, downgrading is not supported", imageSpecVersion.String(), clusterRunningVersion.String())
				return true, nil
			}
		}
	}

	return false, nil
}

func (c *cluster) validateCephVersion(version *cephver.CephVersion) error {
	if !c.Spec.External.Enable {
		if !version.IsAtLeast(cephver.Minimum) {
			return errors.Errorf("the version does not meet the minimum version %q", cephver.Minimum.String())
		}

		if !version.Supported() {
			if !c.Spec.CephVersion.AllowUnsupported {
				return errors.Errorf("allowUnsupported must be set to true to run with this version %q", version.String())
			}
			log.NamespacedWarning(c.Namespace, logger, "unsupported ceph version detected: %q, pursuing", version)
		}

		if version.Unsupported() {
			log.NamespacedError(c.Namespace, logger, "UNSUPPORTED: ceph version %q detected, it is recommended to rollback to the previous pin-point stable release, pursuing anyways", version)
		}
	}

	// The following tries to determine if the operator can proceed with an upgrade because we come from an OnAdd() call
	// If the cluster was unhealthy and someone injected a new image version, an upgrade was triggered but failed because the cluster is not healthy
	// Then after this, if the operator gets restarted we are not able to fail if the cluster is not healthy, the following tries to determine the
	// state we are in and if we should upgrade or not

	// Try to load clusterInfo so we can compare the running version with the one from the spec image
	clusterInfo, _, _, err := controller.LoadClusterInfo(c.context, c.ClusterInfo.Context, c.Namespace, c.Spec)
	if err == nil {
		clusterInfo.Context = c.ClusterInfo.Context
		// Write connection info (ceph config file and keyring) for ceph commands
		err = mon.WriteConnectionConfig(c.context, clusterInfo)
		if err != nil {
			log.NamespacedError(c.Namespace, logger, "failed to write config. attempting to continue. %v", err)
		}
	}

	if err := clusterInfo.IsInitialized(); err != nil {
		// If not initialized, this is likely a new cluster so there is nothing to do
		log.NamespacedDebug(c.Namespace, logger, "cluster not initialized, nothing to validate. %v", err)
		return nil
	}

	clusterInfo.CephVersion = *version
	if c.Spec.External.Enable && c.Spec.CephVersion.Image != "" {
		c.ClusterInfo.CephVersion, err = controller.ValidateCephVersionsBetweenLocalAndExternalClusters(c.context, c.ClusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to validate ceph version between external and local")
		}
	}

	// On external cluster setup, if we don't bootstrap any resources in the Kubernetes cluster then
	// there is no need to validate the Ceph image further
	if c.Spec.External.Enable && c.Spec.CephVersion.Image == "" {
		log.NamespacedDebug(c.Namespace, logger, "no spec image specified on external cluster, not validating Ceph version.")
		return nil
	}

	// Get cluster running versions
	versions, err := daemonclient.GetAllCephDaemonVersions(c.context, c.ClusterInfo)
	if err != nil {
		log.NamespacedError(c.Namespace, logger, "failed to get ceph daemons versions, this typically happens during the first cluster initialization. %v", err)
		return nil
	}

	runningVersions := *versions
	differentImages, err := c.diffImageSpecAndClusterRunningVersion(*version, runningVersions)
	if err != nil {
		log.NamespacedError(c.Namespace, logger, "failed to determine if we should upgrade or not. %v", err)
		// we shouldn't block the orchestration if we can't determine the version of the image spec, we proceed anyway in best effort
		// we won't be able to check if there is an update or not and what to do, so we don't check the cluster status either
		// This will happen if someone uses ceph/daemon:latest-master for instance
		return nil
	}

	if differentImages {
		// If the image version changed let's make sure we can safely upgrade
		// check ceph's status, if not healthy we fail
		cephHealthy := daemonclient.IsCephHealthy(c.context, c.ClusterInfo)
		if !cephHealthy {
			if c.Spec.SkipUpgradeChecks {
				log.NamespacedWarning(c.Namespace, logger, "ceph is not healthy but SkipUpgradeChecks is set, forcing upgrade.")
			} else {
				return errors.Errorf("ceph status in namespace %s is not healthy, refusing to upgrade. Either fix the health issue or force an update by setting skipUpgradeChecks to true in the cluster CR", c.Namespace)
			}
		}
		// This is an upgrade
		log.NamespacedInfo(c.Namespace, logger, "upgrading ceph cluster to %q", version.String())
		c.isUpgrade = true
	}

	return nil
}
