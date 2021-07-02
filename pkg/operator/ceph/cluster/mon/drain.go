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
	"context"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/k8sutil"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	monPDBName = "rook-ceph-mon-pdb"
)

func (c *Cluster) reconcileMonPDB() error {
	if !c.spec.DisruptionManagement.ManagePodBudgets {
		//TODO: Delete mon PDB
		return nil
	}

	monCount := c.spec.Mon.Count
	if monCount <= 2 {
		logger.Debug("managePodBudgets is set, but mon-count <= 2. Not creating a disruptionbudget for Mons")
		return nil
	}

	op, err := c.createOrUpdateMonPDB(1)
	if err != nil {
		return errors.Wrapf(err, "failed to reconcile mon pdb on op %q", op)
	}
	return nil
}

func (c *Cluster) createOrUpdateMonPDB(maxUnavailable int32) (controllerutil.OperationResult, error) {
	usePDBV1, err := UsePDBV1Version(c.context.Clientset)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}
	objectMeta := metav1.ObjectMeta{
		Name:      monPDBName,
		Namespace: c.Namespace,
	}
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{k8sutil.AppAttr: AppName},
	}
	if usePDBV1 {
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: objectMeta}

		mutateFunc := func() error {
			pdb.Spec = policyv1.PodDisruptionBudgetSpec{
				Selector:       selector,
				MaxUnavailable: &intstr.IntOrString{IntVal: maxUnavailable},
			}
			return nil
		}
		return controllerutil.CreateOrUpdate(context.TODO(), c.context.Client, pdb, mutateFunc)
	}
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: objectMeta}

	mutateFunc := func() error {
		pdb.Spec = policyv1beta1.PodDisruptionBudgetSpec{
			Selector:       selector,
			MaxUnavailable: &intstr.IntOrString{IntVal: maxUnavailable},
		}
		return nil
	}
	return controllerutil.CreateOrUpdate(context.TODO(), c.context.Client, pdb, mutateFunc)
}

// blockMonDrain makes MaxUnavailable in mon PDB to 0 to block any voluntary mon drains
func (c *Cluster) blockMonDrain(request types.NamespacedName) error {
	if !c.spec.DisruptionManagement.ManagePodBudgets {
		return nil
	}
	logger.Info("prevent voluntary mon drain while failing over")
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
	logger.Info("allow voluntary mon drain after failover")
	// change MaxUnavailable mon PDB to 1
	_, err := c.createOrUpdateMonPDB(1)
	if err != nil {
		return errors.Wrapf(err, "failed to update MaxUnavailable for mon PDB %q", request.Name)
	}
	return nil
}

func UsePDBV1Version(Clientset kubernetes.Interface) (bool, error) {
	k8sVersion, err := k8sutil.GetK8SVersion(Clientset)
	if err != nil {
		return false, errors.Wrap(err, "failed to fetch k8s version")
	}
	logger.Debugf("kubernetes version fetched %v", k8sVersion)
	// minimum k8s version required for v1 PodDisruptionBudget is 'v1.21.0'. Apply v1 if k8s version is at least 'v1.21.0', else apply v1beta1 PodDisruptionBudget.
	minVersionForPDBV1 := "1.21.0"
	usePDBV1 := k8sVersion.AtLeast(version.MustParseSemantic(minVersionForPDBV1))
	if usePDBV1 {
		return true, nil
	}
	return false, nil
}
