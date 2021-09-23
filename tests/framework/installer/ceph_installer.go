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
	"github.com/pkg/errors"
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
	nautilusTestImage = "quay.io/ceph/ceph:v14"
	// nautilusTestImagePartition is the image that contains working ceph-volume code to deploy OSDs on partitions
	// currently only used for the upgrade test from 1.5 to 1.6, this cannot be changed to v14 since ceph-volume will fail to deploy OSD on partition on Rook 1.5
	nautilusTestImagePartition = "quay.io/ceph/ceph:v14.2.12"
	// test with the latest octopus build
	octopusTestImage = "quay.io/ceph/ceph:v15"
	// test with the latest pacific build
	pacificTestImage = "quay.io/ceph/ceph:v16.2.6"
	// test with the latest master image
	masterTestImage    = "ceph/daemon-base:latest-master-devel"
	cephOperatorLabel  = "app=rook-ceph-operator"
	defaultclusterName = "test-cluster"

	clusterCustomSettings = `
[global]
osd_pool_default_size = 1
bdev_flock_retry = 20
`
)

var (
	NautilusVersion          = cephv1.CephVersionSpec{Image: nautilusTestImage}
	NautilusPartitionVersion = cephv1.CephVersionSpec{Image: nautilusTestImagePartition}
	OctopusVersion           = cephv1.CephVersionSpec{Image: octopusTestImage}
	PacificVersion           = cephv1.CephVersionSpec{Image: pacificTestImage}
	MasterVersion            = cephv1.CephVersionSpec{Image: masterTestImage, AllowUnsupported: true}
)

// CephInstaller wraps installing and uninstalling rook on a platform
type CephInstaller struct {
	settings         *TestCephSettings
	Manifests        CephManifests
	k8shelper        *utils.K8sHelper
	hostPathToDelete string
	helmHelper       *utils.HelmHelper
	k8sVersion       string
	changeHostnames  bool
	T                func() *testing.T
}

// CreateCephOperator creates rook-operator via kubectl
func (h *CephInstaller) CreateCephOperator() (err error) {
	// creating rook resources
	logger.Info("Creating Rook CRDs")
	resources := h.Manifests.GetCRDs(h.k8shelper)
	if _, err = h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...); err != nil {
		return err
	}

	if h.changeHostnames {
		// give nodes a hostname that is different from its k8s node name to confirm that all the daemons will be initialized properly
		err = h.k8shelper.ChangeHostnames()
		assert.NoError(h.T(), err)
	}

	// The operator namespace needs to be created explicitly, while the cluster namespace is created with the common.yaml
	if err := h.k8shelper.CreateNamespace(h.settings.OperatorNamespace); err != nil {
		return err
	}

	// Create the namespace and RBAC before starting the operator
	_, err = h.k8shelper.KubectlWithStdin(h.Manifests.GetCommon(), createFromStdinArgs...)
	if err != nil {
		return errors.Errorf("Failed to create rook-operator pod: %v ", err)
	}

	err = h.startAdmissionController()
	if err != nil {
		return errors.Errorf("Failed to start admission controllers: %v", err)
	}

	_, err = h.k8shelper.KubectlWithStdin(h.Manifests.GetOperator(), createFromStdinArgs...)
	if err != nil {
		return errors.Errorf("Failed to create rook-operator pod: %v", err)
	}

	logger.Infof("Rook operator started")
	return nil
}

func (h *CephInstaller) startAdmissionController() error {
	if !h.k8shelper.VersionAtLeast("v1.16.0") {
		logger.Info("skipping the admission controller on K8s version older than v1.16")
		return nil
	}
	if !h.settings.EnableAdmissionController {
		logger.Info("skipping admission controller for this test suite")
		return nil
	}
	if utils.IsPlatformOpenShift() {
		logger.Info("skipping the admission controller on OpenShift")
		return nil
	}

	rootPath, err := utils.FindRookRoot()
	if err != nil {
		return errors.Errorf("failed to find rook root. %v", err)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return errors.Errorf("failed to find user home directory. %v", err)
	}
	scriptPath := path.Join(rootPath, "tests/scripts/deploy_admission_controller_test.sh")
	err = h.k8shelper.MakeContext().Executor.ExecuteCommandWithEnv([]string{fmt.Sprintf("NAMESPACE=%s", h.settings.OperatorNamespace), fmt.Sprintf("HOME=%s", userHome)}, "bash", scriptPath)
	if err != nil {
		return err
	}

	return nil
}

func (h *CephInstaller) WaitForToolbox(namespace string) error {
	if err := h.k8shelper.WaitForLabeledPodsToRun("app=rook-ceph-tools", namespace); err != nil {
		return errors.Wrap(err, "Rook Toolbox couldn't start")
	}
	logger.Infof("Rook Toolbox started")

	podNames, err := h.k8shelper.GetPodNamesForApp("rook-ceph-tools", namespace)
	assert.NoError(h.T(), err)
	for _, podName := range podNames {
		// All e2e tests should run ceph commands in the toolbox since we are not inside a container
		logger.Infof("found active toolbox pod: %q", podName)
		client.RunAllCephCommandsInToolboxPod = podName
		return nil
	}

	return errors.Errorf("could not find toolbox pod")
}

// CreateRookToolbox creates rook-ceph-tools via kubectl
func (h *CephInstaller) CreateRookToolbox(manifests CephManifests) (err error) {
	logger.Infof("Starting Rook toolbox")

	_, err = h.k8shelper.KubectlWithStdin(manifests.GetToolbox(), createFromStdinArgs...)
	if err != nil {
		return errors.Wrap(err, "failed to create rook-toolbox pod")
	}

	return h.WaitForToolbox(manifests.Settings().Namespace)
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

// CreateCephCluster creates rook cluster via kubectl
func (h *CephInstaller) CreateCephCluster() error {

	ctx := context.TODO()
	var err error
	h.settings.DataDirHostPath, err = h.initTestDir(h.settings.Namespace)
	if err != nil {
		return errors.Errorf("failed to create test dir. %+v", err)
	}
	logger.Infof("Creating cluster with settings: %+v", h.settings)

	logger.Infof("Creating custom ceph.conf settings")
	customSettings := map[string]string{"config": clusterCustomSettings}
	customCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-config-override",
			Namespace: h.settings.Namespace,
		},
		Data: customSettings,
	}
	if _, err := h.k8shelper.Clientset.CoreV1().ConfigMaps(h.settings.Namespace).Create(ctx, customCM, metav1.CreateOptions{}); err != nil {
		return errors.Errorf("failed to create custom ceph.conf. %+v", err)
	}

	logger.Info("Starting Rook Cluster")
	rookCluster := h.Manifests.GetCephCluster()
	logger.Info(rookCluster)
	maxTry := 10
	for i := 0; i < maxTry; i++ {
		_, err := h.k8shelper.KubectlWithStdin(rookCluster, createFromStdinArgs...)
		if err == nil {
			break
		}
		if i == maxTry-1 {
			return errors.Errorf("failed to create rook cluster. %v", err)
		}
		logger.Infof("failed to create rook cluster, trying again... %v", err)
		time.Sleep(5 * time.Second)
	}

	return nil
}

func (h *CephInstaller) waitForCluster() error {
	if err := h.k8shelper.WaitForPodCount("app=rook-ceph-mon", h.settings.Namespace, h.settings.Mons); err != nil {
		return err
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-ceph-mgr", h.settings.Namespace, 1); err != nil {
		return err
	}

	if !h.settings.SkipOSDCreation {
		if err := h.k8shelper.WaitForPodCount("app=rook-ceph-osd", h.settings.Namespace, 1); err != nil {
			return err
		}
	}

	if h.settings.UseCrashPruner {
		if err := h.k8shelper.WaitForCronJob("rook-ceph-crashcollector-pruner", h.settings.Namespace); err != nil {
			return err
		}
	}

	logger.Infof("Rook Cluster started")
	if !h.settings.SkipOSDCreation {
		return h.k8shelper.WaitForLabeledPodsToRun("app=rook-ceph-osd", h.settings.Namespace)
	}

	return nil
}

// CreateRookExternalCluster creates rook external cluster via kubectl
func (h *CephInstaller) CreateRookExternalCluster(externalManifests CephManifests) error {
	var err error
	externalSettings := externalManifests.Settings()
	externalSettings.DataDirHostPath, err = h.initTestDir(externalSettings.Namespace)
	if err != nil {
		return errors.Errorf("failed to create test dir. %+v", err)
	}

	logger.Infof("Creating external cluster %q with core storage namespace %q", externalSettings.Namespace, h.settings.Namespace)

	logger.Infof("Creating external cluster roles")
	roles := externalManifests.GetCommonExternal()
	if _, err := h.k8shelper.KubectlWithStdin(roles, createFromStdinArgs...); err != nil {
		return errors.Wrap(err, "failed to create cluster roles")
	}

	// Inject connection information from the first cluster
	logger.Info("Injecting cluster connection information")
	err = h.injectRookExternalClusterInfo(externalSettings)
	if err != nil {
		return errors.Wrap(err, "failed to inject cluster information into the external cluster")
	}

	// Start the external cluster
	logger.Infof("Starting Rook External Cluster with yaml")
	rookCluster := externalManifests.GetExternalCephCluster()
	if _, err := h.k8shelper.KubectlWithStdin(rookCluster, createFromStdinArgs...); err != nil {
		return errors.Wrap(err, "failed to create rook external cluster")
	}

	logger.Infof("Running toolbox on external namespace %q", externalSettings.Namespace)
	if err := h.CreateRookToolbox(externalManifests); err != nil {
		return errors.Wrap(err, "failed to start toolbox on external cluster")
	}

	var clusterStatus cephv1.ClusterStatus
	for i := 0; i < 8; i++ {
		ctx := context.TODO()
		clusterResource, err := h.k8shelper.RookClientset.CephV1().CephClusters(externalSettings.Namespace).Get(ctx, externalSettings.ClusterName, metav1.GetOptions{})
		if err != nil {
			logger.Warningf("failed to get external cluster CR, retrying. %v", err)
			time.Sleep(time.Second * 5)
			continue
		}

		clusterStatus = clusterResource.Status
		clusterPhase := string(clusterResource.Status.Phase)
		if clusterPhase != "Connected" {
			logger.Warningf("failed to start external cluster, retrying, state: %v", clusterResource.Status)
			time.Sleep(time.Second * 5)
		} else if clusterPhase == "Connected" {
			logger.Info("Rook external cluster connected")
			return nil
		}

	}

	return errors.Errorf("failed to start external cluster, state: %v", clusterStatus)
}

// InjectRookExternalClusterInfo inject connection information for an external cluster
func (h *CephInstaller) injectRookExternalClusterInfo(externalSettings *TestCephSettings) error {
	ctx := context.TODO()
	// get config map
	cm, err := h.GetRookExternalClusterMonConfigMap()
	if err != nil {
		return errors.Errorf("failed to get configmap. %v", err)
	}

	// create config map
	_, err = h.k8shelper.Clientset.CoreV1().ConfigMaps(externalSettings.Namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		return errors.Errorf("failed to create configmap. %v", err)
	}

	// get secret
	secret, err := h.GetRookExternalClusterMonSecret()
	if err != nil {
		return errors.Errorf("failed to get secret. %v", err)
	}

	// create secret
	_, err = h.k8shelper.Clientset.CoreV1().Secrets(externalSettings.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return errors.Errorf("failed to create secret. %v", err)
	}

	return nil
}

// GetRookExternalClusterMonConfigMap gets the monitor kubernetes configmap of the external cluster
func (h *CephInstaller) GetRookExternalClusterMonConfigMap() (*v1.ConfigMap, error) {
	ctx := context.TODO()
	configMapName := "rook-ceph-mon-endpoints"
	externalCM, err := h.k8shelper.Clientset.CoreV1().ConfigMaps(h.settings.Namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Errorf("failed to get secret. %v", err)
	}
	newCM := &v1.ConfigMap{}
	newCM.Name = externalCM.Name
	newCM.Data = externalCM.Data

	return newCM, nil
}

// GetRookExternalClusterMonSecret gets the monitor kubernetes secret of the external cluster
func (h *CephInstaller) GetRookExternalClusterMonSecret() (*v1.Secret, error) {
	ctx := context.TODO()
	secretName := "rook-ceph-mon" //nolint:gosec // We safely suppress gosec in tests file

	externalSecret, err := h.k8shelper.Clientset.CoreV1().Secrets(h.settings.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Errorf("failed to get secret. %v", err)
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
		return nil, errors.Errorf("failed to get k8s nodes. %+v", err)
	}
	var names []string
	for _, node := range nodes.Items {
		names = append(names, node.Labels[v1.LabelHostname])
	}

	return names, nil
}

func (h *CephInstaller) installRookOperator() (bool, error) {
	ctx := context.TODO()
	var err error

	startDiscovery := h.settings.EnableDiscovery

	h.k8shelper.CreateAnonSystemClusterBinding()

	// Create rook operator
	logger.Infof("Starting Rook Operator")
	if h.settings.UseHelm {
		// enable the discovery daemonset with the helm chart
		startDiscovery = true
		err := h.CreateRookOperatorViaHelm(map[string]interface{}{
			"enableDiscoveryDaemon": true,
			"image":                 map[string]interface{}{"tag": LocalBuildTag},
		})
		if err != nil {
			return false, errors.Wrap(err, "failed to configure helm")
		}
	} else {
		err := h.CreateCephOperator()
		if err != nil {
			return false, errors.Wrap(err, "failed to configure ceph operator")
		}
	}
	if !h.k8shelper.IsPodInExpectedState("rook-ceph-operator", h.settings.OperatorNamespace, "Running") {
		logger.Error("rook-ceph-operator is not running")
		h.k8shelper.GetLogsFromNamespace(h.settings.OperatorNamespace, "test-setup", utils.TestEnvName())
		logger.Error("rook-ceph-operator is not Running, abort!")
		return false, err
	}

	discovery, err := h.k8shelper.Clientset.AppsV1().DaemonSets(h.settings.OperatorNamespace).Get(ctx, "rook-discover", metav1.GetOptions{})
	if startDiscovery {
		assert.NoError(h.T(), err)
		assert.NotNil(h.T(), discovery)
	} else {
		assert.Error(h.T(), err)
		assert.True(h.T(), kerrors.IsNotFound(err))
	}

	return true, nil
}

func (h *CephInstaller) InstallRook() (bool, error) {
	if h.settings.RookVersion != LocalBuildTag {
		// make sure we have the images from a previous release locally so the test doesn't hit a timeout
		assert.NoError(h.T(), h.k8shelper.GetDockerImage("rook/ceph:"+h.settings.RookVersion))
	}

	assert.NoError(h.T(), h.k8shelper.GetDockerImage(h.settings.CephVersion.Image))

	k8sversion := h.k8shelper.GetK8sServerVersion()

	logger.Infof("Installing rook on K8s %s", k8sversion)
	success, err := h.installRookOperator()
	if err != nil {
		return false, err
	}
	if !success {
		return false, nil
	}

	if h.settings.UseHelm {
		err = h.CreateRookCephClusterViaHelm(map[string]interface{}{
			"image": "rook/ceph:" + LocalBuildTag,
		})
		if err != nil {
			return false, errors.Wrap(err, "failed to install ceph cluster using Helm")
		}
	} else {
		// Create rook cluster
		err = h.CreateCephCluster()
		if err != nil {
			logger.Errorf("Cluster %q install failed. %v", h.settings.Namespace, err)
			return false, err
		}
	}

	logger.Info("Waiting for Rook Cluster")
	if err := h.waitForCluster(); err != nil {
		return false, err
	}

	if h.settings.UseHelm {
		err := h.WaitForToolbox(h.settings.Namespace)
		if err != nil {
			return false, err
		}
	} else {
		err = h.CreateRookToolbox(h.Manifests)
		if err != nil {
			return false, errors.Wrapf(err, "failed to install toolbox in cluster %s", h.settings.Namespace)
		}
	}

	const loopCount = 20
	for i := 0; i < loopCount; i++ {
		_, err = client.Status(h.k8shelper.MakeContext(), client.AdminClusterInfo(h.settings.Namespace))
		if err == nil {
			logger.Infof("toolbox ready")
			break
		}
		logger.Infof("toolbox is not ready")
		if i == loopCount-1 {
			return false, errors.Errorf("toolbox cannot connect to cluster")
		}

		time.Sleep(5 * time.Second)
	}

	if h.settings.UseHelm {
		logger.Infof("Confirming ceph cluster installed correctly")
		if err := h.ConfirmHelmClusterInstalledCorrectly(); err != nil {
			return false, errors.Wrap(err, "the ceph cluster storage CustomResources did not install correctly")
		}
		if !h.settings.RetainHelmDefaultStorageCRs {
			err = h.RemoveRookCephClusterHelmDefaultCustomResources()
			if err != nil {
				return false, errors.Wrap(err, "failed to remove the default helm CustomResources")
			}
		}
	}

	logger.Infof("installed rook operator and cluster %s on k8s %s", h.settings.Namespace, h.k8sVersion)

	return true, nil
}

// UninstallRook uninstalls rook from k8s
func (h *CephInstaller) UninstallRook() {
	h.UninstallRookFromMultipleNS(h.Manifests)
}

// UninstallRookFromMultipleNS uninstalls rook from multiple namespaces in k8s
func (h *CephInstaller) UninstallRookFromMultipleNS(manifests ...CephManifests) {
	ctx := context.TODO()
	var clusterNamespaces []string
	for _, manifest := range manifests {
		clusterNamespaces = append(clusterNamespaces, manifest.Settings().Namespace)
	}

	// Gather logs after status checks
	h.GatherAllRookLogs(h.T().Name(), append([]string{h.settings.OperatorNamespace}, clusterNamespaces...)...)

	// If test failed do not teardown and leave the cluster in the state it is
	if h.T().Failed() {
		logger.Info("one of the tests failed, leaving the cluster in its bad shape for investigation")
		return
	}

	logger.Infof("Uninstalling Rook")
	var err error
	skipOperatorCleanup := false
	for _, manifest := range manifests {
		namespace := manifest.Settings().Namespace
		clusterName := manifest.Settings().ClusterName
		if manifest.Settings().SkipCleanupPolicy && manifest.Settings().SkipClusterCleanup {
			logger.Infof("SKIPPING ALL CLEANUP for namespace %q", namespace)
			skipOperatorCleanup = true
			continue
		}

		testCleanupPolicy := !h.settings.UseHelm && !manifest.Settings().IsExternal && !manifest.Settings().SkipCleanupPolicy
		if testCleanupPolicy {
			// Add cleanup policy to the core ceph cluster
			err = h.addCleanupPolicy(namespace, clusterName)
			if err != nil {
				assert.NoError(h.T(), err)
				// no need to check for cleanup policy later if it already failed
				testCleanupPolicy = false
			}

			// if the test passed, check that the ceph status is HEALTH_OK before we tear the cluster down
			if !h.T().Failed() {
				// Only check the Ceph status for the core cluster
				// The check won't work for an external cluster since the core cluster is already gone
				h.checkCephHealthStatus()
			}
		}

		// The pool CRs should already be removed by the tests that created them
		pools, err := h.k8shelper.RookClientset.CephV1().CephBlockPools(namespace).List(ctx, metav1.ListOptions{})
		assert.NoError(h.T(), err, "failed to retrieve pool CRs")
		for _, pool := range pools.Items {
			logger.Infof("found pools: %v", pools)
			assert.Fail(h.T(), fmt.Sprintf("pool %q still exists", pool.Name))
			// Get the operator log
			h.GatherAllRookLogs(h.T().Name()+"poolcheck", h.settings.OperatorNamespace)
		}

		if h.settings.UseHelm {
			// helm rook-ceph-cluster cleanup
			if h.settings.RetainHelmDefaultStorageCRs {
				err = h.RemoveRookCephClusterHelmDefaultCustomResources()
				if err != nil {
					assert.Fail(h.T(), "failed to remove the default helm CustomResources")
				}
			}
			err = h.helmHelper.DeleteLocalRookHelmChart(namespace, CephClusterChartName)
			checkError(h.T(), err, fmt.Sprintf("cannot uninstall helm chart %s", CephClusterChartName))
		} else {
			err = h.k8shelper.DeleteResourceAndWait(false, "-n", namespace, "cephcluster", clusterName)
			checkError(h.T(), err, fmt.Sprintf("cannot remove cluster %s", namespace))

			clusterDeleteRetries := 0
			crdCheckerFunc := func() error {
				_, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(ctx, clusterName, metav1.GetOptions{})
				clusterDeleteRetries++
				if clusterDeleteRetries > 10 {
					// If the operator really isn't going to remove the finalizer, just force remove it
					h.removeClusterFinalizers(namespace, clusterName)
				}

				return err
			}
			err = h.k8shelper.WaitForCustomResourceDeletion(namespace, clusterName, crdCheckerFunc)
			checkError(h.T(), err, fmt.Sprintf("failed to wait for cluster crd %s deletion", namespace))
		}

		if testCleanupPolicy {
			err = h.waitForCleanupJobs(namespace)
			if err != nil {
				assert.NoError(h.T(), err)
				h.GatherAllRookLogs(h.T().Name()+"cleanup-job", append([]string{h.settings.OperatorNamespace}, clusterNamespaces...)...)
			}
		}

		// helm operator cleanup
		if h.settings.UseHelm {
			err = h.helmHelper.DeleteLocalRookHelmChart(namespace, OperatorChartName)
			checkError(h.T(), err, fmt.Sprintf("cannot uninstall helm chart %s", OperatorChartName))

			// delete the entire namespace (in non-helm installs it's removed with the common.yaml)
			err = h.k8shelper.DeleteResourceAndWait(false, "namespace", namespace)
			checkError(h.T(), err, fmt.Sprintf("cannot delete namespace %s", namespace))
			continue
		}

		// Skip the remainder of cluster cleanup if desired
		if manifest.Settings().SkipClusterCleanup {
			logger.Infof("SKIPPING CLUSTER CLEANUP")
			skipOperatorCleanup = true
			continue
		}

		// non-helm cleanup
		if manifest.Settings().IsExternal {
			logger.Infof("Deleting all the resources in the common external manifest")
			_, err = h.k8shelper.KubectlWithStdin(manifest.GetCommonExternal(), deleteFromStdinArgs...)
			if err != nil {
				logger.Errorf("failed to remove common external resources. %v", err)
			} else {
				logger.Infof("done deleting all the resources in the common external manifest")
			}
		} else {
			h.k8shelper.PrintResources(namespace, "cephblockpools.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephclients.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephclusters.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephfilesystemmirrors.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephfilesystems.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephnfses.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephobjectrealms.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephobjectstores.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephobjectstoreusers.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephobjectzonegroups.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephobjectzones.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "cephrbdmirrors.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "objectbucketclaims.ceph.rook.io")
			h.k8shelper.PrintResources(namespace, "objectbuckets.ceph.rook.io")
			h.k8shelper.PrintPodStatus(namespace)
			h.k8shelper.PrintPVs(true)
			logger.Infof("Deleting all the resources in the common manifest")
			_, err = h.k8shelper.KubectlWithStdin(h.Manifests.GetCommon(), deleteFromStdinArgs...)
			if err != nil {
				logger.Errorf("failed to remove common manifest. %v", err)
			} else {
				logger.Infof("done deleting all the resources in the common manifest")
			}
		}
	}

	// Skip the remainder of cluster cleanup if desired
	if skipOperatorCleanup {
		logger.Infof("SKIPPING OPERATOR CLEANUP")
		return
	}

	if !h.settings.UseHelm {
		logger.Infof("Deleting all the resources in the operator manifest")
		_, err = h.k8shelper.KubectlWithStdin(h.Manifests.GetOperator(), deleteFromStdinArgs...)
		if err != nil {
			logger.Errorf("failed to remove operator resources. %v", err)
		} else {
			logger.Infof("done deleting all the resources in the operator manifest")
		}
	}

	logger.Info("removing the CRDs")
	_, err = h.k8shelper.KubectlWithStdin(h.Manifests.GetCRDs(h.k8shelper), deleteFromStdinArgs...)
	if err != nil {
		logger.Errorf("failed to remove CRDS. %v", err)
	} else {
		logger.Infof("done deleting all the CRDs")
	}

	err = h.k8shelper.DeleteResourceAndWait(false, "namespace", h.settings.OperatorNamespace)
	checkError(h.T(), err, fmt.Sprintf("cannot delete operator namespace %s", h.settings.OperatorNamespace))

	err = h.k8shelper.Clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(ctx, "rook-ceph-webhook", metav1.DeleteOptions{})
	checkError(h.T(), err, "failed to delete webhook configuration")

	logger.Infof("done removing the operator from namespace %s", h.settings.OperatorNamespace)
	logger.Infof("removing host data dir %s", h.hostPathToDelete)
	// removing data dir if exists
	if h.hostPathToDelete != "" {
		nodes, err := h.GetNodeHostnames()
		checkError(h.T(), err, "cannot get node names")
		for _, node := range nodes {
			err = h.verifyDirCleanup(node, h.hostPathToDelete)
			logger.Infof("verified cleanup of %s from node %s", h.hostPathToDelete, node)
			assert.NoError(h.T(), err)
		}
	}
	if h.changeHostnames {
		// revert the hostname labels for the test
		_, err = h.k8shelper.RestoreHostnames()
		assert.NoError(h.T(), err)
	}

	// wait a bit longer for the system namespace to be cleaned up after their deletion
	for i := 0; i < 15; i++ {
		_, err := h.k8shelper.Clientset.CoreV1().Namespaces().Get(ctx, h.settings.OperatorNamespace, metav1.GetOptions{})
		if err != nil && kerrors.IsNotFound(err) {
			logger.Infof("operator namespace %q removed", h.settings.OperatorNamespace)
			break
		}
		logger.Infof("operator namespace %q still found...", h.settings.OperatorNamespace)
		time.Sleep(5 * time.Second)
	}
}

func (h *CephInstaller) removeClusterFinalizers(namespace, clusterName string) {
	ctx := context.TODO()
	// Get the latest cluster instead of using the same instance in case it has been changed
	cluster, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(ctx, clusterName, metav1.GetOptions{})
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

func (h *CephInstaller) checkCephHealthStatus() {
	ctx := context.TODO()
	clusterResource, err := h.k8shelper.RookClientset.CephV1().CephClusters(h.settings.Namespace).Get(ctx, h.settings.ClusterName, metav1.GetOptions{})
	assert.Nil(h.T(), err)
	clusterPhase := string(clusterResource.Status.Phase)
	if clusterPhase != "Ready" && clusterPhase != "Connected" && clusterPhase != "Progressing" {
		assert.Equal(h.T(), "Ready", string(clusterResource.Status.Phase))
	}

	// Depending on the tests, the health may be fluctuating with different components being started or stopped.
	// If needed, give it a few seconds to settle and check the status again.
	logger.Infof("checking ceph cluster health in namespace %q", h.settings.Namespace)
	if clusterResource.Status.CephStatus.Health != "HEALTH_OK" {
		time.Sleep(10 * time.Second)
		clusterResource, err = h.k8shelper.RookClientset.CephV1().CephClusters(h.settings.Namespace).Get(ctx, h.settings.ClusterName, metav1.GetOptions{})
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

func (h *CephInstaller) verifyDirCleanup(node, dir string) error {
	resources := h.GetCleanupVerificationPod(node, dir)
	_, err := h.k8shelper.KubectlWithStdin(resources, createFromStdinArgs...)
	return err
}

func (h *CephInstaller) CollectOperatorLog(suiteName, testName string) {
	if !h.T().Failed() && TestLogCollectionLevel() != "all" {
		return
	}
	name := fmt.Sprintf("%s_%s", suiteName, testName)
	h.k8shelper.CollectPodLogsFromLabel(cephOperatorLabel, h.settings.OperatorNamespace, name, utils.TestEnvName())
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
func NewCephInstaller(t func() *testing.T, clientset *kubernetes.Clientset, settings *TestCephSettings) *CephInstaller {

	// By default set a cluster name that is different from the namespace so we don't rely on the namespace
	// in expected places
	if settings.ClusterName == "" {
		settings.ClusterName = defaultclusterName
	}

	version, err := clientset.ServerVersion()
	if err != nil {
		logger.Infof("failed to get kubectl server version. %+v", err)
	}

	k8shelp, err := utils.CreateK8sHelper(t)
	if err != nil {
		panic("failed to get kubectl client :" + err.Error())
	}
	logger.Infof("Rook Version: %s", settings.RookVersion)
	logger.Infof("Ceph Version: %s", settings.CephVersion.Image)
	h := &CephInstaller{
		settings:        settings,
		Manifests:       NewCephManifests(settings),
		k8shelper:       k8shelp,
		helmHelper:      utils.NewHelmHelper(testHelmPath()),
		k8sVersion:      version.String(),
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
                image: rook/ceph:` + LocalBuildTag + `
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
                image: rook/ceph:` + LocalBuildTag + `
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

func (h *CephInstaller) addCleanupPolicy(namespace, clusterName string) error {
	// Retry updating the CR a few times in case of random failure
	var returnErr error
	for i := 0; i < 3; i++ {
		ctx := context.TODO()
		cluster, err := h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Get(ctx, clusterName, metav1.GetOptions{})
		if err != nil {
			return errors.Errorf("failed to get ceph cluster. %+v", err)
		}
		cluster.Spec.CleanupPolicy.Confirmation = cephv1.DeleteDataDirOnHostsConfirmation
		cluster.Spec.CleanupPolicy.AllowUninstallWithVolumes = true
		_, err = h.k8shelper.RookClientset.CephV1().CephClusters(namespace).Update(ctx, cluster, metav1.UpdateOptions{})
		if err != nil {
			returnErr = errors.Errorf("failed to add clean up policy to the cluster. %+v", err)
			logger.Warningf("could not add cleanup policy, trying again... %v", err)
		} else {
			logger.Info("successfully added cleanup policy to the ceph cluster and skipping checks for existing volumes")
			return nil
		}
	}
	return returnErr
}

func (h *CephInstaller) waitForCleanupJobs(namespace string) error {
	ctx := context.TODO()
	allRookCephCleanupJobs := func() (done bool, err error) {
		appLabelSelector := fmt.Sprintf("app=%s", cluster.CleanupAppName)
		cleanupJobs, err := h.k8shelper.Clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{LabelSelector: appLabelSelector})
		if err != nil {
			return false, errors.Errorf("failed to get cleanup jobs. %+v", err)
		}
		// Clean up jobs might take some time to start
		if len(cleanupJobs.Items) == 0 {
			logger.Infof("no jobs with label selector %q found.", appLabelSelector)
			return false, nil
		}
		for _, job := range cleanupJobs.Items {
			logger.Infof("job %q status: %+v", job.Name, job.Status)
			if job.Status.Failed > 0 {
				return false, errors.Errorf("job %s failed", job.Name)
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
		return errors.Errorf("failed to wait for clean up jobs to complete. %+v", err)
	}

	logger.Info("successfully executed all the ceph clean up jobs")
	return nil
}
