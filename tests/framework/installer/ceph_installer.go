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
	"math/rand"
	"os"
	"path"
	"testing"

	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"

	"flag"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	rookOperatorCreatedCrd = "clusters.ceph.rook.io"
	helmChartName          = "local/rook-ceph"
	helmDeployName         = "rook-ceph"
)

//CephInstaller wraps installing and uninstalling rook on a platform
type CephInstaller struct {
	Manifests        CephManifests
	k8shelper        *utils.K8sHelper
	hostPathToDelete string
	helmHelper       *utils.HelmHelper
	k8sVersion       string
	T                func() *testing.T
}

func (h *CephInstaller) CreateCephCRDs() (err error) {
	var resources string
	logger.Info("Creating Rook CRDs")

	resources = h.Manifests.GetRookCRDs()

	_, err = h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...)

	return
}

//CreateCephOperator creates rook-operator via kubectl
func (h *CephInstaller) CreateCephOperator(namespace string) (err error) {
	logger.Infof("Starting Rook Operator")
	//creating clusterrolebinding for kubeadm env.
	h.k8shelper.CreateAnonSystemClusterBinding()

	//creating rook resources
	if err = h.CreateCephCRDs(); err != nil {
		return err
	}

	rookOperator := h.Manifests.GetRookOperator(namespace)

	_, err = h.k8shelper.KubectlWithStdin(rookOperator, createFromStdinArgs...)
	if err != nil {
		return fmt.Errorf("Failed to create rook-operator pod : %v ", err)
	}

	if !h.k8shelper.IsCRDPresent(rookOperatorCreatedCrd) {
		return fmt.Errorf("Failed to start Rook Operator; k8s CustomResourceDefinition did not appear")
	}

	logger.Infof("Rook Operator started")

	return nil
}

//CreateK8sRookOperatorViaHelm creates rook operator via Helm chart named local/rook present in local repo
func (h *CephInstaller) CreateK8sRookOperatorViaHelm(namespace string) error {
	//creating clusterrolebinding for kubeadm env.
	h.k8shelper.CreateAnonSystemClusterBinding()

	helmTag, err := h.helmHelper.GetLocalRookHelmChartVersion(helmChartName)

	if err != nil {
		return fmt.Errorf("Failed to get Version of helm chart %v, err : %v", helmChartName, err)
	}

	err = h.helmHelper.InstallLocalRookHelmChart(helmChartName, helmDeployName, helmTag, namespace)
	if err != nil {
		return fmt.Errorf("failed to install rook operator via helm, err : %v", err)

	}

	if !h.k8shelper.IsCRDPresent(rookOperatorCreatedCrd) {
		return fmt.Errorf("Failed to start Rook Operator; k8s CustomResourceDefinition did not appear")
	}

	return nil
}

//CreateK8sRookToolbox creates rook-ceph-tools via kubectl
func (h *CephInstaller) CreateK8sRookToolbox(namespace string) (err error) {
	logger.Infof("Starting Rook toolbox")

	rookToolbox := h.Manifests.GetRookToolBox(namespace)

	_, err = h.k8shelper.KubectlWithStdin(rookToolbox, createFromStdinArgs...)

	if err != nil {
		return fmt.Errorf("Failed to create rook-toolbox pod : %v ", err)
	}

	if !h.k8shelper.IsPodRunning("rook-ceph-tools", namespace) {
		return fmt.Errorf("Rook Toolbox couldn't start")
	}
	logger.Infof("Rook Toolbox started")

	return nil
}

func (h *CephInstaller) CreateK8sRookCluster(namespace, systemNamespace string, storeType string) (err error) {
	return h.CreateK8sRookClusterWithHostPathAndDevices(namespace, systemNamespace, storeType, false,
		cephv1beta1.MonSpec{Count: 3, AllowMultiplePerNode: true}, true /* startWithAllNodes */)
}

//CreateK8sRookCluster creates rook cluster via kubectl
func (h *CephInstaller) CreateK8sRookClusterWithHostPathAndDevices(namespace, systemNamespace, storeType string,
	useAllDevices bool, mon cephv1beta1.MonSpec, startWithAllNodes bool) error {

	dataDirHostPath, err := h.initTestDir(namespace)
	if err != nil {
		return fmt.Errorf("failed to create test dir. %+v", err)
	}
	logger.Infof("Creating cluster: namespace=%s, systemNamespace=%s, storeType=%s, dataDirHostPath=%s, useAllDevices=%t, startWithAllNodes=%t, mons=%+v",
		namespace, systemNamespace, storeType, dataDirHostPath, useAllDevices, startWithAllNodes, mon)

	logger.Infof("Creating namespace %s", namespace)
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err = h.k8shelper.Clientset.CoreV1().Namespaces().Create(ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace %s. %+v", namespace, err)
	}

	logger.Infof("Creating cluster roles")
	roles := h.Manifests.GetClusterRoles(namespace, systemNamespace)
	if _, err := h.k8shelper.KubectlWithStdin(roles, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create cluster roles. %+v", err)
	}

	if h.k8shelper.IsRookClientsetAvailable() {
		logger.Infof("Starting Rook cluster with strongly typed clientset")

		clust := &cephv1beta1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespace,
				Namespace: namespace,
			},
			Spec: cephv1beta1.ClusterSpec{
				ServiceAccount:  "rook-ceph-cluster",
				DataDirHostPath: dataDirHostPath,
				Mon: cephv1beta1.MonSpec{
					Count:                mon.Count,
					AllowMultiplePerNode: mon.AllowMultiplePerNode,
				},
				Storage: rookalpha.StorageScopeSpec{
					UseAllNodes: startWithAllNodes,
					Selection: rookalpha.Selection{
						UseAllDevices: &useAllDevices,
					},
					Config: map[string]string{
						config.StoreTypeKey:      storeType,
						config.DatabaseSizeMBKey: "1024",
						config.JournalSizeMBKey:  "1024",
					},
				},
			},
		}
		_, err := h.k8shelper.RookClientset.CephV1beta1().Clusters(namespace).Create(clust)
		if err != nil {
			return fmt.Errorf("failed to create cluster %s. %+v", clust.Name, err)
		}

		if !startWithAllNodes {
			// now that the cluster is created, let's get all the k8s nodes so we can update the cluster CRD with them
			logger.Info("cluster was started without all nodes, will update cluster to add nodes now.")
			nodeNames, err := h.GetNodeHostnames()
			if err != nil {
				return fmt.Errorf("failed to get k8s nodes to add to cluster CRD: %+v", err)
			}

			// add all discovered k8s nodes to the cluster CRD
			rookNodes := make([]rookalpha.Node, len(nodeNames))
			for i, hostname := range nodeNames {
				rookNodes[i] = rookalpha.Node{Name: hostname}
			}
			clust, err = h.k8shelper.RookClientset.CephV1beta1().Clusters(namespace).Get(namespace, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get rook cluster to add nodes to it: %+v", err)
			}
			clust.Spec.Storage.Nodes = rookNodes

			// update the cluster CRD now
			_, err = h.k8shelper.RookClientset.CephV1beta1().Clusters(namespace).Update(clust)
			if err != nil {
				return fmt.Errorf("failed to update cluster %s with nodes. %+v", clust.Name, err)
			}
		}
	} else {
		logger.Infof("Starting Rook Cluster with yaml")
		rookCluster := h.Manifests.GetRookCluster(&ClusterSettings{namespace, storeType, dataDirHostPath, useAllDevices, mon.Count})
		if _, err := h.k8shelper.KubectlWithStdin(rookCluster, createFromStdinArgs...); err != nil {
			return fmt.Errorf("Failed to create rook cluster : %v ", err)
		}
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-ceph-mon", namespace, mon.Count); err != nil {
		return err
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-ceph-osd", namespace, 1); err != nil {
		return err
	}

	logger.Infof("Rook Cluster started")
	err = h.k8shelper.WaitForLabeledPodToRun("app=rook-ceph-osd", namespace)
	return err
}

func (h *CephInstaller) initTestDir(namespace string) (string, error) {
	h.hostPathToDelete = path.Join(baseTestDir, "rook-test")
	testDir := path.Join(h.hostPathToDelete, namespace)

	if createBaseTestDir {
		// Create the test dir on the local host
		if err := os.MkdirAll(testDir, 0777); err != nil {
			return "", err
		}

		var err error
		if testDir, err = ioutil.TempDir(testDir, "test-"); err != nil {
			return "", err
		}
	} else {
		// Compose a random test directory name without actually creating it since not running on the localhost
		r := rand.Int()
		testDir = path.Join(testDir, fmt.Sprintf("test-%d", r))
	}
	return testDir, nil
}

func (h *CephInstaller) GetNodeHostnames() ([]string, error) {
	nodes, err := h.k8shelper.Clientset.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get k8s nodes. %+v", err)
	}
	var names []string
	for _, node := range nodes.Items {
		names = append(names, node.Labels[apis.LabelHostname])
	}

	return names, nil
}

//InstallRookOnK8sWithHostPathAndDevices installs rook on k8s
func (h *CephInstaller) InstallRookOnK8sWithHostPathAndDevices(namespace, storeType string,
	helmInstalled, useDevices bool, mon cephv1beta1.MonSpec, startWithAllNodes bool) (bool, error) {

	var err error
	//flag used for local debuggin purpose, when rook is pre-installed
	if Env.SkipInstallRook {
		return true, nil
	}

	k8sversion := h.k8shelper.GetK8sServerVersion()

	logger.Infof("Installing rook on k8s %s", k8sversion)

	onamespace := namespace
	//Create rook operator
	if helmInstalled {
		err = h.CreateK8sRookOperatorViaHelm(namespace)
		if err != nil {
			logger.Errorf("Rook Operator not installed ,error -> %v", err)
			return false, err

		}
	} else {
		onamespace = SystemNamespace(namespace)

		err := h.CreateCephOperator(onamespace)
		if err != nil {
			logger.Errorf("Rook Operator not installed ,error -> %v", err)
			return false, err
		}
	}
	if !h.k8shelper.IsPodInExpectedState("rook-ceph-operator", onamespace, "Running") {
		logger.Error("rook-ceph-operator is not running")
		h.k8shelper.GetRookLogs("rook-ceph-operator", Env.HostType, onamespace, "test-setup")
		logger.Error("rook-ceph-operator is not Running, abort!")
		return false, err
	}

	if forceUseDevices {
		logger.Infof("Forcing the use of devices")
		useDevices = true
	} else if useDevices {
		// This check only looks at the local machine for devices. If you want to force using devices,
		// set the forceUseDevices flag
		useDevices = IsAdditionalDeviceAvailableOnCluster()
	}

	//Create rook cluster
	err = h.CreateK8sRookClusterWithHostPathAndDevices(namespace, onamespace, storeType,
		useDevices, cephv1beta1.MonSpec{Count: mon.Count, AllowMultiplePerNode: mon.AllowMultiplePerNode}, startWithAllNodes)
	if err != nil {
		logger.Errorf("Rook cluster %s not installed, error -> %v", namespace, err)
		return false, err
	}

	//Create rook client
	err = h.CreateK8sRookToolbox(namespace)
	if err != nil {
		logger.Errorf("Rook toolbox in cluster %s not installed, error -> %v", namespace, err)
		return false, err
	}
	logger.Infof("installed rook operator and cluster : %s on k8s %s", namespace, h.k8sVersion)
	return true, nil
}

//UninstallRookFromK8s uninstalls rook from k8s
func (h *CephInstaller) UninstallRook(helmInstalled bool, namespace string) {
	h.UninstallRookFromMultipleNS(helmInstalled, SystemNamespace(namespace), namespace)
}

//UninstallRookFromK8s uninstalls rook from multiple namespaces in k8s
func (h *CephInstaller) UninstallRookFromMultipleNS(helmInstalled bool, systemNamespace string, namespaces ...string) {
	//flag used for local debugging purpose, when rook is pre-installed
	if Env.SkipInstallRook {
		return
	}

	logger.Infof("Uninstalling Rook")
	var err error
	for _, namespace := range namespaces {

		if !h.k8shelper.VersionAtLeast("v1.8.0") {
			_, err = h.k8shelper.DeleteResource("-n", namespace, "serviceaccount", "rook-ceph-cluster")
			checkError(h.T(), err, "cannot remove serviceaccount rook-ceph-cluster")
			assert.NoError(h.T(), err, "%s  err -> %v", namespace, err)

			err = h.k8shelper.DeleteRoleAndBindings("rook-ceph-cluster", namespace)
			checkError(h.T(), err, "rook-ceph-cluster cluster role and binding cannot be deleted")
			assert.NoError(h.T(), err, "rook-ceph-cluster cluster role and binding cannot be deleted: %+v", err)

			err = h.k8shelper.DeleteRoleBinding("rook-ceph-cluster-mgmt", namespace)
			checkError(h.T(), err, "rook-ceph-cluster-mgmt binding cannot be deleted")
			assert.NoError(h.T(), err, "rook-ceph-cluster-mgmt binding cannot be deleted: %+v", err)
		}

		_, err = h.k8shelper.DeleteResourceAndWait(false, "-n", namespace, "cluster.ceph.rook.io", namespace)
		checkError(h.T(), err, fmt.Sprintf("cannot remove cluster %s", namespace))

		crdCheckerFunc := func() error {
			_, err := h.k8shelper.RookClientset.RookV1alpha1().Clusters(namespace).Get(namespace, metav1.GetOptions{})
			return err
		}
		err = h.k8shelper.WaitForCustomResourceDeletion(namespace, crdCheckerFunc)
		checkError(h.T(), err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

		_, err = h.k8shelper.DeleteResourceAndWait(false, "namespace", namespace)
		checkError(h.T(), err, fmt.Sprintf("cannot delete namespace %s", namespace))
	}

	logger.Infof("removing the operator from namespace %s", systemNamespace)
	_, err = h.k8shelper.DeleteResource("crd", "clusters.ceph.rook.io", "pools.ceph.rook.io", "objectstores.ceph.rook.io", "filesystems.ceph.rook.io", "volumes.rook.io")
	checkError(h.T(), err, "cannot delete CRDs")

	if helmInstalled {
		err = h.helmHelper.DeleteLocalRookHelmChart(helmDeployName)
	} else {
		rookOperator := h.Manifests.GetRookOperator(systemNamespace)
		_, err = h.k8shelper.KubectlWithStdin(rookOperator, deleteFromStdinArgs...)
	}
	checkError(h.T(), err, "cannot uninstall rook-operator")

	h.k8shelper.Clientset.RbacV1beta1().RoleBindings(systemNamespace).Delete("rook-ceph-system", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rook-ceph-global", nil)
	h.k8shelper.Clientset.CoreV1().ServiceAccounts(systemNamespace).Delete("rook-ceph-system", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-cluster-mgmt", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-global", nil)
	h.k8shelper.Clientset.RbacV1beta1().Roles(systemNamespace).Delete("rook-ceph-system", nil)

	logger.Infof("done removing the operator from namespace %s", systemNamespace)
	logger.Infof("removing host data dir %s", h.hostPathToDelete)
	// removing data dir if exists
	if h.hostPathToDelete != "" {
		nodes, err := h.GetNodeHostnames()
		checkError(h.T(), err, "cannot get node names")
		for _, node := range nodes {
			err = h.cleanupDir(node, h.hostPathToDelete)
			logger.Infof("removing %s from node %s. err=%v", h.hostPathToDelete, node, err)
		}
	}
}

func (h *CephInstaller) cleanupDir(node, dir string) error {
	resources := h.Manifests.GetCleanupPod(node, dir)
	_, err := h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...)
	return err
}

func (h *CephInstaller) GatherAllRookLogs(namespace, systemNamespace string, testName string) {
	logger.Infof("Gathering all logs from Rook Cluster %s", namespace)
	h.k8shelper.GetRookLogs("rook-ceph-operator", Env.HostType, systemNamespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-agent", Env.HostType, systemNamespace, testName)
	h.k8shelper.GetRookLogs("rook-discover", Env.HostType, systemNamespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-mgr", Env.HostType, namespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-mon", Env.HostType, namespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-osd", Env.HostType, namespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-osd-prepare", Env.HostType, namespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-rgw", Env.HostType, namespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-mds", Env.HostType, namespace, testName)
}

// NewCephInstaller creates new instance of CephInstaller
func NewCephInstaller(t func() *testing.T, clientset *kubernetes.Clientset, rookVersion string) *CephInstaller {

	// All e2e tests should run ceph commands in the toolbox since we are not inside a container
	client.RunAllCephCommandsInToolbox = true

	version, err := clientset.ServerVersion()
	if err != nil {
		logger.Infof("failed to get kubectl server version. %+v", err)
	}

	k8shelp, err := utils.CreateK8sHelper(t)
	if err != nil {
		panic("failed to get kubectl client :" + err.Error())
	}
	h := &CephInstaller{
		Manifests:  NewCephManifests(rookVersion),
		k8shelper:  k8shelp,
		helmHelper: utils.NewHelmHelper(),
		k8sVersion: version.String(),
		T:          t,
	}
	flag.Parse()
	return h
}
