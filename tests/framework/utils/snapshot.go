/*
Copyright 2020 The Rook Authors. All rights reserved.
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

package utils

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// snapshotterVersion from which the snapshotcontroller and CRD will be
	// installed
	snapshotterVersion = "v8.5.0"
	repoURL            = "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter"
	rbacPath           = "deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml"
	controllerPath     = "deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml"
	// snapshot CRD path
	snapshotClassCRDPath          = "client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml"
	volumeSnapshotContentsCRDPath = "client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml"
	volumeSnapshotCRDPath         = "client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml"
	// volumegroupsnapshot CRD path
	volumeGroupSnapshotClassCRDPath    = "client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshotclasses.yaml"
	volumeGroupSnapshotContentsCRDPath = "client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshotcontents.yaml"
	volumeGroupSnapshotCRDPath         = "client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshots.yaml"
)

// CheckSnapshotISReadyToUse checks snapshot is ready to use
func (k8sh *K8sHelper) CheckSnapshotISReadyToUse(name, namespace string, retries int) (bool, error) {
	for i := 0; i < retries; i++ {
		// sleep first and try to check snapshot is ready to cover the error cases.
		time.Sleep(time.Duration(i) * time.Second)
		ready, err := k8sh.executor.ExecuteCommandWithOutput("kubectl", "get", "volumesnapshot", name, "--namespace", namespace, "-o", "jsonpath={.status.readyToUse}")
		if err != nil {
			return false, err
		}
		val, err := strconv.ParseBool(ready)
		if err != nil {
			logger.Errorf("failed to parse ready state of snapshot %q in namespace %q: error %+v", name, namespace, err)
			continue
		}
		if val {
			return true, nil
		}
	}
	return false, fmt.Errorf("giving up waiting for %q snapshot in namespace %q", name, namespace)
}

// kubectlWithURLRetry runs kubectl with a manifest URL argument, retrying on
// failure since fetching manifests from raw.githubusercontent.com fails
// transiently in CI.
func (k8sh *K8sHelper) kubectlWithURLRetry(args ...string) error {
	var err error
	for i := 0; i < 5; i++ {
		if _, err = k8sh.Kubectl(args...); err == nil {
			return nil
		}
		logger.Warningf("failed to run kubectl %v, will retry. %v", args, err)
		time.Sleep(5 * time.Second)
	}
	return err
}

// snapshotController creates/applies or deletes the snapshotcontroller and required RBAC
func (k8sh *K8sHelper) snapshotController(action string) error {
	controllerURL := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, controllerPath)
	controllerManifest, err := getManifestFromURL(controllerURL)
	if err != nil {
		return err
	}
	controllerManifest = strings.ReplaceAll(controllerManifest, "canary", snapshotterVersion)
	logger.Infof("snapshot controller: %s", controllerManifest)

	_, err = k8sh.KubectlWithStdin(controllerManifest, action, "-f", "-")
	if err != nil {
		return err
	}

	rbac := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, rbacPath)
	return k8sh.kubectlWithURLRetry(action, "-f", rbac)
}

// WaitForSnapshotController checks snapshotcontroller is ready within given
// retries count.
func (k8sh *K8sHelper) WaitForSnapshotController(retries int) error {
	namespace := "kube-system"
	ctx := context.TODO()
	snapshotterName := "snapshot-controller"
	for i := 0; i < retries; i++ {
		ss, err := k8sh.Clientset.AppsV1().Deployments(namespace).Get(ctx, snapshotterName, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if ss.Status.ReadyReplicas > 0 && ss.Status.ReadyReplicas == ss.Status.Replicas {
			return nil
		}
		logger.Infof("waiting for %q deployment in namespace %q (readyreplicas %d < replicas %d)", snapshotterName, namespace, ss.Status.ReadyReplicas, ss.Status.Replicas)
		time.Sleep(RetryInterval * time.Second)
	}
	return fmt.Errorf("giving up waiting for %q deployment in namespace %q", snapshotterName, namespace)
}

// CreateSnapshotController creates the snapshotcontroller and required RBAC
func (k8sh *K8sHelper) CreateSnapshotController(action string) error {
	return k8sh.snapshotController(action)
}

// DeleteSnapshotController deletes the snapshotcontroller and required RBAC
func (k8sh *K8sHelper) DeleteSnapshotController() error {
	return k8sh.snapshotController("delete")
}

// snapshotCRD can be used for creating, applying or deleting the snapshot CRDs
func (k8sh *K8sHelper) snapshotCRD(action string) error {
	// setting validate=false to skip CRD validation during create/apply to
	// support lower Kubernetes versions.
	args := func(crdpath string) []string {
		a := []string{
			action,
			"-f",
			crdpath,
		}
		if action == "create" || action == "apply" {
			a = append(a, "--validate=false")
		}
		return a
	}
	crdPaths := []string{
		snapshotClassCRDPath,
		volumeSnapshotContentsCRDPath,
		volumeSnapshotCRDPath,
		volumeGroupSnapshotClassCRDPath,
		volumeGroupSnapshotContentsCRDPath,
		volumeGroupSnapshotCRDPath,
	}
	for _, crdPath := range crdPaths {
		crdURL := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, crdPath)
		if err := k8sh.kubectlWithURLRetry(args(crdURL)...); err != nil {
			return err
		}
	}

	return nil
}

// CreateSnapshotCRD creates the snapshot CRD
func (k8sh *K8sHelper) CreateSnapshotCRD(action string) error {
	return k8sh.snapshotCRD(action)
}

// DeleteSnapshotCRD deletes the snapshot CRD
func (k8sh *K8sHelper) DeleteSnapshotCRD() error {
	return k8sh.snapshotCRD("delete")
}
