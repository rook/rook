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

	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/transport"
	"github.com/rook/rook/tests/framework/utils"
)

const (
	rookOperatorFileName   = "rook-operator.yaml"
	rookClusterFileName    = "rook-cluster.yaml"
	rookToolsFileName      = "rook-tools.yaml"
	podSpecPath            = "src/github.com/rook/rook/demo/kubernetes"
	rookOperatorCreatedTpr = "cluster.rook.io"
)

var (
	r    *InstallHelper
	once Once
)

type InstallHelper struct {
	transportClient contracts.ITransportClient
	isRookInstalled bool
}

func getPodSpecPath() string {
	return filepath.Join(os.Getenv("GOPATH"), podSpecPath)
}

//method for create rook-operator via kubectl
func (h *InstallHelper) createK8sRookOperator(k8sHelper *utils.K8sHelper) error {

	raw, err := ioutil.ReadFile(path.Join(getPodSpecPath(), rookOperatorFileName))

	if err != nil {
		return err
	}
	rookOperator := string(raw)

	_, _, exitCode := h.transportClient.CreateWithStdin(rookOperator)

	if exitCode != 0 {
		return fmt.Errorf(string("Failed to create rook-operator pod; kubectl exit code = " + string(exitCode)))
	}

	fmt.Println()
	if !k8sHelper.IsThirdPartyResourcePresent(rookOperatorCreatedTpr) {
		return fmt.Errorf("Failed to start Rook Operator; k8s thirdpartyresource did not appear")
	} else {
		fmt.Println("Rook Operator started")
	}

	return nil
}

func (h *InstallHelper) createK8sRookToolbox(k8sHelper *utils.K8sHelper) (err error) {

	//Create rook toolbox
	raw, err := ioutil.ReadFile(path.Join(getPodSpecPath(), rookToolsFileName))

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
	} else {
		fmt.Println("Rook Toolbox started")
	}

	return nil
}

func (h *InstallHelper) createk8sRookCluster(k8sHelper *utils.K8sHelper) error {

	raw, err := ioutil.ReadFile(path.Join(getPodSpecPath(), rookClusterFileName))

	if err != nil {
		return err
	}
	rookCluster := string(raw)

	_, _, exitCode := h.transportClient.CreateWithStdin(rookCluster)

	if exitCode != 0 {
		return fmt.Errorf("Failed to create rook-cluster pod; kubectl exit code = " + string(exitCode))
	}

	if !k8sHelper.IsServiceUpInNameSpace("rook-api") {
		fmt.Println("Rook Cluster couldn't start")
	} else {
		fmt.Println("Rook Cluster started")
	}

	return nil
}

func (h *InstallHelper) InstallRookOnK8s() (err error) {
	if h.isRookInstalled {
		return
	}

	//Create rook operator
	k8sHelp := utils.CreatK8sHelper()

	err = h.createK8sRookOperator(k8sHelp)
	if err != nil {
		panic(err)
	}

	time.Sleep(10 * time.Second) ///TODO: add real check here

	//Create rook cluster
	err = h.createk8sRookCluster(k8sHelp)
	if err != nil {
		panic(err)
	}

	time.Sleep(5 * time.Second)

	//Create rook client
	err = h.createK8sRookToolbox(k8sHelp)
	if err != nil {
		panic(err)
	}

	h.isRookInstalled = true

	return nil
}

func (h *InstallHelper) UninstallRookFromK8s() {

	k8sHelp := utils.CreatK8sHelper()
	var err error
	_, err = k8sHelp.ResourceOperation("delete", path.Join(getPodSpecPath(), rookOperatorFileName))
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
	_, err = k8sHelp.DeleteResource([]string{"clusterrole", "rook-api"})
	if err != nil {
		panic(err)
	}
	_, err = k8sHelp.DeleteResource([]string{"clusterrolebinding", "rook-api"})
	if err != nil {
		panic(err)
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
		fmt.Println("Rook uninstalled successfully")
		once.Reset()
		return
	}
	panic(fmt.Errorf("Rook not uninstalled"))
}

func NewK8sRookhelper() (*InstallHelper, error) {

	transportClient := transport.CreateNewk8sTransportClient()
	once.Do(func() {
		r = &InstallHelper{
			transportClient: transportClient,
			isRookInstalled: false,
		}
	})
	return r, nil
}
