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
	CRDFullyQualifiedName         = "ybclusters.yugabytedb.rook.io"
	CRDFullyQualifiedNameSingular = "ybcluster.yugabytedb.rook.io"
)

type YugabyteDBInstaller struct {
	T                            func() *testing.T
	k8sHelper                    *utils.K8sHelper
	manifests                    *YugabyteDBManifests
	hostPathProvisionerInstalled bool
}

func NewYugabyteDBInstaller(t func() *testing.T, k8shelper *utils.K8sHelper) *YugabyteDBInstaller {
	return &YugabyteDBInstaller{t, k8shelper, &YugabyteDBManifests{}, false}
}

func (y *YugabyteDBInstaller) InstallYugabyteDB(systemNS, ns string, count int) error {
	logger.Info("Yugabytedb cluster install started")
	y.k8sHelper.CreateAnonSystemClusterBinding()

	if isDefStorageClassPresent, err := y.k8sHelper.IsDefaultStorageClassPresent(); err != nil {
		return err
	} else if !isDefStorageClassPresent {
		logger.Info("Default storage class not set. Creating one.")

		// Mark the installationa attempt of a host path provisioner, for removal later.
		y.hostPathProvisionerInstalled = true

		if err := InstallHostPathProvisioner(y.k8sHelper); err != nil {
			return err
		}
	} else {
		logger.Info("Default storage class found.")
	}

	if err := y.CreateOperator(systemNS); err != nil {
		return err
	}

	// install yugabytedb cluster instance
	if err := y.CreateYugabyteDBCluster(ns, count); err != nil {
		return err
	}

	return nil
}

func (y *YugabyteDBInstaller) CreateOperator(SystemNamespace string) error {
	logger.Info("Starting YugabyteDB operator")

	logger.Info("Creating CRDs.")
	if _, err := y.k8sHelper.KubectlWithStdin(y.manifests.GetYugabyteDBCRDSpecs(), createFromStdinArgs...); err != nil {
		return err
	}

	logger.Info("Creating RBAC & deployment for operator.")
	if _, err := y.k8sHelper.KubectlWithStdin(y.manifests.GetYugabyteDBOperatorSpecs(SystemNamespace), createFromStdinArgs...); err != nil {
		return err
	}

	logger.Info("Checking if CRD definition persisted.")
	if !y.k8sHelper.IsCRDPresent(CRDFullyQualifiedName) {
		logger.Errorf("YugabyteDB CRD %s creation failed!", CRDFullyQualifiedName)
	}

	logger.Info("Checking if operator pod is created")
	if !y.k8sHelper.IsPodInExpectedState("rook-yugabytedb-operator", SystemNamespace, "Running") {
		return fmt.Errorf("YugabyteDB operator isn't running!")
	}

	logger.Infof("yugabytedb operator started")
	return nil
}

func (y *YugabyteDBInstaller) CreateYugabyteDBCluster(namespace string, replicaCount int) error {
	logger.Info("Creating cluster namespace")
	if err := y.k8sHelper.CreateNamespace(namespace); err != nil {
		return err
	}

	logger.Info("Creating yugabytedb cluster")
	if _, err := y.k8sHelper.KubectlWithStdin(y.manifests.GetYugabyteDBClusterSpecs(namespace, replicaCount), createFromStdinArgs...); err != nil {
		logger.Errorf("Failed to create YugabyteDB cluster")
		return err
	}

	if err := y.k8sHelper.WaitForPodCount("app=yb-master-rook-yugabytedb", namespace, replicaCount); err != nil {
		logger.Error("YugabyteDB cluster master pods are not running")
		return err
	}

	if err := y.k8sHelper.WaitForPodCount("app=yb-tserver-rook-yugabytedb", namespace, replicaCount); err != nil {
		logger.Error("YugabyteDB cluster tserver pods are not running")
		return err
	}

	logger.Infof("Yugabytedb cluster has started")
	return nil
}

func (y *YugabyteDBInstaller) RemoveAllYugabyteDBResources(systemNS, namespace string) error {
	logger.Info("Removing YugabyteDB cluster")
	err := y.k8sHelper.DeleteResourceAndWait(false, "-n", namespace, CRDFullyQualifiedNameSingular, namespace)
	checkError(y.T(), err, fmt.Sprintf("cannot remove cluster %s", namespace))

	crdCheckerFunc := func() error {
		_, err := y.k8sHelper.RookClientset.YugabytedbV1alpha1().YBClusters(namespace).Get(namespace, metav1.GetOptions{})
		return err
	}
	err = y.k8sHelper.WaitForCustomResourceDeletion(namespace, crdCheckerFunc)
	checkError(y.T(), err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

	logger.Info("Removing yugabytedb cluster namespace")
	err = y.k8sHelper.DeleteResourceAndWait(false, "namespace", namespace)
	checkError(y.T(), err, fmt.Sprintf("cannot delete namespace %s", namespace))

	logger.Infof("removing the operator from namespace %s", systemNS)
	err = y.k8sHelper.DeleteResource("crd", CRDFullyQualifiedName)
	checkError(y.T(), err, "cannot delete CRDs")

	logger.Info("Removing yugabytedb operator system namespace")
	_, err = y.k8sHelper.KubectlWithStdin(y.manifests.GetYugabyteDBOperatorSpecs(systemNS), deleteFromStdinArgs...)
	checkError(y.T(), err, "cannot uninstall rook-yugabytedb-operator")

	// Remove host path provisioner resources, if installed.
	if y.hostPathProvisionerInstalled {
		err = UninstallHostPathProvisioner(y.k8sHelper)
		checkError(y.T(), err, "cannot uninstall hostpath provisioner")
	}

	y.k8sHelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)
	logger.Infof("done removing the operator from namespace %s", systemNS)

	return nil
}

func (y *YugabyteDBInstaller) GatherAllLogs(systemNS, namespace, testName string) {
	if !y.T().Failed() && Env.Logs != "all" {
		return
	}
	logger.Infof("Gathering all logs from yugabytedb cluster %s", namespace)
	y.k8sHelper.GetLogsFromNamespace(systemNS, testName, Env.HostType)
	y.k8sHelper.GetLogsFromNamespace(namespace, testName, Env.HostType)
}
