/*
Copyright 2018 The Rook Authors. All rights reserved.

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

	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/tests/framework/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	cassandraCRD = "clusters.cassandra.rook.io"
)

type CassandraInstaller struct {
	k8sHelper *utils.K8sHelper
	manifests *CassandraManifests
	T         func() *testing.T
}

func NewCassandraInstaller(k8sHelper *utils.K8sHelper, t func() *testing.T) *CassandraInstaller {
	return &CassandraInstaller{k8sHelper, &CassandraManifests{}, t}
}

func (ci *CassandraInstaller) InstallCassandra(systemNamespace, namespace string, count int, mode cassandrav1alpha1.ClusterMode) error {

	ci.k8sHelper.CreateAnonSystemClusterBinding()

	// Check if a default storage class exists
	defaultExists, err := ci.k8sHelper.IsDefaultStorageClassPresent()
	if err != nil {
		return err
	}
	if !defaultExists {
		if err := InstallHostPathProvisioner(ci.k8sHelper); err != nil {
			return err
		}
	} else {
		logger.Info("skipping install of host path provisioner because a default storage class already exists")
	}

	// Install cassandra operator
	if err := ci.CreateCassandraOperator(systemNamespace); err != nil {
		return err
	}
	// Create a Cassandra Cluster instance
	if err := ci.CreateCassandraCluster(namespace, count, mode); err != nil {
		return err
	}
	return nil
}

func (ci *CassandraInstaller) CreateCassandraOperator(namespace string) error {

	logger.Info("Starting cassandra operator")

	logger.Info("Creating Cassandra CRD...")
	if _, err := ci.k8sHelper.KubectlWithStdin(ci.manifests.GetCassandraCRDs(), createFromStdinArgs...); err != nil {
		return err
	}

	cassandraOperator := ci.manifests.GetCassandraOperator(namespace)
	if _, err := ci.k8sHelper.KubectlWithStdin(cassandraOperator, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create rook-cassandra-operator pod: %+v", err)
	}

	if !ci.k8sHelper.IsCRDPresent(cassandraCRD) {
		return fmt.Errorf("Failed to find cassandra CRD %s", cassandraCRD)
	}

	if !ci.k8sHelper.IsPodInExpectedState("rook-cassandra-operator", namespace, "Running") {
		return fmt.Errorf("rook-cassandra-operator is not running, aborting")
	}

	logger.Infof("cassandra operator started")
	return nil

}

func (ci *CassandraInstaller) CreateCassandraCluster(namespace string, count int, mode cassandrav1alpha1.ClusterMode) error {

	// if err := ci.k8sHelper.CreateNamespace(namespace); err != nil {
	// 	return err
	// }

	logger.Info("Starting Cassandra Cluster with kubectl and yaml")
	cassandraCluster := ci.manifests.GetCassandraCluster(namespace, count, mode)
	if _, err := ci.k8sHelper.KubectlWithStdin(cassandraCluster, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create Cassandra Cluster: %s", err.Error())
	}

	if err := ci.k8sHelper.WaitForPodCount("app=rook-cassandra", namespace, count); err != nil {
		return fmt.Errorf("Cassandra Cluster pods in namespace %s not found: %s", namespace, err.Error())
	}

	if err := ci.k8sHelper.WaitForLabeledPodsToRun("app=rook-cassandra", namespace); err != nil {
		return fmt.Errorf("Cassandra Cluster Pods in namespace %s are not running: %s", namespace, err.Error())
	}

	logger.Infof("Cassandra Cluster started")
	return nil
}

func (ci *CassandraInstaller) DeleteCassandraCluster(namespace string) {

	// Delete Cassandra Cluster
	logger.Infof("Uninstalling Cassandra from namespace %s", namespace)
	err := ci.k8sHelper.DeleteResourceAndWait(true, "-n", namespace, cassandraCRD, namespace)
	checkError(ci.T(), err, fmt.Sprintf("cannot remove cluster %s", namespace))

	crdCheckerFunc := func() error {
		_, err := ci.k8sHelper.RookClientset.CassandraV1alpha1().Clusters(namespace).Get(namespace, metav1.GetOptions{})
		return err
	}
	err = ci.k8sHelper.WaitForCustomResourceDeletion(namespace, crdCheckerFunc)

	// Delete Namespace
	logger.Infof("Deleting Cassandra Cluster namespace %s", namespace)
	err = ci.k8sHelper.DeleteResourceAndWait(true, "namespace", namespace)
	checkError(ci.T(), err, fmt.Sprintf("cannot delete namespace %s", namespace))
}

func (ci *CassandraInstaller) UninstallCassandra(systemNamespace string, namespace string) {

	// Delete deployed Cluster
	// ci.DeleteCassandraCluster(namespace)
	cassandraCluster := ci.manifests.GetCassandraCluster(namespace, 0, "")
	_, err := ci.k8sHelper.KubectlWithStdin(cassandraCluster, deleteFromStdinArgs...)
	checkError(ci.T(), err, "cannot uninstall cluster")

	// Delete Operator, CRD and RBAC related to them
	cassandraOperator := ci.manifests.GetCassandraOperator(systemNamespace)
	_, err = ci.k8sHelper.KubectlWithStdin(cassandraOperator, deleteFromStdinArgs...)
	checkError(ci.T(), err, "cannot uninstall rook-cassandra-operator")

	cassandraCRDs := ci.manifests.GetCassandraCRDs()
	_, err = ci.k8sHelper.KubectlWithStdin(cassandraCRDs, deleteFromStdinArgs...)
	checkError(ci.T(), err, "cannot uninstall cassandra CRDs")

	//Remove "anon-user-access"
	logger.Info("Removing anon-user-access ClusterRoleBinding")
	ci.k8sHelper.Clientset.RbacV1().ClusterRoleBindings().Delete("anon-user-access", nil)

	logger.Info("Successfully deleted all cassandra operator related objects.")
}

func (ci *CassandraInstaller) GatherAllCassandraLogs(systemNamespace, namespace, testName string) {
	if !ci.T().Failed() && Env.Logs != "all" {
		return
	}
	logger.Infof("Gathering all logs from Cassandra Cluster %s", namespace)
	ci.k8sHelper.GetLogsFromNamespace(systemNamespace, testName, Env.HostType)
	ci.k8sHelper.GetLogsFromNamespace(namespace, testName, Env.HostType)
}
