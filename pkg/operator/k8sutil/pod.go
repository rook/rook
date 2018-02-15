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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"fmt"
	"os"
	"path"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

const (
	// AppAttr app label
	AppAttr = "app"
	// ClusterAttr cluster label
	ClusterAttr = "ceph_cluster"
	// PublicIPEnvVar public IP env var
	PublicIPEnvVar = "ROOK_PUBLIC_IPV4"
	// PrivateIPEnvVar pod IP env var
	PrivateIPEnvVar = "ROOK_PRIVATE_IPV4"

	// DefaultRepoPrefix repo prefix
	DefaultRepoPrefix = "rook"
	// ConfigOverrideName config override name
	ConfigOverrideName = "rook-config-override"
	// ConfigOverrideVal config override value
	ConfigOverrideVal = "config"
	defaultVersion    = "rook/rook:latest"
	configMountDir    = "/etc/rook/config"
	overrideFilename  = "override.conf"
)

// ConfigOverrideMount is an override mount
func ConfigOverrideMount() v1.VolumeMount {
	return v1.VolumeMount{Name: ConfigOverrideName, MountPath: configMountDir}
}

// ConfigOverrideVolume is an override volume
func ConfigOverrideVolume() v1.Volume {
	cmSource := &v1.ConfigMapVolumeSource{Items: []v1.KeyToPath{{Key: ConfigOverrideVal, Path: overrideFilename}}}
	cmSource.Name = ConfigOverrideName
	return v1.Volume{Name: ConfigOverrideName, VolumeSource: v1.VolumeSource{ConfigMap: cmSource}}
}

// ConfigOverrideEnvVar config override env var
func ConfigOverrideEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_CEPH_CONFIG_OVERRIDE", Value: path.Join(configMountDir, overrideFilename)}
}

// PodIPEnvVar private ip env var
func PodIPEnvVar(property string) v1.EnvVar {
	return v1.EnvVar{Name: property, ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "status.podIP"}}}
}

// NamespaceEnvVar namespace env var
func NamespaceEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: PodNamespaceEnvVar, ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}}
}

// NameEnvVar pod name env var
func NameEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: PodNameEnvVar, ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.name"}}}
}

// NodeEnvVar node env var
func NodeEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: NodeNameEnvVar, ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}}
}

// ConfigDirEnvVar config dir env var
func ConfigDirEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_CONFIG_DIR", Value: DataDir}
}

func GetContainerImage(clientset kubernetes.Interface) (string, error) {

	podName := os.Getenv(PodNameEnvVar)
	if podName == "" {
		return "", fmt.Errorf("cannot detect the pod name. Please provide it using the downward API in the manifest file")
	}
	podNamespace := os.Getenv(PodNamespaceEnvVar)
	if podName == "" {
		return "", fmt.Errorf("cannot detect the pod namespace. Please provide it using the downward API in the manifest file")
	}

	pod, err := clientset.Core().Pods(podNamespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	if len(pod.Spec.Containers) != 1 {
		return "", fmt.Errorf("failed to get container image. There should only be exactly one container in this pod")
	}

	return pod.Spec.Containers[0].Image, nil
}

// MakeRookImage formats the container name
func MakeRookImage(version string) string {
	if version == "" {
		return defaultVersion
	}

	return version
}

// DeleteDeployment makes a best effort at deleting a deployment and its pods, then waits for them to be deleted
func DeleteDeployment(clientset kubernetes.Interface, namespace, name string) error {
	logger.Infof("removing %s deployment if it exists", name)
	deleteAction := func(options *metav1.DeleteOptions) error {
		return clientset.ExtensionsV1beta1().Deployments(namespace).Delete(name, options)
	}
	getAction := func() error {
		_, err := clientset.ExtensionsV1beta1().Deployments(namespace).Get(name, metav1.GetOptions{})
		return err
	}
	return deletePodsAndWait(namespace, name, deleteAction, getAction)
}

// DeleteDaemonset makes a best effort at deleting a daemonset and its pods, then waits for them to be deleted
func DeleteDaemonset(clientset kubernetes.Interface, namespace, name string) error {
	logger.Infof("removing %s daemonset if it exists", name)
	deleteAction := func(options *metav1.DeleteOptions) error {
		return clientset.ExtensionsV1beta1().DaemonSets(namespace).Delete(name, options)
	}
	getAction := func() error {
		_, err := clientset.ExtensionsV1beta1().DaemonSets(namespace).Get(name, metav1.GetOptions{})
		return err
	}
	return deletePodsAndWait(namespace, name, deleteAction, getAction)
}

// deletePodsAndWait will delete a resource, then wait for it to be purged from the system
func deletePodsAndWait(namespace, name string,
	deleteAction func(*metav1.DeleteOptions) error,
	getAction func() error) error {

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the deployment if it exists
	err := deleteAction(options)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete %s. %+v", name, err)
	}

	// wait for the daemonset and deployments to be deleted
	sleepTime := 2 * time.Second
	for i := 0; i < 30; i++ {
		// check for the existence of the deployment
		err = getAction()
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Infof("confirmed %s does not exist", name)
				return nil
			}
			return fmt.Errorf("failed to get %s. %+v", name, err)
		}

		logger.Infof("%s still found. waiting...", name)
		time.Sleep(sleepTime)
	}

	return fmt.Errorf("gave up waiting for %s pods to be terminated", name)
}
