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
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/rook/rook/tests/framework/utils"
	"k8s.io/apimachinery/pkg/api/errors"
)

const (
	hostPathStorageClassName           = "hostpath"
	hostPathProvisionerResourceBaseURL = `https://raw.githubusercontent.com/MaZderMind/hostpath-provisioner/master/manifests/%s`
	hostPathProvisionerRBAC            = `rbac.yaml`
	hostPathProvisionerDeployment      = `deployment.yaml`
	hostPathProvisionerStorageClass    = `storageclass.yaml`
)

// ************************************************************************************************
// HostPath provisioner functions
// ************************************************************************************************
func InstallHostPathProvisioner(k8shelper *utils.K8sHelper) error {
	logger.Info("installing host path provisioner")

	rbacResourceURL := fmt.Sprintf(hostPathProvisionerResourceBaseURL, hostPathProvisionerRBAC)
	args := append(createArgs, rbacResourceURL)
	out, err := k8shelper.Kubectl(args...)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create hostpath provisioner RBAC: %+v. %s", err, out)
	}

	deployment, err := getHostPathProvisionerDeployment()
	if err != nil {
		return err
	}
	out, err = k8shelper.KubectlWithStdin(deployment, createFromStdinArgs...)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create hostpath provisioner deployment: %+v. %s", err, out)
	}

	storageClassResourceURL := fmt.Sprintf(hostPathProvisionerResourceBaseURL, hostPathProvisionerStorageClass)
	args = append(createArgs, storageClassResourceURL)
	out, err = k8shelper.Kubectl(args...)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create hostpath provisioner StorageClass: %+v. %s", err, out)
	}

	err = k8shelper.WaitForLabeledPodsToRun("k8s-app=hostpath-provisioner", "kube-system")
	if err != nil {
		logger.Errorf("hostpath provisioner pod is not running: %+v", err)
		k8shelper.GetPodDescribeFromNamespace("kube-system", "hostPathProvisioner", Env.HostType)
		k8shelper.PrintStorageClasses(true /*detailed*/)
		return err
	}

	err = k8shelper.IsStorageClassPresent(hostPathStorageClassName)
	if err != nil {
		logger.Errorf("storageClass %s not found: %+v", hostPathStorageClassName, err)
		k8shelper.PrintStorageClasses(true /*detailed*/)
		return err
	}

	return nil
}

func UninstallHostPathProvisioner(k8shelper *utils.K8sHelper) error {
	logger.Info("uninstalling host path provisioner")

	storageClassResourceURL := fmt.Sprintf(hostPathProvisionerResourceBaseURL, hostPathProvisionerStorageClass)
	args := append(deleteArgs, storageClassResourceURL)
	out, err := k8shelper.Kubectl(args...)
	if err != nil && !utils.IsKubectlErrorNotFound(out, err) {
		return fmt.Errorf("failed to delete hostpath provisioner StorageClass: %+v. %s", err, out)
	}

	deployment, err := getHostPathProvisionerDeployment()
	if err != nil {
		return err
	}
	out, err = k8shelper.KubectlWithStdin(deployment, deleteFromStdinArgs...)
	if err != nil && !utils.IsKubectlErrorNotFound(out, err) {
		return fmt.Errorf("failed to delete hostpath provisioner deployment: %+v. %s", err, out)
	}

	rbacResourceURL := fmt.Sprintf(hostPathProvisionerResourceBaseURL, hostPathProvisionerRBAC)
	args = append(deleteArgs, rbacResourceURL)
	out, err = k8shelper.Kubectl(args...)
	if err != nil && !utils.IsKubectlErrorNotFound(out, err) {
		return fmt.Errorf("failed to delete hostpath provisioner RBAC: %+v. %s", err, out)
	}

	return nil
}

func getHostPathProvisionerDeployment() (string, error) {
	// The hostpath provisioner project is readonly so we can't submit a PR.
	// Until we replace this completely, we'll just replace the necessary string.
	deploymentResourceURL := fmt.Sprintf(hostPathProvisionerResourceBaseURL, hostPathProvisionerDeployment)
	response, err := http.Get(deploymentResourceURL)
	if err != nil {
		return "", fmt.Errorf("failed to get hostpath provisioner deployment. %+v", err)
	}
	defer response.Body.Close()
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read hostpath provisioner deployment. %+v", err)
	}
	// K8s 1.16 requires apps/v1
	return strings.Replace(buf.String(), "apiVersion: extensions/v1beta1", "apiVersion: apps/v1", 1), nil
}
