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
	"time"

	"k8s.io/client-go/kubernetes"

	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/objects"
	"github.com/rook/rook/tests/framework/utils"
	utilversion "k8s.io/kubernetes/pkg/util/version"
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
}

func init() {
	env = objects.NewManifest()
}

//CreateK8sRookOperator creates rook-operator via kubectl
func (h *InstallHelper) CreateK8sRookOperator() (err error) {
	logger.Infof("Starting Rook Operator")

	rookOperator := h.installData.getRookOperator(h.k8shelper.GetK8sServerVersion())

	_, err = h.k8shelper.KubectlWithStdin(rookOperator, createArgs...)

	if err != nil {
		return fmt.Errorf("Failed to create rook-operator pod : %v ", err)
	}
	kubeVersion := utilversion.MustParseSemantic(h.k8shelper.GetK8sServerVersion())

	if kubeVersion.AtLeast(utilversion.MustParseSemantic("v1.7.0")) {
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

//CreateK8sRookOperatorViaHelm creates rook operator via Execute chart named local/rook present in local repo
func (h *InstallHelper) CreateK8sRookOperatorViaHelm() (err error) {
	//creating clusterrolebinding for kubeadm env.
	h.k8shelper.Kubectl([]string{"create", "clusterrolebinding", "anon-user-access", "--clusterrole", "cluster-admin", "--user", "system:anonymous"}...)

	helmTag, err := h.helmHelper.GetLocalRookHelmChartVersion(helmChartName)

	if err != nil {
		return fmt.Errorf("Failed to get Version of helm chart %v, err : %v", helmChartName, err)
	}

	err = h.helmHelper.InstallLocalRookHelmChart(helmChartName, helmDeployName, helmTag, "rook")
	if err != nil {
		return fmt.Errorf("failed toinstall rook operator via helm, err : %v", err)

	}

	kubeVersion := utilversion.MustParseSemantic(h.k8shelper.GetK8sServerVersion())

	if kubeVersion.AtLeast(utilversion.MustParseSemantic("v1.7.0")) {
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
func (h *InstallHelper) CreateK8sRookToolbox() (err error) {
	logger.Infof("Starting Rook toolbox")

	rookToolbox := h.installData.getRookToolBox()

	_, err = h.k8shelper.KubectlWithStdin(rookToolbox, createArgs...)

	if err != nil {
		return fmt.Errorf("Failed to create rook-toolbox pod : %v ", err)
	}

	if !h.k8shelper.IsPodRunningInNamespace("rook-tools") {
		return fmt.Errorf("Rook Toolbox couldn't start")
	}
	logger.Infof("Rook Toolbox started")

	return nil
}

//CreateK8sRookCluster creates rook cluster via kubectl
func (h *InstallHelper) CreateK8sRookCluster() (err error) {
	logger.Infof("Starting Rook Cluster")

	rookCluster := h.installData.getRookCluster()

	_, err = h.k8shelper.KubectlWithStdin(rookCluster, createArgs...)

	if err != nil {
		return fmt.Errorf("Failed to create rook cluster : %v ", err)
	}

	if !h.k8shelper.IsServiceUpInNameSpace("rook-api") {
		logger.Infof("Rook Cluster couldn't start")
	} else {
		logger.Infof("Rook Cluster started")
	}

	return nil
}

//InstallRookOnK8s installs rook on k8s
func (h *InstallHelper) InstallRookOnK8s() (err error) {
	//creating clusterrolebinding for kubeadm env.
	h.k8shelper.Kubectl([]string{"create", "clusterrolebinding", "anon-user-access", "--clusterrole", "cluster-admin", "--user", "system:anonymous"}...)

	//flag used for local debuggin purpose, when rook is pre-installed
	skipRookInstall := strings.EqualFold(h.Env.SkipInstallRook, "true")
	if skipRookInstall {
		return
	}

	k8sversion := h.k8shelper.GetK8sServerVersion()

	logger.Infof("Installing rook on k8s %s", k8sversion)
	//Create rook operator
	if err != nil {
		panic(err)
	}

	err = h.CreateK8sRookOperator()
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	if !h.k8shelper.IsPodInExpectedState("rook-operator", "default", "Running") {
		fmt.Println("rook-operator is not running")
		h.k8shelper.GetRookLogs("rook-operator", "default", "test-setup")
		panic("rook-operator is not Running, abort!")
	}

	time.Sleep(10 * time.Second)

	//Create rook cluster
	err = h.CreateK8sRookCluster()
	if err != nil {
		panic(err)
	}

	time.Sleep(5 * time.Second)

	//Create rook client
	err = h.CreateK8sRookToolbox()
	if err != nil {
		panic(err)
	}
	logger.Infof("installed rook on k8s %s", h.Env.K8sVersion)
	return nil
}

//UninstallRookFromK8s uninstalls rook from k8s
func (h *InstallHelper) UninstallRookFromK8s() {
	//flag used for local debugging purpose, when rook is pre-installed
	skipRookInstall := strings.EqualFold(h.Env.SkipInstallRook, "true")
	if skipRookInstall {
		return
	}
	k8sVersion := h.k8shelper.GetK8sServerVersion()
	serverVersion, err := h.k8shelper.Clientset.Discovery().ServerVersion()
	if err != nil {
		panic(err)
	}
	kubeVersion := utilversion.MustParseSemantic(serverVersion.GitVersion)

	logger.Infof("Uninstalling Rook")
	k8sHelp, err := utils.CreateK8sHelper()
	if err != nil {
		panic(err)
	}
	rookOperator := h.installData.getRookOperator(k8sVersion)

	_, err = h.k8shelper.KubectlWithStdin(rookOperator, deleteArgs...)
	if err != nil {
		panic(err)
	}
	_, err = k8sHelp.DeleteResource([]string{"-n", "rook", "cluster", "rook"})
	if err != nil {
		panic(err)
	}
	_, err = k8sHelp.DeleteResource([]string{"-n", "rook", "serviceaccount", "rook-api"})
	if err != nil {
		panic(err)
	}
	if kubeVersion.AtLeast(utilversion.MustParseSemantic("v1.6.0")) {
		_, err = k8sHelp.DeleteResource([]string{"clusterrole", "rook-api"})
		if err != nil {
			panic(err)
		}
		_, err = k8sHelp.DeleteResource([]string{"clusterrolebinding", "rook-api"})
		if err != nil {
			panic(err)
		}
	}

	if kubeVersion.AtLeast(utilversion.MustParseSemantic("v1.7.0")) {
		_, err = k8sHelp.DeleteResource([]string{"crd", "clusters.rook.io", "pools.rook.io"})
		if err != nil {
			panic(err)
		}
	} else {
		_, err = k8sHelp.DeleteResource([]string{"thirdpartyresources", "cluster.rook.io", "pool.rook.io"})
		if err != nil {
			panic(err)
		}
	}
	_, err = k8sHelp.DeleteResource([]string{"secret", "rook-rook-user"})
	if err != nil {
		panic(err)
	}
	_, err = k8sHelp.DeleteResource([]string{"namespace", "rook"})
	if err != nil {
		panic(err)
	}

	isRookUninstalled := k8sHelp.WaitUntilPodInNamespaceIsDeleted("rook-ceph-mon", "rook")
	isNameSpaceDeleted := k8sHelp.WaitUntilNameSpaceIsDeleted("rook")
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)

	time.Sleep(time.Second * 10)

	if isRookUninstalled && isNameSpaceDeleted {
		logger.Infof("Rook uninstalled successfully")
		return
	}
	panic(fmt.Errorf("Rook not uninstalled"))
}

//UninstallRookFromK8sViaHelm uninstalls rook from k8s and deletes helm deploy
func (h *InstallHelper) UninstallRookFromK8sViaHelm() {
	//flag used for local debugging purpose, when rook is pre-installed
	skipRookInstall := strings.EqualFold(h.Env.SkipInstallRook, "true")
	if skipRookInstall {
		return
	}

	serverVersion, err := h.k8shelper.Clientset.Discovery().ServerVersion()
	if err != nil {
		panic(err)
	}
	kubeVersion := utilversion.MustParseSemantic(serverVersion.GitVersion)

	logger.Infof("Uninstalling Rook")
	k8sHelp, err := utils.CreateK8sHelper()
	if err != nil {
		panic(err)
	}

	err = h.helmHelper.DeleteLocalRookHelmChart(helmDeployName)
	if err != nil {
		panic(err)
	}

	_, err = k8sHelp.DeleteResource([]string{"-n", "rook", "cluster", "rook"})
	if err != nil {
		panic(err)
	}
	_, err = k8sHelp.DeleteResource([]string{"-n", "rook", "serviceaccount", "rook-api"})
	if err != nil {
		panic(err)
	}
	if kubeVersion.AtLeast(utilversion.MustParseSemantic("v1.6.0")) {
		_, err = k8sHelp.DeleteResource([]string{"clusterrole", "rook-api"})
		if err != nil {
			panic(err)
		}
		_, err = k8sHelp.DeleteResource([]string{"clusterrolebinding", "rook-api"})
		if err != nil {
			panic(err)
		}
	}

	if kubeVersion.AtLeast(utilversion.MustParseSemantic("v1.7.0")) {
		_, err = k8sHelp.DeleteResource([]string{"crd", "clusters.rook.io", "pools.rook.io"})
		if err != nil {
			panic(err)
		}
	} else {
		_, err = k8sHelp.DeleteResource([]string{"thirdpartyresources", "cluster.rook.io", "pool.rook.io"})
		if err != nil {
			panic(err)
		}
	}
	_, err = k8sHelp.DeleteResource([]string{"secret", "rook-rook-user"})
	if err != nil {
		panic(err)
	}
	_, err = k8sHelp.DeleteResource([]string{"namespace", "rook"})
	if err != nil {
		panic(err)
	}

	isRookUninstalled := k8sHelp.WaitUntilPodInNamespaceIsDeleted("rook-ceph-mon", "rook")
	isNameSpaceDeleted := k8sHelp.WaitUntilNameSpaceIsDeleted("rook")
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)

	time.Sleep(time.Second * 10)

	if isRookUninstalled && isNameSpaceDeleted {
		logger.Infof("Rook uninstalled successfully")
		return
	}
	panic(fmt.Errorf("Rook not uninstalled"))
}

//NewK8sRookhelper creates new instance of InstallHelper
func NewK8sRookhelper(clientset *kubernetes.Clientset) *InstallHelper {

	version, err := clientset.ServerVersion()
	if err != nil {
		logger.Infof("failed to get kubectl server version. %+v", err)
	} else {
		env.K8sVersion = version.String()
	}

	k8shelp, err := utils.CreateK8sHelper()
	if err != nil {
		panic("failed to get kubectl client :" + err.Error())
	}
	return &InstallHelper{
		k8shelper:   k8shelp,
		installData: NewK8sInstallData(),
		helmHelper:  utils.NewHelmHelper(),
		Env:         env,
	}
}
