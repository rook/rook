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
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	err := controller.CheckPodMemory(cephRBDMirror.Spec.Resources, cephRbdMirrorPodMinimumMemory)
	if err != nil {
		return errors.Wrap(err, "error checking pod memory")
	}

	// Create the controller owner ref
	// It will be associated to all resources of the CephRBDMirror
	ref, err := opcontroller.GetControllerObjectOwnerReference(cephRBDMirror, r.scheme)
	if err != nil || ref == nil {
		return errors.Wrapf(err, "failed to get controller %q owner reference", cephRBDMirror.Name)
	}

	logger.Infof("configure rbd-mirroring with %d workers", cephRBDMirror.Spec.Count)

	for i := 0; i < cephRBDMirror.Spec.Count; i++ {
		daemonID := k8sutil.IndexToName(i)
		resourceName := fmt.Sprintf("%s-%s", AppName, daemonID)
		daemonConf := &daemonConfig{
			DaemonID:     daemonID,
			ResourceName: resourceName,
			DataPathMap:  config.NewDatalessDaemonDataPathMap(cephRBDMirror.Namespace, r.cephClusterSpec.DataDirHostPath),
			ownerRef:     *ref,
			namespace:    cephRBDMirror.Namespace,
		}

		_, err := r.generateKeyring(daemonConf)
		if err != nil {
			return errors.Wrapf(err, "failed to generate keyring for %q", resourceName)
		}

		// Start the deployment
		d := r.makeDeployment(daemonConf, cephRBDMirror)

		// Set owner ref to cephRBDMirror object
		err = controllerutil.SetControllerReference(cephRBDMirror, d, r.scheme)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference for ceph filesystem %q secret", d.Name)
		}

		// Set the deployment hash as an annotation
		err = patch.DefaultAnnotator.SetLastAppliedAnnotation(d)
		if err != nil {
			return errors.Wrapf(err, "failed to set annotation for deployment %q", d.Name)
		}

		if _, err := r.context.Clientset.AppsV1().Deployments(cephRBDMirror.Namespace).Create(d); err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create %q deployment", resourceName)
			}
			logger.Infof("deployment for rbd-mirror %q already exists. updating if needed", resourceName)

			if err := updateDeploymentAndWait(r.context, d, cephRBDMirror.Namespace, config.RbdMirrorType, daemonConf.DaemonID, r.cephClusterSpec.SkipUpgradeChecks, false); err != nil {
				// fail could be an issue updating label selector (immutable), so try del and recreate
				logger.Debugf("updateDeploymentAndWait failed for rbd-mirror %q. Attempting del-and-recreate. %v", resourceName, err)
				err = r.context.Clientset.AppsV1().Deployments(cephRBDMirror.Namespace).Delete(cephRBDMirror.Name, &metav1.DeleteOptions{})
				if err != nil {
					return errors.Wrapf(err, "failed to delete rbd-mirror %q during del-and-recreate update attempt", resourceName)
				}
				if _, err := r.context.Clientset.AppsV1().Deployments(cephRBDMirror.Namespace).Create(d); err != nil {
					return errors.Wrapf(err, "failed to recreate rbd-mirror deployment %q during del-and-recreate update attempt", resourceName)
				}
			}
		}

		logger.Infof("%q deployment started", resourceName)
	}

	// Remove extra rbd-mirror deployments if necessary
	err = r.removeExtraMirrors(cephRBDMirror)
	if err != nil {
		logger.Errorf("failed to remove extra mirrors. %v", err)
	}

	return nil
}

func (r *ReconcileCephRBDMirror) removeExtraMirrors(cephRBDMirror *cephv1.CephRBDMirror) error {
	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", AppName)}
	d, err := r.context.Clientset.AppsV1().Deployments(cephRBDMirror.Namespace).List(opts)
	if err != nil {
		return errors.Wrap(err, "failed to get mirrors")
	}

	if len(d.Items) <= cephRBDMirror.Spec.Count {
		logger.Info("no extra daemons to remove")
		return nil
	}

	for _, deploy := range d.Items {
		daemonName, ok := deploy.Labels["rbd-mirror"]
		if !ok {
			logger.Warningf("unrecognized rbdmirror %s", deploy.Name)
			continue
		}
		index, err := k8sutil.NameToIndex(daemonName)
		if err != nil {
			logger.Warningf("unrecognized rbdmirror %s with label %s", deploy.Name, daemonName)
			continue
		}
		if index >= cephRBDMirror.Spec.Count {
			logger.Infof("removing extra rbd-mirror %q", daemonName)
			var gracePeriod int64
			propagation := metav1.DeletePropagationForeground
			deleteOpts := metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
			if err = r.context.Clientset.AppsV1().Deployments(cephRBDMirror.Namespace).Delete(deploy.Name, &deleteOpts); err != nil {
				logger.Warningf("failed to delete rbd-mirror %q. %v", daemonName, err)
			}

			logger.Infof("removed rbd-mirror %q", daemonName)
		}
	}
	return nil
}
