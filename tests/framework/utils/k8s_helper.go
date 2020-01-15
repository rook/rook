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

package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/stretchr/testify/require"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	storagev1util "k8s.io/kubernetes/pkg/apis/storage/v1/util"
)

// K8sHelper is a helper for common kubectl commads
type K8sHelper struct {
	executor         *exec.CommandExecutor
	Clientset        *kubernetes.Clientset
	RookClientset    *rookclient.Clientset
	RunningInCluster bool
	T                func() *testing.T
}

const (
	// RetryLoop params for tests.
	RetryLoop = 60
	// RetryInterval param for test - wait time while in RetryLoop
	RetryInterval = 5
	// TestMountPath is the path inside a test pod where storage is mounted
	TestMountPath = "/tmp/testrook"
	//hostnameTestPrefix is a prefix added to the node hostname
	hostnameTestPrefix = "test-prefix-this-is-a-very-long-hostname-"
)

// CreateK8sHelper creates a instance of k8sHelper
func CreateK8sHelper(t func() *testing.T) (*K8sHelper, error) {
	executor := &exec.CommandExecutor{}
	config, err := getKubeConfig(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to get kube client. %+v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset. %+v", err)
	}
	rookClientset, err := rookclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get rook clientset. %+v", err)
	}

	h := &K8sHelper{executor: executor, Clientset: clientset, RookClientset: rookClientset, T: t}
	if strings.Index(config.Host, "//10.") != -1 {
		h.RunningInCluster = true
	}
	return h, err
}

var k8slogger = capnslog.NewPackageLogger("github.com/rook/rook", "utils")

// GetK8sServerVersion returns k8s server version under test
func (k8sh *K8sHelper) GetK8sServerVersion() string {
	versionInfo, err := k8sh.Clientset.ServerVersion()
	require.Nil(k8sh.T(), err)
	return versionInfo.GitVersion
}

func (k8sh *K8sHelper) VersionAtLeast(minVersion string) bool {
	v := version.MustParseSemantic(k8sh.GetK8sServerVersion())
	return v.AtLeast(version.MustParseSemantic(minVersion))
}

func (k8sh *K8sHelper) VersionMinorMatches(minVersion string) (string, bool) {
	kubeVersion := k8sh.GetK8sServerVersion()
	v := version.MustParseSemantic(kubeVersion)
	requestedVersion := version.MustParseSemantic(minVersion)
	return kubeVersion, v.Major() == requestedVersion.Major() && v.Minor() == requestedVersion.Minor()
}

func (k8sh *K8sHelper) MakeContext() *clusterd.Context {
	return &clusterd.Context{Clientset: k8sh.Clientset, RookClientset: k8sh.RookClientset, Executor: k8sh.executor}
}

func (k8sh *K8sHelper) GetDockerImage(image string) error {
	dockercmd := os.Getenv("DOCKERCMD")
	if dockercmd == "" {
		dockercmd = "docker"
	}
	return k8sh.executor.ExecuteCommand(false, "", dockercmd, "pull", image)
}

// SetDeploymentVersion sets the container version on the deployment. It is assumed to be the rook/ceph image.
func (k8sh *K8sHelper) SetDeploymentVersion(namespace, deploymentName, containerName, version string) error {
	_, err := k8sh.Kubectl("-n", namespace, "set", "image", "deploy/"+deploymentName, containerName+"=rook/ceph:"+version)
	return err
}

// Kubectl is wrapper for executing kubectl commands
func (k8sh *K8sHelper) Kubectl(args ...string) (string, error) {
	result, err := k8sh.executor.ExecuteCommandWithTimeout(false, 15*time.Second, "kubectl", "kubectl", args...)
	if err != nil {
		k8slogger.Errorf("Failed to execute: kubectl %+v : %+v. %s", args, err, result)
		if args[0] == "delete" {
			// allow the tests to continue if we were deleting a resource that timed out
			return result, nil
		}
		return result, fmt.Errorf("Failed to run: kubectl %v : %v", args, err)
	}
	return result, nil
}

// KubectlWithStdin is wrapper for executing kubectl commands in stdin
func (k8sh *K8sHelper) KubectlWithStdin(stdin string, args ...string) (string, error) {

	cmdStruct := CommandArgs{Command: "kubectl", PipeToStdIn: stdin, CmdArgs: args}
	cmdOut := ExecuteCommand(cmdStruct)

	if cmdOut.ExitCode != 0 {
		k8slogger.Errorf("Failed to execute stdin: kubectl %v : %v", args, cmdOut.Err.Error())
		if strings.Index(cmdOut.Err.Error(), "(NotFound)") != -1 || strings.Index(cmdOut.StdErr, "(NotFound)") != -1 {
			return cmdOut.StdErr, errors.NewNotFound(schema.GroupResource{}, "")
		}
		return cmdOut.StdErr, fmt.Errorf("Failed to run stdin: kubectl %v : %v", args, cmdOut.StdErr)
	}
	if cmdOut.StdOut == "" {
		return cmdOut.StdErr, nil
	}

	return cmdOut.StdOut, nil

}

func getKubeConfig(executor exec.Executor) (*rest.Config, error) {
	context, err := executor.ExecuteCommandWithOutput(false, "", "kubectl", "config", "view", "-o", "json")
	if err != nil {
		k8slogger.Errorf("Errors Encountered while executing kubectl command : %v", err)
	}

	// Parse the kubectl context to get the settings for client connections
	var kc kubectlContext
	if err := json.Unmarshal([]byte(context), &kc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubectl config: %+v", err)
	}

	// find the current context
	var currentContext kContext
	found := false
	for _, c := range kc.Contexts {
		if kc.Current == c.Name {
			currentContext = c
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("failed to find current context %s in %+v", kc.Current, kc.Contexts)
	}

	// find the current cluster
	var currentCluster kclusterContext
	found = false
	for _, c := range kc.Clusters {
		if currentContext.Cluster.Cluster == c.Name {
			currentCluster = c
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("failed to find cluster %s in %+v", kc.Current, kc.Clusters)
	}
	config := &rest.Config{Host: currentCluster.Cluster.Server}

	if currentContext.Cluster.User == "" {
		config.Insecure = true
	} else {
		config.Insecure = false

		// find the current user
		var currentUser kuserContext
		found = false
		for _, u := range kc.Users {
			if currentContext.Cluster.User == u.Name {
				currentUser = u
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("failed to find kube user %s in %+v", kc.Current, kc.Users)
		}

		config.TLSClientConfig = rest.TLSClientConfig{
			CAFile:   currentCluster.Cluster.CertAuthority,
			KeyFile:  currentUser.Cluster.ClientKey,
			CertFile: currentUser.Cluster.ClientCert,
		}
		// Set Insecure to true if cert information is missing
		if currentUser.Cluster.ClientCert == "" {
			config.Insecure = true
		}
	}

	logger.Infof("Loaded kubectl context %s at %s. secure=%t",
		currentCluster.Name, config.Host, !config.Insecure)
	return config, nil
}

type kubectlContext struct {
	Contexts []kContext        `json:"contexts"`
	Users    []kuserContext    `json:"users"`
	Clusters []kclusterContext `json:"clusters"`
	Current  string            `json:"current-context"`
}
type kContext struct {
	Name    string `json:"name"`
	Cluster struct {
		Cluster string `json:"cluster"`
		User    string `json:"user"`
	} `json:"context"`
}
type kclusterContext struct {
	Name    string `json:"name"`
	Cluster struct {
		Server        string `json:"server"`
		Insecure      bool   `json:"insecure-skip-tls-verify"`
		CertAuthority string `json:"certificate-authority"`
	} `json:"cluster"`
}
type kuserContext struct {
	Name    string `json:"name"`
	Cluster struct {
		ClientCert string `json:"client-certificate"`
		ClientKey  string `json:"client-key"`
	} `json:"user"`
}

func (k8sh *K8sHelper) Exec(namespace, podName, command string, commandArgs []string) (string, error) {
	return k8sh.ExecWithRetry(1, namespace, podName, command, commandArgs)
}

// ExecWithRetry will attempt to run a command "retries" times, waiting 3s between each call. Upon success, returns the output.
func (k8sh *K8sHelper) ExecWithRetry(retries int, namespace, podName, command string, commandArgs []string) (string, error) {
	var err error
	for i := 0; i < retries; i++ {
		args := []string{"exec", "-n", namespace, podName, "--", command}
		args = append(args, commandArgs...)
		var result string
		result, err = k8sh.Kubectl(args...)
		if err == nil {
			return result, nil
		}
		if i < retries-1 {
			time.Sleep(3 * time.Second)
		}
	}
	return "", fmt.Errorf("kubectl exec command %s failed on pod %s in namespace %s. %+v", command, podName, namespace, err)
}

// ResourceOperationFromTemplate performs a kubectl action from a template file after replacing its context
func (k8sh *K8sHelper) ResourceOperationFromTemplate(action string, podDefinition string, config map[string]string) (string, error) {

	t := template.New("testTemplate")
	t, err := t.Parse(podDefinition)
	if err != nil {
		return err.Error(), err
	}
	var tpl bytes.Buffer

	if err := t.Execute(&tpl, config); err != nil {
		return err.Error(), err
	}

	podDef := tpl.String()

	args := []string{action, "-f", "-"}
	result, err := k8sh.KubectlWithStdin(podDef, args...)
	if err == nil {
		return result, nil
	}
	logger.Errorf("Failed to execute kubectl %v %v -- %v", args, podDef, err)
	return "", fmt.Errorf("Could not %s resource in args : %v  %v-- %v", action, args, podDef, err)
}

// ResourceOperation performs a kubectl action on a pod definition
func (k8sh *K8sHelper) ResourceOperation(action string, manifest string) error {
	args := []string{action, "-f", "-"}
	logger.Infof("kubectl %s manifest:\n%s", action, manifest)
	_, err := k8sh.KubectlWithStdin(manifest, args...)
	if err == nil {
		return nil
	}
	logger.Errorf("Failed to execute kubectl %v -- %v", args, err)
	return fmt.Errorf("Could Not create resource in args : %v -- %v", args, err)
}

// DeletePod performs a kubectl delete pod on the given pod
func (k8sh *K8sHelper) DeletePod(namespace, name string) error {
	args := append([]string{"--grace-period=0", "pod"}, name)
	if namespace != "" {
		args = append(args, []string{"-n", namespace}...)
	}
	return k8sh.DeleteResourceAndWait(true, args...)
}

// DeletePods performs a kubectl delete pod on the given pods
func (k8sh *K8sHelper) DeletePods(pods ...string) (msg string, err error) {
	for _, pod := range pods {
		if perr := k8sh.DeletePod("", pod); perr != nil {
			err = perr
		}
	}
	return
}

// DeleteResource performs a kubectl delete on the given args
func (k8sh *K8sHelper) DeleteResource(args ...string) error {
	return k8sh.DeleteResourceAndWait(true, args...)
}

// WaitForCustomResourceDeletion waits for the CRD deletion
func (k8sh *K8sHelper) WaitForCustomResourceDeletion(namespace string, checkerFunc func() error) error {

	// wait for the operator to finalize and delete the CRD
	for i := 0; i < 10; i++ {
		err := checkerFunc()
		if err == nil {
			logger.Infof("custom resource %s still exists", namespace)
			time.Sleep(2 * time.Second)
			continue
		}
		if errors.IsNotFound(err) {
			logger.Infof("custom resource %s deleted", namespace)
			return nil
		}
		return err
	}
	logger.Errorf("gave up deleting custom resource %s", namespace)
	return nil
}

// DeleteResource performs a kubectl delete on give args.
// If wait is false, a flag will be passed to indicate the delete should return immediately
func (k8sh *K8sHelper) DeleteResourceAndWait(wait bool, args ...string) error {
	if !wait {
		// new flag in k8s 1.11
		v := version.MustParseSemantic(k8sh.GetK8sServerVersion())
		if v.AtLeast(version.MustParseSemantic("1.11.0")) {
			args = append(args, "--wait=false")
		}
	}
	args = append([]string{"delete"}, args...)
	_, err := k8sh.Kubectl(args...)
	if err == nil {
		return nil
	}
	return fmt.Errorf("Could Not delete resource in k8s -- %v", err)
}

// GetResource performs a kubectl get on give args
func (k8sh *K8sHelper) GetResource(args ...string) (string, error) {
	args = append([]string{"get"}, args...)
	result, err := k8sh.Kubectl(args...)
	if err == nil {
		return result, nil
	}
	return "", fmt.Errorf("Could Not get resource in k8s -- %v", err)

}

func (k8sh *K8sHelper) CreateNamespace(namespace string) error {
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err := k8sh.Clientset.CoreV1().Namespaces().Create(ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace %s. %+v", namespace, err)
	}

	return nil
}

func (k8sh *K8sHelper) CountPodsWithLabel(label string, namespace string) (int, error) {
	options := metav1.ListOptions{LabelSelector: label}
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(options)
	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return len(pods.Items), nil
}

// WaitForPodCount waits until the desired number of pods with the label are started
func (k8sh *K8sHelper) WaitForPodCount(label, namespace string, count int) error {
	options := metav1.ListOptions{LabelSelector: label}
	inc := 0
	for inc < RetryLoop {
		pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(options)
		if err != nil {
			return fmt.Errorf("failed to find pod with label %s. %+v", label, err)
		}

		if len(pods.Items) >= count {
			logger.Infof("found %d pods with label %s", count, label)
			return nil
		}
		inc++
		time.Sleep(RetryInterval * time.Second)
		logger.Infof("waiting for %d pods (found %d) with label %s in namespace %s", count, len(pods.Items), label, namespace)

	}
	return fmt.Errorf("Giving up waiting for pods with label %s in namespace %s", label, namespace)
}

// IsPodWithLabelPresent return true if there is at least one Pod with the label is present.
func (k8sh *K8sHelper) IsPodWithLabelPresent(label string, namespace string) bool {
	count, err := k8sh.CountPodsWithLabel(label, namespace)
	if err != nil {
		return false
	}
	return count > 0
}

// WaitForLabeledPodsToRun calls WaitForLabeledPodsToRunWithRetries with the default number of retries
func (k8sh *K8sHelper) WaitForLabeledPodsToRun(label, namespace string) error {
	return k8sh.WaitForLabeledPodsToRunWithRetries(label, namespace, RetryLoop)
}

// WaitForLabeledPodsToRunWithRetries returns true if a Pod is running status or goes to Running status within 90s else returns false
func (k8sh *K8sHelper) WaitForLabeledPodsToRunWithRetries(label string, namespace string, retries int) error {
	options := metav1.ListOptions{LabelSelector: label}
	var lastPod v1.Pod
	for i := 0; i < retries; i++ {
		pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(options)
		lastStatus := ""
		running := 0
		if err == nil && len(pods.Items) > 0 {
			for _, pod := range pods.Items {
				if pod.Status.Phase == "Running" {
					running++
				}
				lastPod = pod
				lastStatus = string(pod.Status.Phase)
			}
			if running == len(pods.Items) {
				logger.Infof("All %d pod(s) with label %s are running", len(pods.Items), label)
				return nil
			}
		}
		logger.Infof("waiting for pod(s) with label %s in namespace %s to be running. status=%s, running=%d/%d, err=%+v",
			label, namespace, lastStatus, running, len(pods.Items), err)
		time.Sleep(RetryInterval * time.Second)
	}

	if len(lastPod.Name) == 0 {
		logger.Infof("no pod was found with label %s", label)
	} else {
		k8sh.PrintPodDescribe(namespace, lastPod.Name)
	}
	return fmt.Errorf("Giving up waiting for pod with label %s in namespace %s to be running", label, namespace)
}

// WaitUntilPodWithLabelDeleted returns true if a Pod is deleted within 90s else returns false
func (k8sh *K8sHelper) WaitUntilPodWithLabelDeleted(label string, namespace string) bool {
	options := metav1.ListOptions{LabelSelector: label}
	for i := 0; i < RetryLoop; i++ {
		pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(options)
		if errors.IsNotFound(err) {
			logger.Infof("error Found err %v", err)
			return true
		}
		if len(pods.Items) == 0 {
			logger.Infof("no (more) pods with label %s in namespace %s to be deleted", label, namespace)
			return true
		}

		time.Sleep(RetryInterval * time.Second)
		logger.Infof("waiting for pod with label %s in namespace %s to be deleted", label, namespace)

	}
	logger.Infof("Giving up waiting for pod with label %s in namespace %s to be deleted", label, namespace)
	return false
}

// PrintPodStatus log out the status phase of a pod
func (k8sh *K8sHelper) PrintPodStatus(namespace string) {
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to get pod status in namespace %s. %+v", namespace, err)
		return
	}
	for _, pod := range pods.Items {
		logger.Infof("%s (%s) pod status: %+v", pod.Name, namespace, pod.Status)
	}
}

func (k8sh *K8sHelper) GetPodDescribeFromNamespace(namespace, testName, platformName string) {
	logger.Infof("Gathering pod describe for all pods in namespace %s", namespace)

	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to list pods in namespace %s. %+v", namespace, err)
		return
	}

	file, err := k8sh.createTestLogFile(platformName, "podDescribe", namespace, testName, "")
	if err != nil {
		return
	}
	defer file.Close()

	for _, p := range pods.Items {
		k8sh.appendPodDescribe(file, namespace, p.Name)
	}
}

func (k8sh *K8sHelper) appendPodDescribe(file *os.File, namespace, name string) {
	description := k8sh.getPodDescribe(namespace, name)
	if description == "" {
		return
	}
	writeHeader(file, fmt.Sprintf("Pod: %s\n", name))
	file.WriteString(description)
	file.WriteString("\n")
}

func (k8sh *K8sHelper) PrintPodDescribe(namespace string, args ...string) {
	description := k8sh.getPodDescribe(namespace, args...)
	if description == "" {
		return
	}
	logger.Infof("POD Description:\n%s", description)
}

func (k8sh *K8sHelper) getPodDescribe(namespace string, args ...string) string {
	args = append([]string{"describe", "pod", "-n", namespace}, args...)
	description, err := k8sh.Kubectl(args...)
	if err != nil {
		logger.Errorf("failed to describe pod. %v %+v", args, err)
		return ""
	}
	return description
}

func (k8sh *K8sHelper) PrintEventsForNamespace(namespace string) {
	events, err := k8sh.Clientset.CoreV1().Events(namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Warningf("failed to get events in namespace %s. %+v", namespace, err)
		return
	}
	logger.Infof("DUMPING events in namespace %s", namespace)
	for _, event := range events.Items {
		logger.Infof("%+v", event)
	}
	logger.Infof("DONE DUMPING events in namespace %s", namespace)
}

// IsPodRunning returns true if a Pod is running status or goes to Running status within 90s else returns false
func (k8sh *K8sHelper) IsPodRunning(name string, namespace string) bool {
	getOpts := metav1.GetOptions{}
	inc := 0
	for inc < RetryLoop {
		pod, err := k8sh.Clientset.CoreV1().Pods(namespace).Get(name, getOpts)
		if err == nil {
			if pod.Status.Phase == "Running" {
				return true
			}
		}
		inc++
		time.Sleep(RetryInterval * time.Second)
		logger.Infof("waiting for pod %s in namespace %s to be running", name, namespace)

	}
	pod, _ := k8sh.Clientset.CoreV1().Pods(namespace).Get(name, getOpts)
	k8sh.PrintPodDescribe(namespace, pod.Name)
	logger.Infof("Giving up waiting for pod %s in namespace %s to be running", name, namespace)
	return false
}

// IsPodTerminated wrapper around IsPodTerminatedWithOpts()
func (k8sh *K8sHelper) IsPodTerminated(name string, namespace string) bool {
	return k8sh.IsPodTerminatedWithOpts(name, namespace, metav1.GetOptions{})
}

// IsPodTerminatedWithOpts returns true if a Pod is terminated status or goes to Terminated status
// within 90s else returns false\
func (k8sh *K8sHelper) IsPodTerminatedWithOpts(name string, namespace string, getOpts metav1.GetOptions) bool {
	inc := 0
	for inc < RetryLoop {
		pod, err := k8sh.Clientset.CoreV1().Pods(namespace).Get(name, getOpts)
		if err != nil {
			k8slogger.Infof("Pod  %s in namespace %s terminated ", name, namespace)
			return true
		}
		k8slogger.Infof("waiting for Pod %s in namespace %s to terminate, status : %+v", name, namespace, pod.Status)
		time.Sleep(RetryInterval * time.Second)
		inc++
	}
	k8slogger.Infof("Pod %s in namespace %s did not terminate", name, namespace)
	return false
}

// IsServiceUp returns true if a service is up or comes up within 150s, else returns false
func (k8sh *K8sHelper) IsServiceUp(name string, namespace string) bool {
	getOpts := metav1.GetOptions{}
	inc := 0
	for inc < RetryLoop {
		_, err := k8sh.Clientset.CoreV1().Services(namespace).Get(name, getOpts)
		if err == nil {
			k8slogger.Infof("Service: %s in namespace: %s is up", name, namespace)
			return true
		}
		k8slogger.Infof("waiting for Service %s in namespace %s ", name, namespace)
		time.Sleep(RetryInterval * time.Second)
		inc++
	}
	k8slogger.Infof("Giving up waiting for service: %s in namespace %s ", name, namespace)
	return false
}

// GetService returns output from "kubectl get svc $NAME" command
func (k8sh *K8sHelper) GetService(servicename string, namespace string) (*v1.Service, error) {
	getOpts := metav1.GetOptions{}
	result, err := k8sh.Clientset.CoreV1().Services(namespace).Get(servicename, getOpts)
	if err != nil {
		return nil, fmt.Errorf("Cannot find service %s in namespace %s, err-- %v", servicename, namespace, err)
	}
	return result, nil
}

// IsCRDPresent returns true if custom resource definition is present
func (k8sh *K8sHelper) IsCRDPresent(crdName string) bool {

	cmdArgs := []string{"get", "crd", crdName}
	inc := 0
	for inc < RetryLoop {
		_, err := k8sh.Kubectl(cmdArgs...)
		if err == nil {
			k8slogger.Infof("Found the CRD resource: " + crdName)
			return true
		}
		time.Sleep(RetryInterval * time.Second)
		inc++
	}

	return false
}

// WriteToPod write file in Pod
func (k8sh *K8sHelper) WriteToPod(namespace, podName, filename, message string) error {
	return k8sh.WriteToPodRetry(namespace, podName, filename, message, 1)
}

// WriteToPodRetry WriteToPod in a retry loop
func (k8sh *K8sHelper) WriteToPodRetry(namespace, podName, filename, message string, retries int) error {
	logger.Infof("Writing file %s to pod %s", filename, podName)
	var err error
	for i := 0; i < retries; i++ {
		if i > 0 {
			logger.Infof("retrying write in 5s...")
			time.Sleep(5 * time.Second)
		}

		err = k8sh.writeToPod(namespace, podName, filename, message)
		if err == nil {
			logger.Infof("write file %s in pod %s was successful", filename, podName)
			return nil
		}
	}

	return fmt.Errorf("failed to write file %s to pod %s. %+v", filename, podName, err)
}

func (k8sh *K8sHelper) ReadFromPod(namespace, podName, filename, expectedMessage string) error {
	return k8sh.ReadFromPodRetry(namespace, podName, filename, expectedMessage, 1)
}

func (k8sh *K8sHelper) ReadFromPodRetry(namespace, podName, filename, expectedMessage string, retries int) error {
	logger.Infof("Reading file %s from pod %s", filename, podName)
	var err error
	for i := 0; i < retries; i++ {
		if i > 0 {
			logger.Infof("retrying read in 5s...")
			time.Sleep(5 * time.Second)
		}

		var data string
		data, err = k8sh.readFromPod(namespace, podName, filename)
		if err == nil {
			logger.Infof("read file %s from pod %s was successful after %d attempt(s)", filename, podName, (i + 1))
			if !strings.Contains(data, expectedMessage) {
				return fmt.Errorf(`file %s in pod %s returned message "%s" instead of "%s"`, filename, podName, data, expectedMessage)
			}
			return nil
		}
	}

	return fmt.Errorf("failed to read file %s from pod %s. %+v", filename, podName, err)
}

func (k8sh *K8sHelper) writeToPod(namespace, name, filename, message string) error {
	wt := "echo \"" + message + "\">" + path.Join(TestMountPath, filename)
	args := []string{"exec", name}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, "--", "sh", "-c", wt)

	_, err := k8sh.Kubectl(args...)
	if err != nil {
		return fmt.Errorf("failed to write file %s to pod %s. %+v", filename, name, err)
	}

	return nil
}

func (k8sh *K8sHelper) readFromPod(namespace, name, filename string) (string, error) {
	rd := path.Join(TestMountPath, filename)
	args := []string{"exec", name}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, "--", "cat", rd)

	result, err := k8sh.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s from pod %s. %+v", filename, name, err)
	}
	return result, nil
}

// GetVolumeResourceName gets the Volume object name from the PVC
func (k8sh *K8sHelper) GetVolumeResourceName(namespace, pvcName string) (string, error) {

	getOpts := metav1.GetOptions{}
	pvc, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(pvcName, getOpts)
	if err != nil {
		return "", err
	}
	return pvc.Spec.VolumeName, nil
}

// IsVolumeResourcePresent returns true if Volume resource is present
func (k8sh *K8sHelper) IsVolumeResourcePresent(namespace, volumeName string) bool {
	err := k8sh.waitForVolume(namespace, volumeName, true)
	if err != nil {
		k8slogger.Error(err.Error())
		return false
	}
	return true
}

// IsVolumeResourceAbsent returns true if the Volume resource is deleted/absent within 90s else returns false
func (k8sh *K8sHelper) IsVolumeResourceAbsent(namespace, volumeName string) bool {

	err := k8sh.waitForVolume(namespace, volumeName, false)
	if err != nil {
		k8slogger.Error(err.Error())
		return false
	}
	return true
}

func (k8sh *K8sHelper) waitForVolume(namespace, volumeName string, exist bool) error {

	action := "exist"
	if !exist {
		action = "not " + action
	}

	inc := 0
	for inc < RetryLoop {
		isExist, err := k8sh.isVolumeExist(namespace, volumeName)
		if err != nil {
			return fmt.Errorf("Errors encountered while getting Volume %s/%s: %v", namespace, volumeName, err)
		}
		if isExist == exist {
			return nil
		}

		k8slogger.Infof("waiting for Volume %s in namespace %s to %s", volumeName, namespace, action)
		time.Sleep(RetryInterval * time.Second)
		inc++

	}

	k8sh.printVolumes(namespace, volumeName)
	k8sh.PrintPVs(false /*detailed*/)
	k8sh.PrintPVCs(namespace, false /*detailed*/)
	return fmt.Errorf("timeout for Volume %s in namespace %s wait to %s", volumeName, namespace, action)
}

func (k8sh *K8sHelper) PrintPVs(detailed bool) {
	pvs, err := k8sh.Clientset.CoreV1().PersistentVolumes().List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to list pvs. %+v", err)
		return
	}

	if detailed {
		logger.Infof("Found %d PVs", len(pvs.Items))
		for _, pv := range pvs.Items {
			logger.Infof("PV %s: %+v", pv.Name, pv)
		}
	} else {
		var names []string
		for _, pv := range pvs.Items {
			names = append(names, pv.Name)
		}
		logger.Infof("Found PVs: %v", names)
	}
}

func (k8sh *K8sHelper) PrintPVCs(namespace string, detailed bool) {
	pvcs, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to list pvcs. %+v", err)
		return
	}

	if detailed {
		logger.Infof("Found %d PVCs", len(pvcs.Items))
		for _, pvc := range pvcs.Items {
			logger.Infof("PVC %s: %+v", pvc.Name, pvc)
		}
	} else {
		var names []string
		for _, pvc := range pvcs.Items {
			names = append(names, pvc.Name)
		}
		logger.Infof("Found PVCs: %v", names)
	}
}

func (k8sh *K8sHelper) PrintStorageClasses(detailed bool) {
	scs, err := k8sh.Clientset.StorageV1().StorageClasses().List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to list StorageClasses: %+v", err)
		return
	}

	if detailed {
		logger.Infof("Found %d StorageClasses", len(scs.Items))
		for _, sc := range scs.Items {
			logger.Infof("StorageClass %s: %+v", sc.Name, sc)
		}
	} else {
		var names []string
		for _, sc := range scs.Items {
			names = append(names, sc.Name)
		}
		logger.Infof("Found StorageClasses: %v", names)
	}
}

func (k8sh *K8sHelper) printVolumes(namespace, desiredVolume string) {
	volumes, err := k8sh.RookClientset.RookV1alpha2().Volumes(namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Infof("failed to list volumes in ns %s. %+v", namespace, err)
	}

	var names []string
	for _, volume := range volumes.Items {
		names = append(names, volume.Name)
	}
	logger.Infof("looking for volume %s in namespace %s. Found volumes: %v", desiredVolume, namespace, names)
}

func (k8sh *K8sHelper) isVolumeExist(namespace, name string) (bool, error) {
	_, err := k8sh.RookClientset.RookV1alpha2().Volumes(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k8sh *K8sHelper) GetPodNamesForApp(appName, namespace string) ([]string, error) {
	args := []string{"get", "pod", "-n", namespace, "-l", fmt.Sprintf("app=%s", appName),
		"-o", "jsonpath={.items[*].metadata.name}"}
	result, err := k8sh.Kubectl(args...)

	if err != nil {
		return nil, fmt.Errorf("failed to get pod names for app %s: %+v. output: %s", appName, err, result)
	}

	podNames := strings.Split(result, " ")
	return podNames, nil
}

// GetPodDetails returns details about a  pod
func (k8sh *K8sHelper) GetPodDetails(podNamePattern string, namespace string) (string, error) {
	args := []string{"get", "pods", "-l", "app=" + podNamePattern, "-o", "wide", "--no-headers=true", "-o", "name"}
	if namespace != "" {
		args = append(args, []string{"-n", namespace}...)
	}
	result, err := k8sh.Kubectl(args...)
	if err != nil || strings.Contains(result, "No resources found") {
		return "", fmt.Errorf("Cannot find pod in with name like %s in namespace : %s -- %v", podNamePattern, namespace, err)
	}
	return strings.TrimSpace(result), nil
}

// GetPodEvents returns events about a pod
func (k8sh *K8sHelper) GetPodEvents(podNamePattern string, namespace string) (*v1.EventList, error) {
	uri := fmt.Sprintf("api/v1/namespaces/%s/events?fieldSelector=involvedObject.name=%s,involvedObject.namespace=%s", namespace, podNamePattern, namespace)
	result, err := k8sh.Clientset.CoreV1().RESTClient().Get().RequestURI(uri).DoRaw()
	if err != nil {
		logger.Errorf("Cannot get events for pod %v in namespace %v, err: %v", podNamePattern, namespace, err)
		return nil, fmt.Errorf("Cannot get events for pod %s in namespace %s, err: %v", podNamePattern, namespace, err)
	}

	events := v1.EventList{}
	err = json.Unmarshal(result, &events)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal eventlist response: %v", err)
	}

	return &events, nil
}

// IsPodInError returns true if a Pod is in error status with the given reason and contains the given message
func (k8sh *K8sHelper) IsPodInError(podNamePattern, namespace, reason, containingMessage string) bool {
	inc := 0
	for inc < RetryLoop {
		events, err := k8sh.GetPodEvents(podNamePattern, namespace)
		if err != nil {
			k8slogger.Errorf("Cannot get Pod events for %s in namespace %s: %+v ", podNamePattern, namespace, err)
			return false
		}

		for _, e := range events.Items {
			if e.Reason == reason && strings.Contains(e.Message, containingMessage) {
				return true
			}
		}
		k8slogger.Infof("waiting for Pod %s in namespace %s to error with reason %s and containing the message: %s", podNamePattern, namespace, reason, containingMessage)
		time.Sleep(RetryInterval * time.Second)
		inc++

	}
	k8slogger.Infof("Pod %s in namespace %s did not error with reason %s", podNamePattern, namespace, reason)
	return false
}

// GetPodHostID returns HostIP address of a pod
func (k8sh *K8sHelper) GetPodHostID(podNamePattern string, namespace string) (string, error) {
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podNamePattern}
	podList, err := k8sh.Clientset.CoreV1().Pods(namespace).List(listOpts)
	if err != nil {
		logger.Errorf("Cannot get hostIp for app : %v in namespace %v, err: %v", podNamePattern, namespace, err)
		return "", fmt.Errorf("Cannot get hostIp for app : %v in namespace %v, err: %v", podNamePattern, namespace, err)
	}

	if len(podList.Items) < 1 {
		logger.Errorf("Cannot get hostIp for app : %v in namespace %v, err: %v", podNamePattern, namespace, err)
		return "", fmt.Errorf("Cannot get hostIp for app : %v in namespace %v, err: %v", podNamePattern, namespace, err)
	}
	return podList.Items[0].Status.HostIP, nil
}

// GetServiceNodePort returns nodeProt of service
func (k8sh *K8sHelper) GetServiceNodePort(serviceName string, namespace string) (string, error) {
	getOpts := metav1.GetOptions{}
	svc, err := k8sh.Clientset.CoreV1().Services(namespace).Get(serviceName, getOpts)
	if err != nil {
		logger.Errorf("Cannot get service : %v in namespace %v, err: %v", serviceName, namespace, err)
		return "", fmt.Errorf("Cannot get service : %v in namespace %v, err: %v", serviceName, namespace, err)
	}
	np := svc.Spec.Ports[0].NodePort
	return strconv.FormatInt(int64(np), 10), nil
}

// IsStorageClassPresent returns true if storageClass is present, if not false
func (k8sh *K8sHelper) IsStorageClassPresent(name string) error {
	args := []string{"get", "storageclass", "-o", "jsonpath='{.items[*].metadata.name}'"}
	result, err := k8sh.Kubectl(args...)
	if strings.Contains(result, name) {
		return nil
	}
	return fmt.Errorf("Storageclass %s not found, err ->%v", name, err)
}

func (k8sh *K8sHelper) IsDefaultStorageClassPresent() (bool, error) {
	scs, err := k8sh.Clientset.StorageV1().StorageClasses().List(metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list StorageClasses: %+v", err)
	}

	for _, sc := range scs.Items {
		if storagev1util.IsDefaultAnnotation(sc.ObjectMeta) {
			return true, nil
		}
	}

	return false, nil
}

// CheckPvcCount returns True if expected number pvs for a app are found
func (k8sh *K8sHelper) CheckPvcCountAndStatus(podName string, namespace string, expectedPvcCount int, expectedStatus string) bool {
	logger.Infof("wait until %d pvc for app=%s are present", expectedPvcCount, podName)
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podName}
	pvcCountCheck := false

	actualPvcCount := 0
	inc := 0
	for inc < RetryLoop {
		pvcList, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(listOpts)
		if err != nil {
			logger.Errorf("Cannot get pvc for app : %v in namespace %v, err: %v", podName, namespace, err)
			return false
		}
		actualPvcCount = len(pvcList.Items)
		if actualPvcCount == expectedPvcCount {
			pvcCountCheck = true
			break
		}
		inc++
		time.Sleep(RetryInterval * time.Second)
	}

	if !pvcCountCheck {
		logger.Errorf("Expecting %d number of PVCs for %s app, found %d ", expectedPvcCount, podName, actualPvcCount)
		return false
	}

	inc = 0
	for inc < RetryLoop {
		checkAllPVCsStatus := true
		pl, _ := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(listOpts)
		for _, pvc := range pl.Items {
			if !(pvc.Status.Phase == v1.PersistentVolumeClaimPhase(expectedStatus)) {
				checkAllPVCsStatus = false
				logger.Infof("waiting for pvc %v to be in %s Phase, currently in %v Phase", pvc.Name, expectedStatus, pvc.Status.Phase)
			}
		}
		if checkAllPVCsStatus {
			return true
		}
		inc++
		time.Sleep(RetryInterval * time.Second)

	}
	logger.Errorf("Giving up waiting for %d PVCs for %s app to be in %s phase", expectedPvcCount, podName, expectedStatus)
	return false
}

// GetPVCStatus returns status of PVC
func (k8sh *K8sHelper) GetPVCStatus(namespace string, name string) (v1.PersistentVolumeClaimPhase, error) {
	getOpts := metav1.GetOptions{}

	pvc, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(name, getOpts)
	if err != nil {
		return v1.ClaimLost, fmt.Errorf("PVC %s not found,err->%v", name, err)
	}

	return pvc.Status.Phase, nil

}

// GetPVCVolumeName returns volume name of PVC
func (k8sh *K8sHelper) GetPVCVolumeName(namespace string, name string) (string, error) {
	getOpts := metav1.GetOptions{}

	pvc, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(name, getOpts)
	if err != nil {
		return "", fmt.Errorf("PVC %s not found,err->%v", name, err)
	}

	return pvc.Spec.VolumeName, nil
}

// GetPVCAccessModes returns AccessModes on PVC
func (k8sh *K8sHelper) GetPVCAccessModes(namespace string, name string) ([]v1.PersistentVolumeAccessMode, error) {
	getOpts := metav1.GetOptions{}

	pvc, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(name, getOpts)
	if err != nil {
		return []v1.PersistentVolumeAccessMode{}, fmt.Errorf("PVC %s not found,err->%v", name, err)
	}

	return pvc.Status.AccessModes, nil

}

// GetPV returns PV by name
func (k8sh *K8sHelper) GetPV(name string) (*v1.PersistentVolume, error) {
	getOpts := metav1.GetOptions{}

	pv, err := k8sh.Clientset.CoreV1().PersistentVolumes().Get(name, getOpts)
	if err != nil {
		return nil, fmt.Errorf("PV %s not found,err->%v", name, err)
	}
	return pv, nil
}

// IsPodInExpectedState waits for 90s for a pod to be an expected state
// If the pod is in expected state within 90s true is returned,  if not false
func (k8sh *K8sHelper) IsPodInExpectedState(podNamePattern string, namespace string, state string) bool {
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podNamePattern}
	inc := 0
	for inc < RetryLoop {
		podList, err := k8sh.Clientset.CoreV1().Pods(namespace).List(listOpts)
		if err == nil {
			if len(podList.Items) >= 1 {
				if podList.Items[0].Status.Phase == v1.PodPhase(state) {
					return true
				}
			}
		}
		inc++
		time.Sleep(RetryInterval * time.Second)
	}

	return false
}

// CheckPodCountAndState returns true if expected number of pods with matching name are found and are in expected state
func (k8sh *K8sHelper) CheckPodCountAndState(podName string, namespace string, minExpected int, expectedPhase string) bool {
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podName}
	podCountCheck := false
	actualPodCount := 0
	inc := 0
	for inc < RetryLoop {
		podList, err := k8sh.Clientset.CoreV1().Pods(namespace).List(listOpts)
		if err != nil {
			logger.Errorf("Cannot list pods for app=%s in namespace %s, err: %+v", podName, namespace, err)
			return false
		}
		actualPodCount = len(podList.Items)
		if actualPodCount >= minExpected {
			logger.Infof("%d of %d pods with label app=%s were found", actualPodCount, minExpected, podName)
			podCountCheck = true
			break
		}

		inc++
		logger.Infof("waiting for %d pods with label app=%s, found %d", minExpected, podName, actualPodCount)
		time.Sleep(RetryInterval * time.Second)
	}
	if !podCountCheck {
		logger.Errorf("Expecting %d number of pods for %s app, found %d ", minExpected, podName, actualPodCount)
		return false
	}

	for i := 0; i < RetryLoop; i++ {
		checkAllPodsStatus := true
		pl, _ := k8sh.Clientset.CoreV1().Pods(namespace).List(listOpts)
		for _, pod := range pl.Items {
			if !(pod.Status.Phase == v1.PodPhase(expectedPhase)) {
				checkAllPodsStatus = false
				logger.Infof("waiting for pod %v to be in %s Phase, currently in %v Phase", pod.Name, expectedPhase, pod.Status.Phase)
			}
		}
		if checkAllPodsStatus {
			return true
		}
		time.Sleep(RetryInterval * time.Second)
	}

	logger.Errorf("All pods with app Name %v not in %v phase ", podName, expectedPhase)
	k8sh.PrintPodDescribe(namespace, "-l", listOpts.LabelSelector)
	return false

}

// WaitUntilPodInNamespaceIsDeleted waits for 90s for a pod  in a namespace to be terminated
// If the pod disappears within 90s true is returned,  if not false
func (k8sh *K8sHelper) WaitUntilPodInNamespaceIsDeleted(podNamePattern string, namespace string) bool {
	inc := 0
	for inc < RetryLoop {
		out, _ := k8sh.GetResource("-n", namespace, "pods", "-l", "app="+podNamePattern)
		if !strings.Contains(out, podNamePattern) {
			return true
		}

		inc++
		time.Sleep(RetryInterval * time.Second)
	}
	logger.Infof("Pod %s in namespace %s not deleted", podNamePattern, namespace)
	return false
}

// WaitUntilPodIsDeleted waits for 90s for a pod to be terminated
// If the pod disappears within 90s true is returned,  if not false
func (k8sh *K8sHelper) WaitUntilPodIsDeleted(name, namespace string) bool {
	inc := 0
	for inc < RetryLoop {
		_, err := k8sh.Clientset.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
		if err != nil && errors.IsNotFound(err) {
			return true
		}

		inc++
		logger.Infof("pod %s in namespace %s is not deleted yet", name, namespace)
		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

// WaitUntilPVCIsBound waits for a PVC to be in bound state for 90 seconds
// if PVC goes to Bound state within 90s True is returned, if not false
func (k8sh *K8sHelper) WaitUntilPVCIsBound(namespace string, pvcname string) bool {

	inc := 0
	for inc < RetryLoop {
		out, err := k8sh.GetPVCStatus(namespace, pvcname)
		if err == nil {
			if out == v1.PersistentVolumeClaimPhase(v1.ClaimBound) {
				logger.Infof("PVC %s is bound", pvcname)
				return true
			}
		}
		logger.Infof("waiting for PVC %s to be bound. current=%s. err=%+v", pvcname, out, err)
		inc++
		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

// WaitUntilPVCIsExpanded waits for a PVC to be resized for specified value
func (k8sh *K8sHelper) WaitUntilPVCIsExpanded(namespace, pvcname, size string) bool {
	getOpts := metav1.GetOptions{}
	inc := 0
	for inc < RetryLoop {
		// PVC specs changes immediately, but status will change only if resize process is successfully completed.
		pvc, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(pvcname, getOpts)
		if err == nil {
			currentSize := pvc.Status.Capacity[v1.ResourceStorage]
			if currentSize.String() == size {
				logger.Infof("PVC %s is resized", pvcname)
				return true
			}
			logger.Infof("waiting for PVC %s to be resized, current: %s, expected: %s", pvcname, currentSize.String(), size)
		} else {
			logger.Infof("error while getting PVC specs: %+v", err)
		}
		inc++
		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

func (k8sh *K8sHelper) WaitUntilPVCIsDeleted(namespace string, pvcname string) bool {
	getOpts := metav1.GetOptions{}
	inc := 0
	for inc < RetryLoop {
		_, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(pvcname, getOpts)
		if err != nil {
			return true
		}
		logger.Infof("waiting for PVC %s to be deleted.", pvcname)
		inc++
		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

func (k8sh *K8sHelper) DeletePvcWithLabel(namespace string, podName string) bool {
	delOpts := metav1.DeleteOptions{}
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podName}

	err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).DeleteCollection(&delOpts, listOpts)
	if err != nil {
		logger.Errorf("cannot deleted PVCs for pods with label app=%s", podName)
		return false
	}
	inc := 0
	for inc < RetryLoop {
		pvcs, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(listOpts)
		if err == nil {
			if len(pvcs.Items) == 0 {
				return true
			}
		}
		logger.Infof("waiting for PVCs for pods with label=%s  to be deleted.", podName)
		inc++
		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

// WaitUntilNameSpaceIsDeleted waits for namespace to be deleted for 180s.
// If namespace is deleted True is returned, if not false.
func (k8sh *K8sHelper) WaitUntilNameSpaceIsDeleted(namespace string) bool {
	getOpts := metav1.GetOptions{}
	inc := 0
	for inc < RetryLoop {
		ns, err := k8sh.Clientset.CoreV1().Namespaces().Get(namespace, getOpts)
		if err != nil {
			return true
		}
		logger.Infof("Namespace %s %v", namespace, ns.Status.Phase)
		inc++
		time.Sleep(RetryInterval * time.Second)
	}

	return false
}

// CreateExternalRGWService creates a service for rgw access external to the cluster on a node port
func (k8sh *K8sHelper) CreateExternalRGWService(namespace, storeName string) error {
	svcName := "rgw-external-" + storeName
	externalSvc := `apiVersion: v1
kind: Service
metadata:
  name: ` + svcName + `
  namespace: ` + namespace + `
  labels:
    app: rook-ceph-rgw
    rook_cluster: ` + namespace + `
spec:
  ports:
  - name: rook-ceph-rgw
    port: 53390
    protocol: TCP
  selector:
    app: rook-ceph-rgw
    rook_cluster: ` + namespace + `
  sessionAffinity: None
  type: NodePort
`
	_, err := k8sh.KubectlWithStdin(externalSvc, []string{"apply", "-f", "-"}...)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create external service. %+v", err)
	}

	return nil
}

func (k8sh *K8sHelper) GetRGWServiceURL(storeName string, namespace string) (string, error) {
	if k8sh.RunningInCluster {
		return k8sh.GetInternalRGWServiceURL(storeName, namespace)
	}
	return k8sh.GetExternalRGWServiceURL(storeName, namespace)
}

// GetRGWServiceURL returns URL of ceph RGW service in the cluster
func (k8sh *K8sHelper) GetInternalRGWServiceURL(storeName string, namespace string) (string, error) {
	name := "rook-ceph-rgw-" + storeName
	svc, err := k8sh.GetService(name, namespace)
	if err != nil {
		return "", fmt.Errorf("RGW service not found/object. %+v", err)
	}

	endpoint := fmt.Sprintf("%s:%d", svc.Spec.ClusterIP, svc.Spec.Ports[0].Port)
	logger.Infof("internal rgw endpoint: %s", endpoint)
	return endpoint, nil
}

// GetRGWServiceURL returns URL of ceph RGW service in the cluster
func (k8sh *K8sHelper) GetExternalRGWServiceURL(storeName string, namespace string) (string, error) {
	hostip, err := k8sh.GetPodHostID("rook-ceph-rgw", namespace)
	if err != nil {
		return "", fmt.Errorf("RGW pods not found. %+v", err)
	}

	serviceName := "rgw-external-" + storeName
	nodePort, err := k8sh.GetServiceNodePort(serviceName, namespace)
	if err != nil {
		return "", fmt.Errorf("RGW service not found. %+v", err)
	}
	endpoint := hostip + ":" + nodePort
	logger.Infof("external rgw endpoint: %s", endpoint)
	return endpoint, err
}

// ChangeHostnames modifies the node hostname label to run tests in an environment where the node name is different from the hostname label
func (k8sh *K8sHelper) ChangeHostnames() error {
	nodes, err := k8sh.Clientset.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, node := range nodes.Items {
		hostname := node.Labels[v1.LabelHostname]
		if !strings.HasPrefix(hostname, hostnameTestPrefix) {
			node.Labels[v1.LabelHostname] = hostnameTestPrefix + hostname
			logger.Infof("changed hostname of node %s to %s", node.Name, node.Labels[v1.LabelHostname])
			_, err := k8sh.Clientset.CoreV1().Nodes().Update(&node)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// RestoreHostnames removes the test suffix from the node hostname labels
func (k8sh *K8sHelper) RestoreHostnames() ([]string, error) {
	nodes, err := k8sh.Clientset.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, node := range nodes.Items {
		hostname := node.Labels[v1.LabelHostname]
		if strings.HasPrefix(hostname, hostnameTestPrefix) {
			node.Labels[v1.LabelHostname] = hostname[len(hostnameTestPrefix):]
			logger.Infof("restoring hostname of node %s to %s", node.Name, node.Labels[v1.LabelHostname])
			_, err := k8sh.Clientset.CoreV1().Nodes().Update(&node)
			if err != nil {
				return nil, err
			}
		}
	}
	return nil, nil
}

// IsRookInstalled returns true is rook-ceph-mgr service is running(indicating rook is installed)
func (k8sh *K8sHelper) IsRookInstalled(namespace string) bool {
	opts := metav1.GetOptions{}
	_, err := k8sh.Clientset.CoreV1().Services(namespace).Get("rook-ceph-mgr", opts)
	if err == nil {
		return true
	}
	return false
}

// CollectPodLogsFromLabel collects logs for pods with the given label
func (k8sh *K8sHelper) CollectPodLogsFromLabel(podLabel, namespace, testName, platformName string) {
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{LabelSelector: podLabel})
	if err != nil {
		logger.Errorf("failed to list pods in namespace %s. %+v", namespace, err)
		return
	}
	k8sh.getPodsLogs(pods, namespace, testName, platformName)
}

// GetLogsFromNamespace collects logs for all containers in all pods in the namespace
func (k8sh *K8sHelper) GetLogsFromNamespace(namespace, testName, platformName string) {
	logger.Infof("Gathering logs for all pods in namespace %s", namespace)

	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to list pods in namespace %s. %+v", namespace, err)
		return
	}
	k8sh.getPodsLogs(pods, namespace, testName, platformName)
}

func (k8sh *K8sHelper) getPodsLogs(pods *v1.PodList, namespace, testName, platformName string) {
	for _, p := range pods.Items {
		k8sh.getPodLogs(p, platformName, namespace, testName, false)
		if strings.Contains(p.Name, "operator") {
			// get the previous logs for the operator
			k8sh.getPodLogs(p, platformName, namespace, testName, true)
		}
	}
}

func (k8sh *K8sHelper) createTestLogFile(platformName, name, namespace, testName, suffix string) (*os.File, error) {
	dir, _ := os.Getwd()
	logDir := path.Join(dir, "_output/tests/")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err := os.MkdirAll(logDir, 0777)
		if err != nil {
			logger.Errorf("Cannot get logs files dir for app : %v in namespace %v, err: %v", name, namespace, err)
			return nil, err
		}
	}
	fileName := fmt.Sprintf("%s_%s_%s_%s%s_%d.log", testName, platformName, namespace, name, suffix, time.Now().Unix())
	filePath := path.Join(logDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		logger.Errorf("Cannot create file %s. %v", filePath, err)
		return nil, err
	}

	logger.Debugf("created log file: %s", filePath)
	return file, nil
}

func (k8sh *K8sHelper) getPodLogs(pod v1.Pod, platformName, namespace, testName string, previousLog bool) {
	suffix := ""
	if previousLog {
		suffix = "_previous"
	}
	file, err := k8sh.createTestLogFile(platformName, pod.Name, namespace, testName, suffix)
	if err != nil {
		return
	}
	defer file.Close()

	for _, container := range pod.Spec.InitContainers {
		k8sh.appendContainerLogs(file, pod, container.Name, previousLog, true)
	}
	for _, container := range pod.Spec.Containers {
		k8sh.appendContainerLogs(file, pod, container.Name, previousLog, false)
	}
}

func writeHeader(file *os.File, message string) {
	file.WriteString("\n-----------------------------------------\n")
	file.WriteString(message)
	file.WriteString("\n-----------------------------------------\n")
}

func (k8sh *K8sHelper) appendContainerLogs(file *os.File, pod v1.Pod, containerName string, previousLog, initContainer bool) {
	message := fmt.Sprintf("CONTAINER: %s", containerName)
	if initContainer {
		message = "INIT " + message
	}
	writeHeader(file, message)

	logOpts := &v1.PodLogOptions{Previous: previousLog}
	if containerName != "" {
		logOpts.Container = containerName
	}
	res := k8sh.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts).Do()
	rawData, err := res.Raw()
	if err != nil {
		// Sometimes we fail to get logs for pods using this method, notably the operator pod. It is
		// unknown why this happens. Pod logs are VERY important, so try again using kubectl.
		l, err := k8sh.Kubectl("-n", pod.Namespace, "logs", pod.Name, "-c", containerName)
		if err != nil {
			logger.Errorf("Cannot get logs for pod %s and container %s. %v", pod.Name, containerName, err)
			return
		}
		rawData = []byte(l)
	}
	if _, err := file.Write(rawData); err != nil {
		logger.Errorf("Errors while writing logs for pod %s and container %s. %v", pod.Name, containerName, err)
	}
}

// CreateAnonSystemClusterBinding Creates anon-user-access clusterrolebinding for cluster-admin role - used by kubeadm env.
func (k8sh *K8sHelper) CreateAnonSystemClusterBinding() {
	args := []string{"create", "clusterrolebinding", "anon-user-access", "--clusterrole", "cluster-admin", "--user", "system:anonymous"}
	_, err := k8sh.Kubectl(args...)
	if err != nil {
		logger.Warningf("anon-user-access not created")
		return
	}

	logger.Infof("anon-user-access creation completed, waiting for it to exist in API")
	inc := 0
	for inc < RetryLoop {
		var err error
		if _, err = k8sh.Clientset.RbacV1beta1().ClusterRoleBindings().Get("anon-user-access", metav1.GetOptions{}); err == nil {
			break
		}
		logger.Warningf("failed to get anon-user-access clusterrolebinding, will try again: %+v", err)
		inc++
		time.Sleep(RetryInterval * time.Second)
	}
}

func (k8sh *K8sHelper) DeleteRoleAndBindings(name, namespace string) error {
	err := k8sh.DeleteResource("role", name, "-n", namespace)
	if err != nil {
		return err
	}

	err = k8sh.DeleteResource("rolebinding", name, "-n", namespace)
	if err != nil {
		return err
	}

	return nil
}

func (k8sh *K8sHelper) DeleteRoleBinding(name, namespace string) error {
	err := k8sh.DeleteResource("rolebinding", name, "-n", namespace)
	return err
}

func (k8sh *K8sHelper) ScaleStatefulSet(statefulSetName, namespace string, replicationSize int) error {
	args := []string{"-n", namespace, "scale", "statefulsets", statefulSetName, fmt.Sprintf("--replicas=%d", replicationSize)}
	_, err := k8sh.Kubectl(args...)
	return err
}

func IsKubectlErrorNotFound(output string, err error) bool {
	return err != nil && strings.Contains(output, "Error from server (NotFound)")
}

// WaitForDeploymentCount waits until the desired number of deployments with the label exist. The
// deployments are not guaranteed to be running, only existing.
func (k8sh *K8sHelper) WaitForDeploymentCount(label, namespace string, count int) error {
	return k8sh.WaitForDeploymentCountWithRetries(label, namespace, count, RetryLoop)
}

// WaitForDeploymentCountWithRetries waits until the desired number of deployments with the label
// exist, retrying the specified number of times. The deployments are not guaranteed to be running,
// only existing.
func (k8sh *K8sHelper) WaitForDeploymentCountWithRetries(label, namespace string, count, retries int) error {
	options := metav1.ListOptions{LabelSelector: label}
	for i := 0; i < retries; i++ {
		deps, err := k8sh.Clientset.AppsV1().Deployments(namespace).List(options)
		numDeps := 0
		if err == nil {
			numDeps = len(deps.Items)
		}

		if numDeps >= count {
			logger.Infof("found %d of %d deployments with label %s in namespace %s", numDeps, count, label, namespace)
			return nil
		}

		logger.Infof("waiting for %d deployments (found %d) with label %s in namespace %s", count, numDeps, label, namespace)
		time.Sleep(RetryInterval * time.Second)
	}
	return fmt.Errorf("giving up waiting for %d deployments with label %s in namespace %s", count, label, namespace)
}

// WaitForLabeledDeploymentsToBeReady waits for all deployments matching the given label selector to
// be fully ready with a default timeout.
func (k8sh *K8sHelper) WaitForLabeledDeploymentsToBeReady(label, namespace string) error {
	return k8sh.WaitForLabeledDeploymentsToBeReadyWithRetries(label, namespace, RetryLoop)
}

// WaitForLabeledDeploymentsToBeReadyWithRetries waits for all deployments matching the given label
// selector to be fully ready. Retry the number of times given.
func (k8sh *K8sHelper) WaitForLabeledDeploymentsToBeReadyWithRetries(label, namespace string, retries int) error {
	listOpts := metav1.ListOptions{LabelSelector: label}
	var lastDep apps.Deployment
	for i := 0; i < retries; i++ {
		deps, err := k8sh.Clientset.AppsV1().Deployments(namespace).List(listOpts)
		ready := 0
		if err == nil && len(deps.Items) > 0 {
			for _, dep := range deps.Items {
				if dep.Status.Replicas == dep.Status.ReadyReplicas {
					ready++
				} else {
					lastDep = dep // make it the last non-ready dep
				}
				if ready == len(deps.Items) {
					logger.Infof("all %d deployments with label %s are running", len(deps.Items), label)
					return nil
				}
			}
		}
		logger.Infof("waiting for deployment(s) with label %s in namespace %s to be running. ready=%d/%d, err=%+v",
			label, namespace, ready, len(deps.Items), err)
		time.Sleep(RetryInterval * time.Second)
	}
	if len(lastDep.Name) == 0 {
		logger.Infof("no deployment was found with label %s", label)
	} else {
		r, err := k8sh.Kubectl("-n", namespace, "get", "-o", "yaml", "deployments", "--selector", label)
		if err != nil {
			logger.Infof("deployments with label %s:\n%s", label, r)
		}
	}
	return fmt.Errorf("giving up waiting for deployment(s) with label %s in namespace %s to be ready", label, namespace)
}
