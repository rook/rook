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

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// AppName is the ceph rbd mirror  application name
	AppName = "rook-ceph-rbd-mirror"
	// minimum amount of memory in MB to run the pod
	cephRbdMirrorPodMinimumMemory uint64 = 512
)

var updateDeploymentAndWait = mon.UpdateCephDeploymentAndWait

// Start begins the process of running rbd mirroring daemons.
func (r *ReconcileCephRBDMirror) start(cephRBDMirror *cephv1.CephRBDMirror) error {
	// Validate pod's memory if specified
	err := controller.CheckPodMemory(cephv1.ResourcesKeyRBDMirror, cephRBDMirror.Spec.Resources, cephRbdMirrorPodMinimumMemory)
	if err != nil {
		return errors.Wrap(err, "error checking pod memory")
	}

	logger.Infof("configure rbd-mirroring with %d workers", cephRBDMirror.Spec.Count)

	ownerInfo := k8sutil.NewOwnerInfo(cephRBDMirror, r.scheme)
	daemonID := k8sutil.IndexToName(0)
	resourceName := fmt.Sprintf("%s-%s", AppName, daemonID)
	daemonConf := &daemonConfig{
		DaemonID:     daemonID,
		ResourceName: resourceName,
		DataPathMap:  config.NewDatalessDaemonDataPathMap(cephRBDMirror.Namespace, r.cephClusterSpec.DataDirHostPath),
		ownerInfo:    ownerInfo,
	}

	_, err = r.generateKeyring(r.clusterInfo, daemonConf)
	if err != nil {
		return errors.Wrapf(err, "failed to generate keyring for %q", resourceName)
	}

	rbdMirrorToSkipReconcile, err := controller.GetDaemonsToSkipReconcile(r.clusterInfo.Context, r.context, r.clusterInfo.Namespace, config.RbdMirrorType, AppName)
	if err != nil {
		return errors.Wrap(err, "failed to check for RBD Mirror to skip reconcile")
	}
	if rbdMirrorToSkipReconcile.Len() > 0 {
		logger.Warningf("skipping RBD mirror reconcile since RBD mirror daemons are labeled with %s: %v", cephv1.SkipReconcileLabelKey, sets.List(rbdMirrorToSkipReconcile))
		return nil
	}

	// Start the deployment
	d, err := r.makeDeployment(daemonConf, cephRBDMirror)
	if err != nil {
		return errors.Wrap(err, "failed to create rbd-mirror deployment")
	}

	// Set owner ref to cephRBDMirror object
	err = controllerutil.SetControllerReference(cephRBDMirror, d, r.scheme)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference for ceph rbd-mirror deployment %q", d.Name)
	}

	// Set the deployment hash as an annotation
	err = patch.DefaultAnnotator.SetLastAppliedAnnotation(d)
	if err != nil {
		return errors.Wrapf(err, "failed to set annotation for deployment %q", d.Name)
	}

	if _, err := r.context.Clientset.AppsV1().Deployments(cephRBDMirror.Namespace).Create(r.opManagerContext, d, metav1.CreateOptions{}); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create %q deployment", resourceName)
		}
		logger.Infof("deployment for rbd-mirror %q already exists. updating if needed", resourceName)

		if err := updateDeploymentAndWait(r.context, r.clusterInfo, d, config.RbdMirrorType, daemonConf.DaemonID, r.cephClusterSpec.SkipUpgradeChecks, false); err != nil {
			// fail could be an issue updating label selector (immutable), so try del and recreate
			logger.Debugf("updateDeploymentAndWait failed for rbd-mirror %q. Attempting del-and-recreate. %v", resourceName, err)
			err = r.context.Clientset.AppsV1().Deployments(cephRBDMirror.Namespace).Delete(r.opManagerContext, d.Name, metav1.DeleteOptions{})
			if err != nil && !kerrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete rbd-mirror %q during del-and-recreate update attempt", resourceName)
			}
			if _, err := r.context.Clientset.AppsV1().Deployments(cephRBDMirror.Namespace).Create(r.opManagerContext, d, metav1.CreateOptions{}); err != nil {
				return errors.Wrapf(err, "failed to recreate rbd-mirror deployment %q during del-and-recreate update attempt", resourceName)
			}
		}
	}

	logger.Infof("%q deployment started", resourceName)
	return nil
}
