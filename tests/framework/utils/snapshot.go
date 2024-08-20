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
	snapshotterVersion = "v8.0.1"
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

// snapshotController creates or deletes the snapshotcontroller and required RBAC
func (k8sh *K8sHelper) snapshotController(action string) error {
	controllerURL := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, controllerPath)
	controllerManifest, err := getManifestFromURL(controllerURL)
	if err != nil {
		return err
	}
	controllerManifest = strings.Replace(controllerManifest, "canary", snapshotterVersion, -1)
	logger.Infof("snapshot controller: %s", controllerManifest)

	_, err = k8sh.KubectlWithStdin(controllerManifest, action, "-f", "-")
	if err != nil {
		return err
	}

	rbac := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, rbacPath)
	_, err = k8sh.Kubectl(action, "-f", rbac)
	if err != nil {
		return err
	}
	return nil
}

// WaitForSnapshotController check snapshotcontroller is ready within given
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
func (k8sh *K8sHelper) CreateSnapshotController() error {
	return k8sh.snapshotController("create")
}

// DeleteSnapshotController delete the snapshotcontroller and required RBAC
func (k8sh *K8sHelper) DeleteSnapshotController() error {
	return k8sh.snapshotController("delete")
}

// snapshotCRD can be used for creating or deleting the snapshot CRD's
func (k8sh *K8sHelper) snapshotCRD(action string) error {
	// setting validate=false to skip CRD validation during create operation to
	// support lower Kubernetes versions.
	args := func(crdpath string) []string {
		a := []string{
			action,
			"-f",
			crdpath,
		}
		if action == "create" {
			a = append(a, "--validate=false")
		}
		return a
	}
	snapshotClassCRD := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, snapshotClassCRDPath)
	_, err := k8sh.Kubectl(args(snapshotClassCRD)...)
	if err != nil {
		return err
	}

	snapshotContentsCRD := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, volumeSnapshotContentsCRDPath)
	_, err = k8sh.Kubectl(args(snapshotContentsCRD)...)
	if err != nil {
		return err
	}

	snapshotCRD := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, volumeSnapshotCRDPath)
	_, err = k8sh.Kubectl(args(snapshotCRD)...)
	if err != nil {
		return err
	}
	vgsClassCRD := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, volumeGroupSnapshotClassCRDPath)
	_, err = k8sh.Kubectl(args(vgsClassCRD)...)
	if err != nil {
		return err
	}

	vgsContentsCRD := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, volumeGroupSnapshotContentsCRDPath)
	_, err = k8sh.Kubectl(args(vgsContentsCRD)...)
	if err != nil {
		return err
	}

	vgsCRD := fmt.Sprintf("%s/%s/%s", repoURL, snapshotterVersion, volumeGroupSnapshotCRDPath)
	_, err = k8sh.Kubectl(args(vgsCRD)...)
	if err != nil {
		return err
	}

	return nil
}

// CreateSnapshotCRD creates the snapshot CRD
func (k8sh *K8sHelper) CreateSnapshotCRD() error {
	return k8sh.snapshotCRD("create")
}

// DeleteSnapshotCRD deletes the snapshot CRD
func (k8sh *K8sHelper) DeleteSnapshotCRD() error {
	return k8sh.snapshotCRD("delete")
}
