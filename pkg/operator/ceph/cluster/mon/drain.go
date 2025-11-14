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

package mon

import (
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	monPDBName = "rook-ceph-mon-pdb"
)

func (c *Cluster) reconcileMonPDB() error {
	if !c.spec.DisruptionManagement.ManagePodBudgets {
		// TODO: Delete mon PDB
		return nil
	}

	monCount := c.spec.Mon.Count
	if monCount <= 2 {
		log.NamespacedDebug(c.Namespace, logger, "managePodBudgets is set, but mon-count <= 2. Not creating a disruptionbudget for Mons")
		return nil
	}

	op, err := c.createOrUpdateMonPDB(c.getMaxUnavailableMonPodCount())
	if err != nil {
		return errors.Wrapf(err, "failed to reconcile mon pdb on op %q", op)
	}
	return nil
}

func (c *Cluster) createOrUpdateMonPDB(maxUnavailable int32) (controllerutil.OperationResult, error) {
	objectMeta := metav1.ObjectMeta{
		Name:      monPDBName,
		Namespace: c.Namespace,
	}
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{k8sutil.AppAttr: AppName},
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: objectMeta,
	}

	mutateFunc := func() error {
		pdb.Spec = policyv1.PodDisruptionBudgetSpec{
			Selector:       selector,
			MaxUnavailable: &intstr.IntOrString{IntVal: maxUnavailable},
		}
		return nil
	}
	return controllerutil.CreateOrUpdate(c.ClusterInfo.Context, c.context.Client, pdb, mutateFunc)
}

// blockMonDrain makes MaxUnavailable in mon PDB to 0 to block any voluntary mon drains
func (c *Cluster) blockMonDrain(request types.NamespacedName) error {
	if !c.spec.DisruptionManagement.ManagePodBudgets {
		return nil
	}
	log.NamespacedInfo(c.Namespace, logger, "prevent voluntary mon drain while failing over")
	// change MaxUnavailable mon PDB to 0
	_, err := c.createOrUpdateMonPDB(0)
	if err != nil {
		return errors.Wrapf(err, "failed to update MaxUnavailable for mon PDB %q", request.Name)
	}
	return nil
}

// allowMonDrain updates the MaxUnavailable in mon PDB to 1 to allow voluntary mon drains
func (c *Cluster) allowMonDrain(request types.NamespacedName) error {
	if !c.spec.DisruptionManagement.ManagePodBudgets {
		return nil
	}
	log.NamespacedInfo(c.Namespace, logger, "allow voluntary mon drain after failover")
	_, err := c.createOrUpdateMonPDB(c.getMaxUnavailableMonPodCount())
	if err != nil {
		return errors.Wrapf(err, "failed to update MaxUnavailable for mon PDB %q", request.Name)
	}
	return nil
}

func (c *Cluster) getMaxUnavailableMonPodCount() int32 {
	if c.spec.Mon.Count >= 5 {
		log.NamespacedDebug(c.Namespace, logger, "setting the mon pdb max unavailable count to 2 in case there are 5 or more mons")
		return 2
	}

	return 1
}
