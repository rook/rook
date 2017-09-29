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
	"strings"
	"time"

	"strconv"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/util/exec"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

//K8sHelper is a helper for common kubectl commads
type K8sHelper struct {
	executor         *exec.CommandExecutor
	Clientset        *kubernetes.Clientset
	RunningInCluster bool
}

const (
	//RetryLoop params for tests.
	RetryLoop = 30
	//RetryInterval param for test - wait time while in RetryLoop
	RetryInterval = 5
)

//CreateK8sHelper creates a instance of k8sHelper
func CreateK8sHelper() (*K8sHelper, error) {
	executor := &exec.CommandExecutor{}
	config, err := getKubeConfig(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to get kube client. %+v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset. %+v", err)
	}

	h := &K8sHelper{executor: executor, Clientset: clientset}
	if strings.Index(config.Host, "//10.") != -1 {
		h.RunningInCluster = true
	}
	return h, err
}

var k8slogger = capnslog.NewPackageLogger("github.com/rook/rook", "utils")

//GetK8sServerVersion returns k8s server version under test
func (k8sh *K8sHelper) GetK8sServerVersion() string {
	versionInfo, _ := k8sh.Clientset.ServerVersion()
	return versionInfo.GitVersion
}

//Kubectl is wrapper for executing kubectl commands
func (k8sh *K8sHelper) Kubectl(args ...string) (string, error) {
	result, err := k8sh.executor.ExecuteCommandWithOutput(false, "", "kubectl", args...)
	if err != nil {
		k8slogger.Errorf("Errors Encountered while executing kubectl command : %v", err)
		return "", fmt.Errorf("Failed to run kubectl commands on args %v : %v", args, err)

	}
	return result, nil

}

//KubectlWithStdin is wrapper for executing kubectl commands in stdin
func (k8sh *K8sHelper) KubectlWithStdin(stdin string, args ...string) (string, error) {

	cmdStruct := CommandArgs{Command: "kubectl", PipeToStdIn: stdin, CmdArgs: args}

	cmdOut := ExecuteCommand(cmdStruct)

	if cmdOut.ExitCode != 0 {
		k8slogger.Errorf("Errors Encountered while executing kubectl command : %v", cmdOut.Err.Error())
		return cmdOut.StdErr, fmt.Errorf("Failed to run kubectl commands on args %v : %v", args, cmdOut.StdErr)
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
		//set Insecure to true if cert information is missing
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

//ResourceOperationFromTemplate performs a kubectl action from a template file after replacing its context
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
	return "", fmt.Errorf("Could Not create resource in args : %v  %v-- %v", args, podDef, err)

}

//ResourceOperation performs a kubectl action on a pod definition
func (k8sh *K8sHelper) ResourceOperation(action string, podDefiniton string) (string, error) {

	args := []string{action, "-f", "-"}
	result, err := k8sh.KubectlWithStdin(podDefiniton, args...)
	if err == nil {
		return result, nil
	}
	logger.Errorf("Failed to execute kubectl %v -- %v", args, err)
	return "", fmt.Errorf("Could Not create resource in args : %v -- %v", args, err)

}

//DeleteResource performs a kubectl delete on give args
func (k8sh *K8sHelper) DeleteResource(args []string) (string, error) {
	args = append([]string{"delete"}, args...)
	result, err := k8sh.Kubectl(args...)
	if err == nil {
		return result, nil
	}
	return "", fmt.Errorf("Could Not delete resource in k8s -- %v", err)

}

//GetResource performs a kubectl get on give args
func (k8sh *K8sHelper) GetResource(args []string) (string, error) {
	args = append([]string{"get"}, args...)
	result, err := k8sh.Kubectl(args...)
	if err == nil {
		return result, nil
	}
	return "", fmt.Errorf("Could Not get resource in k8s -- %v", err)

}

//GetMonitorServices returns all ceph mon pod names
func (k8sh *K8sHelper) GetMonitorServices(namespace string) (map[string]string, error) {
	listOpts := metav1.ListOptions{LabelSelector: "app=rook-ceph-mon"}

	podList, err := k8sh.Clientset.Services(namespace).List(listOpts)
	if err != nil {
		logger.Errorf("Cannot get rook monitor pods in namespace %s, err: %v", namespace, err)
		return nil, fmt.Errorf("Cannot get rook monitor pods in namespace %s, err: %v", namespace, err)
	}
	mons := []string{}
	for _, svc := range podList.Items {
		mons = append(mons, svc.Spec.ClusterIP)

	}
	if len(mons) != 3 {
		return nil, fmt.Errorf("Unexpected monitors: %+v", mons)
	}

	return map[string]string{
		"mon0": fmt.Sprintf("%s:6790", mons[0]),
		"mon1": fmt.Sprintf("%s:6790", mons[1]),
		"mon2": fmt.Sprintf("%s:6790", mons[2]),
	}, nil
}

//IsPodWithLabelRunning returns true if a Pod is running status or goes to Running status within 90s else returns false
func (k8sh *K8sHelper) IsPodWithLabelRunning(label string, namespace string) bool {
	options := metav1.ListOptions{LabelSelector: label}
	inc := 0
	for inc < RetryLoop {
		pods, err := k8sh.Clientset.Pods(namespace).List(options)
		if err != nil {
			logger.Errorf("failed to find pod with label %s. %+v", label, err)
			return false
		}

		if len(pods.Items) > 0 {
			for _, pod := range pods.Items {
				if pod.Status.Phase == "Running" {
					return true
				}
			}
		}
		inc++
		time.Sleep(RetryInterval * time.Second)
		logger.Infof("waiting for pod with label %s in namespace %s to be running", label, namespace)

	}
	logger.Infof("Giving up waiting for pod with label %s in namespace %s to be running", label, namespace)
	return false
}

//IsPodRunning returns true if a Pod is running status or goes to Running status within 90s else returns false
func (k8sh *K8sHelper) IsPodRunning(name string, namespace string) bool {
	getOpts := metav1.GetOptions{}
	inc := 0
	for inc < RetryLoop {
		pod, err := k8sh.Clientset.Pods(namespace).Get(name, getOpts)
		if err == nil {
			if pod.Status.Phase == "Running" {
				return true
			}
		}
		inc++
		time.Sleep(RetryInterval * time.Second)
		logger.Infof("waiting for pod %s in namespace %s to be running", name, namespace)

	}
	logger.Infof("Giving up waiting for pod %s in namespace %s to be running", name, namespace)
	return false
}

//IsPodTerminated returns true if a Pod is terminated status or goes to Terminated  status
// within 90s else returns false\
func (k8sh *K8sHelper) IsPodTerminated(name string, namespace string) bool {
	getOpts := metav1.GetOptions{}
	inc := 0
	for inc < RetryLoop {
		pod, err := k8sh.Clientset.Pods(namespace).Get(name, getOpts)
		if err != nil {
			k8slogger.Infof("Pod  %s in namespace %s terminated ", name, namespace)
			return true
		}
		k8slogger.Infof("waiting for Pod %s in namespace %s to terminated, status : %v", name, namespace, pod.Status.Phase)
		time.Sleep(RetryInterval * time.Second)
		inc++

	}
	k8slogger.Infof("Pod %s in namespace %s did not terminated", name, namespace)
	return false
}

//IsServiceUp returns true if a service is up or comes up within 150s, else returns false
func (k8sh *K8sHelper) IsServiceUp(name string, namespace string) bool {
	getOpts := metav1.GetOptions{}
	inc := 0
	for inc < RetryLoop {
		_, err := k8sh.Clientset.Services(namespace).Get(name, getOpts)
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

//GetService returns output from "kubectl get svc $NAME" command
func (k8sh *K8sHelper) GetService(servicename string, namespace string) (*v1.Service, error) {
	getOpts := metav1.GetOptions{}
	result, err := k8sh.Clientset.Services(namespace).Get(servicename, getOpts)
	if err != nil {
		return nil, fmt.Errorf("Cannot find service %s in namespace %s, err-- %v", servicename, namespace, err)
	}
	return result, nil
}

//IsThirdPartyResourcePresent returns true if Third party resource is present
func (k8sh *K8sHelper) IsThirdPartyResourcePresent(tprname string) bool {
	args := []string{"get", "thirdpartyresources", tprname}
	inc := 0
	for inc < RetryLoop {
		_, err := k8sh.Kubectl(args...)
		if err == nil {
			k8slogger.Infof("Found the thirdparty resource: " + tprname)
			return true
		}
		time.Sleep(RetryInterval * time.Second)
		inc++
	}

	return false
}

//IsCRDPresent returns true if custom resource definition is present
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

//GetPodDetails returns details about a  pod
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

//GetPodHostID returns HostIP address of a pod
func (k8sh *K8sHelper) GetPodHostID(podNamePattern string, namespace string) (string, error) {
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podNamePattern}
	podList, err := k8sh.Clientset.Pods(namespace).List(listOpts)
	if err != nil {
		logger.Errorf("Cannot get hostIp for app : %v in namespace %v, err: %v", podNamePattern, namespace, err)
		return "", fmt.Errorf("Cannot get hostIp for app : %v in namespace %v, err: %v", podNamePattern, namespace, err)
	}

	if podList.Size() < 1 {
		logger.Errorf("Cannot get hostIp for app : %v in namespace %v, err: %v", podNamePattern, namespace, err)
		return "", fmt.Errorf("Cannot get hostIp for app : %v in namespace %v, err: %v", podNamePattern, namespace, err)
	}
	return podList.Items[0].Status.HostIP, nil
}

//GetServiceNodePort returns nodeProt of service
func (k8sh *K8sHelper) GetServiceNodePort(serviceName string, namespace string) (string, error) {
	getOpts := metav1.GetOptions{}
	svc, err := k8sh.Clientset.Services(namespace).Get(serviceName, getOpts)
	if err != nil {
		logger.Errorf("Cannot get service : %v in namespace %v, err: %v", serviceName, namespace, err)
		return "", fmt.Errorf("Cannot get service : %v in namespace %v, err: %v", serviceName, namespace, err)
	}
	np := svc.Spec.Ports[0].NodePort
	return strconv.FormatInt(int64(np), 10), nil
}

//IsStorageClassPresent returns true if storageClass is present, if not false
func (k8sh *K8sHelper) IsStorageClassPresent(name string) (bool, error) {
	args := []string{"get", "storageclass", "-o", "jsonpath='{.items[*].metadata.name}'"}
	result, err := k8sh.Kubectl(args...)
	if strings.Contains(result, name) {
		return true, nil
	}
	return false, fmt.Errorf("Storageclass %s not found, err ->%v", name, err)

}

//GetPVCStatus returns status of PVC
func (k8sh *K8sHelper) GetPVCStatus(name string) (string, error) {
	args := []string{"get", "pvc", "-o", "jsonpath='{.items[*].metadata.name}'"}
	result, err := k8sh.Kubectl(args...)
	if strings.Contains(result, name) {
		args := []string{"get", "pvc", name, "-o", "jsonpath='{.status.phase}'"}
		res, _ := k8sh.Kubectl(args...)
		return res, nil
	}
	return "PVC NOT FOUND", fmt.Errorf("PVC %s not found,err->%v", name, err)
}

//IsPodInExpectedState waits for 90s for a pod to be an expected state
//If the pod is in expected state within 90s true is returned,  if not false
func (k8sh *K8sHelper) IsPodInExpectedState(podNamePattern string, namespace string, state string) bool {
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podNamePattern}
	inc := 0
	for inc < RetryLoop {
		podList, err := k8sh.Clientset.Pods(namespace).List(listOpts)
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

//CheckPodCountAndState returns true if expected number of pods with matching name are found and are in expected state
func (k8sh *K8sHelper) CheckPodCountAndState(podName string, namespace string, minExpected int, expectedPhase string) bool {
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podName}

	podList, err := k8sh.Clientset.Pods(namespace).List(listOpts)
	if err != nil {
		logger.Errorf("Cannot get logs for app : %v in namespace %v, err: %v", podName, namespace, err)
		return false
	}

	if len(podList.Items) < minExpected {
		logger.Errorf("Expected at least %d pods with name %v, actual count  %d", minExpected, podName, podList.Size())
		return false
	}

	inc := 0
	for inc < RetryLoop {
		r := true
		pl, _ := k8sh.Clientset.Pods(namespace).List(listOpts)
		for _, pod := range pl.Items {
			if !(pod.Status.Phase == v1.PodPhase(expectedPhase)) {
				r = false
				logger.Infof("waiting for pod %v to be in %s Phase, currently in %v Phase", pod.Name, expectedPhase, pod.Status.Phase)
			}
		}
		if r {
			return true
		}
		inc++
		time.Sleep(RetryInterval * time.Second)

	}
	logger.Errorf("All pods with app Name %v not in %v phase ", podName, expectedPhase)
	return false

}

//WaitUntilPodInNamespaceIsDeleted waits for 90s for a pod  in a namespace to be terminated
//If the pod disappears within 90s true is returned,  if not false
func (k8sh *K8sHelper) WaitUntilPodInNamespaceIsDeleted(podNamePattern string, namespace string) bool {
	args := []string{"-n", namespace, "pods", "-l", "app=" + podNamePattern}
	inc := 0
	for inc < RetryLoop {
		out, _ := k8sh.GetResource(args)
		if !strings.Contains(out, podNamePattern) {
			return true
		}

		inc++
		time.Sleep(RetryInterval * time.Second)
	}
	panic(fmt.Errorf("Rook not uninstalled"))
}

//WaitUntilPodIsDeleted waits for 90s for a pod to be terminated
//If the pod disappears within 90s true is returned,  if not false
func (k8sh *K8sHelper) WaitUntilPodIsDeleted(podNamePattern string) bool {
	args := []string{"pods", "-l", "app=" + podNamePattern}
	inc := 0
	for inc < RetryLoop {
		out, _ := k8sh.GetResource(args)
		if !strings.Contains(out, podNamePattern) {
			return true
		}

		inc++
		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

//WaitUntilPVCIsBound waits for a PVC to be in bound state for 90 seconds
//if PVC goes to Bound state within 90s True is returned, if not false
func (k8sh *K8sHelper) WaitUntilPVCIsBound(pvcname string) bool {

	inc := 0
	for inc < RetryLoop {
		out, err := k8sh.GetPVCStatus(pvcname)
		if strings.Contains(out, "Bound") {
			return true
		}

		logger.Infof("waiting for PVC to be bound. current=%s. err=%+v", out, err)
		inc++
		time.Sleep(RetryInterval * time.Second)
	}
	return false
}

//WaitUntilNameSpaceIsDeleted waits for namespace to be deleted for 180s.
//If namespace is deleted True is returned, if not false.
func (k8sh *K8sHelper) WaitUntilNameSpaceIsDeleted(namespace string) bool {
	getOpts := metav1.GetOptions{}
	inc := 0
	for inc < RetryLoop {
		ns, err := k8sh.Clientset.Namespaces().Get(namespace, getOpts)
		if err != nil {
			return true
		}
		logger.Infof("Namespace %s %v", namespace, ns.Status.Phase)
		inc++
		time.Sleep(RetryInterval * time.Second)
	}

	return false
}

//CreateExternalRGWService creates a service for rgw access external to the cluster on a node port
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
	_, err := k8sh.KubectlWithStdin(externalSvc, []string{"create", "-f", "-"}...)
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

//GetRGWServiceURL returns URL of ceph RGW service in the cluster
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

//GetRGWServiceURL returns URL of ceph RGW service in the cluster
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

//IsRookInstalled returns true is rook-api service is running(indicating rook is installed)
func (k8sh *K8sHelper) IsRookInstalled(namespace string) bool {
	opts := metav1.GetOptions{}
	_, err := k8sh.Clientset.Services(namespace).Get("rook-api", opts)
	if err == nil {
		return true
	}
	return false
}

//GetRookLogs captures logs from specified rook pod and writes it to specified file
func (k8sh *K8sHelper) GetRookLogs(podAppName string, namespace string, testName string) {
	logOpts := &v1.PodLogOptions{}
	listOpts := metav1.ListOptions{LabelSelector: "app=" + podAppName}

	podList, err := k8sh.Clientset.Pods(namespace).List(listOpts)
	if err != nil {
		logger.Errorf("Cannot get logs for app : %v in namespace %v, err: %v", podAppName, namespace, err)
		return
	}

	for _, pod := range podList.Items {
		podName := pod.Name
		logger.Infof("getting logs for pod : %v", podName)
		res := k8sh.Clientset.Pods(namespace).GetLogs(podName, logOpts).Do()
		rawData, err := res.Raw()
		if err != nil {
			logger.Errorf("Cannot get logs for app : %v in namespace %v, err: %v", podName, namespace, err)
			continue
		}
		dir, _ := os.Getwd()
		fpath := path.Join(dir, "_output/tests/")
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			err := os.MkdirAll(fpath, 0777)
			if err != nil {
				logger.Errorf("Cannot get logs files dir for app : %v in namespace %v, err: %v", podName, namespace, err)
				continue
			}
		}
		fileName := fmt.Sprintf("%s_%s_%s_%d.log", testName, podName, namespace, time.Now().Unix())
		fpath = path.Join(fpath, fileName)
		file, err := os.Create(fpath)
		if err != nil {
			logger.Errorf("Cannot get logs files for app : %v in namespace %v, err: %v", podName, namespace, err)
			continue
		}

		defer file.Close()
		_, err = file.Write(rawData)
		if err != nil {
			logger.Errorf("Errors while writing logs for : %v to file, err : %v", podName, err)
			continue
		}
	}
}

//CreateAnonSystemClusterBinding Creates anon-user-access clusterrolebinding for cluster-admin role - used by kubeadm env.
func (k8sh *K8sHelper) CreateAnonSystemClusterBinding() {
	args := []string{"create", "clusterrolebinding", "anon-user-access", "--clusterrole", "cluster-admin", "--user", "system:anonymous"}
	_, err := k8sh.Kubectl(args...)
	if err != nil {
		logger.Warningf("anon-user-access not created")
		return
	}
	logger.Infof("anon-user-access created")
}
