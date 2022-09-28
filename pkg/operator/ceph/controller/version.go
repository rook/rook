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

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/k8sutil/cmdreporter"
	"k8s.io/client-go/kubernetes"
)

const detectCephVersionTimeout = 15 * time.Minute

// ValidateCephVersionsBetweenLocalAndExternalClusters makes sure an external cluster can be connected
// by checking the external ceph versions available and comparing it with the local image provided
func ValidateCephVersionsBetweenLocalAndExternalClusters(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) (cephver.CephVersion, error) {
	// health check should tell us if the external cluster has been upgraded and display a message
	externalVersion, err := cephclient.GetCephMonVersion(context, clusterInfo)
	if err != nil {
		return cephver.CephVersion{}, errors.Wrap(err, "failed to get ceph mon version")
	}

	return *externalVersion, cephver.ValidateCephVersionsBetweenLocalAndExternalClusters(clusterInfo.CephVersion, *externalVersion)
}

// GetImageVersion returns the CephVersion registered for a specified image (if any) and whether any image was found.
func GetImageVersion(cephCluster cephv1.CephCluster) (*cephver.CephVersion, error) {
	// If the Ceph cluster has not yet recorded the image and version for the current image in its spec, then the Crash
	// controller should wait for the version to be detected.
	if cephCluster.Status.CephVersion != nil && cephCluster.Spec.CephVersion.Image == cephCluster.Status.CephVersion.Image {
		logger.Debugf("ceph version found %q", cephCluster.Status.CephVersion.Version)
		return ExtractCephVersionFromLabel(cephCluster.Status.CephVersion.Version)
	}

	return nil, errors.New("attempt to determine ceph version for the current cluster image timed out")
}

// DetectCephVersion loads the ceph version from the image and checks that it meets the version requirements to
// run in the cluster
func DetectCephVersion(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, clientset kubernetes.Interface, cephClusterSpec *cephv1.ClusterSpec) (*cephver.CephVersion, error) {
	cephImage := cephClusterSpec.CephVersion.Image
	logger.Infof("detecting the ceph image version for image %s...", cephImage)
	versionReporter, err := cmdreporter.New(
		clientset,
		ownerInfo,
		jobName,
		jobName,
		namespace,
		[]string{"ceph"},
		[]string{"--version"},
		rookImage,
		cephImage,
		cephClusterSpec.CephVersion.ImagePullPolicy,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up ceph version job")
	}

	job := versionReporter.Job()
	job.Spec.Template.Spec.ServiceAccountName = "rook-ceph-cmd-reporter"

	// Apply the same placement for the ceph version detection as the mon daemons except for PodAntiAffinity
	cephv1.GetMonPlacement(cephClusterSpec.Placement).ApplyToPodSpec(&job.Spec.Template.Spec)
	job.Spec.Template.Spec.Affinity.PodAntiAffinity = nil

	stdout, stderr, retcode, err := versionReporter.Run(ctx, detectCephVersionTimeout)
	if err != nil {
		return nil, errors.Wrap(err, "failed to complete ceph version job")
	}
	if retcode != 0 {
		return nil, errors.Errorf(`ceph version job returned failure with retcode %d. `+
			`stdout: %s. `+
			`stderr: %s`,
			retcode,
			stdout,
			stderr,
		)
	}

	version, err := cephver.ExtractCephVersion(stdout)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract ceph version")
	}
	logger.Infof("detected ceph image version: %q", version)
	return version, nil
}

func CurrentAndDesiredCephVersion(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, context *clusterd.Context, cephClusterSpec *cephv1.ClusterSpec, clusterInfo *cephclient.ClusterInfo) (*cephver.CephVersion, *cephver.CephVersion, error) {
	// Detect desired CephCluster version
	desiredCephVersion, err := DetectCephVersion(ctx, rookImage, namespace, fmt.Sprintf("%s-detect-version", jobName), ownerInfo, context.Clientset, cephClusterSpec)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to detect ceph image version")
	}

	// Check the ceph version of the running monitors
	runningMonDaemonVersion, err := cephclient.LeastUptodateDaemonVersion(context, clusterInfo, config.MonType)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to retrieve current ceph %q version", config.MonType)
	}

	return desiredCephVersion, &runningMonDaemonVersion, nil
}

func ErrorCephUpgradingRequeue(runningCephVersion, desiredCephVersion *cephver.CephVersion) error {
	return errors.Errorf(`waiting for ceph monitors upgrade to finish. `+
		`current version: %s. `+
		`expected version: %s. `+
		`will reconcile again in %s`,
		runningCephVersion.String(),
		desiredCephVersion.String(),
		WaitForRequeueIfCephClusterIsUpgrading.RequeueAfter.String(),
	)
}
