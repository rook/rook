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
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	"github.com/google/uuid"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	// test with the latest nautilus build
	nautilusTestImage = "ceph/ceph:v14"
	// test with the latest nautilus build. ceph-volume is not allowing OSDs on partitions on v14.2.13 and newer.
	nautilusTestImageOnPartitions = "ceph/ceph:v14.2.12"
	// test with the latest octopus build
	octopusTestImage = "ceph/ceph:v15"
	// test with the latest octopus build. ceph-volume is not allowing OSDs on partitions on v15.2.8 and newer.
	octopusTestImageOnPartitions = "ceph/ceph:v15.2.7"
	// test with the latest master image
	masterTestImage    = "ceph/daemon-base:latest-master-devel"
	cephOperatorLabel  = "app=rook-ceph-operator"
	defaultclusterName = "test-cluster"
	// if false, expect to create OSDs on raw devices,
	// otherwise use a version of ceph that is compatible with OSDs on partitions
	usePartitionEnvVar = "TEST_OSDS_ON_PARTITIONS"
)

var (
	MasterVersion = cephv1.CephVersionSpec{Image: masterTestImage, AllowUnsupported: true}
)

// CephInstaller wraps installing and uninstalling rook on a platform
type CephInstaller struct {
	Manifests        CephManifests
	k8shelper        *utils.K8sHelper
	hostPathToDelete string
	helmHelper       *utils.HelmHelper
	useHelm          bool
	clusterName      string
	k8sVersion       string
	changeHostnames  bool
	CephVersion      cephv1.CephVersionSpec
	T                func() *testing.T
	cleanupHost      bool
}

func NautilusVersion() cephv1.CephVersionSpec {
	if os.Getenv(usePartitionEnvVar) == "false" {
		return cephv1.CephVersionSpec{Image: nautilusTestImage}
	}
	return cephv1.CephVersionSpec{Image: nautilusTestImageOnPartitions}
}

func OctopusVersion() cephv1.CephVersionSpec {
	if os.Getenv(usePartitionEnvVar) == "false" {
		return cephv1.CephVersionSpec{Image: octopusTestImage}
	}
	return cephv1.CephVersionSpec{Image: octopusTestImageOnPartitions}
}

// CreateCephOperator creates rook-operator via kubectl
func (h *CephInstaller) CreateCephOperator(namespace string) (err error) {
	logger.Infof("Starting Rook Operator")
	// creating clusterrolebinding for kubeadm env.
	h.k8shelper.CreateAnonSystemClusterBinding()

	var v1ExtensionsSupported bool
	minVersion := "v1.16.0"
	if h.k8shelper.VersionAtLeast(minVersion) {
		v1ExtensionsSupported = true
	}
	// creating rook resources
	logger.Info("Creating Rook CRDs")
	resources := h.Manifests.GetRookCRDs(v1ExtensionsSupported)
	if _, err = h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...); err != nil {
		return err
	}

	if h.changeHostnames {
		// give nodes a hostname that is different from its k8s node name to confirm that all the daemons will be initialized properly
		err = h.k8shelper.ChangeHostnames()
		assert.NoError(h.T(), err)
	}

	err = h.k8shelper.CreateNamespace(namespace)
	if err != nil {
		if kerrors.IsAlreadyExists(err) {
			logger.Warningf("Namespace %q already exists!!!", namespace)
		} else {
			return fmt.Errorf("failed to create namespace %q. %v", namespace, err)
		}
	}

	// disable admission controller for upgrade test as api version v1 require minimum v0.7 controller runtime and upgrade test still using
	// older version of controller runtime.
	if !utils.IsPlatformOpenShift() && h.k8shelper.VersionAtLeast("v1.16.0") && namespace != "upgrade-ns-system" {
		err = h.startAdmissionController(namespace)
		if err != nil {
			return fmt.Errorf("Failed to start admission controllers: %v", err)
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

func (h *CephInstaller) startAdmissionController(namespace string) error {
	err := h.k8shelper.CreateNamespace(namespace)
	if err != nil {
		if kerrors.IsAlreadyExists(err) {
			logger.Warningf("Namespace %q already exists!!!", namespace)
		} else {
			return fmt.Errorf("failed to create namespace %q. %v", namespace, err)
		}
	}
	if !h.k8shelper.VersionAtLeast("v1.15.0") {
		return nil
	}
	rootPath, err := utils.FindRookRoot()
	if err != nil {
		return fmt.Errorf("failed to find rook root. %v", err)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to find user home directory. %v", err)
	}
	scriptPath := path.Join(rootPath, "tests/scripts/deploy_admission_controller_test.sh")
	err = h.k8shelper.MakeContext().Executor.ExecuteCommandWithEnv([]string{fmt.Sprintf("NAMESPACE=%s", namespace), fmt.Sprintf("HOME=%s", userHome)}, "bash", scriptPath)
	if err != nil {
		return err
	}

	return nil
}

// CreateRookOperatorViaHelm creates rook operator via Helm chart named local/rook present in local repo
func (h *CephInstaller) CreateRookOperatorViaHelm(namespace, chartSettings string) error {
	// creating clusterrolebinding for kubeadm env.
	h.k8shelper.CreateAnonSystemClusterBinding()

	if !utils.IsPlatformOpenShift() {
		if err := h.startAdmissionController(namespace); err != nil {
			return fmt.Errorf("Failed to start admission controllers: %v", err)
		}
	}

	if err := h.helmHelper.InstallLocalRookHelmChart(namespace, chartSettings); err != nil {
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

// Execute a command in the ceph toolbox
func (h *CephInstaller) Execute(command string, parameters []string, namespace string) (error, string) {
	clusterInfo := client.AdminClusterInfo(namespace)
	cmd, args := client.FinalizeCephCommandArgs(command, clusterInfo, parameters, h.k8shelper.MakeContext().ConfigDir)
	result, err := h.k8shelper.MakeContext().Executor.ExecuteCommandWithOutput(cmd, args...)
	if err != nil {
		logger.Warningf("Error executing command %q: <%v>", command, err)
		return err, result
	}
	return nil, result
}

// CreateRookCluster creates rook cluster via kubectl
func (h *CephInstaller) CreateRookCluster(namespace, systemNamespace, storeType string, usePVC bool, storageClassName string,
	mon cephv1.MonSpec, startWithAllNodes bool, skipOSDCreation bool, useCrashPruner bool, cephVersion cephv1.CephVersionSpec) error {

	ctx := context.TODO()
	dataDirHostPath, err := h.initTestDir(namespace)
	if err != nil {
		return fmt.Errorf("failed to create test dir. %+v", err)
	}
	logger.Infof("Creating cluster: namespace=%s, systemNamespace=%s, storeType=%s, dataDirHostPath=%s, usePVC=%v, storageClassName=%s, startWithAllNodes=%t, mons=%+v",
		namespace, systemNamespace, storeType, dataDirHostPath, usePVC, storageClassName, startWithAllNodes, mon)

	logger.Infof("Creating namespace %s", namespace)
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err = h.k8shelper.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !kerrors.IsAlreadyExists(err) {
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
	if _, err := h.k8shelper.Clientset.CoreV1().ConfigMaps(namespace).Create(ctx, customCM, metav1.CreateOptions{}); err != nil {
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

	logger.Infof("Starting Rook Cluster with yaml")
	settings := &clusterSettings{h.clusterName, namespace, storeType, dataDirHostPath, mon.Count, 0, usePVC, storageClassName, skipOSDCreation, cephVersion, useCrashPruner}
	rookCluster := h.Manifests.GetRookCluster(settings)
	logger.Info(rookCluster)
	if _, err := h.k8shelper.KubectlWithStdin(rookCluster, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create rook cluster : %v ", err)
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-ceph-mon", namespace, mon.Count); err != nil {
		return err
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-ceph-mgr", namespace, 1); err != nil {
		return err
	}

	if !skipOSDCreation {
		if err := h.k8shelper.WaitForPodCount("app=rook-ceph-osd", namespace, 1); err != nil {
			return err
		}
	}

	if useCrashPruner {
		if err := h.k8shelper.WaitForCronJob("rook-ceph-crashcollector-pruner", namespace); err != nil {
			return err
		}
	}

	logger.Infof("Rook Cluster started")
	if !skipOSDCreation {
		err = h.k8shelper.WaitForLabeledPodsToRun("app=rook-ceph-osd", namespace)
		return err
	}

	return nil
}

// CreateRookExternalCluster creates rook external cluster via kubectl
func (h *CephInstaller) CreateRookExternalCluster(namespace, firstClusterNamespace string) error {
	ctx := context.TODO()
	dataDirHostPath, err := h.initTestDir(namespace)
	if err != nil {
		return fmt.Errorf("failed to create test dir. %+v", err)
	}
	logger.Infof("Creating external cluster: namespace=%q, firstClusterNamespace=%q", namespace, firstClusterNamespace)

	logger.Infof("Creating namespace %s", namespace)
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err = h.k8shelper.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !kerrors.IsAlreadyExists(err) {
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
	settings := &clusterExternalSettings{namespace, dataDirHostPath}
	rookCluster := h.Manifests.GetRookExternalCluster(settings)
	if _, err := h.k8shelper.KubectlWithStdin(rookCluster, createFromStdinArgs...); err != nil {
		return fmt.Errorf("failed to create rook external cluster. %v ", err)
	}

	logger.Infof("Rook external cluster started")
	return err
}

// InjectRookExternalClusterInfo inject connection information for an external cluster
func (h *CephInstaller) InjectRookExternalClusterInfo(namespace, firstClusterNamespace string) error {
	ctx := context.TODO()
	// get config map
	cm, err := h.GetRookExternalClusterMonConfigMap(firstClusterNamespace)
	if err != nil {
		return fmt.Errorf("failed to get configmap. %v", err)
	}

	// create config map
	_, err = h.k8shelper.Clientset.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create configmap. %v", err)
	}

	// get secret
	secret, err := h.GetRookExternalClusterMonSecret(firstClusterNamespace)
	if err != nil {
		return fmt.Errorf("failed to get secret. %v", err)
	}

	// create secret
	_, err = h.k8shelper.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create secret. %v", err)
	}

	return nil
}

// GetRookExternalClusterMonConfigMap gets the monitor kubernetes configmap of the external cluster
func (h *CephInstaller) GetRookExternalClusterMonConfigMap(namespace string) (*v1.ConfigMap, error) {
	ctx := context.TODO()
	configMapName := "rook-ceph-mon-endpoints"
	externalCM, err := h.k8shelper.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
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
	ctx := context.TODO()
	secretName := "rook-ceph-mon" //nolint:gosec // We safely suppress gosec in tests file

	externalSecret, err := h.k8shelper.Clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret. %v", err)
	}
	newSecret := &v1.Secret{}
	newSecret.Name = externalSecret.Name
	newSecret.Data = externalSecret.Data

	return newSecret, nil
}

func (h *CephInstaller) initTestDir(namespace string) (string, error) {
	val, err := baseTestDir()
	if err != nil {
		return "", err
	}

	h.hostPathToDelete = path.Join(val, "rook-test")
	testDir := path.Join(h.hostPathToDelete, namespace)

	// skip the test dir creation if we are not running under "/data"
	if val != "/data" {
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
		r := rand.Int() //nolint:gosec // We safely suppress gosec in tests file
		testDir = path.Join(testDir, fmt.Sprintf("test-%d", r))
	}
	return testDir, nil
}

// GetNodeHostnames returns the list of nodes in the k8s cluster
func (h *CephInstaller) GetNodeHostnames() ([]string, error) {
	ctx := context.TODO()
	nodes, err := h.k8shelper.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
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
	mon cephv1.MonSpec, startWithAllNodes bool, rbdMirrorWorkers int, skipOSDCreation bool, rookVersion string) (bool, error) {

	ctx := context.TODO()
	var err error
	k8sversion := h.k8shelper.GetK8sServerVersion()

	logger.Infof("Installing rook on k8s %s", k8sversion)

	startDiscovery := true
	onamespace := namespace
	// Create rook operator
	if h.useHelm {
		// disable the discovery daemonset with the helm chart
		settings := "enableDiscoveryDaemon=false,image.tag=master"
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
		h.k8shelper.GetLogsFromNamespace(onamespace, "test-setup", utils.TestEnvName())
		logger.Error("rook-ceph-operator is not Running, abort!")
		return false, err
	}

	useCrashPruner := rookVersion == VersionMaster

	// Create rook cluster
	err = h.CreateRookCluster(namespace, onamespace, storeType, usePVC, storageClassName,
		cephv1.MonSpec{Count: mon.Count, AllowMultiplePerNode: mon.AllowMultiplePerNode}, startWithAllNodes,
		skipOSDCreation, useCrashPruner, h.CephVersion)
	if err != nil {
		logger.Errorf("Rook cluster %s not installed, error -> %v", namespace, err)
		return false, err
	}

	discovery, err := h.k8shelper.Clientset.AppsV1().DaemonSets(onamespace).Get(ctx, "rook-discover", metav1.GetOptions{})
	if startDiscovery {
		assert.NoError(h.T(), err)
		assert.NotNil(h.T(), discovery)
	} else {
		assert.Error(h.T(), err)
		assert.True(h.T(), kerrors.IsNotFound(err))
	}

	// Create rook client
	err = h.CreateRookToolbox(namespace)
	if err != nil {
		logger.Errorf("Rook toolbox in cluster %s not installed, error -> %v", namespace, err)
		return false, err
	}
	logger.Infof("installed rook operator and cluster : %s on k8s %s", namespace, h.k8sVersion)

	if !utils.IsPlatformOpenShift() && h.k8shelper.VersionAtLeast("v1.16.0") && namespace != "upgrade-ns" {
		if !h.k8shelper.IsPodInExpectedState("rook-ceph-admission-controller", onamespace, "Running") {
			assert.Fail(h.T(), "admission controller is not running")
		}
	}

	return true, nil
}

// UninstallRook uninstalls rook from k8s
func (h *CephInstaller) UninstallRook(namespace string) {
	h.UninstallRookFromMultipleNS(SystemNamespace(namespace), namespace)
}

// UninstallRookFromMultipleNS uninstalls rook from multiple namespaces in k8s
func (h *CephInstaller) UninstallRookFromMultipleNS(systemNamespace string, namespaces ...string) {
	ctx := context.TODO()
	// Gather logs after status checks
	h.GatherAllRookLogs(h.T().Name(), append([]string{systemNamespace}, namespaces...)...)

	logger.Infof("Uninstalling Rook")
	var err error
	for clusterNum, namespace := range namespaces {
		if h.cleanupHost {
			//Add cleanup policy to the ceph cluster
			// Add cleanup policy to the ceph cluster but NOT the external one
			if clusterNum == 0 {
				err = h.addCleanupPolicy(namespace)
				assert.NoError(h.T(), err)
			}
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
		pools, err := h.k8shelper.RookClientset.CephV1().CephBlockPools(namespace).List(ctx, metav1.ListOptions{})
		assert.NoError(h.T(), err, "failed to retrieve pool CRs")
		for _, pool := range pools.Items {
			logger.Infof("found pools: %v", pools)
			assert.Fail(h.T(), fmt.Sprintf("pool %q still exists", pool.Name))
			// Get the operator log
			h.GatherAllRookLogs(h.T().Name()+"poolcheck", systemNamespace)
		}

		err = h.k8shelper.DeleteResourceAndWait(false, "-n", namespace, "cephcluster", h.clusterName)
		checkError(h.T(), err, fmt.Sprintf("cannot remove cluster %s", namespace))

		if h.cleanupHost {
			// The second cluster is external so the cleanup pod will never exist!
			if clusterNum == 0 {
				err = h.waitForCleanupJobs(namespace)
				assert.NoError(h.T(), err)
			}
		}

		roles := h.Manifests.GetClusterRoles(namespace, systemNamespace)
		_, err = h.k8shelper.KubectlWithStdin(roles, deleteFromStdinArgs...)
		if err != nil {
			logger.Errorf("failed to delete cluster roles. %v ", err)
		}

		crdCheckerFunc := func() error {
			_, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(ctx, h.clusterName, metav1.GetOptions{})
			// ensure the finalizer(s) are removed
			h.removeClusterFinalizers(namespace)
			return err
		}
		err = h.k8shelper.WaitForCustomResourceDeletion(namespace, crdCheckerFunc)
		checkError(h.T(), err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

		if h.useHelm {
			err = h.helmHelper.DeleteLocalRookHelmChart(namespace, utils.HelmDeployName)
			checkError(h.T(), err, "cannot uninstall helm chart")
		}

		// delete the entire namespace
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
		"cephobjectrealms.ceph.rook.io",
		"cephobjectzonegroups.ceph.rook.io",
		"cephobjectzones.ceph.rook.io",
		"cephfilesystems.ceph.rook.io",
		"cephnfses.ceph.rook.io",
		"cephclients.ceph.rook.io",
		"volumes.rook.io",
		"objectbuckets.objectbucket.io",
		"objectbucketclaims.objectbucket.io",
		"cephrbdmirrors.ceph.rook.io",
		"cephfilesystemmirrors.ceph.rook.io")
	checkError(h.T(), err, "cannot delete CRDs")

	if !h.useHelm {
		logger.Infof("Deleting all the resources in the operator manifest")
		rookOperator := h.Manifests.GetRookOperator(systemNamespace)
		_, err = h.k8shelper.KubectlWithStdin(rookOperator, deleteFromStdinArgs...)
		logger.Infof("DONE deleting all the resources in the operator manifest")
		checkError(h.T(), err, "cannot uninstall rook-operator")
	}

	err = h.k8shelper.Clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(ctx, "rook-ceph-webhook", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete webhook configuration")
	err = h.k8shelper.Clientset.RbacV1().RoleBindings(systemNamespace).Delete(ctx, "rook-ceph-system", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "rook-ceph-global", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "rook-ceph-mgr-cluster", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete role binding")
	err = h.k8shelper.Clientset.CoreV1().ServiceAccounts(systemNamespace).Delete(ctx, "rook-ceph-system", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete service account")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete(ctx, "rook-ceph-cluster-mgmt", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete(ctx, "rook-ceph-mgr-cluster", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete(ctx, "rook-ceph-mgr-system", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete(ctx, "rook-ceph-global", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role")
	err = h.k8shelper.Clientset.RbacV1().Roles(systemNamespace).Delete(ctx, "rook-ceph-system", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete role")

	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "rbd-csi-nodeplugin", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete(ctx, "rbd-csi-nodeplugin", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "rbd-csi-provisioner-role", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete(ctx, "rbd-external-provisioner-runner", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role")

	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "cephfs-csi-nodeplugin", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete(ctx, "cephfs-csi-nodeplugin", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "cephfs-csi-provisioner-role", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete(ctx, "cephfs-external-provisioner-runner", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role")

	err = h.k8shelper.Clientset.PolicyV1beta1().PodSecurityPolicies().Delete(ctx, "rook-privileged", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete policy")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete(ctx, "psp:rook", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "rook-ceph-system-psp", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "rook-csi-rbd-provisioner-sa-psp", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "rook-csi-rbd-plugin-sa-psp", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "rook-csi-cephfs-provisioner-sa-psp", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "rook-csi-cephfs-plugin-sa-psp", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")

	err = h.k8shelper.Clientset.CoreV1().ConfigMaps(systemNamespace).Delete(ctx, "csi-rbd-config", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete config map")
	err = h.k8shelper.Clientset.CoreV1().ConfigMaps(systemNamespace).Delete(ctx, "csi-cephfs-config", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete config map")

	err = h.k8shelper.Clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "rook-ceph-object-bucket", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role binding")
	err = h.k8shelper.Clientset.RbacV1().ClusterRoles().Delete(ctx, "rook-ceph-object-bucket", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete cluster role")

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
		_, err = h.k8shelper.RestoreHostnames()
		assert.NoError(h.T(), err)
	}

	// wait a bit longer for the system namespace to be cleaned up after their deletion
	for i := 0; i < 15; i++ {
		_, err := h.k8shelper.Clientset.CoreV1().Namespaces().Get(ctx, systemNamespace, metav1.GetOptions{})
		if err != nil && kerrors.IsNotFound(err) {
			logger.Infof("system namespace %q removed", systemNamespace)
			break
		}
		logger.Infof("system namespace %q still found...", systemNamespace)
		time.Sleep(5 * time.Second)
	}
}

func (h *CephInstaller) removeClusterFinalizers(namespace string) {
	ctx := context.TODO()
	// Get the latest cluster instead of using the same instance in case it has been changed
	cluster, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(ctx, h.clusterName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to remove finalizer. failed to get cluster. %+v", err)
		return
	}
	objectMeta := &cluster.ObjectMeta
	if len(objectMeta.Finalizers) == 0 {
		logger.Infof("no finalizers to remove from cluster %s", namespace)
		return
	}
	objectMeta.Finalizers = nil
	_, err = h.k8shelper.RookClientset.CephV1().CephClusters(cluster.Namespace).Update(ctx, cluster, metav1.UpdateOptions{})
	if err != nil {
		logger.Errorf("failed to remove finalizers from cluster %s. %+v", objectMeta.Name, err)
		return
	}
	logger.Infof("removed finalizers from cluster %s", objectMeta.Name)
}

func (h *CephInstaller) checkCephHealthStatus(namespace string) {
	ctx := context.TODO()
	clusterResource, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(ctx, h.clusterName, metav1.GetOptions{})
	assert.Nil(h.T(), err)
	clusterPhase := string(clusterResource.Status.Phase)
	if clusterPhase != "Ready" && clusterPhase != "Connected" && clusterPhase != "Progressing" {
		assert.Equal(h.T(), "Ready", string(clusterResource.Status.Phase))
	}

	// Depending on the tests, the health may be fluctuating with different components being started or stopped.
	// If needed, give it a few seconds to settle and check the status again.
	if clusterResource.Status.CephStatus.Health != "HEALTH_OK" {
		time.Sleep(10 * time.Second)
		clusterResource, err = h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(ctx, h.clusterName, metav1.GetOptions{})
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
	resources := h.GetCleanupPod(node, dir)
	_, err := h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...)
	return err
}

func (h *CephInstaller) verifyDirCleanup(node, dir string) error {
	resources := h.GetCleanupVerificationPod(node, dir)
	_, err := h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...)
	return err
}

func (h *CephInstaller) CollectOperatorLog(suiteName, testName, namespace string) {
	if !h.T().Failed() && TestLogCollectionLevel() != "all" {
		return
	}
	name := fmt.Sprintf("%s_%s", suiteName, testName)
	h.k8shelper.CollectPodLogsFromLabel(cephOperatorLabel, namespace, name, utils.TestEnvName())
}

func (h *CephInstaller) GatherAllRookLogs(testName string, namespaces ...string) {
	if !h.T().Failed() && TestLogCollectionLevel() != "all" {
		return
	}
	logger.Infof("gathering all logs from the test")
	for _, namespace := range namespaces {
		h.k8shelper.GetLogsFromNamespace(namespace, testName, utils.TestEnvName())
		h.k8shelper.GetPodDescribeFromNamespace(namespace, testName, utils.TestEnvName())
		h.k8shelper.GetEventsFromNamespace(namespace, testName, utils.TestEnvName())
	}
}

// NewCephInstaller creates new instance of CephInstaller
func NewCephInstaller(t func() *testing.T, clientset *kubernetes.Clientset, useHelm bool, clusterName, rookVersion string,
	cephVersion cephv1.CephVersionSpec, cleanupHost bool) *CephInstaller {

	// By default set a cluster name that is different from the namespace so we don't rely on the namespace
	// in expected places
	if clusterName == "" {
		clusterName = defaultclusterName
	}

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
		helmHelper:      utils.NewHelmHelper(testHelmPath()),
		useHelm:         useHelm,
		clusterName:     clusterName,
		k8sVersion:      version.String(),
		CephVersion:     cephVersion,
		cleanupHost:     cleanupHost,
		changeHostnames: k8shelp.VersionAtLeast("v1.18.0"),
		T:               t,
	}
	flag.Parse()
	return h
}

// GetCleanupPod gets a cleanup Pod that cleans up the dataDirHostPath
func (h *CephInstaller) GetCleanupPod(node, removalDir string) string {
	return `apiVersion: batch/v1
kind: Job
metadata:
  name: rook-cleanup-` + uuid.Must(uuid.NewRandom()).String() + `
spec:
    template:
      spec:
          restartPolicy: Never
          containers:
              - name: rook-cleaner
                image: rook/ceph:` + VersionMaster + `
                securityContext:
                    privileged: true
                volumeMounts:
                    - name: cleaner
                      mountPath: /scrub
                command:
                    - "sh"
                    - "-c"
                    - "rm -rf /scrub/*"
          nodeSelector:
            kubernetes.io/hostname: ` + node + `
          volumes:
              - name: cleaner
                hostPath:
                   path:  ` + removalDir
}

// GetCleanupVerificationPod verifies that the dataDirHostPath is empty
func (h *CephInstaller) GetCleanupVerificationPod(node, hostPathDir string) string {
	return `apiVersion: batch/v1
kind: Job
metadata:
  name: rook-verify-cleanup-` + uuid.Must(uuid.NewRandom()).String() + `
spec:
    template:
      spec:
          restartPolicy: Never
          containers:
              - name: rook-cleaner
                image: rook/ceph:` + VersionMaster + `
                securityContext:
                    privileged: true
                volumeMounts:
                    - name: cleaner
                      mountPath: /scrub
                command:
                    - "sh"
                    - "-c"
                    - |
                      set -xEeuo pipefail
                      #Assert dataDirHostPath is empty
                      if [ "$(ls -A /scrub/)" ]; then
                          exit 1
                      fi
          nodeSelector:
            kubernetes.io/hostname: ` + node + `
          volumes:
              - name: cleaner
                hostPath:
                   path:  ` + hostPathDir
}

func (h *CephInstaller) addCleanupPolicy(namespace string) error {
	ctx := context.TODO()
	cluster, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(ctx, h.clusterName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ceph cluster. %+v", err)
	}
	cluster.Spec.CleanupPolicy.Confirmation = cephv1.DeleteDataDirOnHostsConfirmation
	_, err = h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Update(ctx, cluster, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to add clean up policy to the cluster. %+v", err)
	}
	logger.Info("successfully added cleanup policy to the ceph cluster")
	return nil
}

func (h *CephInstaller) waitForCleanupJobs(namespace string) error {
	ctx := context.TODO()
	allRookCephCleanupJobs := func() (done bool, err error) {
		appLabelSelector := fmt.Sprintf("app=%s", cluster.CleanupAppName)
		cleanupJobs, err := h.k8shelper.Clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{LabelSelector: appLabelSelector})
		if err != nil {
			return false, fmt.Errorf("failed to get cleanup jobs. %+v", err)
		}
		// Clean up jobs might take some time to start
		if len(cleanupJobs.Items) == 0 {
			logger.Infof("no jobs with label selector %q found.", appLabelSelector)
			return false, nil
		}
		for _, job := range cleanupJobs.Items {
			logger.Infof("job %q status: %+v", job.Name, job.Status)
			if job.Status.Failed > 0 {
				return false, fmt.Errorf("job %s failed", job.Name)
			}
			if job.Status.Succeeded == 1 {
				l, err := h.k8shelper.Kubectl("-n", namespace, "logs", fmt.Sprintf("job.batch/%s", job.Name))
				if err != nil {
					logger.Errorf("cannot get logs for pod %s. %v", job.Name, err)
				}
				rawData := []byte(l)
				logger.Infof("cleanup job %s done. logs: %s", job.Name, string(rawData))
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
