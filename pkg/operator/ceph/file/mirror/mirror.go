/*
Copyright 2021 The Rook Authors. All rights reserved.

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

// Package mirror for mirroring
package mirror

import (
	"context"
	"fmt"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// AppName is the ceph filesystem mirror application name
	AppName = "rook-ceph-fs-mirror"
	// minimum amount of memory in MB to run the pod
	cephFilesystemMirrorPodMinimumMemory uint64 = 512
)

var updateDeploymentAndWait = mon.UpdateCephDeploymentAndWait

// Start begins the process of running filesystem mirroring daemons.
func (r *ReconcileFilesystemMirror) start(filesystemMirror *cephv1.CephFilesystemMirror) error {
	ctx := context.TODO()
	// Validate pod's memory if specified
	err := controller.CheckPodMemory(cephv1.ResourcesKeyFilesystemMirror, filesystemMirror.Spec.Resources, cephFilesystemMirrorPodMinimumMemory)
	if err != nil {
		return errors.Wrap(err, "error checking pod memory")
	}

	// Create the controller owner ref
	// It will be associated to all resources of the CephRBDMirror
	ref, err := opcontroller.GetControllerObjectOwnerReference(filesystemMirror, r.scheme)
	if err != nil || ref == nil {
		return errors.Wrapf(err, "failed to get controller %q owner reference", filesystemMirror.Name)
	}

	daemonID := k8sutil.IndexToName(0)
	resourceName := fmt.Sprintf("%s-%s", AppName, daemonID)
	daemonConf := &daemonConfig{
		DaemonID:     daemonID,
		ResourceName: resourceName,
		DataPathMap:  config.NewDatalessDaemonDataPathMap(filesystemMirror.Namespace, r.cephClusterSpec.DataDirHostPath),
		ownerRef:     *ref,
	}

	_, err = r.generateKeyring(r.clusterInfo, daemonConf)
	if err != nil {
		return errors.Wrapf(err, "failed to generate keyring for %q", resourceName)
	}

	// Start the deployment
	d, err := r.makeDeployment(daemonConf, filesystemMirror)
	if err != nil {
		return errors.Wrap(err, "failed to create filesystem-mirror deployment")
	}

	// Set owner ref to filesystemMirror object
	err = controllerutil.SetControllerReference(filesystemMirror, d, r.scheme)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference for ceph filesystem-mirror %q secret", d.Name)
	}

	// Set the deployment hash as an annotation
	err = patch.DefaultAnnotator.SetLastAppliedAnnotation(d)
	if err != nil {
		return errors.Wrapf(err, "failed to set annotation for deployment %q", d.Name)
	}

	if _, err := r.context.Clientset.AppsV1().Deployments(filesystemMirror.Namespace).Create(ctx, d, metav1.CreateOptions{}); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create %q deployment", resourceName)
		}
		logger.Infof("deployment for filesystem-mirror %q already exists. updating if needed", resourceName)

		if err := updateDeploymentAndWait(r.context, r.clusterInfo, d, config.RbdMirrorType, daemonConf.DaemonID, r.cephClusterSpec.SkipUpgradeChecks, false); err != nil {
			// fail could be an issue updating label selector (immutable), so try del and recreate
			logger.Debugf("updateDeploymentAndWait failed for filesystem-mirror %q. Attempting del-and-recreate. %v", resourceName, err)
			err = r.context.Clientset.AppsV1().Deployments(filesystemMirror.Namespace).Delete(ctx, filesystemMirror.Name, metav1.DeleteOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to delete filesystem-mirror %q during del-and-recreate update attempt", resourceName)
			}
			if _, err := r.context.Clientset.AppsV1().Deployments(filesystemMirror.Namespace).Create(ctx, d, metav1.CreateOptions{}); err != nil {
				return errors.Wrapf(err, "failed to recreate filesystem-mirror deployment %q during del-and-recreate update attempt", resourceName)
			}
		}
	}

	logger.Infof("%q deployment started", resourceName)

	return nil
}
