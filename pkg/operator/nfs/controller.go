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

package nfs

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	nfsv1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"github.com/coreos/pkg/capnslog"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	nfsConfigMapPath = "/nfs-ganesha/config"
	nfsPort          = 2049
	rpcPort          = 111
)

type NFSServerReconciler struct {
	client.Client
	Context  *clusterd.Context
	Scheme   *runtime.Scheme
	Log      *capnslog.PackageLogger
	Recorder record.EventRecorder
}

func (r *NFSServerReconciler) Reconcile(context context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {

	instance := &nfsv1alpha1.NFSServer{}
	if err := r.Client.Get(context, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	// Initialize patcher utility and store the initial cr object state to be compare later.
	patcher, err := k8sutil.NewPatcher(instance, r.Client)
	if err != nil {
		return reconcile.Result{}, err
	}

	defer func() {
		// Always patch the cr object if any changes at the end of each reconciliation.
		if err := patcher.Patch(context, instance); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Add Finalizer if not present
	controllerutil.AddFinalizer(instance, nfsv1alpha1.Finalizer)

	// Handle for deletion. Just remove finalizer
	if !instance.DeletionTimestamp.IsZero() {
		r.Log.Infof("Deleting NFSServer %s in %s namespace", instance.Name, instance.Namespace)

		// no operation since we don't need do anything when nfsserver deleted.
		controllerutil.RemoveFinalizer(instance, nfsv1alpha1.Finalizer)
	}

	// Check status state. if it's empty then initialize it
	// otherwise if has error state then skip reconciliation to prevent requeue on error.
	switch instance.Status.State {
	case "":
		instance.Status.State = nfsv1alpha1.StateInitializing
		r.Log.Info("Initialize status state")
		return reconcile.Result{Requeue: true}, nil
	case nfsv1alpha1.StateError:
		r.Log.Info("Error state detected, skip reconciliation")
		return reconcile.Result{Requeue: false}, nil
	}

	// Validate cr spec and give warning event when validation fail.
	if err := instance.ValidateSpec(); err != nil {
		r.Recorder.Eventf(instance, corev1.EventTypeWarning, nfsv1alpha1.EventFailed, "Invalid NFSServer spec: %+v", err)
		r.Log.Errorf("Invalid NFSServer spec: %+v", err)
		instance.Status.State = nfsv1alpha1.StateError
		return reconcile.Result{}, err
	}

	if err := r.reconcileNFSServerConfig(context, instance); err != nil {
		r.Recorder.Eventf(instance, corev1.EventTypeWarning, nfsv1alpha1.EventFailed, "Failed reconciling nfsserver config: %+v", err)
		r.Log.Errorf("Error reconciling nfsserver config: %+v", err)
		return reconcile.Result{}, err
	}

	if err := r.reconcileNFSServer(context, instance); err != nil {
		r.Recorder.Eventf(instance, corev1.EventTypeWarning, nfsv1alpha1.EventFailed, "Failed reconciling nfsserver: %+v", err)
		r.Log.Errorf("Error reconciling nfsserver: %+v", err)
		return reconcile.Result{}, err
	}

	// Reconcile status state based on statefulset ready replicas.
	sts := &appsv1.StatefulSet{}
	if err := r.Client.Get(context, req.NamespacedName, sts); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	switch int(sts.Status.ReadyReplicas) {
	case instance.Spec.Replicas:
		instance.Status.State = nfsv1alpha1.StateRunning
		return reconcile.Result{}, nil
	default:
		instance.Status.State = nfsv1alpha1.StatePending
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

func (r *NFSServerReconciler) reconcileNFSServerConfig(ctx context.Context, cr *nfsv1alpha1.NFSServer) error {
	var exportsList []string

	id := 10
	for _, export := range cr.Spec.Exports {
		claimName := export.PersistentVolumeClaim.ClaimName
		var accessType string
		// validateNFSServerSpec guarantees `access` will be one of these values at this point
		switch strings.ToLower(export.Server.AccessMode) {
		case "readwrite":
			accessType = "RW"
		case "readonly":
			accessType = "RO"
		case "none":
			accessType = "None"
		}

		nfsGaneshaConfig := `
EXPORT {
	Export_Id = ` + fmt.Sprintf("%v", id) + `;
	Path = ` + path.Join("/", claimName) + `;
	Pseudo = ` + path.Join("/", claimName) + `;
	Protocols = 4;
	Transports = TCP;
	Sectype = sys;
	Access_Type = ` + accessType + `;
	Squash = ` + strings.ToLower(export.Server.Squash) + `;
	FSAL {
		Name = VFS;
	}
}`

		exportsList = append(exportsList, nfsGaneshaConfig)
		id++
	}

	nfsGaneshaAdditionalConfig := `
NFS_Core_Param {
	fsid_device = true;
}
`

	exportsList = append(exportsList, nfsGaneshaAdditionalConfig)
	configdata := make(map[string]string)
	configdata[cr.Name] = strings.Join(exportsList, "\n")
	cm := newConfigMapForNFSServer(cr)
	cmop, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if err := controllerutil.SetOwnerReference(cr, cm, r.Scheme); err != nil {
			return err
		}

		cm.Data = configdata
		return nil
	})

	if err != nil {
		return err
	}

	r.Log.Info("Reconciling NFSServer ConfigMap", "Operation.Result ", cmop)
	switch cmop {
	case controllerutil.OperationResultCreated:
		r.Recorder.Eventf(cr, corev1.EventTypeNormal, nfsv1alpha1.EventCreated, "%s nfs-server config configmap: %s", strings.Title(string(cmop)), cm.Name)
		return nil
	case controllerutil.OperationResultUpdated:
		r.Recorder.Eventf(cr, corev1.EventTypeNormal, nfsv1alpha1.EventUpdated, "%s nfs-server config configmap: %s", strings.Title(string(cmop)), cm.Name)
		return nil
	default:
		return nil
	}
}

func (r *NFSServerReconciler) reconcileNFSServer(ctx context.Context, cr *nfsv1alpha1.NFSServer) error {
	svc := newServiceForNFSServer(cr)
	svcop, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if !svc.ObjectMeta.CreationTimestamp.IsZero() {
			return nil
		}

		if err := controllerutil.SetControllerReference(cr, svc, r.Scheme); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	r.Log.Info("Reconciling NFSServer Service", "Operation.Result ", svcop)
	switch svcop {
	case controllerutil.OperationResultCreated:
		r.Recorder.Eventf(cr, corev1.EventTypeNormal, nfsv1alpha1.EventCreated, "%s nfs-server service: %s", strings.Title(string(svcop)), svc.Name)
	case controllerutil.OperationResultUpdated:
		r.Recorder.Eventf(cr, corev1.EventTypeNormal, nfsv1alpha1.EventUpdated, "%s nfs-server service: %s", strings.Title(string(svcop)), svc.Name)
	}

	sts, err := newStatefulSetForNFSServer(cr, r.Context.Clientset, ctx)
	if err != nil {
		return fmt.Errorf("unable to generate the NFS StatefulSet spec: %v", err)
	}

	stsop, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		if sts.ObjectMeta.CreationTimestamp.IsZero() {
			sts.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: newLabels(cr),
			}
		}

		if err := controllerutil.SetControllerReference(cr, sts, r.Scheme); err != nil {
			return err
		}

		volumes := []corev1.Volume{
			{
				Name: cr.Name,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cr.Name,
						},
						Items: []corev1.KeyToPath{
							{
								Key:  cr.Name,
								Path: cr.Name,
							},
						},
						DefaultMode: pointer.Int32Ptr(corev1.ConfigMapVolumeSourceDefaultMode),
					},
				},
			},
		}
		volumeMounts := []corev1.VolumeMount{
			{
				Name:      cr.Name,
				MountPath: nfsConfigMapPath,
			},
		}
		for _, export := range cr.Spec.Exports {
			shareName := export.Name
			claimName := export.PersistentVolumeClaim.ClaimName
			volumes = append(volumes, corev1.Volume{
				Name: shareName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
					},
				},
			})

			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      shareName,
				MountPath: path.Join("/", claimName),
			})
		}

		sts.Spec.Template.Spec.Volumes = volumes
		for i, container := range sts.Spec.Template.Spec.Containers {
			if container.Name == "nfs-server" || container.Name == "nfs-provisioner" {
				sts.Spec.Template.Spec.Containers[i].VolumeMounts = volumeMounts
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	r.Log.Info("Reconciling NFSServer StatefulSet", "Operation.Result ", stsop)
	switch stsop {
	case controllerutil.OperationResultCreated:
		r.Recorder.Eventf(cr, corev1.EventTypeNormal, nfsv1alpha1.EventCreated, "%s nfs-server statefulset: %s", strings.Title(string(stsop)), sts.Name)
		return nil
	case controllerutil.OperationResultUpdated:
		r.Recorder.Eventf(cr, corev1.EventTypeNormal, nfsv1alpha1.EventUpdated, "%s nfs-server statefulset: %s", strings.Title(string(stsop)), sts.Name)
		return nil
	default:
		return nil
	}
}
