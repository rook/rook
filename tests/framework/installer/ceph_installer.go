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
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	// test with the latest mimic build
	mimicTestImage = "ceph/ceph:v13"
	// test with the latest nautilus build
	nautilusTestImage = "ceph/ceph:v14"
	// test with the latest octopus build
	octopusTestImage  = "ceph/ceph:v15"
	helmChartName     = "local/rook-ceph"
	helmDeployName    = "rook-ceph"
	cephOperatorLabel = "app=rook-ceph-operator"
)

var (
	MimicVersion       = cephv1.CephVersionSpec{Image: mimicTestImage}
	NautilusVersion    = cephv1.CephVersionSpec{Image: nautilusTestImage}
	OctopusVersion     = cephv1.CephVersionSpec{Image: octopusTestImage}
	globalTestJobIndex = 0
)

// CephInstaller wraps installing and uninstalling rook on a platform
type CephInstaller struct {
	Manifests        CephManifests
	k8shelper        *utils.K8sHelper
	hostPathToDelete string
	helmHelper       *utils.HelmHelper
	useHelm          bool
	k8sVersion       string
	changeHostnames  bool
	CephVersion      cephv1.CephVersionSpec
	T                func() *testing.T
	cleanupHost      bool
}

// CreateCephOperator creates rook-operator via kubectl
func (h *CephInstaller) CreateCephOperator(namespace string) (err error) {
	logger.Infof("Starting Rook Operator")
	// creating clusterrolebinding for kubeadm env.
	h.k8shelper.CreateAnonSystemClusterBinding()

	// creating rook resources
	logger.Info("Creating Rook CRDs")
	resources := h.Manifests.GetRookCRDs()
	if _, err = h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...); err != nil {
		return err
	}

	if h.changeHostnames {
		// give nodes a hostname that is different from its k8s node name to confirm that all the daemons will be initialized properly
		h.k8shelper.ChangeHostnames()
	}

	err = h.k8shelper.CreateNamespace(namespace)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			logger.Warningf("Namespace %q already exists!!!", namespace)
		} else {
			return fmt.Errorf("failed to create namespace %q. %v", namespace, err)
		}
	}

	rookOperator := h.Manifests.GetRookOperator(namespace)
	_, err = h.k8shelper.KubectlWithStdin(rookOperator, createFromStdinArgs...)
	if err != nil {
		return fmt.Errorf("Failed to create rook-operator pod: %v ", err)
	}

	logger.Infof("Rook Operator started")

	return nil
}

// CreateRookOperatorViaHelm creates rook operator via Helm chart named local/rook present in local repo
func (h *CephInstaller) CreateRookOperatorViaHelm(namespace, chartSettings string) error {
	// creating clusterrolebinding for kubeadm env.
	h.k8shelper.CreateAnonSystemClusterBinding()

	helmTag, err := h.helmHelper.GetLocalRookHelmChartVersion(helmChartName)

	if err != nil {
		return fmt.Errorf("Failed to get Version of helm chart %v, err : %v", helmChartName, err)
	}

	err = h.helmHelper.InstallLocalRookHelmChart(helmChartName, helmDeployName, helmTag, namespace, chartSettings)
	if err != nil {
		return fmt.Errorf("failed to install rook operator via helm, err : %v", err)

	}

	return nil
}

// CreateRookToolbox creates rook-ceph-tools via kubectl
func (h *CephInstaller) CreateRookToolbox(namespace string) (err error) {
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

// CreateRookCluster creates rook cluster via kubectl
func (h *CephInstaller) CreateRookCluster(namespace, systemNamespace, storeType string, usePVC bool, storageClassName string,
	mon cephv1.MonSpec, startWithAllNodes bool, rbdMirrorWorkers int, cephVersion cephv1.CephVersionSpec) error {

	dataDirHostPath, err := h.initTestDir(namespace)
	if err != nil {
		return fmt.Errorf("failed to create test dir. %+v", err)
	}
	logger.Infof("Creating cluster: namespace=%s, systemNamespace=%s, storeType=%s, dataDirHostPath=%s, usePVC=%v, storageClassName=%s, startWithAllNodes=%t, mons=%+v",
		namespace, systemNamespace, storeType, dataDirHostPath, usePVC, storageClassName, startWithAllNodes, mon)

	logger.Infof("Creating namespace %s", namespace)
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err = h.k8shelper.Clientset.CoreV1().Namespaces().Create(ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace %s. %+v", namespace, err)
	}

	logger.Infof("Creating custom ceph.conf settings")
	customSettings := map[string]string{
		"config": `
[global]
osd_pool_default_size = 1
`}
	customCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-config-override",
		},
		Data: customSettings,
	}
	if _, err := h.k8shelper.Clientset.CoreV1().ConfigMaps(namespace).Create(customCM); err != nil {
		return fmt.Errorf("failed to create custom ceph.conf. %+v", err)
	}

	// Skip this step since the helm chart already includes the roles and bindings
	if !h.useHelm {
		logger.Infof("Creating cluster roles")
		roles := h.Manifests.GetClusterRoles(namespace, systemNamespace)
		if _, err := h.k8shelper.KubectlWithStdin(roles, createFromStdinArgs...); err != nil {
			return fmt.Errorf("Failed to create cluster roles. %+v", err)
		}
	}

	if err := h.WipeClusterDisks(namespace); err != nil {
		logger.Warningf("failed to wipe cluster disks. %+v. trying again...", err)
		if err = h.WipeClusterDisks(namespace); err != nil {
			return fmt.Errorf("failed to wipe cluster disks. %+v", err)
		}
	}

	logger.Infof("Starting Rook Cluster with yaml")
	settings := &ClusterSettings{namespace, storeType, dataDirHostPath, mon.Count, rbdMirrorWorkers, usePVC, storageClassName, cephVersion}
	rookCluster := h.Manifests.GetRookCluster(settings)
	if _, err := h.k8shelper.KubectlWithStdin(rookCluster, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create rook cluster : %v ", err)
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-ceph-mon", namespace, mon.Count); err != nil {
		return err
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-ceph-mgr", namespace, 1); err != nil {
		return err
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-ceph-osd", namespace, 1); err != nil {
		return err
	}

	if rbdMirrorWorkers > 0 {
		if err := h.k8shelper.WaitForPodCount("app=rook-ceph-rbd-mirror", namespace, rbdMirrorWorkers); err != nil {
			return err
		}
	}

	logger.Infof("Rook Cluster started")
	err = h.k8shelper.WaitForLabeledPodsToRun("app=rook-ceph-osd", namespace)
	return err
}

// CreateRookExternalCluster creates rook external cluster via kubectl
func (h *CephInstaller) CreateRookExternalCluster(namespace, firstClusterNamespace string) error {

	dataDirHostPath, err := h.initTestDir(namespace)
	if err != nil {
		return fmt.Errorf("failed to create test dir. %+v", err)
	}
	logger.Infof("Creating external cluster: namespace=%q, firstClusterNamespace=%q", namespace, firstClusterNamespace)

	logger.Infof("Creating namespace %s", namespace)
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err = h.k8shelper.Clientset.CoreV1().Namespaces().Create(ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace %q. %v", namespace, err)
	}

	// Skip this step since the helm chart already includes the roles and bindings
	if !h.useHelm {
		logger.Infof("Creating external cluster roles")
		roles := h.Manifests.GetClusterExternalRoles(namespace, firstClusterNamespace)
		if _, err := h.k8shelper.KubectlWithStdin(roles, createFromStdinArgs...); err != nil {
			return fmt.Errorf("failed to create cluster roles. %v", err)
		}
	}

	// Inject connection information from the first cluster
	logger.Info("Injecting cluster connection information")
	err = h.InjectRookExternalClusterInfo(namespace, firstClusterNamespace)
	if err != nil {
		return fmt.Errorf("failed to inject cluster information into the external cluster. %v", err)
	}

	// Start the external cluster
	logger.Infof("Starting Rook External Cluster with yaml")
	settings := &ClusterExternalSettings{namespace, dataDirHostPath}
	rookCluster := h.Manifests.GetRookExternalCluster(settings)
	if _, err := h.k8shelper.KubectlWithStdin(rookCluster, createFromStdinArgs...); err != nil {
		return fmt.Errorf("failed to create rook external cluster. %v ", err)
	}

	logger.Infof("Rook external cluster started")
	return err
}

// InjectRookExternalClusterInfo inject connection information for an external cluster
func (h *CephInstaller) InjectRookExternalClusterInfo(namespace, firstClusterNamespace string) error {
	// get config map
	cm, err := h.GetRookExternalClusterMonConfigMap(firstClusterNamespace)
	if err != nil {
		return fmt.Errorf("failed to get configmap. %v", err)
	}

	// create config map
	_, err = h.k8shelper.Clientset.CoreV1().ConfigMaps(namespace).Create(cm)
	if err != nil {
		return fmt.Errorf("failed to create configmap. %v", err)
	}

	// get secret
	secret, err := h.GetRookExternalClusterMonSecret(firstClusterNamespace)
	if err != nil {
		return fmt.Errorf("failed to get secret. %v", err)
	}

	// create secret
	_, err = h.k8shelper.Clientset.CoreV1().Secrets(namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to create secret. %v", err)
	}

	return nil
}

// GetRookExternalClusterMonConfigMap gets the monitor kubernetes configmap of the external cluster
func (h *CephInstaller) GetRookExternalClusterMonConfigMap(namespace string) (*v1.ConfigMap, error) {
	configMapName := "rook-ceph-mon-endpoints"
	externalCM, err := h.k8shelper.Clientset.CoreV1().ConfigMaps(namespace).Get(configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret. %v", err)
	}
	newCM := &v1.ConfigMap{}
	newCM.Name = externalCM.Name
	newCM.Data = externalCM.Data

	return newCM, nil
}

// GetRookExternalClusterMonSecret gets the monitor kubernetes secret of the external cluster
func (h *CephInstaller) GetRookExternalClusterMonSecret(namespace string) (*v1.Secret, error) {
	secretName := "rook-ceph-mon"

	externalSecret, err := h.k8shelper.Clientset.CoreV1().Secrets(namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret. %v", err)
	}
	newSecret := &v1.Secret{}
	newSecret.Name = externalSecret.Name
	newSecret.Data = externalSecret.Data

	return newSecret, nil
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

// GetNodeHostnames returns the list of nodes in the k8s cluster
func (h *CephInstaller) GetNodeHostnames() ([]string, error) {
	nodes, err := h.k8shelper.Clientset.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get k8s nodes. %+v", err)
	}
	var names []string
	for _, node := range nodes.Items {
		names = append(names, node.Labels[v1.LabelHostname])
	}

	return names, nil
}

// InstallRook installs rook on k8s
func (h *CephInstaller) InstallRook(namespace, storeType string, usePVC bool, storageClassName string,
	mon cephv1.MonSpec, startWithAllNodes bool, rbdMirrorWorkers int) (bool, error) {

	var err error
	k8sversion := h.k8shelper.GetK8sServerVersion()

	logger.Infof("Installing rook on k8s %s", k8sversion)

	startDiscovery := true
	onamespace := namespace
	// Create rook operator
	if h.useHelm {
		// disable the discovery daemonset with the helm chart
		settings := "enableDiscoveryDaemon=false"
		startDiscovery = false
		err = h.CreateRookOperatorViaHelm(namespace, settings)
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
		h.k8shelper.GetLogsFromNamespace(onamespace, "test-setup", Env.HostType)
		logger.Error("rook-ceph-operator is not Running, abort!")
		return false, err
	}

	// Create rook cluster
	err = h.CreateRookCluster(namespace, onamespace, storeType, usePVC, storageClassName,
		cephv1.MonSpec{Count: mon.Count, AllowMultiplePerNode: mon.AllowMultiplePerNode}, startWithAllNodes,
		rbdMirrorWorkers, h.CephVersion)
	if err != nil {
		logger.Errorf("Rook cluster %s not installed, error -> %v", namespace, err)
		return false, err
	}

	discovery, err := h.k8shelper.Clientset.AppsV1().DaemonSets(onamespace).Get("rook-discover", metav1.GetOptions{})
	if startDiscovery {
		assert.NoError(h.T(), err)
		assert.NotNil(h.T(), discovery)
	} else {
		assert.Error(h.T(), err)
		assert.True(h.T(), errors.IsNotFound(err))
	}

	// Create rook client
	err = h.CreateRookToolbox(namespace)
	if err != nil {
		logger.Errorf("Rook toolbox in cluster %s not installed, error -> %v", namespace, err)
		return false, err
	}
	logger.Infof("installed rook operator and cluster : %s on k8s %s", namespace, h.k8sVersion)
	return true, nil
}

// UninstallRook uninstalls rook from k8s
func (h *CephInstaller) UninstallRook(namespace string) {
	h.UninstallRookFromMultipleNS(SystemNamespace(namespace), namespace)
}

// UninstallRookFromMultipleNS uninstalls rook from multiple namespaces in k8s
func (h *CephInstaller) UninstallRookFromMultipleNS(systemNamespace string, namespaces ...string) {
	// Gather logs after status checks
	h.GatherAllRookLogs(h.T().Name(), append([]string{systemNamespace}, namespaces...)...)

	logger.Infof("Uninstalling Rook")
	var err error
	for clusterNum, namespace := range namespaces {
		if h.cleanupHost {
			//Add cleanup policy to the ceph cluster
			err = h.addCleanupPolicy(namespace)
			assert.NoError(h.T(), err)
		}

		if !h.T().Failed() {
			// Only check the Ceph status for the first cluster
			// The second cluster is external so the check won't work since the first cluster is gone
			if clusterNum == 0 {
				// if the test passed, check that the ceph status is HEALTH_OK before we tear the cluster down
				h.checkCephHealthStatus(namespace)
			}
		}

		// The pool CRs should already be removed by the tests that created them
		pools, err := h.k8shelper.RookClientset.CephV1().CephBlockPools(namespace).List(metav1.ListOptions{})
		assert.NoError(h.T(), err, "failed to retrieve pool CRs")
		for _, pool := range pools.Items {
			logger.Infof("found pools: %v", pools)
			assert.Failf(h.T(), "pool %q still exists", pool.Name)
			// Get the operator log
			h.GatherAllRookLogs(h.T().Name()+"poolcheck", systemNamespace)
		}

		roles := h.Manifests.GetClusterRoles(namespace, systemNamespace)
		_, err = h.k8shelper.KubectlWithStdin(roles, deleteFromStdinArgs...)

		err = h.k8shelper.DeleteResourceAndWait(false, "-n", namespace, "cephcluster", namespace)
		checkError(h.T(), err, fmt.Sprintf("cannot remove cluster %s", namespace))

		if h.cleanupHost {
			err = h.waitForCleanupJobs(namespace)
			assert.NoError(h.T(), err)
		}

		crdCheckerFunc := func() error {
			_, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(namespace, metav1.GetOptions{})
			// ensure the finalizer(s) are removed
			h.removeClusterFinalizers(namespace, namespace)
			return err
		}
		err = h.k8shelper.WaitForCustomResourceDeletion(namespace, crdCheckerFunc)
		checkError(h.T(), err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

		err = h.k8shelper.DeleteResourceAndWait(false, "namespace", namespace)
		checkError(h.T(), err, fmt.Sprintf("cannot delete namespace %s", namespace))
	}

	err = h.k8shelper.DeleteResourceAndWait(false, "namespace", systemNamespace)
	checkError(h.T(), err, fmt.Sprintf("cannot delete system namespace %s", systemNamespace))

	logger.Infof("removing the operator from namespace %s", systemNamespace)
	err = h.k8shelper.DeleteResource(
		"crd",
		"cephclusters.ceph.rook.io",
		"cephblockpools.ceph.rook.io",
		"cephobjectstores.ceph.rook.io",
		"cephobjectstoreusers.ceph.rook.io",
		"cephfilesystems.ceph.rook.io",
		"cephnfses.ceph.rook.io",
		"cephclients.ceph.rook.io",
		"volumes.rook.io",
		"objectbuckets.objectbucket.io",
		"objectbucketclaims.objectbucket.io")
	checkError(h.T(), err, "cannot delete CRDs")

	if h.useHelm {
		err = h.helmHelper.DeleteLocalRookHelmChart(helmDeployName)
	} else {
		logger.Infof("Deleting all the resources in the operator manifest")
		rookOperator := h.Manifests.GetRookOperator(systemNamespace)
		_, err = h.k8shelper.KubectlWithStdin(rookOperator, deleteFromStdinArgs...)
		logger.Infof("DONE deleting all the resources in the operator manifest")
	}
	checkError(h.T(), err, "cannot uninstall rook-operator")

	h.k8shelper.Clientset.RbacV1beta1().RoleBindings(systemNamespace).Delete("rook-ceph-system", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rook-ceph-global", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rook-ceph-mgr-cluster", nil)
	h.k8shelper.Clientset.CoreV1().ServiceAccounts(systemNamespace).Delete("rook-ceph-system", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-cluster-mgmt", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-cluster-mgmt-rules", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-mgr-cluster", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-mgr-cluster-rules", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-mgr-system", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-mgr-system-rules", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-global", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-global-rules", nil)
	h.k8shelper.Clientset.RbacV1beta1().Roles(systemNamespace).Delete("rook-ceph-system", nil)

	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rbd-csi-nodeplugin", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rbd-csi-nodeplugin", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rbd-csi-nodeplugin-rules", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rbd-csi-provisioner-role", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rbd-external-provisioner-runner", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rbd-external-provisioner-runner-rules", nil)

	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("cephfs-csi-nodeplugin", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("cephfs-csi-nodeplugin", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("cephfs-csi-nodeplugin-rules", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("cephfs-csi-provisioner-role", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("cephfs-external-provisioner-runner", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("cephfs-external-provisioner-runner-rules", nil)

	h.k8shelper.Clientset.PolicyV1beta1().PodSecurityPolicies().Delete("rook-privileged", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("psp:rook", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rook-ceph-system-psp", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rook-csi-rbd-provisioner-sa-psp", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rook-csi-rbd-plugin-sa-psp", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rook-csi-cephfs-provisioner-sa-psp", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rook-csi-cephfs-plugin-sa-psp", nil)

	h.k8shelper.Clientset.CoreV1().ConfigMaps(systemNamespace).Delete("csi-rbd-config", nil)
	h.k8shelper.Clientset.CoreV1().ConfigMaps(systemNamespace).Delete("csi-cephfs-config", nil)

	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("rook-ceph-object-bucket", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("rook-ceph-object-bucket", nil)

	logger.Infof("done removing the operator from namespace %s", systemNamespace)
	logger.Infof("removing host data dir %s", h.hostPathToDelete)
	// removing data dir if exists
	if h.hostPathToDelete != "" {
		nodes, err := h.GetNodeHostnames()
		checkError(h.T(), err, "cannot get node names")
		for _, node := range nodes {
			if h.cleanupHost {
				err = h.verifyDirCleanup(node, h.hostPathToDelete)
				logger.Infof("verifying clean up of %s from node %s. err=%v", h.hostPathToDelete, node, err)
				assert.NoError(h.T(), err)
			} else {
				err = h.cleanupDir(node, h.hostPathToDelete)
				logger.Infof("removing %s from node %s. err=%v", h.hostPathToDelete, node, err)
			}
		}
	}
	if h.changeHostnames {
		// revert the hostname labels for the test
		h.k8shelper.RestoreHostnames()
	}

	// wait a bit longer for the system namespace to be cleaned up after their deletion
	for i := 0; i < 15; i++ {
		_, err := h.k8shelper.Clientset.CoreV1().Namespaces().Get(systemNamespace, metav1.GetOptions{})
		if err != nil && errors.IsNotFound(err) {
			logger.Infof("system namespace %q removed", systemNamespace)
			break
		}
		logger.Infof("system namespace %q still found...", systemNamespace)
		time.Sleep(5 * time.Second)
	}
}

func (h *CephInstaller) removeClusterFinalizers(namespace, name string) {
	// Get the latest cluster instead of using the same instance in case it has been changed
	cluster, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to remove finalizer. failed to get cluster. %+v", err)
		return
	}
	objectMeta := &cluster.ObjectMeta
	if len(objectMeta.Finalizers) == 0 {
		logger.Infof("no finalizers to remove from cluster %s", name)
		return
	}
	objectMeta.Finalizers = nil
	_, err = h.k8shelper.RookClientset.CephV1().CephClusters(cluster.Namespace).Update(cluster)
	if err != nil {
		logger.Errorf("failed to remove finalizers from cluster %s. %+v", objectMeta.Name, err)
		return
	}
	logger.Infof("removed finalizers from cluster %s", objectMeta.Name)
}

func (h *CephInstaller) checkCephHealthStatus(namespace string) {
	clusterResource, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(namespace, metav1.GetOptions{})
	assert.Nil(h.T(), err)
	clusterPhase := string(clusterResource.Status.Phase)
	if clusterPhase != "Ready" && clusterPhase != "Connected" {
		assert.Equal(h.T(), "Ready", string(clusterResource.Status.Phase))
	}

	// Depending on the tests, the health may be fluctuating with different components being started or stopped.
	// If needed, give it a few seconds to settle and check the status again.
	if clusterResource.Status.CephStatus.Health != "HEALTH_OK" {
		time.Sleep(10 * time.Second)
		clusterResource, err = h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(namespace, metav1.GetOptions{})
		assert.Nil(h.T(), err)
	}

	// The health status is not stable enough for the integration tests to rely on.
	// We should enable this check if we can get the ceph status to be stable despite all the changing configurations performed by rook.
	//assert.Equal(h.T(), "HEALTH_OK", clusterResource.Status.CephStatus.Health)
	assert.NotEqual(h.T(), "", clusterResource.Status.CephStatus.LastChecked)

	// Print the details if the health is not ok
	if clusterResource.Status.CephStatus.Health != "HEALTH_OK" {
		logger.Errorf("Ceph health status: %s", clusterResource.Status.CephStatus.Health)
		for name, message := range clusterResource.Status.CephStatus.Details {
			logger.Errorf("Ceph health message: %s. %s: %s", name, message.Severity, message.Message)
		}
	}
}

func (h *CephInstaller) cleanupDir(node, dir string) error {
	resources := h.Manifests.GetCleanupPod(node, dir)
	_, err := h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...)
	return err
}

func (h *CephInstaller) verifyDirCleanup(node, dir string) error {
	resources := h.Manifests.GetCleanupVerificationPod(node, dir)
	_, err := h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...)
	return err
}

func (h *CephInstaller) CollectOperatorLog(suiteName, testName, namespace string) {
	if !h.T().Failed() && Env.Logs != "all" {
		return
	}
	name := fmt.Sprintf("%s_%s", suiteName, testName)
	h.k8shelper.CollectPodLogsFromLabel(cephOperatorLabel, namespace, name, Env.HostType)
}

func (h *CephInstaller) GatherAllRookLogs(testName string, namespaces ...string) {
	if !h.T().Failed() && Env.Logs != "all" {
		return
	}
	logger.Infof("gathering all logs from the test")
	for _, namespace := range namespaces {
		h.k8shelper.GetLogsFromNamespace(namespace, testName, Env.HostType)
		h.k8shelper.GetPodDescribeFromNamespace(namespace, testName, Env.HostType)
	}
}

// NewCephInstaller creates new instance of CephInstaller
func NewCephInstaller(t func() *testing.T, clientset *kubernetes.Clientset, useHelm bool, rookVersion string,
	cephVersion cephv1.CephVersionSpec, cleanupHost bool) *CephInstaller {

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
	logger.Infof("Rook Version: %s", rookVersion)
	logger.Infof("Ceph Version: %s", cephVersion.Image)
	h := &CephInstaller{
		Manifests:       NewCephManifests(rookVersion),
		k8shelper:       k8shelp,
		helmHelper:      utils.NewHelmHelper(Env.Helm),
		useHelm:         useHelm,
		k8sVersion:      version.String(),
		CephVersion:     cephVersion,
		cleanupHost:     cleanupHost,
		changeHostnames: rookVersion != Version1_1 && k8shelp.VersionAtLeast("v1.13.0"),
		T:               t,
	}
	flag.Parse()
	return h
}

// WipeClusterDisks runs a disk wipe job on all nodes in the k8s cluster.
func (h *CephInstaller) WipeClusterDisks(namespace string) error {
	// Wipe clean disks on all nodes
	nodes, err := h.GetNodeHostnames()
	if err != nil {
		return fmt.Errorf("failed to get node hostnames. %+v", err)
	}
	var jobNames []string
	for _, node := range nodes {
		// Start the new job
		globalTestJobIndex++
		jobName := fmt.Sprintf("rook-ceph-disk-wipe-%d", globalTestJobIndex)
		jobNames = append(jobNames, jobName)
		job := h.GetDiskWipeJob(node, jobName, namespace)
		_, err := h.k8shelper.KubectlWithStdin(job, createFromStdinArgs...)
		if err != nil {
			return fmt.Errorf("failed to create disk wipe job for host %s. %+v", node, err)
		}
	}

	allJobsAreComplete := func() (done bool, err error) {
		for _, jobName := range jobNames {
			j, err := h.k8shelper.Clientset.BatchV1().Jobs(namespace).Get(jobName, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			if j.Status.Failed > 0 {
				return false, fmt.Errorf("job %s failed", jobName)
			}
			if j.Status.Succeeded == 0 {
				return false, nil
			}
		}
		return true, nil
	}

	// return the error below after cleaning up the jobs
	err = wait.Poll(5*time.Second, 90*time.Second, allJobsAreComplete)
	if err != nil {
		return fmt.Errorf("failed to wait for wipe jobs to complete. %+v", err)
	}
	return nil
}

// GetDiskWipeJob returns a YAML manifest string for a job which will wipe clean the extra
// (non-boot) disks on a node, allowing Ceph to use the disk(s) during its testing.
func (h *CephInstaller) GetDiskWipeJob(nodeName, jobName, namespace string) string {
	// put the wipe job in the cluster namespace so that logs get picked up in failure conditions
	return `apiVersion: batch/v1
kind: Job
metadata:
  name: ` + jobName + `
  namespace: ` + namespace + `
spec:
    template:
      spec:
          restartPolicy: OnFailure
          containers:
              - name: disk-wipe
                image: ` + h.CephVersion.Image + `
                securityContext:
                    privileged: true
                volumeMounts:
                    - name: dev
                      mountPath: /dev
                    - name: run-udev
                      mountPath: /run/udev
                command:
                    - "sh"
                    - "-c"
                    - |
                      set -xEeuo pipefail
                      # Wipe LVM data from disks
                      #
                      udevadm trigger || true
                      vgimport -a || true
                      # We should prepopulate the PVs corresponding to Ceph VGs here.
                      # It's because getting the relationship of the PVs and the VGs
                      # is impossible after removing the VGs.
                      TMP=$(pvs --noheadings --separator=',' -o pv_name,vg_name)
                      PVS=
                      for line in $TMP ; do
                        if [[ $line =~ ,ceph- ]]; then
                          PVS="$PVS ${line%,*}"
                        fi
                      done
                      # Wipe VGs
                      for vg in $(vgs --noheadings --readonly --separator=' ' -o vg_name); do
                        if [[ $vg =~ ^ceph- ]]; then
                          lvremove --yes --force "$vg"
                          vgremove --yes --force "$vg"
                        fi
                      done
                      # Wipe PVs
                      for pv in $PVS; do
                        pvremove --yes --force "$pv"
                        wipefs --all "$pv"
                        dd if=/dev/zero of="$pv" bs=1M count=100 oflag=direct,dsync
                        # do not fail the timeout command
                        # the command seems to hang sometimes for no reason so let's retry
                        set +Ee
                        for i in $(seq 1 3); do
                            timeout --preserve-status 5s sgdisk --zap-all "$pv"
                            if [ "$?" -eq 0 ]; then
                                break
                            fi
                            sleep 5
                        done
                        set -Ee
                      done
                      # Wipe the specific disk in the CI that was running in raw mode
                      set +Ee
                      block=/dev/nvme0n1
                      wipefs --all "$block"
                      dd if=/dev/zero of="$block" bs=1M count=100 oflag=direct,dsync
                      set -Ee
                      # Useful debug commands
                      lsblk
                      blkid
                      df /
                      #
                      # some devices might still be mapped that lock the disks
                      # this can fail with long-gone vestigial devices, so just assume this is successful
                      ls /dev/mapper/ceph-* | xargs -I% -- dmsetup remove --force % || true
                      rm -rf /dev/mapper/ceph-*  # clutter
                      #
                      # ceph-volume setup also leaves ceph-UUID directories in /dev (just clutter)
                      rm -rf /dev/ceph-*
          nodeSelector:
            kubernetes.io/hostname: ` + nodeName + `
          volumes:
              - name: dev
                hostPath:
                  path: /dev
              - name: run-udev
                hostPath:
                  path: /run/udev
`
}

func (h *CephInstaller) addCleanupPolicy(namespace string) error {
	cluster, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(namespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ceph cluster. %+v", err)
	}
	cluster.Spec.CleanupPolicy.Confirmation = cephv1.DeleteDataDirOnHostsConfirmation
	_, err = h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Update(cluster)
	if err != nil {
		return fmt.Errorf("failed to add clean up policy to the cluster. %+v", err)
	}
	logger.Info("successfully added cleanup policy to the ceph cluster")
	return nil
}

func (h *CephInstaller) waitForCleanupJobs(namespace string) error {
	allRookCephCleanupJobs := func() (done bool, err error) {
		appLabelSelector := fmt.Sprintf("app=%s", cluster.CleanupAppName)
		cleanupJobs, err := h.k8shelper.Clientset.BatchV1().Jobs(namespace).List(metav1.ListOptions{LabelSelector: appLabelSelector})
		if err != nil {
			return false, fmt.Errorf("failed to get cleanup jobs. %+v", err)
		}
		// Clean up jobs might take some time to start
		if len(cleanupJobs.Items) == 0 {
			logger.Infof("no jobs with label selector %q found.", appLabelSelector)
			return false, nil
		}
		for _, job := range cleanupJobs.Items {
			logger.Debugf("job %q status: %+v", job.Name, job.Status)
			if job.Status.Failed > 0 {
				return false, fmt.Errorf("job %s failed", job.Name)
			}
			if job.Status.Succeeded == 0 {
				return false, nil
			}
		}
		logger.Infof("cleanup job(s) completed")
		return true, nil
	}

	logger.Info("waiting for job(s) to cleanup the host...")
	err := wait.Poll(5*time.Second, 90*time.Second, allRookCephCleanupJobs)
	if err != nil {
		return fmt.Errorf("failed to wait for clean up jobs to complete. %+v", err)
	}

	logger.Info("successfully executed all the ceph clean up jobs")
	return nil
}
