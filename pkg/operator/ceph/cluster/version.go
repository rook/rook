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
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	daemonclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil/cmdreporter"
)

func (c *ClusterController) detectAndValidateCephVersion(cluster *cluster) (*cephver.CephVersion, bool, error) {
	version, err := cluster.detectCephVersion(c.rookImage, cluster.Spec.CephVersion.Image, detectCephVersionTimeout)
	if err != nil {
		return nil, false, err
	}

	logger.Info("validating ceph version from provided image")
	if err := cluster.validateCephVersion(version); err != nil {
		return nil, cluster.isUpgrade, err
	}

	// Update ceph version field in cluster object status
	c.updateClusterCephVersion(cluster.Spec.CephVersion.Image, *version)

	return version, cluster.isUpgrade, nil
}

func (c *cluster) printOverallCephVersion() {
	versions, err := daemonclient.GetAllCephDaemonVersions(c.context, c.Namespace)
	if err != nil {
		logger.Errorf("failed to get ceph daemons versions. %v", err)
		return
	}

	if len(versions.Overall) == 1 {
		for v := range versions.Overall {
			version, err := cephver.ExtractCephVersion(v)
			if err != nil {
				logger.Errorf("failed to extract ceph version. %v", err)
				return
			}
			vv := *version
			logger.Infof("successfully upgraded cluster to version: %q", vv.String())
		}
	} else {
		// This shouldn't happen, but let's log just in case
		logger.Warningf("upgrade orchestration completed but somehow we still have more than one Ceph version running. %v:", versions.Overall)
	}
}

// This function compare the Ceph spec image and the cluster running version
// It returns true if the image is different and false if identical
func diffImageSpecAndClusterRunningVersion(imageSpecVersion cephver.CephVersion, runningVersions client.CephDaemonsVersions) (bool, error) {
	numberOfCephVersions := len(runningVersions.Overall)
	if numberOfCephVersions == 0 {
		// let's return immediately
		return false, errors.Errorf("no 'overall' section in the ceph versions. %+v", runningVersions.Overall)
	}

	if numberOfCephVersions > 1 {
		// let's return immediately
		logger.Warningf("it looks like we have more than one ceph version running. triggering upgrade. %+v:", runningVersions.Overall)
		return true, nil
	}

	if numberOfCephVersions == 1 {
		for v := range runningVersions.Overall {
			version, err := cephver.ExtractCephVersion(v)
			if err != nil {
				logger.Errorf("failed to extract ceph version. %v", err)
				return false, err
			}
			clusterRunningVersion := *version

			// If this is the same version
			if cephver.IsIdentical(clusterRunningVersion, imageSpecVersion) {
				logger.Debugf("both cluster and image spec versions are identical, doing nothing %s", imageSpecVersion.String())
				return false, nil
			}

			if cephver.IsSuperior(imageSpecVersion, clusterRunningVersion) {
				logger.Infof("image spec version %s is higher than the running cluster version %s, upgrading", imageSpecVersion.String(), clusterRunningVersion.String())
				return true, nil
			}

			if cephver.IsInferior(imageSpecVersion, clusterRunningVersion) {
				return true, errors.Errorf("image spec version %s is lower than the running cluster version %s, downgrading is not supported", imageSpecVersion.String(), clusterRunningVersion.String())
			}
		}
	}

	return false, nil
}

// detectCephVersion loads the ceph version from the image and checks that it meets the version requirements to
// run in the cluster
func (c *cluster) detectCephVersion(rookImage, cephImage string, timeout time.Duration) (*cephver.CephVersion, error) {
	logger.Infof("detecting the ceph image version for image %s...", cephImage)
	versionReporter, err := cmdreporter.New(
		c.context.Clientset, &c.ownerRef,
		detectVersionName, detectVersionName, c.Namespace,
		[]string{"ceph"}, []string{"--version"},
		rookImage, cephImage)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up ceph version job")
	}

	job := versionReporter.Job()
	job.Spec.Template.Spec.ServiceAccountName = "rook-ceph-cmd-reporter"

	// Apply the same node selector and tolerations for the ceph version detection as the mon daemons
	cephv1.GetMonPlacement(c.Spec.Placement).ApplyToPodSpec(&job.Spec.Template.Spec)

	stdout, stderr, retcode, err := versionReporter.Run(timeout)
	if err != nil {
		return nil, errors.Wrap(err, "failed to complete ceph version job")
	}
	if retcode != 0 {
		return nil, errors.Errorf(`ceph version job returned failure with retcode %d.
  stdout: %s
  stderr: %s`, retcode, stdout, stderr)
	}

	version, err := cephver.ExtractCephVersion(stdout)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract ceph version")
	}
	logger.Infof("detected ceph image version: %q", version)
	return version, nil
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
			logger.Warningf("unsupported ceph version detected: %q, pursuing", version)
		}
	}

	// The following tries to determine if the operator can proceed with an upgrade because we come from an OnAdd() call
	// If the cluster was unhealthy and someone injected a new image version, an upgrade was triggered but failed because the cluster is not healthy
	// Then after this, if the operator gets restarted we are not able to fail if the cluster is not healthy, the following tries to determine the
	// state we are in and if we should upgrade or not

	// Try to load clusterInfo so we can compare the running version with the one from the spec image
	clusterInfo, _, _, err := mon.LoadClusterInfo(c.context, c.Namespace)
	if err == nil {
		// Write connection info (ceph config file and keyring) for ceph commands
		err = mon.WriteConnectionConfig(c.context, clusterInfo)
		if err != nil {
			logger.Errorf("failed to write config. attempting to continue. %v", err)
		}
	}

	if !clusterInfo.IsInitialized(false) {
		// If not initialized, this is likely a new cluster so there is nothing to do
		logger.Debug("cluster not initialized, nothing to validate")
		return nil
	}

	if c.Spec.External.Enable && c.Spec.CephVersion.Image != "" {
		c.Info.CephVersion, err = controller.ValidateCephVersionsBetweenLocalAndExternalClusters(c.context, c.Namespace, *version)
		if err != nil {
			return errors.Wrap(err, "failed to validate ceph version between external and local")
		}
	}

	// On external cluster setup, if we don't bootstrap any resources in the Kubernetes cluster then
	// there is no need to validate the Ceph image further
	if c.Spec.External.Enable && c.Spec.CephVersion.Image == "" {
		logger.Debug("no spec image specified on external cluster, not validating Ceph version.")
		return nil
	}

	// Get cluster running versions
	versions, err := client.GetAllCephDaemonVersions(c.context, c.Namespace)
	if err != nil {
		logger.Errorf("failed to get ceph daemons versions, this typically happens during the first cluster initialization. %v", err)
		return nil
	}

	runningVersions := *versions
	differentImages, err := diffImageSpecAndClusterRunningVersion(*version, runningVersions)
	if err != nil {
		logger.Errorf("failed to determine if we should upgrade or not. %v", err)
		// we shouldn't block the orchestration if we can't determine the version of the image spec, we proceed anyway in best effort
		// we won't be able to check if there is an update or not and what to do, so we don't check the cluster status either
		// This will happen if someone uses ceph/daemon:latest-master for instance
		return nil
	}

	if differentImages {
		// If the image version changed let's make sure we can safely upgrade
		// check ceph's status, if not healthy we fail
		cephHealthy := client.IsCephHealthy(c.context, c.Namespace)
		if !cephHealthy {
			if c.Spec.SkipUpgradeChecks {
				logger.Warning("ceph is not healthy but SkipUpgradeChecks is set, forcing upgrade.")
			} else {
				return errors.Errorf("ceph status in namespace %s is not healthy, refusing to upgrade. fix the cluster and re-edit the cluster CR to trigger a new orchestation update", c.Namespace)
			}
		}
		// This is an upgrade
		logger.Info("upgrading ceph cluster to %q", version.String())
		c.isUpgrade = true
	}

	return nil
}
