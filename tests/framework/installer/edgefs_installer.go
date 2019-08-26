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

package installer

import (
	"fmt"
	"testing"

	"github.com/rook/rook/tests/framework/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	edgefsCRD = "clusters.edgefs.rook.io"
)

type EdgefsInstaller struct {
	k8shelper *utils.K8sHelper
	manifests *EdgefsManifests
	T         func() *testing.T
}

func NewEdgefsInstaller(k8shelper *utils.K8sHelper, t func() *testing.T) *EdgefsInstaller {
	return &EdgefsInstaller{k8shelper, &EdgefsManifests{}, t}
}

func (h *EdgefsInstaller) InstallEdgefs(systemNamespace, namespace string) error {

	// Creating clusterrolebinding for kubeadm env.
	h.k8shelper.CreateAnonSystemClusterBinding()

	// Install edgefs operator
	if err := h.CreateEdgefsOperator(systemNamespace); err != nil {
		return err
	}

	// Install edgefs cluster instance
	if err := h.CreateEdgefsCluster(namespace); err != nil {
		return err
	}

	return nil
}

func (h *EdgefsInstaller) CreateEdgefsOperator(systemNamespace string) error {
	logger.Infof("starting EdgeFS operator")

	logger.Info("creating EdgeFS CRDs")
	if _, err := h.k8shelper.KubectlWithStdin(h.manifests.GetEdgefsCRDs(), createFromStdinArgs...); err != nil {
		return err
	}

	edgefsOperator := h.manifests.GetEdgefsOperator(systemNamespace)
	_, err := h.k8shelper.KubectlWithStdin(edgefsOperator, createFromStdinArgs...)
	if err != nil {
		return fmt.Errorf("failed to create rook-edgefs-operator pod: %+v ", err)
	}

	if !h.k8shelper.IsCRDPresent(edgefsCRD) {
		return fmt.Errorf("failed to find edgefs CRD %s", edgefsCRD)
	}

	if !h.k8shelper.IsPodInExpectedState("rook-edgefs-operator", systemNamespace, "Running") {
		return fmt.Errorf("rook-edgefs-operator is not running, aborting")
	}

	logger.Infof("Edgefs operator started")
	return nil
}

func (h *EdgefsInstaller) CreateEdgefsCluster(namespace string) error {
	if err := h.k8shelper.CreateNamespace(namespace); err != nil {
		return err
	}

	logger.Infof("starting Edgefs cluster with kubectl and yaml")
	edgefsCluster := h.manifests.GetEdgefsCluster(namespace)
	if _, err := h.k8shelper.KubectlWithStdin(edgefsCluster, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create Edgefs cluster: %+v ", err)
	}

	// Waiting for edgefs manager pod labeled as app=rook-edgefs-mgr (1 mgr per cluster)
	logger.Infof("Waiting for Edgefs Manager pod")
	if err := h.k8shelper.WaitForPodCount("app=rook-edgefs-mgr", namespace, 1); err != nil {
		logger.Errorf("Edgefs cluster manager pods in namespace %s not found", namespace)
		return err
	}

	// Waiting for target pods
	logger.Infof("Waiting for Edgefs Target pods")
	err := h.k8shelper.WaitForLabeledPodsToRun("app=rook-edgefs-target", namespace)
	if err != nil {
		logger.Errorf("Edgefs targets pods in namespace %s are not running", namespace)
		return err
	}

	logger.Infof("Edgefs cluster started")
	return nil
}

func (h *EdgefsInstaller) UninstallEdgefs(systemNamespace, namespace string) {
	logger.Infof("uninstalling Edgefs from namespace %s", namespace)

	err := h.k8shelper.DeleteResourceAndWait(false, "-n", namespace, "cluster.edgefs.rook.io", namespace)
	checkError(h.T(), err, fmt.Sprintf("cannot remove cluster %s", namespace))

	crdCheckerFunc := func() error {
		_, err := h.k8shelper.RookClientset.EdgefsV1().Clusters(namespace).Get(namespace, metav1.GetOptions{})
		return err
	}

	err = h.k8shelper.WaitForCustomResourceDeletion(namespace, crdCheckerFunc)
	checkError(h.T(), err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

	logger.Infof("removing the operator from namespace %s", systemNamespace)
	err = h.k8shelper.DeleteResource("crd", edgefsCRD)
	checkError(h.T(), err, "cannot delete CRDs")

	edgefsOperator := h.manifests.GetEdgefsOperator(systemNamespace)
	_, err = h.k8shelper.KubectlWithStdin(edgefsOperator, deleteFromStdinArgs...)
	checkError(h.T(), err, "cannot uninstall rook-edgefs-operator")

	logger.Info("Removing privileged-psp-user ClusterRoles")
	h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete("privileged-psp-user", nil)

	logger.Info("Removing rook-edgefs-cluster-psp ClusterRoleBinding")
	h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete("rook-edgefs-cluster-psp", nil)

	logger.Info("Removing rook-edgefs-system-psp ClusterRoleBinding")
	h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete("rook-edgefs-system-psp", nil)

	err = h.k8shelper.DeleteResourceAndWait(false, "podsecuritypolicy", "privileged")
	checkError(h.T(), err, fmt.Sprintf("cannot delete podsecuritypolicy `privileged`"))

	err = h.k8shelper.DeleteResourceAndWait(false, "namespace", namespace)
	checkError(h.T(), err, fmt.Sprintf("cannot delete namespace %s", namespace))

}

func (h *EdgefsInstaller) GatherAllEdgefsLogs(systemNamespace, namespace, testName string) {
	if !h.T().Failed() && Env.Logs != "all" {
		return
	}
	logger.Infof("Gathering all logs from edgefs cluster %s", namespace)
	h.k8shelper.GetLogsFromNamespace(systemNamespace, testName, Env.HostType)
	h.k8shelper.GetLogsFromNamespace(namespace, testName, Env.HostType)
}
