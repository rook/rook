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
	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
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
	nsName := controller.NsName(filesystemMirror.Namespace, filesystemMirror.Name)
	// Validate pod's memory if specified
	err := controller.CheckPodMemory(cephv1.ResourcesKeyFilesystemMirror, filesystemMirror.Spec.Resources, cephFilesystemMirrorPodMinimumMemory)
	if err != nil {
		return errors.Wrap(err, "error checking pod memory")
	}

	ownerInfo := k8sutil.NewOwnerInfo(filesystemMirror, r.scheme)
	daemonConf := &daemonConfig{
		ResourceName: AppName,
		DataPathMap:  config.NewDatalessDaemonDataPathMap(filesystemMirror.Namespace, r.cephClusterSpec.DataDirHostPath),
		ownerInfo:    ownerInfo,
	}

	secretResourceVersion, err := r.generateKeyring(daemonConf)
	if err != nil {
		return errors.Wrapf(err, "failed to generate keyring for %q", AppName)
	}

	// Start the deployment
	d, err := r.makeDeployment(daemonConf, filesystemMirror)
	if err != nil {
		return errors.Wrap(err, "failed to create filesystem-mirror deployment")
	}

	// apply cephx secret resource version to the deployment to ensure it restarts when keyring updates
	d.Spec.Template.Annotations[keyring.CephxKeyIdentifierAnnotation] = secretResourceVersion

	// Set owner ref to filesystemMirror object
	err = controllerutil.SetControllerReference(filesystemMirror, d, r.scheme)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference for ceph filesystem-mirror deployment %q", d.Name)
	}

	// Set the deployment hash as an annotation
	err = patch.DefaultAnnotator.SetLastAppliedAnnotation(d)
	if err != nil {
		return errors.Wrapf(err, "failed to set annotation for deployment %q", d.Name)
	}

	if _, err := r.context.Clientset.AppsV1().Deployments(filesystemMirror.Namespace).Create(r.opManagerContext, d, metav1.CreateOptions{}); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create %q deployment", d.Name)
		}
		log.NamedInfo(nsName, logger, "deployment for filesystem-mirror already exists. updating if needed")

		if err := updateDeploymentAndWait(r.context, r.clusterInfo, d, config.FilesystemMirrorType, AppName, r.cephClusterSpec.SkipUpgradeChecks, false); err != nil {
			// fail could be an issue updating label selector (immutable), so try del and recreate
			log.NamedDebug(nsName, logger, "updateDeploymentAndWait failed for filesystem-mirror %q. Attempting del-and-recreate. %v", d.Name, err)
			err = r.context.Clientset.AppsV1().Deployments(filesystemMirror.Namespace).Delete(r.opManagerContext, d.Name, metav1.DeleteOptions{})
			if err != nil && !kerrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete filesystem-mirror deployment %q during del-and-recreate update attempt", d.Name)
			}
			if _, err := r.context.Clientset.AppsV1().Deployments(filesystemMirror.Namespace).Create(r.opManagerContext, d, metav1.CreateOptions{}); err != nil {
				return errors.Wrapf(err, "failed to recreate filesystem-mirror deployment %q during del-and-recreate update attempt", d.Name)
			}
		}
	}

	log.NamedInfo(nsName, logger, "%q deployment started", AppName)

	return nil
}
