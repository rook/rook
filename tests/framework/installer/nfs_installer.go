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

	"github.com/rook/rook/tests/framework/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	nfsServerCRD = "nfsservers.nfs.rook.io"
)

type NFSInstaller struct {
	k8shelper *utils.K8sHelper
	manifests *NFSManifests
	T         func() *testing.T
}

func NewNFSInstaller(k8shelper *utils.K8sHelper, t func() *testing.T) *NFSInstaller {
	return &NFSInstaller{k8shelper, &NFSManifests{}, t}
}

// InstallNFSServer installs NFS operator, NFS CRD instance and NFS volume
func (h *NFSInstaller) InstallNFSServer(systemNamespace, namespace string, count int) error {
	h.k8shelper.CreateAnonSystemClusterBinding()

	// install hostpath provisioner if there isn't already a default storage class
	storageClassName := ""
	defaultExists, err := h.k8shelper.IsDefaultStorageClassPresent()
	if err != nil {
		return err
	} else if !defaultExists {
		if err := InstallHostPathProvisioner(h.k8shelper); err != nil {
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
func (h *NFSInstaller) CreateNFSServerOperator(namespace string) error {
	logger.Infof("starting nfsserver operator")

	logger.Info("creating nfsserver CRDs")
	if _, err := h.k8shelper.KubectlWithStdin(h.manifests.GetNFSServerCRDs(), createFromStdinArgs...); err != nil {
		return err
	}

	nfsOperator := h.manifests.GetNFSServerOperator(namespace)
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
func (h *NFSInstaller) CreateNFSServer(namespace string, count int, storageClassName string) error {
	if err := h.k8shelper.CreateNamespace(namespace); err != nil {
		return err
	}

	logger.Infof("starting nfs server with kubectl and yaml")
	nfsServer := h.manifests.GetNFSServer(namespace, count, storageClassName)
	if _, err := h.k8shelper.KubectlWithStdin(nfsServer, createFromStdinArgs...); err != nil {
		return fmt.Errorf("Failed to create nfs server: %+v ", err)
	}

	if err := h.k8shelper.WaitForPodCount("app="+namespace, namespace, 1); err != nil {
		logger.Errorf("nfs server pods in namespace %s not found", namespace)
		return err
	}

	err := h.k8shelper.WaitForLabeledPodsToRun("app="+namespace, namespace)
	if err != nil {
		logger.Errorf("nfs server pods in namespace %s are not running", namespace)
		return err
	}

	logger.Infof("nfs server started")
	return nil
}

// CreateNFSServerVolume creates NFS export PV and PVC
func (h *NFSInstaller) CreateNFSServerVolume(namespace string) error {
	logger.Info("creating volume from nfs server in namespace %s", namespace)

	nfsServerPVC := h.manifests.GetNFSServerPVC(namespace)

	logger.Info("creating nfs server pvc")
	if _, err := h.k8shelper.KubectlWithStdin(nfsServerPVC, createFromStdinArgs...); err != nil {
		return err
	}

	return nil
}

// UninstallNFSServer uninstalls the NFS Server from the given namespace
func (h *NFSInstaller) UninstallNFSServer(systemNamespace, namespace string) {
	logger.Infof("uninstalling nfsserver from namespace %s", namespace)

	err := h.k8shelper.DeleteResource("pvc", "nfs-pv-claim")
	checkError(h.T(), err, fmt.Sprintf("cannot remove nfs pvc : nfs-pv-claim"))

	err = h.k8shelper.DeleteResource("pvc", "nfs-pv-claim-bigger")
	checkError(h.T(), err, fmt.Sprintf("cannot remove nfs pvc : nfs-pv-claim-bigger"))

	err = h.k8shelper.DeleteResource("pv", "nfs-pv")
	checkError(h.T(), err, fmt.Sprintf("cannot remove nfs pv : nfs-pv"))

	err = h.k8shelper.DeleteResource("pv", "nfs-pv1")
	checkError(h.T(), err, fmt.Sprintf("cannot remove nfs pv : nfs-pv1"))

	err = h.k8shelper.DeleteResource("-n", namespace, "nfsservers.nfs.rook.io", namespace)
	checkError(h.T(), err, fmt.Sprintf("cannot remove nfsserver %s", namespace))

	crdCheckerFunc := func() error {
		_, err := h.k8shelper.RookClientset.NfsV1alpha1().NFSServers(namespace).Get(namespace, metav1.GetOptions{})
		return err
	}
	err = h.k8shelper.WaitForCustomResourceDeletion(namespace, crdCheckerFunc)
	checkError(h.T(), err, fmt.Sprintf("failed to wait for crd %s deletion", namespace))

	err = h.k8shelper.DeleteResource("namespace", namespace)
	checkError(h.T(), err, fmt.Sprintf("cannot delete namespace %s", namespace))

	logger.Infof("removing the operator from namespace %s", systemNamespace)
	err = h.k8shelper.DeleteResource("crd", "nfsservers.nfs.rook.io")
	checkError(h.T(), err, "cannot delete CRDs")

	nfsOperator := h.manifests.GetNFSServerOperator(systemNamespace)
	_, err = h.k8shelper.KubectlWithStdin(nfsOperator, deleteFromStdinArgs...)
	checkError(h.T(), err, "cannot uninstall rook-nfs-operator")

	err = UninstallHostPathProvisioner(h.k8shelper)
	checkError(h.T(), err, "cannot uninstall hostpath provisioner")

	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("run-nfs-client-provisioner", nil)
	h.k8shelper.Clientset.RbacV1beta1().ClusterRoles().Delete("nfs-client-provisioner-runner", nil)
	logger.Infof("done removing the operator from namespace %s", systemNamespace)
}

// GatherAllNFSServerLogs gathers all NFS Server logs
func (h *NFSInstaller) GatherAllNFSServerLogs(systemNamespace, namespace, testName string) {
	if !h.T().Failed() && Env.Logs != "all" {
		return
	}
	logger.Infof("Gathering all logs from NFSServer %s", namespace)
	h.k8shelper.GetLogsFromNamespace(systemNamespace, testName, Env.HostType)
	h.k8shelper.GetLogsFromNamespace(namespace, testName, Env.HostType)
}
