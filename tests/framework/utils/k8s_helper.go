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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	bktclient "github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// K8sHelper is a helper for common kubectl commands
type K8sHelper struct {
	executor         *exec.CommandExecutor
	remoteExecutor   *exec.RemotePodCommandExecutor
	Clientset        *kubernetes.Clientset
	RookClientset    *rookclient.Clientset
	BucketClientset  *bktclient.Clientset
	RunningInCluster bool
	T                func() *testing.T
}

const (
	// RetryInterval param for test - wait time while in RetryLoop
	RetryInterval = 5
	// TestMountPath is the path inside a test pod where storage is mounted
	TestMountPath = "/tmp/testrook"
	// hostnameTestPrefix is a prefix added to the node hostname
	hostnameTestPrefix = "test-prefix-this-is-a-very-long-hostname-"
)

// getCmd returns kubectl or oc if env var rook_test_openshift is
// set to true
func getCmd() string {
	cmd := "kubectl"
	if IsPlatformOpenShift() {
		cmd = "oc"
	}
	return cmd
}

// CreateK8sHelper creates a instance of k8sHelper
func CreateK8sHelper(t func() *testing.T) (*K8sHelper, error) {
	executor := &exec.CommandExecutor{}
	config, err := config.GetConfig()
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
	bucketClientset, err := bktclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get lib-bucket-provisioner clientset. %+v", err)
	}

	remoteExecutor := &exec.RemotePodCommandExecutor{
		ClientSet:  clientset,
		RestClient: config,
	}

	h := &K8sHelper{executor: executor, Clientset: clientset, RookClientset: rookClientset, BucketClientset: bucketClientset, T: t, remoteExecutor: remoteExecutor}
	if strings.Contains(config.Host, "//10.") {
		h.RunningInCluster = true
	}
	return h, err
}

var (
	k8slogger = capnslog.NewPackageLogger("github.com/rook/rook", "utils")
	cmd       = getCmd()
	// RetryLoop params for tests.
	RetryLoop = TestRetryNumber()
)

// GetK8sServerVersion returns k8s server version under test
func (k8sh *K8sHelper) GetK8sServerVersion() string {
	versionInfo, err := k8sh.Clientset.ServerVersion()
	require.Nil(k8sh.T(), err)
	return versionInfo.GitVersion
}

func VersionAtLeast(actualVersion, minVersion string) bool {
	v := version.MustParseSemantic(actualVersion)
	return v.AtLeast(version.MustParseSemantic(minVersion))
}

func (k8sh *K8sHelper) VersionAtLeast(minVersion string) bool {
	v := version.MustParseSemantic(k8sh.GetK8sServerVersion())
	return v.AtLeast(version.MustParseSemantic(minVersion))
}

func (k8sh *K8sHelper) MakeContext() *clusterd.Context {
	return &clusterd.Context{Clientset: k8sh.Clientset, RookClientset: k8sh.RookClientset, Executor: k8sh.executor}
}

func (k8sh *K8sHelper) GetDockerImage(image string) error {
	dockercmd := os.Getenv("DOCKERCMD")
	if dockercmd == "" {
		dockercmd = "docker"
	}
	return k8sh.executor.ExecuteCommand(dockercmd, "pull", image)
}

// SetDeploymentVersion sets the container version on the deployment. It is assumed to be the rook/ceph image.
func (k8sh *K8sHelper) SetDeploymentVersion(namespace, deploymentName, containerName, version string) error {
	_, err := k8sh.Kubectl("-n", namespace, "set", "image", "deploy/"+deploymentName, containerName+"=rook/ceph:"+version)
	return err
}

// KubectlWithTimeout is wrapper for executing kubectl commands
func (k8sh *K8sHelper) KubectlWithTimeout(timeout time.Duration, args ...string) (string, error) {
	result, err := k8sh.executor.ExecuteCommandWithTimeout(timeout*time.Second, "kubectl", args...)
	if err != nil {
		k8slogger.Errorf("Failed to execute: %s %+v : %+v. %s", cmd, args, err, result)
		if args[0] == "delete" {
			// allow the tests to continue if we were deleting a resource that timed out
			return result, nil
		}
		return result, fmt.Errorf("Failed to run: %s %v : %v", cmd, args, err)
	}
	return result, nil
}

// Kubectl is wrapper for executing kubectl commands and a timeout of 15 seconds
func (k8sh *K8sHelper) Kubectl(args ...string) (string, error) {
	return k8sh.KubectlWithTimeout(15, args...)
}

// KubectlWithStdin is wrapper for executing kubectl commands in stdin
func (k8sh *K8sHelper) KubectlWithStdin(stdin string, args ...string) (string, error) {
	cmdStruct := CommandArgs{Command: cmd, PipeToStdIn: stdin, CmdArgs: args}
	cmdOut := ExecuteCommand(cmdStruct)

	if cmdOut.ExitCode != 0 {
		k8slogger.Errorf("Failed to execute stdin: %s %v : %v", cmd, args, cmdOut.Err.Error())
		if strings.Contains(cmdOut.Err.Error(), "(NotFound)") || strings.Contains(cmdOut.StdErr, "(NotFound)") {
			return cmdOut.StdErr, kerrors.NewNotFound(schema.GroupResource{}, "")
		}
		return cmdOut.StdErr, fmt.Errorf("Failed to run stdin: %s %v : %v", cmd, args, cmdOut.StdErr)
	}
	if cmdOut.StdOut == "" {
		return cmdOut.StdErr, nil
	}

	return cmdOut.StdOut, nil
}

func getManifestFromURL(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get manifest from url %s", url)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read manifest from url %s", url)
	}
	return string(body), nil
}

// ExecToolboxWithRetry will attempt to run a toolbox command "retries" times, waiting 3s between each call. Upon success, returns the output.
func (k8sh *K8sHelper) ExecToolboxWithRetry(retries int, namespace, command string, commandArgs []string) (string, error) {
	var err error
	var output, stderr string
	cliFinal := append([]string{command}, commandArgs...)
	for i := 0; i < retries; i++ {
		output, stderr, err = k8sh.remoteExecutor.ExecCommandInContainerWithFullOutput(context.TODO(), "rook-ceph-tools", "rook-ceph-tools", namespace, cliFinal...)
		if err == nil {
			return output, nil
		}
		if i < retries-1 {
			logger.Warningf("remote command %v execution failed trying again... %v", cliFinal, kerrors.ReasonForError(err))
			time.Sleep(3 * time.Second)
		}
	}
	return "", fmt.Errorf("remote exec command %v failed on pod in namespace %s. %s. %s. %+v", cliFinal, namespace, output, stderr, err)
}

// ResourceOperation performs a kubectl action on a pod definition
func (k8sh *K8sHelper) ResourceOperation(action string, manifest string) error {
	args := []string{action, "-f", "-"}
	maxManifestCharsToPrint := 4000
	if len(manifest) > maxManifestCharsToPrint {
		logger.Infof("kubectl %s manifest (too long to print)", action)
	} else {
		logger.Infof("kubectl %s manifest:\n%s", action, manifest)
	}
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

// DeleteResource performs a kubectl delete on the given args
func (k8sh *K8sHelper) DeleteResource(args ...string) error {
	return k8sh.DeleteResourceAndWait(true, args...)
}

// WaitForCustomResourceDeletion waits for the CRD deletion
func (k8sh *K8sHelper) WaitForCustomResourceDeletion(namespace, name string, checkerFunc func() error) error {
	// wait for the operator to finalize and delete the CRD
	for i := 0; i < 90; i++ {
		err := checkerFunc()
		if err == nil {
			logger.Infof("custom resource %q in namespace %q still exists", name, namespace)
			time.Sleep(2 * time.Second)
			continue
		}
		if kerrors.IsNotFound(err) {
			logger.Infof("custom resource %q in namespace %s deleted", name, namespace)
			return nil
		}
		return err
	}
	logger.Errorf("gave up deleting custom resource %q ", name)
	return fmt.Errorf("Timed out waiting for deletion of custom resource %q", name)
}

// DeleteResource performs a kubectl delete on give args.
// If wait is false, a flag will be passed to indicate the delete should return immediately
func (k8sh *K8sHelper) DeleteResourceAndWait(wait bool, args ...string) error {
	if !wait {
		args = append(args, "--wait=false")
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
	return result, fmt.Errorf("Could Not get resource in k8s -- %v", err)
}

func (k8sh *K8sHelper) CreateNamespace(namespace string) error {
	ctx := context.TODO()
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err := k8sh.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace %s. %+v", namespace, err)
	}

	return nil
}

func (k8sh *K8sHelper) CountPodsWithLabel(label string, namespace string) (int, error) {
	ctx := context.TODO()
	options := metav1.ListOptions{LabelSelector: label}
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, options)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return len(pods.Items), nil
}

// WaitForPodCount waits until the desired number of pods with the label are started
func (k8sh *K8sHelper) WaitForPodCount(label, namespace string, count int) error {
	options := metav1.ListOptions{LabelSelector: label}
	ctx := context.TODO()
	for i := 0; i < RetryLoop; i++ {
		pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, options)
		if err != nil {
			return fmt.Errorf("failed to find pod with label %s. %+v", label, err)
		}

		if len(pods.Items) >= count {
			logger.Infof("found %d pods with label %s", count, label)
			for _, pod := range pods.Items {
				logger.Infof("pod found includes %q with status %q", pod.Name, pod.Status.Phase)
			}
			return nil
		}

		logger.Infof("waiting for %d pods (found %d) with label %s in namespace %s", count, len(pods.Items), label, namespace)
		time.Sleep(RetryInterval * time.Second)
	}
	return fmt.Errorf("Giving up waiting for pods with label %s in namespace %s", label, namespace)
}

func (k8sh *K8sHelper) WaitForStatusPhase(namespace, kind, name, desiredPhase string, timeout time.Duration) error {
	baseErr := fmt.Sprintf("waiting for resource %q %q in namespace %q to have status.phase %q", kind, name, namespace, desiredPhase)
	err := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, timeout, true, func(context context.Context) (done bool, err error) {
		phase, err := k8sh.GetResource("--namespace", namespace, kind, name, "--output", "jsonpath={.status.phase}")
		if err != nil {
			logger.Warningf("error %s. %v", baseErr, err)
		}

		if phase == desiredPhase {
			return true, nil
		}

		logger.Infof("%s", baseErr)
		return false, nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed %s", baseErr)
	}

	return nil
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
	ctx := context.TODO()
	var lastPod v1.Pod
	for i := 0; i < retries; i++ {
		pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, options)
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
	ctx := context.TODO()
	for i := 0; i < RetryLoop; i++ {
		pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, options)
		if kerrors.IsNotFound(err) {
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
	ctx := context.TODO()
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to get pod status in namespace %s. %+v", namespace, err)
		return
	}
	for _, pod := range pods.Items {
		logger.Infof("%s (%s) pod status: %+v", pod.Name, namespace, pod.Status)
	}
}

func (k8sh *K8sHelper) GetPodRestartsFromNamespace(namespace, testName, platformName string) {
	logger.Infof("will alert if any pods were restarted in namespace %s", namespace)
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to list pods in namespace %s. %+v", namespace, err)
		return
	}
	for _, pod := range pods.Items {
		podName := pod.Name
		for _, status := range pod.Status.ContainerStatuses {
			if strings.Contains(podName, status.Name) {
				if status.RestartCount > int32(0) {
					logger.Infof("number of time pod %s has restarted is %d", podName, status.RestartCount)
				}

				// Skipping `mgr` pod count to get the CI green and seems like this is related to ceph Reef.
				// Refer to this issue https://github.com/rook/rook/issues/12646 and remove once it is fixed.
				if !strings.Contains(podName, "rook-ceph-mgr") && status.RestartCount == int32(1) {
					assert.Equal(k8sh.T(), int32(0), status.RestartCount)
				}
			}
		}
	}
}

func (k8sh *K8sHelper) GetPodDescribeFromNamespace(namespace, testName, platformName string) {
	ctx := context.TODO()
	logger.Infof("Gathering pod describe for all pods in namespace %s", namespace)
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
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

func (k8sh *K8sHelper) GetEventsFromNamespace(namespace, testName, platformName string) {
	logger.Infof("Gathering events in namespace %q", namespace)

	file, err := k8sh.createTestLogFile(platformName, "events", namespace, testName, "")
	if err != nil {
		logger.Errorf("failed to create event file. %v", err)
		return
	}
	defer file.Close()

	args := []string{"get", "events", "-n", namespace}
	events, err := k8sh.Kubectl(args...)
	if err != nil {
		logger.Errorf("failed to get events. %v. %v", args, err)
	}
	if events == "" {
		return
	}
	file.WriteString(events) //nolint // ok to ignore this test logging
}

func (k8sh *K8sHelper) appendPodDescribe(file *os.File, namespace, name string) {
	description := k8sh.getPodDescribe(namespace, name)
	if description == "" {
		return
	}
	writeHeader(file, fmt.Sprintf("Pod: %s\n", name)) //nolint // ok to ignore this test logging
	file.WriteString(description)                     //nolint // ok to ignore this test logging
	file.WriteString("\n")                            //nolint // ok to ignore this test logging
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

// IsPodRunning returns true if a Pod is running status or goes to Running status within 90s else returns false
func (k8sh *K8sHelper) IsPodRunning(name string, namespace string) bool {
	ctx := context.TODO()
	getOpts := metav1.GetOptions{}
	for i := 0; i < 60; i++ {
		pod, err := k8sh.Clientset.CoreV1().Pods(namespace).Get(ctx, name, getOpts)
		if err == nil {
			if pod.Status.Phase == "Running" {
				return true
			}
		}
		time.Sleep(RetryInterval * time.Second)
		logger.Infof("waiting for pod %s in namespace %s to be running", name, namespace)
	}
	pod, _ := k8sh.Clientset.CoreV1().Pods(namespace).Get(ctx, name, getOpts)
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
	ctx := context.TODO()
	for i := 0; i < RetryLoop; i++ {
		pod, err := k8sh.Clientset.CoreV1().Pods(namespace).Get(ctx, name, getOpts)
		if err != nil {
			k8slogger.Infof("Pod  %s in namespace %s terminated ", name, namespace)
			return true
		}
		k8slogger.Infof("waiting for Pod %s in namespace %s to terminate, status : %+v", name, namespace, pod.Status)
		time.Sleep(RetryInterval * time.Second)

	}
	k8slogger.Infof("Pod %s in namespace %s did not terminate", name, namespace)
	return false
}

// IsServiceUp returns true if a service is up or comes up within 150s, else returns false
func (k8sh *K8sHelper) IsServiceUp(name string, namespace string) bool {
	getOpts := metav1.GetOptions{}
	ctx := context.TODO()

	for i := 0; i < RetryLoop; i++ {
		_, err := k8sh.Clientset.CoreV1().Services(namespace).Get(ctx, name, getOpts)
		if err == nil {
			k8slogger.Infof("Service: %s in namespace: %s is up", name, namespace)
			return true
		}
		k8slogger.Infof("waiting for Service %s in namespace %s ", name, namespace)
		time.Sleep(RetryInterval * time.Second)

	}
	k8slogger.Infof("Giving up waiting for service: %s in namespace %s ", name, namespace)
	return false
}

// GetService returns output from "kubectl get svc $NAME" command
func (k8sh *K8sHelper) GetService(servicename string, namespace string) (*v1.Service, error) {
	getOpts := metav1.GetOptions{}
	ctx := context.TODO()
	result, err := k8sh.Clientset.CoreV1().Services(namespace).Get(ctx, servicename, getOpts)
	if err != nil {
		return nil, fmt.Errorf("Cannot find service %s in namespace %s, err-- %v", servicename, namespace, err)
	}
	return result, nil
}

// IsCRDPresent returns true if custom resource definition is present
func (k8sh *K8sHelper) IsCRDPresent(crdName string) bool {
	cmdArgs := []string{"get", "crd", crdName}

	for i := 0; i < RetryLoop; i++ {
		_, err := k8sh.Kubectl(cmdArgs...)
		if err == nil {
			k8slogger.Infof("Found the CRD resource: %s", crdName)
			return true
		}
		time.Sleep(RetryInterval * time.Second)

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

// RunCommandInPod runs the provided command inside the pod
func (k8sh *K8sHelper) RunCommandInPod(namespace, name, cmd string) (string, error) {
	args := []string{"exec", name}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, "--", "sh", "-c", cmd)
	resp, err := k8sh.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("failed to execute command %q in pod %s. %+v", cmd, name, err)
	}
	return resp, err
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
	ctx := context.TODO()
	getOpts := metav1.GetOptions{}
	pvc, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, getOpts)
	if err != nil {
		return "", err
	}
	return pvc.Spec.VolumeName, nil
}

func (k8sh *K8sHelper) PrintPVs(detailed bool) {
	ctx := context.TODO()
	pvs, err := k8sh.Clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
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
	ctx := context.TODO()
	pvcs, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
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

func (k8sh *K8sHelper) PrintResources(namespace, name string) {
	args := []string{"-n", namespace, "get", name, "-o", "yaml"}
	result, err := k8sh.Kubectl(args...)
	if err != nil {
		logger.Warningf("failed to get resource %s. %v", name, err)
	} else {
		logger.Infof("%s\n", result)
	}
}

func (k8sh *K8sHelper) PrintStorageClasses(detailed bool) {
	ctx := context.TODO()
	scs, err := k8sh.Clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
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

func (k8sh *K8sHelper) GetPodNamesForApp(appName, namespace string) ([]string, error) {
	args := []string{
		"get", "pod", "-n", namespace, "-l", fmt.Sprintf("app=%s", appName),
		"-o", "jsonpath={.items[*].metadata.name}",
	}
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
	ctx := context.TODO()
	uri := fmt.Sprintf("api/v1/namespaces/%s/events?fieldSelector=involvedObject.name=%s,involvedObject.namespace=%s", namespace, podNamePattern, namespace)
	result, err := k8sh.Clientset.CoreV1().RESTClient().Get().RequestURI(uri).DoRaw(ctx)
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
	for i := 0; i < RetryLoop; i++ {
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

	}
	k8slogger.Infof("Pod %s in namespace %s did not error with reason %s", podNamePattern, namespace, reason)
	return false
}

// GetPodHostIP returns HostIP address of a pod
func (k8sh *K8sHelper) GetPodHostIP(podNamePattern string, namespace string) (string, error) {
	ctx := context.TODO()
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podNamePattern}
	podList, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, listOpts)
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
	ctx := context.TODO()
	getOpts := metav1.GetOptions{}
	svc, err := k8sh.Clientset.CoreV1().Services(namespace).Get(ctx, serviceName, getOpts)
	if err != nil {
		logger.Errorf("Cannot get service : %v in namespace %v, err: %v", serviceName, namespace, err)
		return "", fmt.Errorf("Cannot get service : %v in namespace %v, err: %v", serviceName, namespace, err)
	}
	np := svc.Spec.Ports[0].NodePort
	return strconv.FormatInt(int64(np), 10), nil
}

// IsStorageClassPresent returns true if storageClass is present, if not false
func (k8sh *K8sHelper) IsStorageClassPresent(name string) (bool, error) {
	args := []string{"get", "storageclass", "-o", "jsonpath='{.items[*].metadata.name}'"}
	result, err := k8sh.Kubectl(args...)
	if strings.Contains(result, name) {
		return true, nil
	}
	return false, fmt.Errorf("Storageclass %s not found, err ->%v", name, err)
}

func (k8sh *K8sHelper) IsDefaultStorageClassPresent() (bool, error) {
	ctx := context.TODO()
	scs, err := k8sh.Clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list StorageClasses: %+v", err)
	}

	for _, sc := range scs.Items {
		if isDefaultAnnotation(sc.ObjectMeta) {
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
	ctx := context.TODO()

	actualPvcCount := 0

	for i := 0; i < RetryLoop; i++ {
		pvcList, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, listOpts)
		if err != nil {
			logger.Errorf("Cannot get pvc for app : %v in namespace %v, err: %v", podName, namespace, err)
			return false
		}
		actualPvcCount = len(pvcList.Items)
		if actualPvcCount == expectedPvcCount {
			pvcCountCheck = true
			break
		}

		time.Sleep(RetryInterval * time.Second)
	}

	if !pvcCountCheck {
		logger.Errorf("Expecting %d number of PVCs for %s app, found %d ", expectedPvcCount, podName, actualPvcCount)
		return false
	}

	for i := 0; i < RetryLoop; i++ {
		checkAllPVCsStatus := true
		pl, _ := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, listOpts)
		for _, pvc := range pl.Items {
			if !(pvc.Status.Phase == v1.PersistentVolumeClaimPhase(expectedStatus)) {
				checkAllPVCsStatus = false
				logger.Infof("waiting for pvc %v to be in %s Phase, currently in %v Phase", pvc.Name, expectedStatus, pvc.Status.Phase)
			}
		}
		if checkAllPVCsStatus {
			return true
		}

		time.Sleep(RetryInterval * time.Second)

	}
	logger.Errorf("Giving up waiting for %d PVCs for %s app to be in %s phase", expectedPvcCount, podName, expectedStatus)
	return false
}

// GetPVCStatus returns status of PVC
func (k8sh *K8sHelper) GetPVCStatus(namespace string, name string) (v1.PersistentVolumeClaimPhase, error) {
	getOpts := metav1.GetOptions{}
	ctx := context.TODO()
	pvc, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, getOpts)
	if err != nil {
		return v1.ClaimLost, fmt.Errorf("PVC %s not found,err->%v", name, err)
	}

	return pvc.Status.Phase, nil
}

// GetPVCVolumeName returns volume name of PVC
func (k8sh *K8sHelper) GetPVCVolumeName(namespace string, name string) (string, error) {
	getOpts := metav1.GetOptions{}
	ctx := context.TODO()
	pvc, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, getOpts)
	if err != nil {
		return "", fmt.Errorf("PVC %s not found,err->%v", name, err)
	}

	return pvc.Spec.VolumeName, nil
}

// GetPVCAccessModes returns AccessModes on PVC
func (k8sh *K8sHelper) GetPVCAccessModes(namespace string, name string) ([]v1.PersistentVolumeAccessMode, error) {
	getOpts := metav1.GetOptions{}
	ctx := context.TODO()
	pvc, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, getOpts)
	if err != nil {
		return []v1.PersistentVolumeAccessMode{}, fmt.Errorf("PVC %s not found,err->%v", name, err)
	}

	return pvc.Status.AccessModes, nil
}

// GetPV returns PV by name
func (k8sh *K8sHelper) GetPV(name string) (*v1.PersistentVolume, error) {
	getOpts := metav1.GetOptions{}
	ctx := context.TODO()
	pv, err := k8sh.Clientset.CoreV1().PersistentVolumes().Get(ctx, name, getOpts)
	if err != nil {
		return nil, fmt.Errorf("PV %s not found,err->%v", name, err)
	}
	return pv, nil
}

// IsPodInExpectedState waits for 90s for a pod to be an expected state
// If the pod is in expected state within 90s true is returned,  if not false
func (k8sh *K8sHelper) IsPodInExpectedState(podNamePattern string, namespace string, state string) bool {
	return k8sh.IsPodInExpectedStateWithLabel("app="+podNamePattern, namespace, state)
}

func (k8sh *K8sHelper) IsPodInExpectedStateWithLabel(label, namespace, state string) bool {
	listOpts := metav1.ListOptions{LabelSelector: label}
	ctx := context.TODO()
	for i := 0; i < RetryLoop; i++ {
		podList, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, listOpts)
		if err == nil {
			for _, pod := range podList.Items {
				if pod.Status.Phase == v1.PodPhase(state) {
					return true
				}
			}
		}

		logger.Infof("waiting for pod with label %s in namespace %q to be in state %q...", label, namespace, state)
		time.Sleep(RetryInterval * time.Second)
	}

	return false
}

// CheckPodCountAndState returns true if expected number of pods with matching name are found and are in expected state
func (k8sh *K8sHelper) CheckPodCountAndState(podName string, namespace string, minExpected int, expectedPhase string) bool {
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podName}
	podCountCheck := false
	actualPodCount := 0
	ctx := context.TODO()
	for i := 0; i < RetryLoop; i++ {
		podList, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, listOpts)
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

		logger.Infof("waiting for %d pods with label app=%s, found %d", minExpected, podName, actualPodCount)
		time.Sleep(RetryInterval * time.Second)
	}
	if !podCountCheck {
		logger.Errorf("Expecting %d number of pods for %s app, found %d ", minExpected, podName, actualPodCount)
		return false
	}

	for i := 0; i < RetryLoop; i++ {
		checkAllPodsStatus := true
		pl, _ := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, listOpts)
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
	for i := 0; i < RetryLoop; i++ {
		out, _ := k8sh.GetResource("-n", namespace, "pods", "-l", "app="+podNamePattern)
		if !strings.Contains(out, podNamePattern) {
			return true
		}

		time.Sleep(RetryInterval * time.Second)
	}
	logger.Infof("Pod %s in namespace %s not deleted", podNamePattern, namespace)
	return false
}

// WaitUntilPodIsDeleted waits for 90s for a pod to be terminated
// If the pod disappears within 90s true is returned,  if not false
func (k8sh *K8sHelper) WaitUntilPodIsDeleted(name, namespace string) bool {
	ctx := context.TODO()
	for i := 0; i < RetryLoop; i++ {
		_, err := k8sh.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil && kerrors.IsNotFound(err) {
			return true
		}

		logger.Infof("pod %s in namespace %s is not deleted yet", name, namespace)
		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

// WaitUntilPVCIsBound waits for a PVC to be in bound state for 90 seconds
// if PVC goes to Bound state within 90s True is returned, if not false
func (k8sh *K8sHelper) WaitUntilPVCIsBound(namespace string, pvcname string) bool {
	for i := 0; i < RetryLoop; i++ {
		out, err := k8sh.GetPVCStatus(namespace, pvcname)
		if err == nil {
			if out == v1.PersistentVolumeClaimPhase(v1.ClaimBound) {
				logger.Infof("PVC %s is bound", pvcname)
				return true
			}
		}
		logger.Infof("waiting for PVC %s to be bound. current=%s. err=%+v", pvcname, out, err)

		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

// WaitUntilPVCIsExpanded waits for a PVC to be resized for specified value
func (k8sh *K8sHelper) WaitUntilPVCIsExpanded(namespace, pvcname, size string) bool {
	getOpts := metav1.GetOptions{}
	ctx := context.TODO()
	for i := 0; i < RetryLoop; i++ {
		// PVC specs changes immediately, but status will change only if resize process is successfully completed.
		pvc, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcname, getOpts)
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

		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

func (k8sh *K8sHelper) WaitUntilPVCIsDeleted(namespace string, pvcname string) bool {
	getOpts := metav1.GetOptions{}
	ctx := context.TODO()
	for i := 0; i < RetryLoop; i++ {
		_, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcname, getOpts)
		if err != nil && kerrors.IsNotFound(err) {
			return true
		}
		logger.Infof("waiting for PVC %s to be deleted.", pvcname)

		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

func (k8sh *K8sHelper) WaitUntilZeroPVs() bool {
	ListOpts := metav1.ListOptions{}
	ctx := context.TODO()
	for i := 0; i < RetryLoop; i++ {
		pvList, err := k8sh.Clientset.CoreV1().PersistentVolumes().List(ctx, ListOpts)
		if err != nil && kerrors.IsNotFound(err) {
			return true
		}
		if len(pvList.Items) == 0 {
			return true
		}
		logger.Infof("waiting for PV count to be zero.")

		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

func (k8sh *K8sHelper) DeletePvcWithLabel(namespace string, podName string) bool {
	delOpts := metav1.DeleteOptions{}
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podName}
	ctx := context.TODO()
	err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).DeleteCollection(ctx, delOpts, listOpts)
	if err != nil {
		logger.Errorf("cannot deleted PVCs for pods with label app=%s", podName)
		return false
	}

	for i := 0; i < RetryLoop; i++ {
		pvcs, err := k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, listOpts)
		if err == nil {
			if len(pvcs.Items) == 0 {
				return true
			}
		}
		logger.Infof("waiting for PVCs for pods with label=%s  to be deleted.", podName)

		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

// WaitUntilNameSpaceIsDeleted waits for namespace to be deleted for 180s.
// If namespace is deleted True is returned, if not false.
func (k8sh *K8sHelper) WaitUntilNameSpaceIsDeleted(namespace string) bool {
	getOpts := metav1.GetOptions{}
	ctx := context.TODO()
	for i := 0; i < RetryLoop; i++ {
		ns, err := k8sh.Clientset.CoreV1().Namespaces().Get(ctx, namespace, getOpts)
		if err != nil {
			return true
		}
		logger.Infof("Namespace %s %v", namespace, ns.Status.Phase)

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
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create external service. %+v", err)
	}

	return nil
}

func (k8sh *K8sHelper) GetRGWServiceURL(storeName string, namespace string) (string, error) {
	if k8sh.RunningInCluster {
		return k8sh.getInternalRGWServiceURL(storeName, namespace)
	}
	return k8sh.getExternalRGWServiceURL(storeName, namespace)
}

// GetRGWServiceURL returns URL of ceph RGW service in the cluster
func (k8sh *K8sHelper) getInternalRGWServiceURL(storeName string, namespace string) (string, error) {
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
func (k8sh *K8sHelper) getExternalRGWServiceURL(storeName string, namespace string) (string, error) {
	hostip, err := k8sh.GetPodHostIP("rook-ceph-rgw", namespace)
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
	ctx := context.TODO()
	nodes, err := k8sh.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, node := range nodes.Items {
		hostname := node.Labels[v1.LabelHostname]
		if !strings.HasPrefix(hostname, hostnameTestPrefix) {
			node.Labels[v1.LabelHostname] = hostnameTestPrefix + hostname
			logger.Infof("changed hostname of node %s to %s", node.Name, node.Labels[v1.LabelHostname])
			_, err := k8sh.Clientset.CoreV1().Nodes().Update(ctx, &node, metav1.UpdateOptions{}) //nolint:gosec // We safely suppress gosec in tests file
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// RestoreHostnames removes the test suffix from the node hostname labels
func (k8sh *K8sHelper) RestoreHostnames() ([]string, error) {
	ctx := context.TODO()
	nodes, err := k8sh.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, node := range nodes.Items {
		hostname := node.Labels[v1.LabelHostname]
		if strings.HasPrefix(hostname, hostnameTestPrefix) {
			node.Labels[v1.LabelHostname] = hostname[len(hostnameTestPrefix):]
			logger.Infof("restoring hostname of node %s to %s", node.Name, node.Labels[v1.LabelHostname])
			_, err := k8sh.Clientset.CoreV1().Nodes().Update(ctx, &node, metav1.UpdateOptions{}) //nolint:gosec // We safely suppress gosec in tests file
			if err != nil {
				return nil, err
			}
		}
	}
	return nil, nil
}

// IsRookInstalled returns true is rook-ceph-mgr service is running(indicating rook is installed)
func (k8sh *K8sHelper) IsRookInstalled(namespace string) bool {
	ctx := context.TODO()
	opts := metav1.GetOptions{}
	_, err := k8sh.Clientset.CoreV1().Services(namespace).Get(ctx, "rook-ceph-mgr", opts)
	return err == nil
}

// CollectPodLogsFromLabel collects logs for pods with the given label
func (k8sh *K8sHelper) CollectPodLogsFromLabel(podLabel, namespace, testName, platformName string) {
	ctx := context.TODO()
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: podLabel})
	if err != nil {
		logger.Errorf("failed to list pods in namespace %s. %+v", namespace, err)
		return
	}
	k8sh.getPodsLogs(pods, namespace, testName, platformName)
}

// GetLogsFromNamespace collects logs for all containers in all pods in the namespace
func (k8sh *K8sHelper) GetLogsFromNamespace(namespace, testName, platformName string) {
	ctx := context.TODO()
	logger.Infof("Gathering logs for all pods in namespace %s", namespace)
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
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
		err := os.MkdirAll(logDir, 0o777)
		if err != nil {
			logger.Errorf("Cannot get logs files dir for app : %v in namespace %v, err: %v", name, namespace, err)
			return nil, err
		}
	}
	fileName := fmt.Sprintf("%s_%s_%s_%s%s_%d.log", testName, platformName, namespace, name, suffix, time.Now().Unix())
	filePath := path.Join(logDir, strings.ReplaceAll(fileName, "/", "_"))
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

func writeHeader(file *os.File, message string) error {
	file.WriteString("\n-----------------------------------------\n") //nolint // ok to ignore this test logging
	file.WriteString(message)                                         //nolint // ok to ignore this test logging
	file.WriteString("\n-----------------------------------------\n") //nolint // ok to ignore this test logging

	return nil
}

func (k8sh *K8sHelper) appendContainerLogs(file *os.File, pod v1.Pod, containerName string, previousLog, initContainer bool) {
	message := fmt.Sprintf("CONTAINER: %s", containerName)
	if initContainer {
		message = "INIT " + message
	}
	writeHeader(file, message) //nolint // ok to ignore this test logging
	ctx := context.TODO()
	logOpts := &v1.PodLogOptions{Previous: previousLog}
	if containerName != "" {
		logOpts.Container = containerName
	}
	res := k8sh.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts).Do(ctx)
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
	ctx := context.TODO()
	_, err := k8sh.Clientset.RbacV1beta1().ClusterRoleBindings().Get(ctx, "anon-user-access", metav1.GetOptions{})
	if err != nil {
		logger.Warningf("anon-user-access clusterrolebinding not found. %v", err)
		args := []string{"create", "clusterrolebinding", "anon-user-access", "--clusterrole", "cluster-admin", "--user", "system:anonymous"}
		_, err := k8sh.Kubectl(args...)
		if err != nil {
			logger.Errorf("failed to create anon-user-access. %v", err)
			return
		}
		logger.Info("anon-user-access creation completed, waiting for it to exist in API")
	}

	for i := 0; i < RetryLoop; i++ {
		var err error
		if _, err = k8sh.Clientset.RbacV1().ClusterRoleBindings().Get(ctx, "anon-user-access", metav1.GetOptions{}); err == nil {
			break
		}
		logger.Warningf("failed to get anon-user-access clusterrolebinding, will try again: %+v", err)

		time.Sleep(RetryInterval * time.Second)
	}
}

func IsKubectlErrorNotFound(output string, err error) bool {
	return err != nil && strings.Contains(output, "Error from server (NotFound)")
}

// WaitForDeploymentCount waits until the desired number of deployments with the label exist. The
// deployments are not guaranteed to be running, only existing.
func (k8sh *K8sHelper) WaitForDeploymentCount(label, namespace string, count int) error {
	return k8sh.waitForDeploymentCountWithRetries(label, namespace, count, RetryLoop)
}

// WaitForDeploymentCountWithRetries waits until the desired number of deployments with the label
// exist, retrying the specified number of times. The deployments are not guaranteed to be running,
// only existing.
func (k8sh *K8sHelper) waitForDeploymentCountWithRetries(label, namespace string, count, retries int) error {
	ctx := context.TODO()
	options := metav1.ListOptions{LabelSelector: label}
	for i := 0; i < retries; i++ {
		deps, err := k8sh.Clientset.AppsV1().Deployments(namespace).List(ctx, options)
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
	ctx := context.TODO()
	var lastDep apps.Deployment
	for i := 0; i < retries; i++ {
		deps, err := k8sh.Clientset.AppsV1().Deployments(namespace).List(ctx, listOpts)
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

func (k8sh *K8sHelper) WaitForCronJob(name, namespace string) error {
	for i := 0; i < RetryLoop; i++ {
		_, err := k8sh.Clientset.BatchV1().CronJobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Infof("waiting for CronJob named %s in namespace %s", name, namespace)
				time.Sleep(RetryInterval * time.Second)
				continue
			}

			return fmt.Errorf("failed to find CronJob named %s. %+v", name, err)
		}

		logger.Infof("found CronJob with name %s in namespace %s", name, namespace)
		return nil
	}
	return fmt.Errorf("giving up waiting for CronJob named %s in namespace %s", name, namespace)
}

func (k8sh *K8sHelper) GetResourceStatus(kind, name, namespace string) (string, error) {
	return k8sh.Kubectl("-n", namespace, "get", kind, name) // TODO: -o status
}

func (k8sh *K8sHelper) WaitUntilResourceIsDeleted(kind, namespace, name string) error {
	var err error
	var out string
	for i := 0; i < RetryLoop; i++ {
		out, err = k8sh.Kubectl("-n", namespace, "get", kind, name, "-o", "name")
		if strings.Contains(out, "Error from server (NotFound): ") {
			return nil
		}
		logger.Infof("waiting %d more seconds for resource %s %q to be deleted:\n%s", RetryInterval, kind, name, out)
		time.Sleep(RetryInterval * time.Second)
	}
	return errors.Wrapf(err, "timed out waiting for resource %s %q to be deleted:\n%s", kind, name, out)
}
