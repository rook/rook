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

package machinelabel

import (
	"context"

	"github.com/coreos/pkg/capnslog"
	mapiv1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	controllerName                  = "machinelabel-controller"
	MachineFencingLabelKey          = "fencegroup.rook.io/cluster"
	MachineFencingNamespaceLabelKey = "fencegroup.rook.io/clusterNamespace"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

type ReconcileMachineLabel struct {
	scheme  *runtime.Scheme
	client  client.Client
	options *controllerconfig.Context
}

type machine struct {
	isOccupiedByOSD bool
	RawMachine      mapiv1.Machine
}

// Reconcile is the implementation of reconcile function for ReconcileMachineLabel
// which ensures that the machineLabel for the osd pods are in correct state
// The Controller will requeue the Request to be processed again if an error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMachineLabel) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// wrapping reconcile because the rook logging mechanism is not compatible with the controller-runtime logging interface
	result, err := r.reconcile(request)
	if err != nil {
		logger.Error(err)
	}
	return result, err
}

func (r *ReconcileMachineLabel) reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger.Debugf("reconciling %s", request.NamespacedName)

	// Fetch list of osd pods for the requested ceph cluster
	pods := &corev1.PodList{}
	err := r.client.List(r.options.OpManagerContext, pods, client.InNamespace(request.Namespace),
		client.MatchingLabels{k8sutil.AppAttr: osd.AppName, k8sutil.ClusterAttr: request.Name})
	if err != nil {
		return reconcile.Result{}, err
	}

	// Fetching the cephCluster
	cephClusterInstance := &cephv1.CephCluster{}
	err = r.client.Get(r.options.OpManagerContext, request.NamespacedName, cephClusterInstance)
	if kerrors.IsNotFound(err) {
		logger.Infof("cephCluster instance not found for %s", request.NamespacedName)
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to fetch cephCluster %s", request.NamespacedName)
	}

	// skipping the reconcile since the feature is switched off
	if !cephClusterInstance.Spec.DisruptionManagement.ManageMachineDisruptionBudgets {
		logger.Debugf("Skipping reconcile for cephCluster %s as manageMachineDisruption is turned off", request.NamespacedName)
		return reconcile.Result{}, nil
	}

	// Fetch list of machines available
	machines := &mapiv1.MachineList{}
	err = r.client.List(r.options.OpManagerContext, machines, client.InNamespace(cephClusterInstance.Spec.DisruptionManagement.MachineDisruptionBudgetNamespace))
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed tp fetch machine list")
	}

	nodeMachineMap := map[string]machine{}

	// Adding machines to nodeMachineMap
	for _, m := range machines.Items {
		if m.Status.NodeRef != nil {
			nodeMachineMap[m.Status.NodeRef.Name] = machine{RawMachine: m}
		}
	}

	// Marking machines that are occupied by the osd pods
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" {
			if machine, p := nodeMachineMap[pod.Spec.NodeName]; p {
				machine.isOccupiedByOSD = true
				nodeMachineMap[pod.Spec.NodeName] = machine
			}
		}
	}

	// Updating the machine status
	for _, machine := range nodeMachineMap {
		labels := machine.RawMachine.GetLabels()
		if machine.isOccupiedByOSD {
			if shouldSkipMachineUpdate(labels, request.Name, request.Namespace) {
				continue
			}
			labels[MachineFencingLabelKey] = request.Name
			labels[MachineFencingNamespaceLabelKey] = request.Namespace
			machine.RawMachine.SetLabels(labels)
			err = r.client.Update(r.options.OpManagerContext, &machine.RawMachine)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to update machine %s", machine.RawMachine.GetName())
			}
			logger.Infof("Successfully updated the Machine %s", machine.RawMachine.GetName())
		} else {
			if shouldSkipMachineUpdate(labels, "", "") {
				continue
			}
			labels[MachineFencingLabelKey] = ""
			labels[MachineFencingNamespaceLabelKey] = ""
			machine.RawMachine.SetLabels(labels)
			err = r.client.Update(r.options.OpManagerContext, &machine.RawMachine)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to update machine %s", machine.RawMachine.GetName())
			}
			logger.Infof("Successfully updated the Machine %s", machine.RawMachine.GetName())
		}
	}

	return reconcile.Result{}, nil
}

// shouldSkipMachineUpdate return true if the machine labels are already the expected value
func shouldSkipMachineUpdate(labels map[string]string, expectedName, expectedNamespace string) bool {
	clusterName, isClusterNamePresent := labels[MachineFencingLabelKey]
	clusterNamespace, isClusterNamespacePresent := labels[MachineFencingNamespaceLabelKey]
	return isClusterNamePresent && isClusterNamespacePresent && clusterName == expectedName && clusterNamespace == expectedNamespace
}
