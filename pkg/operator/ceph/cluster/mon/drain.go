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
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	monPDBName = "rook-ceph-mon-pdb"
)

func (c *Cluster) reconcileMonPDB() (*cephclient.MonStatusResponse, error) {
	if !c.spec.DisruptionManagement.ManagePodBudgets {
		// TODO: Delete mon PDB
		return nil, nil
	}

	monCount := c.spec.Mon.Count
	if monCount <= 2 {
		log.NamespacedDebug(c.Namespace, logger, "managePodBudgets is set, but mon-count <= 2. Not creating a disruptionbudget for Mons")
		return nil, nil
	}

	// get the status and check for quorum
	quorumStatus, err := cephclient.GetMonQuorumStatus(c.context, c.ClusterInfo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get mon quorum status")
	}
	log.NamespacedDebug(c.Namespace, logger, "Mon quorum status: %+v", quorumStatus)

	downMonCount := len(quorumStatus.MonMap.Mons) - len(quorumStatus.Quorum)

	// If any mons are currently down, reduce the number of mons that can be drained
	// to prevent more drains until the mon quorum can sufficiently handle another
	// mon going down. This may block drains temporarily for longer than strictly needed,
	// but will also prevent race conditions that would allow quorum to be lost
	// if another node is drained before a down mon is fully back in quorum.
	// nolint:gosec // G115 - casting will not cause overflow
	allowedDown := c.getMaxUnavailableMonPodCount() - int32(downMonCount)
	if allowedDown < 0 {
		allowedDown = 0
	}

	// only update the mon pdb if the maxunavailable changed
	currentMaxUnavailable, err := c.getExistingMaxUnavailable()
	if err != nil {
		log.NamespacedWarning(c.Namespace, logger, "failed to get current mon pdb maxunavailable, proceeding with update. %v", err)
	} else if currentMaxUnavailable == allowedDown {
		log.NamespacedDebug(c.Namespace, logger, "mon pdb maxunavailable is already set to %d", allowedDown)
		return &quorumStatus, nil
	}

	// update the mon pdb since the maxunavailable changed
	log.NamespacedInfo(c.Namespace, logger, "setting mon pdb maxUnavailable=%d (%d mons down)", allowedDown, downMonCount)
	op, err := c.createOrUpdateMonPDB(allowedDown)
	if err != nil {
		return &quorumStatus, errors.Wrapf(err, "failed to reconcile mon pdb on op %q", op)
	}
	return &quorumStatus, nil
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

func (c *Cluster) getExistingMaxUnavailable() (int32, error) {
	pdbRequest := types.NamespacedName{Name: monPDBName, Namespace: c.Namespace}
	existingPDB := &policyv1.PodDisruptionBudget{}
	err := c.context.Client.Get(c.ClusterInfo.Context, pdbRequest, existingPDB)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.NamespacedDebug(c.Namespace, logger, "mon pdb %q not found", monPDBName)
			return -1, nil
		}
		return 0, errors.Wrapf(err, "failed to get mon pdb %q", existingPDB.Name)
	}
	log.NamespacedDebug(c.Namespace, logger, "existing mon pdb maxUnavailable=%d", existingPDB.Spec.MaxUnavailable.IntVal)
	return existingPDB.Spec.MaxUnavailable.IntVal, nil
}

func (c *Cluster) getMaxUnavailableMonPodCount() int32 {
	if c.spec.Mon.Count >= 5 {
		log.NamespacedDebug(c.Namespace, logger, "setting the mon pdb max unavailable count to 2 in case there are 5 or more mons")
		return 2
	}

	return 1
}
