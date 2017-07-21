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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"k8s.io/client-go/kubernetes"

	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/objects"
	"github.com/rook/rook/tests/framework/transport"
	"github.com/rook/rook/tests/framework/utils"
)

const (
	rookOperatorFileName   = "rook-operator.yaml"
	rookClusterFileName    = "rook-cluster.yaml"
	rookToolsFileName      = "rook-tools.yaml"
	podSpecPath            = "src/github.com/rook/rook/demo/kubernetes"
	podSpecPath1_5         = "src/github.com/rook/rook/demo/kubernetes/1.5"
	rookOperatorCreatedTpr = "cluster.rook.io"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "installer")
)

//InstallHelper wraps installation and uninstallaion of rook on a platfom
type InstallHelper struct {
	transportClient contracts.ITransportClient
	Env             objects.EnvironmentManifest
}

func getPodSpecPath(k8sverion string) string {
	switch k8sverion {
	case "v1.5":
		return filepath.Join(os.Getenv("GOPATH"), podSpecPath1_5)
	default:
		return filepath.Join(os.Getenv("GOPATH"), podSpecPath)
	}
}

//method for create rook-operator via kubectl
func (h *InstallHelper) createK8sRookOperator(k8sHelper *utils.K8sHelper, k8sversion string) error {
	logger.Infof("Starting Rook Operator")

	raw, err := ioutil.ReadFile(path.Join(getPodSpecPath(k8sversion), rookOperatorFileName))

	if err != nil {
		return err
	}
	rookOperator := string(raw)

	_, _, exitCode := h.transportClient.CreateWithStdin(rookOperator)

	if exitCode != 0 {
		return fmt.Errorf(string("Failed to create rook-operator pod; kubectl exit code = " + string(exitCode)))
	}

	if !k8sHelper.IsThirdPartyResourcePresent(rookOperatorCreatedTpr) {
		return fmt.Errorf("Failed to start Rook Operator; k8s thirdpartyresource did not appear")
	}
	logger.Infof("Rook Operator started")

	return nil
}

func (h *InstallHelper) createK8sRookToolbox(k8sHelper *utils.K8sHelper, k8sversion string) (err error) {
	logger.Infof("Starting Rook toolbox")

	//Create rook toolbox
	raw, err := ioutil.ReadFile(path.Join(getPodSpecPath(k8sversion), rookToolsFileName))

	if err != nil {
		panic(err)
	}

	rookClient := string(raw)

	_, _, exitCode := h.transportClient.CreateWithStdin(rookClient)

	if exitCode != 0 {
		return fmt.Errorf(string(exitCode))
	}

	if !k8sHelper.IsPodRunningInNamespace("rook-tools") {
		return fmt.Errorf("Rook Toolbox couldn't start")
	}
	logger.Infof("Rook Toolbox started")

	return nil
}

func (h *InstallHelper) createk8sRookCluster(k8sHelper *utils.K8sHelper, k8sversion string) error {
	logger.Infof("Starting Rook Cluster")

	raw, err := ioutil.ReadFile(path.Join(getPodSpecPath(k8sversion), rookClusterFileName))

	if err != nil {
		return err
	}
	rookCluster := string(raw)

	_, _, exitCode := h.transportClient.CreateWithStdin(rookCluster)

	if exitCode != 0 {
		return fmt.Errorf("Failed to create rook-cluster pod; kubectl exit code = " + string(exitCode))
	}

	if !k8sHelper.IsServiceUpInNameSpace("rook-api") {
		logger.Infof("Rook Cluster couldn't start")
	} else {
		logger.Infof("Rook Cluster started")
	}

	return nil
}

//InstallRookOnK8s installs rook on k8s
func (h *InstallHelper) InstallRookOnK8s() (err error) {

	//flag used for local debuggin purpose, when rook is pre-installed
	skipRookInstall := strings.EqualFold(h.Env.SkipInstallRook, "true")
	if skipRookInstall {
		return
	}

	logger.Infof("Installing rook on k8s %s", h.Env.K8sVersion)
	//Create rook operator
	k8sHelp, err := utils.CreatK8sHelper()
	if err != nil {
		panic(err)
	}

	err = h.createK8sRookOperator(k8sHelp, h.Env.K8sVersion)
	if err != nil {
		panic(err)
	}

	time.Sleep(10 * time.Second) ///TODO: add real check here

	//Create rook cluster
	err = h.createk8sRookCluster(k8sHelp, h.Env.K8sVersion)
	if err != nil {
		panic(err)
	}

	time.Sleep(5 * time.Second)

	//Create rook client
	err = h.createK8sRookToolbox(k8sHelp, h.Env.K8sVersion)
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

	logger.Infof("Uninstalling Rook")
	k8sHelp, err := utils.CreatK8sHelper()
	if err != nil {
		panic(err)
	}
	_, err = k8sHelp.ResourceOperation("delete", path.Join(getPodSpecPath(h.Env.K8sVersion), rookOperatorFileName))
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
	if !strings.EqualFold(h.Env.K8sVersion, "v1.5") {
		_, err = k8sHelp.DeleteResource([]string{"clusterrole", "rook-api"})
		if err != nil {
			panic(err)
		}
		_, err = k8sHelp.DeleteResource([]string{"clusterrolebinding", "rook-api"})
		if err != nil {
			panic(err)
		}
	}
	_, err = k8sHelp.DeleteResource([]string{"thirdpartyresources", "cluster.rook.io", "pool.rook.io"})
	if err != nil {
		panic(err)
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

	if isRookUninstalled {
		logger.Infof("Rook uninstalled successfully")
		return
	}
	panic(fmt.Errorf("Rook not uninstalled"))
}

//NewK8sRookhelper creates new instance of InstallHelper
func NewK8sRookhelper(clientset *kubernetes.Clientset) *InstallHelper {
	env := objects.NewManifest()

	version, err := clientset.ServerVersion()
	if err != nil {
		logger.Infof("failed to get kubectl server version. %+v", err)
	} else {
		env.K8sVersion = version.String()
	}

	transportClient := transport.CreateNewk8sTransportClient()
	return &InstallHelper{
		transportClient: transportClient,
		Env:             env,
	}
}
