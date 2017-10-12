/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"time"

	"k8s.io/client-go/kubernetes"

	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/objects"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
)

const (
	rookOperatorCreatedTpr = "cluster.rook.io"
	rookOperatorCreatedCrd = "clusters.rook.io"
)

var (
	logger         = capnslog.NewPackageLogger("github.com/rook/rook", "installer")
	createArgs     = []string{"create", "-f", "-"}
	deleteArgs     = []string{"delete", "-f", "-"}
	helmChartName  = "local/rook"
	helmDeployName = "rook"
	env            objects.EnvironmentManifest
)

//InstallHelper wraps installing and uninstalling rook on a platform
type InstallHelper struct {
	k8shelper   *utils.K8sHelper
	installData *InstallData
	helmHelper  *utils.HelmHelper
	Env         objects.EnvironmentManifest
	k8sVersion  string
	T           func() *testing.T
}

func init() {
	env = objects.NewManifest()
}

//CreateK8sRookOperator creates rook-operator via kubectl
func (h *InstallHelper) CreateK8sRookOperator(namespace string) (err error) {
	logger.Infof("Starting Rook Operator")
	//creating clusterrolebinding for kubeadm env.
	h.k8shelper.CreateAnonSystemClusterBinding()

	rookOperator := h.installData.GetRookOperator(namespace)

	_, err = h.k8shelper.KubectlWithStdin(rookOperator, createArgs...)

	if err != nil {
		return fmt.Errorf("Failed to create rook-operator pod : %v ", err)
	}

	if h.k8shelper.VersionAtLeast("v1.7.0") {
		if !h.k8shelper.IsCRDPresent(rookOperatorCreatedCrd) {
			return fmt.Errorf("Failed to start Rook Operator; k8s CustomResourceDefinition did not appear")
		}
	} else {
		if !h.k8shelper.IsThirdPartyResourcePresent(rookOperatorCreatedTpr) {
			return fmt.Errorf("Failed to start Rook Operator; k8s thirdpartyresource did not appear")
		}
	}

	logger.Infof("Rook Operator started")

	return nil
}

//CreateK8sRookOperatorViaHelm creates rook operator via Helm chart named local/rook present in local repo
func (h *InstallHelper) CreateK8sRookOperatorViaHelm(namespace string) error {
	//creating clusterrolebinding for kubeadm env.
	h.k8shelper.CreateAnonSystemClusterBinding()

	helmTag, err := h.helmHelper.GetLocalRookHelmChartVersion(helmChartName)

	if err != nil {
		return fmt.Errorf("Failed to get Version of helm chart %v, err : %v", helmChartName, err)
	}

	err = h.helmHelper.InstallLocalRookHelmChart(helmChartName, helmDeployName, helmTag, namespace)
	if err != nil {
		return fmt.Errorf("failed toinstall rook operator via helm, err : %v", err)

	}

	if h.k8shelper.VersionAtLeast("v1.7.0") {
		if !h.k8shelper.IsCRDPresent(rookOperatorCreatedCrd) {
			return fmt.Errorf("Failed to start Rook Operator; k8s CustomResourceDefinition did not appear")
		}
	} else {
		if !h.k8shelper.IsThirdPartyResourcePresent(rookOperatorCreatedTpr) {
			return fmt.Errorf("Failed to start Rook Operator; k8s thirdpartyresource did not appear")
		}
	}

	return nil
}

//CreateK8sRookToolbox creates rook-tools via kubectl
func (h *InstallHelper) CreateK8sRookToolbox(namespace string) (err error) {
	logger.Infof("Starting Rook toolbox")

	rookToolbox := h.installData.GetRookToolBox(namespace)

	_, err = h.k8shelper.KubectlWithStdin(rookToolbox, createArgs...)

	if err != nil {
		return fmt.Errorf("Failed to create rook-toolbox pod : %v ", err)
	}

	if !h.k8shelper.IsPodRunning("rook-tools", namespace) {
		return fmt.Errorf("Rook Toolbox couldn't start")
	}
	logger.Infof("Rook Toolbox started")

	return nil
}

//CreateK8sRookCluster creates rook cluster via kubectl
func (h *InstallHelper) CreateK8sRookCluster(namespace, storeType string) (err error) {
	logger.Infof("Starting Rook Cluster")

	rookCluster := h.installData.GetRookCluster(namespace, storeType)

	_, err = h.k8shelper.KubectlWithStdin(rookCluster, createArgs...)

	if err != nil {
		return fmt.Errorf("Failed to create rook cluster : %v ", err)
	}

	if !h.k8shelper.IsServiceUp("rook-api", namespace) {
		logger.Infof("Rook Cluster couldn't start")
	} else {
		logger.Infof("Rook Cluster started")
	}

	return nil
}

func SystemNamespace(namespace string) string {
	return fmt.Sprintf("%s-system", namespace)
}

//InstallRookOnK8s installs rook on k8s
func (h *InstallHelper) InstallRookOnK8s(namespace, storeType string) (bool, error) {
	//flag used for local debuggin purpose, when rook is pre-installed
	skipRookInstall := strings.EqualFold(h.Env.SkipInstallRook, "true")
	if skipRookInstall {
		return true, nil
	}

	k8sversion := h.k8shelper.GetK8sServerVersion()

	logger.Infof("Installing rook on k8s %s", k8sversion)

	//Create rook operator
	err := h.CreateK8sRookOperator(SystemNamespace(namespace))
	if err != nil {
		logger.Errorf("Rook Operator not installed ,error -> %v", err)
		return false, err
	}

	if !h.k8shelper.IsPodInExpectedState("rook-operator", SystemNamespace(namespace), "Running") {
		fmt.Println("rook-operator is not running")
		h.k8shelper.GetRookLogs("rook-operator", h.Env.HostType, SystemNamespace(namespace), "test-setup")
		logger.Error("rook-operator is not Running, abort!")
		return false, err
	}

	time.Sleep(10 * time.Second)

	//Create rook cluster
	err = h.CreateK8sRookCluster(namespace, storeType)
	if err != nil {
		logger.Errorf("Rook cluster %s not installed ,error -> %v", namespace, err)
		return false, err
	}

	time.Sleep(5 * time.Second)

	//Create rook client
	err = h.CreateK8sRookToolbox(namespace)
	if err != nil {
		logger.Errorf("Rook toolbox in cluster %s not installed ,error -> %v", namespace, err)
		return false, err
	}
	logger.Infof("installed rook operator and cluster : %s on k8s %s", namespace, h.k8sVersion)
	return true, nil
}

//UninstallRookFromK8s uninstalls rook from k8s
func (h *InstallHelper) UninstallRookFromK8s(namespace string, helmInstalled bool) {
	//flag used for local debugging purpose, when rook is pre-installed
	skipRookInstall := strings.EqualFold(h.Env.SkipInstallRook, "true")
	if skipRookInstall {
		return
	}

	logger.Infof("Uninstalling Rook")
	k8sHelp, err := utils.CreateK8sHelper(h.T)
	assert.NoError(h.T(), err, "cannot uninstall rook err ->  %v", err)

	if helmInstalled {
		err = h.helmHelper.DeleteLocalRookHelmChart(helmDeployName)
		assert.NoError(h.T(), err, "cannot uninstall Rook helm chart err -> %v", err)
	} else {
		rookOperator := h.installData.GetRookOperator(SystemNamespace(namespace))
		_, err = h.k8shelper.KubectlWithStdin(rookOperator, deleteArgs...)
		assert.NoError(h.T(), err, "cannot uninstall rook-operator err -> %v", err)
	}
	_, err = k8sHelp.DeleteResource([]string{"-n", namespace, "cluster", namespace})
	assert.NoError(h.T(), err, "cannot remove cluster %s in namespace %s  err -> %v", namespace, namespace, err)

	_, err = k8sHelp.DeleteResource([]string{"-n", namespace, "serviceaccount", "rook-api"})
	assert.NoError(h.T(), err, "cannot remove serviceaccount rook-api in namespace %s  err -> %v", namespace, err)

	_, err = k8sHelp.DeleteResource([]string{"-n", namespace, "serviceaccount", "rook-ceph-osd"})
	assert.NoError(h.T(), err, "cannot remove serviceaccount rook-ceph-osd in namespace %s  err -> %v", namespace, err)

	if h.k8shelper.VersionAtLeast("v1.6.0") {
		err = h.k8shelper.DeleteRoleAndBindings("rook-api", namespace)
		assert.NoError(h.T(), err, "rook-api cluster role and binding cannot be deleted: %+v", err)

		err = k8sHelp.DeleteRoleAndBindings("rook-ceph-osd", namespace)
		assert.NoError(h.T(), err, "rook-ceph-osd cluster role and binding cannot be deleted: %+v", err)
	}

	if h.k8shelper.VersionAtLeast("v1.7.0") {
		_, err = k8sHelp.DeleteResource([]string{"crd", "clusters.rook.io", "pools.rook.io", "objectstores.rook.io", "filesystems.rook.io"})
		assert.NoError(h.T(), err, "cannot delete CRDs err -> %+v", err)
	} else {
		_, err = k8sHelp.DeleteResource([]string{"thirdpartyresources", "cluster.rook.io", "pool.rook.io", "objectstore.rook.io", "filesystem.rook.io"})
		assert.NoError(h.T(), err, "cannot delete TPRs err -> %+v", err)
	}

	_, err = k8sHelp.DeleteResource([]string{"namespace", namespace})
	assert.NoError(h.T(), err, "cannot delete namespace %s err -> %+v", namespace, err)

	isRookUninstalled := k8sHelp.WaitUntilPodInNamespaceIsDeleted("rook-ceph-mon", namespace)
	isNameSpaceDeleted := k8sHelp.WaitUntilNameSpaceIsDeleted(namespace)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)

	assert.True(h.T(), isRookUninstalled, "make sure rook is uninstalled")
	assert.True(h.T(), isNameSpaceDeleted, "make sure rook is uninstalled")

	if isRookUninstalled && isNameSpaceDeleted {
		logger.Infof("Rook cluster %s uninstalled successfully", namespace)
		return
	}
	logger.Errorf("Rook cluster %s not uninstalled", namespace)
}

//CleanupCluster deletes a rook cluster for a namespace
func (h *InstallHelper) CleanupCluster(clusterName string) {

	logger.Infof("Uninstalling All Rook Clusters - %s", clusterName)
	_, err := h.k8shelper.DeleteResource([]string{"-n", clusterName, "cluster", clusterName})
	if err != nil {
		logger.Errorf("Rook Cluster  %s cannot be deleted,err -> %v", clusterName, err)
	}

	_, err = h.k8shelper.DeleteResource([]string{"-n", clusterName, "serviceaccount", "rook-api"})
	if err != nil {
		logger.Errorf("rook-api service account in namespace %s cannot be deleted,err -> %v", clusterName, err)
	}
	_, err = h.k8shelper.DeleteResource([]string{"-n", clusterName, "serviceaccount", "rook-ceph-osd"})
	if err != nil {
		logger.Errorf("rook-ceph-osd service account in namespace %s cannot be deleted,err -> %v", clusterName, err)
		panic(err)
	}

	_, err = h.k8shelper.DeleteResource([]string{"namespace", clusterName})
	if err != nil {
		logger.Errorf("namespace  %s cannot be deleted,err -> %v", clusterName, err)
	}

	h.k8shelper.WaitUntilPodInNamespaceIsDeleted("rook-ceph-mon", clusterName)
	h.k8shelper.WaitUntilNameSpaceIsDeleted(clusterName)

}

//NewK8sRookhelper creates new instance of InstallHelper
func NewK8sRookhelper(clientset *kubernetes.Clientset, t func() *testing.T) *InstallHelper {

	version, err := clientset.ServerVersion()
	if err != nil {
		logger.Infof("failed to get kubectl server version. %+v", err)
	}

	k8shelp, err := utils.CreateK8sHelper(t)
	if err != nil {
		panic("failed to get kubectl client :" + err.Error())
	}
	return &InstallHelper{
		k8shelper:   k8shelp,
		installData: NewK8sInstallData(),
		helmHelper:  utils.NewHelmHelper(),
		Env:         env,
		k8sVersion:  version.String(),
		T:           t,
	}
}
