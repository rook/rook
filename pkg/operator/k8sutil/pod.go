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
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

// PodIPEnvVar returns an env var such that the pod's ip will be mapped to the given property (env
// var) name within the container.
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

// GetContainerImage returns the container image
// matching the given name for a pod. If the pod
// only has a single container, the name argument
// is ignored.
func GetContainerImage(pod *v1.Pod, name string) (string, error) {
	return GetSpecContainerImage(pod.Spec, name, false)
}

// GetSpecContainerImage returns the container image
// for a podspec, given a container name. The name is
// ignored if the podspec has a single container, in
// which case the image for that container is returned.
func GetSpecContainerImage(spec v1.PodSpec, name string, initContainer bool) (string, error) {
	containers := spec.Containers
	if initContainer {
		containers = spec.InitContainers
	}
	image, err := GetMatchingContainer(containers, name)
	if err != nil {
		return "", err
	}
	return image.Image, nil
}

// Replaces the pod default toleration of 300s used when the node controller
// detect a not ready node (node.kubernetes.io/unreachable)
func AddUnreachableNodeToleration(podSpec *v1.PodSpec) {
	// The amount of time for this pod toleration can be modified by users
	// changing the value of <ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS> Rook operator
	// variable.
	// Node controller will wait 40 seconds by default before mark a node as
	// unreachable. After 40s + ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS the pod
	// will be scheduled in other node
	// Only one <toleration> to <unreachable> nodes can be added
	var tolerationSeconds int64 = 5
	urTolerationSeconds := os.Getenv("ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS")
	if urTolerationSeconds != "" {
		if duration, err := strconv.ParseInt(urTolerationSeconds, 10, 64); err != nil {
			logger.Warningf("using default value for <node.kubernetes.io/unreachable> toleration: %v seconds", tolerationSeconds)
		} else {
			tolerationSeconds = duration
		}
	}
	urToleration := v1.Toleration{Key: "node.kubernetes.io/unreachable",
		Operator:          "Exists",
		Effect:            "NoExecute",
		TolerationSeconds: &tolerationSeconds}

	for index, item := range podSpec.Tolerations {
		if item.Key == "node.kubernetes.io/unreachable" {
			podSpec.Tolerations[index] = urToleration
			return
		}
	}
	podSpec.Tolerations = append(podSpec.Tolerations, urToleration)
}

// GetRunningPod reads the name and namespace of a pod from the
// environment, and returns the pod (if it exists).
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

// GetMatchingContainer takes a list of containers and a name,
// and returns the first container in the list matching the
// name. If the list contains a single container it is always
// returned, even if the name does not match.
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

// PodsRunningWithLabel returns the number of running pods with the given label
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

// ClusterDaemonEnvVars Environment variables used by storage cluster daemon
func ClusterDaemonEnvVars(image string) []v1.EnvVar {
	return []v1.EnvVar{
		{Name: "CONTAINER_IMAGE", Value: image},
		{Name: "POD_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
		{Name: "POD_NAMESPACE", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}},
		{Name: "NODE_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},

		// If limits.memory is not set in the pod definition, Kubernetes will populate that value with the total memory available on the host
		// If a user sets 0, all available memory on the host will be used
		{Name: "POD_MEMORY_LIMIT", ValueFrom: &v1.EnvVarSource{ResourceFieldRef: &v1.ResourceFieldSelector{Resource: "limits.memory"}}}, // Bytes

		// If requests.memory is not set in the pod definition, Kubernetes will use the formula "requests.memory = limits.memory" during pods's scheduling
		// Kubernetes will set this variable to 0 or equal to limits.memory if set
		{Name: "POD_MEMORY_REQUEST", ValueFrom: &v1.EnvVarSource{ResourceFieldRef: &v1.ResourceFieldSelector{Resource: "requests.memory"}}}, // Bytes

		// If limits.cpu is not set in the pod definition, Kubernetes will set this variable to number of CPUs available on the host
		// If a user sets 0, all CPUs will be used
		{Name: "POD_CPU_LIMIT", ValueFrom: &v1.EnvVarSource{ResourceFieldRef: &v1.ResourceFieldSelector{Resource: "limits.cpu", Divisor: resource.MustParse("1")}}},

		// If request.cpu is not set in the pod definition, Kubernetes will use the formula "requests.cpu = limits.cpu" during pods's scheduling
		// Kubernetes will set this variable to 0 or equal to limits.cpu if set
		{Name: "POD_CPU_REQUEST", ValueFrom: &v1.EnvVarSource{ResourceFieldRef: &v1.ResourceFieldSelector{Resource: "requests.cpu"}}},
	}
}
