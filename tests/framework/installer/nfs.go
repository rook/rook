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

	"github.com/rook/rook/tests/framework/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InstallNFSServer installs NFS operator, NFS CRD instance and NFS volume
func (h *InstallHelper) InstallNFSServer(systemNamespace, namespace string, count int) error {
	h.k8shelper.CreateAnonSystemClusterBinding()

	// install hostpath provisioner if there isn't already a default storage class
	storageClassName := ""
	defaultExists, err := h.k8shelper.IsDefaultStorageClassPresent()
	if err != nil {
		return err
	} else if !defaultExists {
		if err := h.InstallHostPathProvisioner(); err != nil {
			return err
		}
		storageClassName = "hostpath"
	} else {
		logger.Info("skipping install of host path provisioner because a default storage class already exists")
	}

	// install nfs operator
	if err := h.CreateNFSServerOperator(systemNamespace); err != nil {
		return err
	}

	// install nfs server instance
	if err := h.CreateNFSServer(namespace, count, storageClassName); err != nil {
		return err
	}

	// install nfs server volume
	if err := h.CreateNFSServerVolume(namespace); err != nil {
		return err
	}

	return nil
}

// CreateNFSServerOperator creates nfs server in the provided namespace
func (h *InstallHelper) CreateNFSServerOperator(namespace string) error {
	logger.Infof("starting nfsserver operator")

	logger.Info("creating nfsserver CRDs")
	if _, err := h.k8shelper.KubectlWithStdin(h.installData.GetNFSServerCRDs(), createFromStdinArgs...); err != nil {
		return err
	}

	nfsOperator := h.installData.GetNFSServerOperator(namespace)
	_, err := h.k8shelper.KubectlWithStdin(nfsOperator, createFromStdinArgs...)
	if err != nil {
		return fmt.Errorf("failed to create rook-nfs-operator pod: %+v ", err)
	}

	if !h.k8shelper.IsCRDPresent(nfsServerCRD) {
		return fmt.Errorf("failed to find nfs CRD %s", nfsServerCRD)
	}

	if !h.k8shelper.IsPodInExpectedState("rook-nfs-operator", namespace, "Running") {
		return fmt.Errorf("rook-nfs-operator is not running, aborting")
	}

	logger.Infof("nfs operator started")
	return nil
}

// CreateNFSServer creates the NFS Server CRD instance
func (h *InstallHelper) CreateNFSServer(namespace string, count int, storageClassName string) error {
	if err := h.k8shelper.CreateNamespace(namespace); err != nil {
		return err
	}

	logger.Infof("starting nfs server with kubectl and yaml")
	nfsServer := h.installData.GetNFSServer(namespace, count, storageClassName)
	if _, err := h.k8shelper.KubectlWithStdin(nfsServer, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create nfs server: %+v ", err)
	}

	if err := h.k8shelper.WaitForPodCount("app=rook-nfs", namespace, 1); err != nil {
		logger.Errorf("nfs server pods in namespace %s not found", namespace)
		return err
	}

	err := h.k8shelper.WaitForLabeledPodToRun("app=rook-nfs", namespace)
	if err != nil {
		logger.Errorf("nfs server pods in namespace %s are not running", namespace)
		return err
	}

	logger.Infof("nfs server started")
	return nil
}

// CreateNFSServerVolume creates NFS export PV and PVC
func (h *InstallHelper) CreateNFSServerVolume(namespace string) error {
	logger.Info("creating volume from nfs server in namespace %s", namespace)

	clusterIP, err := h.GetNFSServerClusterIP(namespace)
	if err != nil {
		return err
	}
	nfsServerPV := h.installData.GetNFSServerPV(namespace, clusterIP)
	nfsServerPVC := h.installData.GetNFSServerPVC()

	logger.Info("creating nfs server pv")
	if _, err := h.k8shelper.KubectlWithStdin(nfsServerPV, createFromStdinArgs...); err != nil {
		return err
	}

	logger.Info("creating nfs server pvc")
	if _, err := h.k8shelper.KubectlWithStdin(nfsServerPVC, createFromStdinArgs...); err != nil {
		return err
	}

	return nil
}

// UninstallNFSServer uninstalls the NFS Server from the given namespace
func (h *InstallHelper) UninstallNFSServer(systemNamespace, namespace string) {
	logger.Infof("uninstalling nfsserver from namespace %s", namespace)

	_, err := h.k8shelper.DeleteResource("pvc", "nfs-pv-claim")
	h.checkError(err, fmt.Sprintf("cannot remove nfs pvc"))

	_, err = h.k8shelper.DeleteResource("pv", "nfs-pv")
	h.checkError(err, fmt.Sprintf("cannot remove nfs pv"))

	_, err = h.k8shelper.DeleteResource("-n", namespace, "nfsservers.nfs.rook.io", namespace)
	h.checkError(err, fmt.Sprintf("cannot remove nfsserver %s", namespace))

	crdCheckerFunc := func() error {
		_, err := h.k8shelper.RookClientset.NfsV1alpha1().NFSServers(namespace).Get(namespace, metav1.GetOptions{})
		return err
	}
	err = h.waitForCustomResourceDeletion(namespace, crdCheckerFunc)
	h.checkError(err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

	_, err = h.k8shelper.DeleteResource("namespace", namespace)
	h.checkError(err, fmt.Sprintf("cannot delete namespace %s", namespace))

	logger.Infof("removing the operator from namespace %s", systemNamespace)
	_, err = h.k8shelper.DeleteResource("crd", "nfsservers.nfs.rook.io")
	h.checkError(err, "cannot delete CRDs")

	nfsOperator := h.installData.GetNFSServerOperator(systemNamespace)
	_, err = h.k8shelper.KubectlWithStdin(nfsOperator, deleteFromStdinArgs...)
	h.checkError(err, "cannot uninstall rook-nfs-operator")

	err = h.UninstallHostPathProvisioner()
	h.checkError(err, "cannot uninstall hostpath provisioner")

	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)
	logger.Infof("done removing the operator from namespace %s", systemNamespace)
}

// GatherAllNFSServerLogs gathers all NFS Server logs
func (h *InstallHelper) GatherAllNFSServerLogs(systemNamespace, namespace, testName string) {
	logger.Infof("Gathering all logs from NFSServer %s", namespace)
	h.k8shelper.GetRookLogs("rook-nfs-operator", h.Env.HostType, systemNamespace, testName)
	h.k8shelper.GetRookLogs("rook-nfs", h.Env.HostType, namespace, testName)
}

// GatherNFSServerDebuggingInfo gathers all the descriptions for pods, pvs and pvcs
func GatherNFSServerDebuggingInfo(k8shelper *utils.K8sHelper, namespace string) {
	k8shelper.PrintPodDescribeForNamespace(namespace)
	k8shelper.PrintPVs(true /*detailed*/)
	k8shelper.PrintPVCs(namespace, true /*detailed*/)
}

// GetNFSServerClusterIP gets the nfs server cluster ip on which it serves
func (h *InstallHelper) GetNFSServerClusterIP(namespace string) (string, error) {
	clusterIP := ""
	service, err := h.k8shelper.GetService("rook-nfs", namespace)
	if err != nil {
		logger.Errorf("nfs server service in namespace %s is not active", namespace)
		return clusterIP, err
	}
	clusterIP = service.Spec.ClusterIP
	return clusterIP, nil
}
