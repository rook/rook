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

package mgr

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/k8sutil"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	mgrPDBName = "rook-ceph-mgr-pdb"
)

func (c *Cluster) reconcileMgrPDB() error {
	var maxUnavailable int32 = 1
	usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(c.context.Clientset)
	if err != nil {
		return errors.Wrap(err, "failed to fetch pdb version")
	}
	objectMeta := metav1.ObjectMeta{
		Name:      mgrPDBName,
		Namespace: c.clusterInfo.Namespace,
	}
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{k8sutil.AppAttr: AppName},
	}
	if usePDBV1Beta1 {
		pdb := &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: objectMeta,
		}
		mutateFunc := func() error {
			pdb.Spec = policyv1beta1.PodDisruptionBudgetSpec{
				Selector:       selector,
				MaxUnavailable: &intstr.IntOrString{IntVal: maxUnavailable},
			}
			return nil
		}
		op, err := controllerutil.CreateOrUpdate(context.TODO(), c.context.Client, pdb, mutateFunc)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile mgr pdb on op %q", op)
		}
		return nil
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
	op, err := controllerutil.CreateOrUpdate(context.TODO(), c.context.Client, pdb, mutateFunc)
	if err != nil {
		return errors.Wrapf(err, "failed to reconcile mgr pdb on op %q", op)
	}
	return nil
}

func (c *Cluster) deleteMgrPDB() {
	pdbRequest := types.NamespacedName{Name: mgrPDBName, Namespace: c.clusterInfo.Namespace}
	usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(c.context.Clientset)
	if err != nil {
		logger.Errorf("failed to fetch pdb version. %v", err)
		return
	}
	if usePDBV1Beta1 {
		mgrPDB := &policyv1beta1.PodDisruptionBudget{}
		err := c.context.Client.Get(context.TODO(), pdbRequest, mgrPDB)
		if err != nil {
			if !kerrors.IsNotFound(err) {
				logger.Errorf("failed to get mgr pdb %q. %v", mgrPDBName, err)
			}
			return
		}
		logger.Debugf("ensuring the mgr pdb %q is deleted", mgrPDBName)
		err = c.context.Client.Delete(context.TODO(), mgrPDB)
		if err != nil {
			logger.Errorf("failed to delete mgr pdb %q. %v", mgrPDBName, err)
			return
		}
	}
	mgrPDB := &policyv1.PodDisruptionBudget{}
	err = c.context.Client.Get(context.TODO(), pdbRequest, mgrPDB)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			logger.Errorf("failed to get mgr pdb %q. %v", mgrPDBName, err)
		}
		return
	}
	logger.Debugf("ensuring the mgr pdb %q is deleted", mgrPDBName)
	err = c.context.Client.Delete(context.TODO(), mgrPDB)
	if err != nil {
		logger.Errorf("failed to delete mgr pdb %q. %v", mgrPDBName, err)
	}
}
