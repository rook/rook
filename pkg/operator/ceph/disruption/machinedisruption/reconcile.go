/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package machinedisruption

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/pkg/capnslog"
	healthchecking "github.com/openshift/machine-api-operator/pkg/apis/healthchecking/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephClient "github.com/rook/rook/pkg/daemon/ceph/client"

	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"github.com/rook/rook/pkg/operator/ceph/disruption/machinelabel"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	controllerName                  = "machinedisruption-controller"
	MDBCephClusterNamespaceLabelKey = "rook.io/cephClusterNamespace"
	MDBCephClusterNameLabelKey      = "rook.io/cephClusterName"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// MachineDisruptionReconciler reconciles MachineDisruption
type MachineDisruptionReconciler struct {
	scheme  *runtime.Scheme
	client  client.Client
	context *controllerconfig.Context
}

// Reconcile is the implementation of reconcile function for MachineDisruptionReconciler
// which ensures that the machineDisruptionBudget for the rook ceph cluster is in correct state
// The Controller will requeue the Request to be processed again if an error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *MachineDisruptionReconciler) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// wrapping reconcile because the rook logging mechanism is not compatible with the controller-runtime logging interface
	result, err := r.reconcile(request)
	if err != nil {
		logger.Error(err)
	}
	return result, err
}

func (r *MachineDisruptionReconciler) reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger.Debugf("reconciling %s", request.NamespacedName)

	// Fetching the cephCluster
	cephClusterInstance := &cephv1.CephCluster{}
	err := r.client.Get(r.context.OpManagerContext, request.NamespacedName, cephClusterInstance)
	if kerrors.IsNotFound(err) {
		logger.Infof("cephCluster instance not found for %s", request.NamespacedName)
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "could not fetch cephCluster %s", request.Name)
	}

	// skipping the reconcile since the feature is switched off
	if !cephClusterInstance.Spec.DisruptionManagement.ManageMachineDisruptionBudgets {
		logger.Debugf("Skipping reconcile for cephCluster %s as manageMachineDisruption is turned off", request.NamespacedName)
		return reconcile.Result{}, nil
	}

	mdb := &healthchecking.MachineDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateMDBInstanceName(request.Name, request.Namespace),
			Namespace: cephClusterInstance.Spec.DisruptionManagement.MachineDisruptionBudgetNamespace,
		},
	}

	err = r.client.Get(r.context.OpManagerContext, types.NamespacedName{Name: mdb.GetName(), Namespace: mdb.GetNamespace()}, mdb)
	if kerrors.IsNotFound(err) {
		// If the MDB is not found creating the MDB for the cephCluster
		maxUnavailable := int32(0)
		// Generating the MDB instance for the cephCluster
		newMDB := &healthchecking.MachineDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      generateMDBInstanceName(request.Name, request.Namespace),
				Namespace: cephClusterInstance.Spec.DisruptionManagement.MachineDisruptionBudgetNamespace,
				Labels: map[string]string{
					MDBCephClusterNamespaceLabelKey: request.Namespace,
					MDBCephClusterNameLabelKey:      request.Name,
				},
			},
			Spec: healthchecking.MachineDisruptionBudgetSpec{
				MaxUnavailable: &maxUnavailable,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						machinelabel.MachineFencingLabelKey:          request.Name,
						machinelabel.MachineFencingNamespaceLabelKey: request.Namespace,
					},
				},
			},
		}
		err = controllerutil.SetControllerReference(cephClusterInstance, newMDB, r.scheme)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to set owner reference of mdb %q", newMDB.Name)
		}
		err = r.client.Create(r.context.OpManagerContext, newMDB)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to create mdb %s", mdb.GetName())
		}
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}
	if mdb.Spec.MaxUnavailable == nil {
		maxUnavailable := int32(0)
		mdb.Spec.MaxUnavailable = &maxUnavailable
	}
	// Check if the cluster is clean or not
	clusterInfo := cephClient.AdminClusterInfo(r.context.OpManagerContext, request.NamespacedName.Namespace, request.NamespacedName.Name)
	_, isClean, err := cephClient.IsClusterClean(r.context.ClusterdContext, clusterInfo)
	if err != nil {
		maxUnavailable := int32(0)
		mdb.Spec.MaxUnavailable = &maxUnavailable
		err = r.client.Update(r.context.OpManagerContext, mdb)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to update mdb %s", mdb.GetName())
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to get cephCluster %s status", request.NamespacedName)
	}
	if isClean && *mdb.Spec.MaxUnavailable != 1 {
		maxUnavailable := int32(1)
		mdb.Spec.MaxUnavailable = &maxUnavailable
		err = r.client.Update(r.context.OpManagerContext, mdb)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to update mdb %s", mdb.GetName())
		}
	} else if !isClean && *mdb.Spec.MaxUnavailable != 0 {
		maxUnavailable := int32(0)
		mdb.Spec.MaxUnavailable = &maxUnavailable
		err = r.client.Update(r.context.OpManagerContext, mdb)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to update mdb %s", mdb.GetName())
		}
	}
	return reconcile.Result{Requeue: true, RequeueAfter: time.Minute}, nil
}

func generateMDBInstanceName(name, namespace string) string {
	return fmt.Sprintf("%s-%s", name, namespace)
}
