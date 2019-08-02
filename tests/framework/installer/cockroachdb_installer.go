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

package installer

import (
	"fmt"
	"testing"

	"github.com/rook/rook/tests/framework/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	cockroachDBCRD = "clusters.cockroachdb.rook.io"
)

type CockroachDBInstaller struct {
	k8shelper *utils.K8sHelper
	manifests *CockroachDBManifests
	T         func() *testing.T
}

func NewCockroachDBInstaller(k8shelper *utils.K8sHelper, t func() *testing.T) *CockroachDBInstaller {
	return &CockroachDBInstaller{k8shelper, &CockroachDBManifests{}, t}
}

func (h *CockroachDBInstaller) InstallCockroachDB(systemNamespace, namespace string, count int) error {
	h.k8shelper.CreateAnonSystemClusterBinding()

	// install hostpath provisioner if there isn't already a default storage class
	defaultExists, err := h.k8shelper.IsDefaultStorageClassPresent()
	if err != nil {
		return err
	} else if !defaultExists {
		if err := InstallHostPathProvisioner(h.k8shelper); err != nil {
			return err
		}
	} else {
		logger.Info("skipping install of host path provisioner because a default storage class already exists")
	}

	// install cockroachdb operator
	if err := h.CreateCockroachDBOperator(systemNamespace); err != nil {
		return err
	}

	// install cockroachdb cluster instance
	if err := h.CreateCockroachDBCluster(namespace, count); err != nil {
		return err
	}

	return nil
}

func (h *CockroachDBInstaller) CreateCockroachDBOperator(namespace string) error {
	logger.Infof("starting cockroachDB operator")

	logger.Info("creating cockroachDB CRDs")
	if _, err := h.k8shelper.KubectlWithStdin(h.manifests.GetCockroachDBCRDs(), createFromStdinArgs...); err != nil {
		return err
	}

	cockroachDBOperator := h.manifests.GetCockroachDBOperator(namespace)
	_, err := h.k8shelper.KubectlWithStdin(cockroachDBOperator, createFromStdinArgs...)
	if err != nil {
		return fmt.Errorf("failed to create rook-cockroachdb-operator pod: %+v ", err)
	}

	if !h.k8shelper.IsCRDPresent(cockroachDBCRD) {
		return fmt.Errorf("failed to find cockroachdb CRD %s", cockroachDBCRD)
	}

	if !h.k8shelper.IsPodInExpectedState("rook-cockroachdb-operator", namespace, "Running") {
		return fmt.Errorf("rook-cockroachdb-operator is not running, aborting")
	}

	logger.Infof("cockroachdb operator started")
	return nil
}

func (h *CockroachDBInstaller) CreateCockroachDBCluster(namespace string, count int) error {
	if err := h.k8shelper.CreateNamespace(namespace); err != nil {
		return err
	}

	logger.Infof("starting cockroachdb cluster with kubectl and yaml")
	cockroachDBCluster := h.manifests.GetCockroachDBCluster(namespace, count)
	if _, err := h.k8shelper.KubectlWithStdin(cockroachDBCluster, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create cockroachdb cluster: %+v ", err)
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-cockroachdb", namespace, count); err != nil {
		logger.Errorf("cockroachdb cluster pods in namespace %s not found", namespace)
		return err
	}

	err := h.k8shelper.WaitForLabeledPodsToRun("app=rook-cockroachdb", namespace)
	if err != nil {
		logger.Errorf("cockroachdb cluster pods in namespace %s are not running", namespace)
		return err
	}

	logger.Infof("cockroachdb cluster started")
	return nil
}

func (h *CockroachDBInstaller) UninstallCockroachDB(systemNamespace, namespace string) {
	logger.Infof("uninstalling cockroachdb from namespace %s", namespace)

	err := h.k8shelper.DeleteResourceAndWait(false, "-n", namespace, "cluster.cockroachdb.rook.io", namespace)
	checkError(h.T(), err, fmt.Sprintf("cannot remove cluster %s", namespace))

	crdCheckerFunc := func() error {
		_, err := h.k8shelper.RookClientset.CockroachdbV1alpha1().Clusters(namespace).Get(namespace, metav1.GetOptions{})
		return err
	}
	err = h.k8shelper.WaitForCustomResourceDeletion(namespace, crdCheckerFunc)
	checkError(h.T(), err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

	err = h.k8shelper.DeleteResourceAndWait(false, "namespace", namespace)
	checkError(h.T(), err, fmt.Sprintf("cannot delete namespace %s", namespace))

	logger.Infof("removing the operator from namespace %s", systemNamespace)
	err = h.k8shelper.DeleteResource("crd", "clusters.cockroachdb.rook.io")
	checkError(h.T(), err, "cannot delete CRDs")

	cockroachDBOperator := h.manifests.GetCockroachDBOperator(systemNamespace)
	_, err = h.k8shelper.KubectlWithStdin(cockroachDBOperator, deleteFromStdinArgs...)
	checkError(h.T(), err, "cannot uninstall rook-cockroachdb-operator")

	err = UninstallHostPathProvisioner(h.k8shelper)
	checkError(h.T(), err, "cannot uninstall hostpath provisioner")

	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)
	logger.Infof("done removing the operator from namespace %s", systemNamespace)
}

func (h *CockroachDBInstaller) GatherAllCockroachDBLogs(systemNamespace, namespace, testName string) {
	if !h.T().Failed() && Env.Logs != "all" {
		return
	}
	logger.Infof("Gathering all logs from cockroachdb cluster %s", namespace)
	h.k8shelper.GetLogsFromNamespace(systemNamespace, testName, Env.HostType)
	h.k8shelper.GetLogsFromNamespace(namespace, testName, Env.HostType)
}
