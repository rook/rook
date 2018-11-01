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
	"io"
	"os"
	"path"
	"strings"
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
	ClusterAttr = "rook_cluster"
	// PublicIPEnvVar public IP env var
	PublicIPEnvVar = "ROOK_PUBLIC_IP"
	// PrivateIPEnvVar pod IP env var
	PrivateIPEnvVar = "ROOK_PRIVATE_IP"

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
func ConfigDirEnvVar(dataDir string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_CONFIG_DIR", Value: dataDir}
}

func GetContainerImage(pod *v1.Pod, name string) (string, error) {
	return GetSpecContainerImage(pod.Spec, name)
}

func GetSpecContainerImage(spec v1.PodSpec, name string) (string, error) {
	image, err := GetMatchingContainer(spec.Containers, name)
	if err != nil {
		return "", err
	}
	return image.Image, nil
}

func GetRunningPod(clientset kubernetes.Interface) (*v1.Pod, error) {
	podName := os.Getenv(PodNameEnvVar)
	if podName == "" {
		return nil, fmt.Errorf("cannot detect the pod name. Please provide it using the downward API in the manifest file")
	}
	podNamespace := os.Getenv(PodNamespaceEnvVar)
	if podName == "" {
		return nil, fmt.Errorf("cannot detect the pod namespace. Please provide it using the downward API in the manifest file")
	}

	pod, err := clientset.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pod, nil
}

func GetMatchingContainer(containers []v1.Container, name string) (v1.Container, error) {
	var result *v1.Container
	if len(containers) == 1 {
		// if there is only one pod, use its image rather than require a set container name
		result = &containers[0]
	} else {
		// if there are multiple pods, we require the container to have the expected name
		for _, container := range containers {
			if container.Name == name {
				result = &container
				break
			}
		}
	}

	if result == nil {
		return v1.Container{}, fmt.Errorf("failed to find image for container %s", name)
	}

	return *result, nil
}

// MakeRookImage formats the container name
func MakeRookImage(version string) string {
	if version == "" {
		return defaultVersion
	}

	return version
}

func PodsRunningWithLabel(clientset kubernetes.Interface, namespace, label string) (int, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{LabelSelector: label})
	if err != nil {
		return 0, err
	}

	running := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == v1.PodRunning {
			running++
		}
	}
	return running, nil
}

// GetPodPhaseMap takes a list of pods and returns a map of pod phases to the names of pods that are in that phase
func GetPodPhaseMap(pods *v1.PodList) map[v1.PodPhase][]string {
	podPhaseMap := map[v1.PodPhase][]string{} // status to list of pod names with that phase
	for _, pod := range pods.Items {
		podPhase := pod.Status.Phase
		podList, ok := podPhaseMap[podPhase]
		if !ok {
			// haven't seen this status yet, create a slice to keep track of pod names with this status
			podPhaseMap[podPhase] = []string{pod.Name}
		} else {
			// add this pod name to the list of pods already seen with this status
			podPhaseMap[podPhase] = append(podList, pod.Name)
		}
	}

	return podPhaseMap
}

// GetJobLog gets the logs for the pod. If there is more than one pod with the label selector, the logs from
// the first pod will be returned.
func GetPodLog(clientset kubernetes.Interface, namespace string, labelSelector string) (string, error) {
	opts := metav1.ListOptions{
		LabelSelector: labelSelector,
	}
	pods, err := clientset.CoreV1().Pods(namespace).List(opts)
	if err != nil {
		return "", fmt.Errorf("failed to get version pod. %+v", err)
	}
	for _, pod := range pods.Items {
		req := clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, &v1.PodLogOptions{})
		readCloser, err := req.Stream()
		if err != nil {
			return "", fmt.Errorf("failed to read from stream. %+v", err)
		}

		builder := &strings.Builder{}
		defer readCloser.Close()
		_, err = io.Copy(builder, readCloser)
		return builder.String(), err
	}

	return "", fmt.Errorf("did not find any pods with label %s", labelSelector)
}

// DeleteDeployment makes a best effort at deleting a deployment and its pods, then waits for them to be deleted
func DeleteDeployment(clientset kubernetes.Interface, namespace, name string) error {
	logger.Debugf("removing %s deployment if it exists", name)
	deleteAction := func(options *metav1.DeleteOptions) error {
		return clientset.ExtensionsV1beta1().Deployments(namespace).Delete(name, options)
	}
	getAction := func() error {
		_, err := clientset.ExtensionsV1beta1().Deployments(namespace).Get(name, metav1.GetOptions{})
		return err
	}
	return deleteResourceAndWait(namespace, name, "deployment", deleteAction, getAction)
}

// DeleteReplicaSet makes a best effort at deleting a deployment and its pods, then waits for them to be deleted
func DeleteReplicaSet(clientset kubernetes.Interface, namespace, name string) error {
	deleteAction := func(options *metav1.DeleteOptions) error {
		return clientset.ExtensionsV1beta1().ReplicaSets(namespace).Delete(name, options)
	}
	getAction := func() error {
		_, err := clientset.ExtensionsV1beta1().ReplicaSets(namespace).Get(name, metav1.GetOptions{})
		return err
	}
	return deleteResourceAndWait(namespace, name, "replicaset", deleteAction, getAction)
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
	return deleteResourceAndWait(namespace, name, "daemonset", deleteAction, getAction)
}

// deleteResourceAndWait will delete a resource, then wait for it to be purged from the system
func deleteResourceAndWait(namespace, name, resourceType string,
	deleteAction func(*metav1.DeleteOptions) error,
	getAction func() error) error {

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the resource if it exists
	err := deleteAction(options)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %s. %+v", name, err)
		}
		return nil
	}
	logger.Infof("Removed %s %s", resourceType, name)

	// wait for the resource to be deleted
	sleepTime := 2 * time.Second
	for i := 0; i < 30; i++ {
		// check for the existence of the resource
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

// Environment variables used by storage cluster daemons
func ClusterDaemonEnvVars() []v1.EnvVar {
	return []v1.EnvVar{
		{Name: "POD_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
		{Name: "POD_NAMESPACE", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}},
		{Name: "NODE_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
	}
}
