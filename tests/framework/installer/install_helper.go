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
	"runtime"
	"strings"
	"testing"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"

	"flag"

	"github.com/coreos/pkg/capnslog"
	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/rook/rook/tests/framework/objects"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	rookOperatorCreatedCrd = "clusters.ceph.rook.io"
	cockroachDBCRD         = "clusters.cockroachdb.rook.io"
	helmChartName          = "local/rook-ceph"
	helmDeployName         = "rook-ceph"
)

var (
	// ** Variables that might need to be changed depending on the dev environment. The init function below will modify some of them automatically. **
	baseTestDir       string
	forceUseDevices   = false
	createBaseTestDir = true
	// ** end of Variables to modify
	logger              = capnslog.NewPackageLogger("github.com/rook/rook", "installer")
	createArgs          = []string{"create", "-f"}
	createFromStdinArgs = append(createArgs, "-")
	deleteArgs          = []string{"delete", "-f"}
	deleteFromStdinArgs = append(deleteArgs, "-")
)

func init() {
	// this default will only work if running kubernetes on the local machine
	baseTestDir, _ = os.Getwd()

	// The following settings could apply to any environment when the kube context is running on the host and the tests are running inside a
	// VM such as minikube. This is a cheap test for this condition, we need to find a better way to automate these settings.
	if runtime.GOOS == "darwin" {
		createBaseTestDir = false
		baseTestDir = "/data"
	}
}

//InstallHelper wraps installing and uninstalling rook on a platform
type InstallHelper struct {
	k8shelper        *utils.K8sHelper
	installData      *InstallData
	hostPathToDelete string
	helmHelper       *utils.HelmHelper
	Env              objects.EnvironmentManifest
	k8sVersion       string
	changeHostnames  bool
	T                func() *testing.T
}

func (h *InstallHelper) CreateK8sRookResources() (err error) {
	var resources string
	logger.Info("Creating Rook CRD's")

	resources = h.installData.GetRookCRDs()

	_, err = h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...)

	return
}

//CreateK8sRookOperator creates rook-operator via kubectl
func (h *InstallHelper) CreateK8sRookOperator(namespace string) (err error) {
	logger.Infof("Starting Rook Operator")
	//creating clusterrolebinding for kubeadm env.
	h.k8shelper.CreateAnonSystemClusterBinding()

	//creating rook resources
	if err = h.CreateK8sRookResources(); err != nil {
		return err
	}

	if h.changeHostnames {
		// give nodes a hostname that is different from its k8s node name to confirm that all the daemons will be initialized properly
		h.k8shelper.ChangeHostnames()
	}

	rookOperator := h.installData.GetRookOperator(namespace)

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

	if !h.k8shelper.IsCRDPresent(rookOperatorCreatedCrd) {
		return fmt.Errorf("Failed to start Rook Operator; k8s CustomResourceDefinition did not appear")
	}

	return nil
}

//CreateK8sRookToolbox creates rook-ceph-tools via kubectl
func (h *InstallHelper) CreateK8sRookToolbox(namespace string) (err error) {
	logger.Infof("Starting Rook toolbox")

	rookToolbox := h.installData.GetRookToolBox(namespace)

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

func (h *InstallHelper) CreateK8sRookCluster(namespace, systemNamespace string, storeType string) (err error) {
	return h.CreateK8sRookClusterWithHostPathAndDevices(namespace, systemNamespace, storeType, false,
		cephv1beta1.MonSpec{Count: 3, AllowMultiplePerNode: true}, true /* startWithAllNodes */)
}

//CreateK8sRookCluster creates rook cluster via kubectl
func (h *InstallHelper) CreateK8sRookClusterWithHostPathAndDevices(namespace, systemNamespace, storeType string,
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
	roles := h.installData.GetClusterRoles(namespace, systemNamespace)
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
		rookCluster := h.installData.GetRookCluster(namespace, storeType, dataDirHostPath, useAllDevices, mon.Count)
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

func (h *InstallHelper) initTestDir(namespace string) (string, error) {
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

func (h *InstallHelper) GetNodeHostnames() ([]string, error) {
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

func SystemNamespace(namespace string) string {
	return fmt.Sprintf("%s-system", namespace)
}

//InstallRookOnK8sWithHostPathAndDevices installs rook on k8s
func (h *InstallHelper) InstallRookOnK8sWithHostPathAndDevices(namespace, storeType string,
	helmInstalled, useDevices bool, mon cephv1beta1.MonSpec, startWithAllNodes bool) (bool, error) {

	var err error
	//flag used for local debuggin purpose, when rook is pre-installed
	if h.Env.SkipInstallRook {
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

		err := h.CreateK8sRookOperator(onamespace)
		if err != nil {
			logger.Errorf("Rook Operator not installed ,error -> %v", err)
			return false, err
		}
	}
	if !h.k8shelper.IsPodInExpectedState("rook-ceph-operator", onamespace, "Running") {
		logger.Error("rook-ceph-operator is not running")
		h.k8shelper.GetRookLogs("rook-ceph-operator", h.Env.HostType, onamespace, "test-setup")
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
func (h *InstallHelper) UninstallRook(helmInstalled bool, namespace string) {
	h.UninstallRookFromMultipleNS(helmInstalled, SystemNamespace(namespace), namespace)
}

//UninstallRookFromK8s uninstalls rook from multiple namespaces in k8s
func (h *InstallHelper) UninstallRookFromMultipleNS(helmInstalled bool, systemNamespace string, namespaces ...string) {
	//flag used for local debugging purpose, when rook is pre-installed
	if h.Env.SkipInstallRook {
		return
	}

	logger.Infof("Uninstalling Rook")
	var err error
	for _, namespace := range namespaces {

		if !h.k8shelper.VersionAtLeast("v1.8.0") {
			_, err = h.k8shelper.DeleteResource("-n", namespace, "serviceaccount", "rook-ceph-cluster")
			h.checkError(err, "cannot remove serviceaccount rook-ceph-cluster")
			assert.NoError(h.T(), err, "%s  err -> %v", namespace, err)

			err = h.k8shelper.DeleteRoleAndBindings("rook-ceph-cluster", namespace)
			h.checkError(err, "rook-ceph-cluster cluster role and binding cannot be deleted")
			assert.NoError(h.T(), err, "rook-ceph-cluster cluster role and binding cannot be deleted: %+v", err)

			err = h.k8shelper.DeleteRoleBinding("rook-ceph-cluster-mgmt", namespace)
			h.checkError(err, "rook-ceph-cluster-mgmt binding cannot be deleted")
			assert.NoError(h.T(), err, "rook-ceph-cluster-mgmt binding cannot be deleted: %+v", err)
		}

		_, err = h.k8shelper.DeleteResource("-n", namespace, "cluster.ceph.rook.io", namespace)
		h.checkError(err, fmt.Sprintf("cannot remove cluster %s", namespace))

		crdCheckerFunc := func() error {
			_, err := h.k8shelper.RookClientset.RookV1alpha1().Clusters(namespace).Get(namespace, metav1.GetOptions{})
			return err
		}
		err = h.waitForCustomResourceDeletion(namespace, crdCheckerFunc)
		h.checkError(err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

		_, err = h.k8shelper.DeleteResource("namespace", namespace)
		h.checkError(err, fmt.Sprintf("cannot delete namespace %s", namespace))
	}

	logger.Infof("removing the operator from namespace %s", systemNamespace)
	_, err = h.k8shelper.DeleteResource("crd", "clusters.ceph.rook.io", "pools.ceph.rook.io", "objectstores.ceph.rook.io", "filesystems.ceph.rook.io", "volumes.rook.io")
	h.checkError(err, "cannot delete CRDs")

	if helmInstalled {
		err = h.helmHelper.DeleteLocalRookHelmChart(helmDeployName)
	} else {
		rookOperator := h.installData.GetRookOperator(systemNamespace)
		_, err = h.k8shelper.KubectlWithStdin(rookOperator, deleteFromStdinArgs...)
	}
	h.checkError(err, "cannot uninstall rook-operator")

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
		h.checkError(err, "cannot get node names")
		for _, node := range nodes {
			err = h.cleanupDir(node, h.hostPathToDelete)
			logger.Infof("removing %s from node %s. err=%v", h.hostPathToDelete, node, err)
		}
	}
	if h.changeHostnames {
		// revert the hostname labels for the test
		h.k8shelper.RestoreHostnames()
	}
}

func (h *InstallHelper) cleanupDir(node, dir string) error {
	resources := h.installData.GetCleanupPod(node, dir)
	_, err := h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...)
	return err
}

func (h *InstallHelper) checkError(err error, message string) {
	// During cleanup the resource might not be found because the test might have failed before the test was done and never created the resource
	if err == nil || errors.IsNotFound(err) {
		return
	}
	assert.NoError(h.T(), err, "%s. %+v", message, err)
}

func (h *InstallHelper) waitForCustomResourceDeletion(namespace string, checkerFunc func() error) error {
	if !h.k8shelper.VersionAtLeast("v1.8.0") {
		// v1.7 has an intermittent issue with long delay to delete resources so we will skip waiting
		return nil
	}

	// wait for the operator to finalize and delete the CRD
	for i := 0; i < 10; i++ {
		err := checkerFunc()
		if err == nil {
			logger.Infof("custom resource %s still exists", namespace)
			time.Sleep(2 * time.Second)
			continue
		}
		if errors.IsNotFound(err) {
			logger.Infof("custom resource %s deleted", namespace)
			return nil
		}
		return err
	}
	logger.Errorf("gave up deleting custom resource %s", namespace)
	return nil
}

func (h *InstallHelper) GatherAllRookLogs(namespace, systemNamespace string, testName string) {
	logger.Infof("Gathering all logs from Rook Cluster %s", namespace)
	h.k8shelper.GetRookLogs("rook-ceph-operator", h.Env.HostType, systemNamespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-agent", h.Env.HostType, systemNamespace, testName)
	h.k8shelper.GetRookLogs("rook-discover", h.Env.HostType, systemNamespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-mgr", h.Env.HostType, namespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-mon", h.Env.HostType, namespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-osd", h.Env.HostType, namespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-osd-prepare", h.Env.HostType, namespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-rgw", h.Env.HostType, namespace, testName)
	h.k8shelper.GetRookLogs("rook-ceph-mds", h.Env.HostType, namespace, testName)
}

//NewK8sRookhelper creates new instance of InstallHelper
func NewK8sRookhelper(clientset *kubernetes.Clientset, t func() *testing.T) *InstallHelper {

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
	ih := &InstallHelper{
		k8shelper:       k8shelp,
		installData:     NewK8sInstallData(),
		helmHelper:      utils.NewHelmHelper(),
		Env:             objects.Env,
		k8sVersion:      version.String(),
		changeHostnames: k8shelp.VersionAtLeast("v1.11.0"),
		T:               t,
	}
	flag.Parse()
	return ih
}

func IsAdditionalDeviceAvailableOnCluster() bool {
	executor := &exec.CommandExecutor{}
	devices, err := sys.ListDevices(executor)
	if err != nil {
		return false
	}
	disks := 0
	logger.Infof("devices : %v", devices)
	for _, device := range devices {
		if strings.Contains(device, "loop") {
			continue
		}
		props, _ := sys.GetDeviceProperties(device, executor)
		if props["TYPE"] != "disk" {
			continue
		}

		ownPartitions, fs, err := sys.CheckIfDeviceAvailable(executor, device)
		if err != nil {
			logger.Warningf("failed to detect device %s availability. %+v", device, err)
			continue
		}
		if !ownPartitions {
			logger.Infof("skipping device %s since don't own partitions", device)
			continue
		}
		if fs != "" {
			logger.Infof("skipping device %s since it has file system %s", device, fs)
			continue
		}
		logger.Infof("available device: %s", device)
		disks++
	}
	if disks > 0 {
		return true
	}
	logger.Info("No additional disks found on cluster")
	return false
}
