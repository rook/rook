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
	"strconv"

	"github.com/rook/rook/tests/framework/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (h *InstallHelper) InstallCockroachDB(systemNamespace, namespace string, count int) error {
	h.k8shelper.CreateAnonSystemClusterBinding()

	// install hostpath provisioner if there isn't already a default storage class
	defaultExists, err := h.k8shelper.IsDefaultStorageClassPresent()
	if err != nil {
		return err
	} else if !defaultExists {
		if err := h.InstallHostPathProvisioner(); err != nil {
			return err
		}
	} else {
		logger.Info("skipping install of host path provisioner because a default storage class already exists")
	}

	// install cockroachdb operator
	if err := h.CreateCockroachDBOperator(systemNamespace); err != nil {
		return err
	}

	// install cockroachdb cluster instance
	if err := h.CreateCockroachDBCluster(namespace, count); err != nil {
		return err
	}

	return nil
}

func (h *InstallHelper) CreateCockroachDBOperator(namespace string) error {
	logger.Infof("starting cockroachDB operator")

	logger.Info("creating cockroachDB CRDs")
	if _, err := h.k8shelper.KubectlWithStdin(h.installData.GetCockroachDBCRDs(), createFromStdinArgs...); err != nil {
		return err
	}

	cockroachDBOperator := h.installData.GetCockroachDBOperator(namespace)
	_, err := h.k8shelper.KubectlWithStdin(cockroachDBOperator, createFromStdinArgs...)
	if err != nil {
		return fmt.Errorf("failed to create rook-cockroachdb-operator pod: %+v ", err)
	}

	if !h.k8shelper.IsCRDPresent(cockroachDBCRD) {
		return fmt.Errorf("failed to find cockroachdb CRD %s", cockroachDBCRD)
	}

	if !h.k8shelper.IsPodInExpectedState("rook-cockroachdb-operator", namespace, "Running") {
		return fmt.Errorf("rook-cockroachdb-operator is not running, aborting")
	}

	logger.Infof("cockroachdb operator started")
	return nil
}

func (h *InstallHelper) CreateCockroachDBCluster(namespace string, count int) error {
	if err := h.k8shelper.CreateNamespace(namespace); err != nil {
		return err
	}

	logger.Infof("starting cockroachdb cluster with kubectl and yaml")
	cockroachDBCluster := h.installData.GetCockroachDBCluster(namespace, count)
	if _, err := h.k8shelper.KubectlWithStdin(cockroachDBCluster, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create cockroachdb cluster: %+v ", err)
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-cockroachdb", namespace, count); err != nil {
		logger.Errorf("cockroachdb cluster pods in namespace %s not found", namespace)
		return err
	}

	err := h.k8shelper.WaitForLabeledPodToRun("app=rook-cockroachdb", namespace)
	if err != nil {
		logger.Errorf("cockroachdb cluster pods in namespace %s are not running", namespace)
		return err
	}

	logger.Infof("cockroachdb cluster started")
	return nil
}

func (h *InstallHelper) UninstallCockroachDB(systemNamespace, namespace string) {
	logger.Infof("uninstalling cockroachdb from namespace %s", namespace)

	_, err := h.k8shelper.DeleteResource("-n", namespace, "cluster.cockroachdb.rook.io", namespace)
	h.checkError(err, fmt.Sprintf("cannot remove cluster %s", namespace))

	crdCheckerFunc := func() error {
		_, err := h.k8shelper.RookClientset.CockroachdbV1alpha1().Clusters(namespace).Get(namespace, metav1.GetOptions{})
		return err
	}
	err = h.waitForCustomResourceDeletion(namespace, crdCheckerFunc)
	h.checkError(err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

	_, err = h.k8shelper.DeleteResource("namespace", namespace)
	h.checkError(err, fmt.Sprintf("cannot delete namespace %s", namespace))

	logger.Infof("removing the operator from namespace %s", systemNamespace)
	_, err = h.k8shelper.DeleteResource("crd", "clusters.cockroachdb.rook.io")
	h.checkError(err, "cannot delete CRDs")

	cockroachDBOperator := h.installData.GetCockroachDBOperator(systemNamespace)
	_, err = h.k8shelper.KubectlWithStdin(cockroachDBOperator, deleteFromStdinArgs...)
	h.checkError(err, "cannot uninstall rook-cockroachdb-operator")

	err = h.UninstallHostPathProvisioner()
	h.checkError(err, "cannot uninstall hostpath provisioner")

	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)
	logger.Infof("done removing the operator from namespace %s", systemNamespace)
}

func (h *InstallHelper) GatherAllCockroachDBLogs(systemNamespace, namespace, testName string) {
	logger.Infof("Gathering all logs from cockroachdb cluster %s", namespace)
	h.k8shelper.GetRookLogs("rook-cockroachdb-operator", h.Env.HostType, systemNamespace, testName)
	h.k8shelper.GetRookLogs("rook-cockroachdb", h.Env.HostType, namespace, testName)
}

func (i *InstallData) GetCockroachDBCRDs() string {
	return `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: clusters.cockroachdb.rook.io
spec:
  group: cockroachdb.rook.io
  names:
    kind: Cluster
    listKind: ClusterList
    plural: clusters
    singular: cluster
  scope: Namespaced
  version: v1alpha1
`
}

func (i *InstallData) GetCockroachDBOperator(namespace string) string {

	return `kind: Namespace
apiVersion: v1
metadata:
  name: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-cockroachdb-operator
rules:
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - get
  - list
- apiGroups:
  - ""
  resources:
  - services
  verbs:
  - get
  - list
  - create
  - update
- apiGroups:
  - apps
  resources:
  - statefulsets
  verbs:
  - create
- apiGroups:
  - policy
  resources:
  - poddisruptionbudgets
  verbs:
  - create
- apiGroups:
  - cockroachdb.rook.io
  resources:
  - "*"
  verbs:
  - "*"
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-cockroachdb-operator
  namespace: ` + namespace + `
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-cockroachdb-operator
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-cockroachdb-operator
subjects:
- kind: ServiceAccount
  name: rook-cockroachdb-operator
  namespace: ` + namespace + `
---
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: rook-cockroachdb-operator
  namespace: ` + namespace + `
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: rook-cockroachdb-operator
    spec:
      serviceAccountName: rook-cockroachdb-operator
      containers:
      - name: rook-cockroachdb-operator
        image: rook/cockroachdb:master
        args: ["cockroachdb", "operator"]
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
`
}

func (i *InstallData) GetCockroachDBCluster(namespace string, count int) string {
	return `apiVersion: cockroachdb.rook.io/v1alpha1
kind: Cluster
metadata:
  name: ` + namespace + `
  namespace: ` + namespace + `
spec:
  scope:
    nodeCount: ` + strconv.Itoa(count) + `
  secure: false
  volumeSize: 1Gi
  cachePercent: 25
  maxSQLMemoryPercent: 25
`
}

func GatherCockroachDBDebuggingInfo(k8shelper *utils.K8sHelper, namespace string) {
	k8shelper.PrintPodDescribeForNamespace(namespace)
	k8shelper.PrintPVs(true /*detailed*/)
	k8shelper.PrintPVCs(namespace, true /*detailed*/)
	k8shelper.PrintStorageClasses(true /*detailed*/)
}
