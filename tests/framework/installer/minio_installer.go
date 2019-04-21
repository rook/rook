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
	minioCRD = "objectstores.minio.rook.io"
)

type MinioInstaller struct {
	k8shelper *utils.K8sHelper
	manifests *MinioManifests
	T         func() *testing.T
}

func NewMinioInstaller(k8shelper *utils.K8sHelper, t func() *testing.T) *MinioInstaller {
	return &MinioInstaller{k8shelper, &MinioManifests{}, t}
}

func (h *MinioInstaller) InstallMinio(systemNamespace, namespace string, count int) error {
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

	if err := h.CreateMinioOperator(systemNamespace); err != nil {
		return err
	}

	if err := h.CreateMinioCluster(namespace, count); err != nil {
		return err
	}

	return nil
}

func (h *MinioInstaller) CreateMinioOperator(namespace string) error {
	logger.Infof("starting minio operator")

	logger.Info("creating minio CRDs")
	if _, err := h.k8shelper.KubectlWithStdin(h.manifests.GetMinioCRDs(), createFromStdinArgs...); err != nil {
		return err
	}

	minioOperator := h.manifests.GetMinioOperator(namespace)
	_, err := h.k8shelper.KubectlWithStdin(minioOperator, createFromStdinArgs...)
	if err != nil {
		return fmt.Errorf("failed to create rook-minio-operator pod: %+v ", err)
	}

	if !h.k8shelper.IsCRDPresent(minioCRD) {
		return fmt.Errorf("failed to find minio CRD %s", minioCRD)
	}

	if !h.k8shelper.IsPodInExpectedState("rook-minio-operator", namespace, "Running") {
		return fmt.Errorf("rook-minio-operator is not running, aborting")
	}

	logger.Infof("minio operator started")
	return nil
}

func (h *MinioInstaller) CreateMinioCluster(namespace string, count int) error {
	if err := h.k8shelper.CreateNamespace(namespace); err != nil {
		return err
	}

	logger.Infof("starting minio cluster with kubectl and yaml")
	minioCluster := h.manifests.GetMinioCluster(namespace, count)
	if _, err := h.k8shelper.KubectlWithStdin(minioCluster, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create minio cluster: %+v ", err)
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-minio", namespace, count); err != nil {
		logger.Errorf("minio cluster pods in namespace %s not found", namespace)
		return err
	}

	err := h.k8shelper.WaitForLabeledPodsToRun("app=rook-minio", namespace)
	if err != nil {
		logger.Errorf("minio cluster pods in namespace %s are not running", namespace)
		return err
	}

	logger.Infof("minio cluster started")
	return nil
}

func (h *MinioInstaller) UninstallMinio(systemNamespace, namespace string) {
	logger.Infof("uninstalling minio from namespace %s", namespace)

	err := h.k8shelper.DeleteResourceAndWait(false, "-n", namespace, "objectstores.minio.rook.io", namespace)
	checkError(h.T(), err, fmt.Sprintf("cannot remove cluster %s", namespace))

	crdCheckerFunc := func() error {
		_, err := h.k8shelper.RookClientset.MinioV1alpha1().ObjectStores(namespace).Get(namespace, metav1.GetOptions{})
		return err
	}
	err = h.k8shelper.WaitForCustomResourceDeletion(namespace, crdCheckerFunc)
	checkError(h.T(), err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

	err = h.k8shelper.DeleteResourceAndWait(false, "namespace", namespace)
	checkError(h.T(), err, fmt.Sprintf("cannot delete namespace %s", namespace))

	logger.Infof("removing the operator from namespace %s", systemNamespace)
	err = h.k8shelper.DeleteResource("crd", "objectstores.minio.rook.io")
	checkError(h.T(), err, "cannot delete CRDs")

	minioOperator := h.manifests.GetMinioOperator(systemNamespace)
	_, err = h.k8shelper.KubectlWithStdin(minioOperator, deleteFromStdinArgs...)
	checkError(h.T(), err, "cannot uninstall rook-minio-operator")

	err = UninstallHostPathProvisioner(h.k8shelper)
	checkError(h.T(), err, "cannot uninstall hostpath provisioner")

	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)
	logger.Infof("done removing the operator from namespace %s", systemNamespace)
}

func (h *MinioInstaller) GatherAllMinioLogs(systemNamespace, namespace, testName string) {
	logger.Infof("Gathering all logs from minio cluster %s", namespace)
	h.k8shelper.GetLogs("rook-minio-operator", Env.HostType, systemNamespace, testName)
	h.k8shelper.GetLogs("rook-minio", Env.HostType, namespace, testName)
}
