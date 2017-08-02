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
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"bytes"

	"github.com/coreos/pkg/capnslog"
	"github.com/jmoiron/jsonq"
	"github.com/rook/rook/pkg/util/exec"
)

//K8sHelper is a helper for common kubectl commads
type K8sHelper struct {
	executor  *exec.CommandExecutor
	Clientset *kubernetes.Clientset
}

const (
	//RetryLoop params for tests.
	RetryLoop = 30
	//RetryInterval param for test - wait time while in RetryLoop
	RetryInterval = 5
)

//CreatK8sHelper creates a instance of k8sHelper
func CreatK8sHelper() (*K8sHelper, error) {
	executor := &exec.CommandExecutor{}
	config, err := getKubeConfig(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to get kube client. %+v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset. %+v", err)
	}

	return &K8sHelper{executor: executor, Clientset: clientset}, err

}

var k8slogger = capnslog.NewPackageLogger("github.com/rook/rook", "k8sutil")

//GetK8sServerVersion returns k8s server version under test
func (k8sh *K8sHelper) GetK8sServerVersion() string {
	versionInfo, _ := k8sh.Clientset.ServerVersion()
	return versionInfo.GitVersion
}

//Kubectl is wrapper for executing kubectl commands
func (k8sh *K8sHelper) Kubectl(args ...string) (string, error) {
	result, err := k8sh.executor.ExecuteCommandWithOutput("", "kubectl", args...)
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
	context, err := executor.ExecuteCommandWithOutput("", "kubectl", "config", "view", "-o", "json")
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
	}
	//work around for kubeadm - api service is https but context has no cert
	config.Insecure = true

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

//GetMonIP returns IP address for a ceph mon pod
func (k8sh *K8sHelper) GetMonIP(mon string) (string, error) {
	//kubectl -n rook get pod mon0 -o json|jq ".status.podIP"|
	cmdArgs := []string{"-n", "rook", "get", "pod", mon, "-o", "json"}
	result, err := k8sh.Kubectl(cmdArgs...)
	if err == nil {
		data := map[string]interface{}{}
		dec := json.NewDecoder(strings.NewReader(result))
		dec.Decode(&data)
		jq := jsonq.NewQuery(data)
		ip, _ := jq.String("status", "podIP")
		return fmt.Sprintf("%s:6790", ip), nil
	}
	return "", fmt.Errorf("Error Getting Monitor IP : %v", err)
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
func (k8sh *K8sHelper) GetMonitorServices() (map[string]string, error) {

	cmdArgs := []string{"-n", "rook", "get", "svc", "-l", "app=rook-ceph-mon", "--no-headers=true"}
	stdout, _, status := ExecuteCmd("kubectl", cmdArgs)
	if status != 0 {
		return nil, fmt.Errorf("Failed to find mon services. %d", status)
	}

	// Get the IP address from the 2nd position in the line
	mons, err := parseMonEndpoints(stdout)
	if err != nil {
		return nil, err
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

func parseMonEndpoints(input string) ([]string, error) {
	lines := strings.Split(input, "\n")
	mons := []string{}
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("Missing ip for mon service. %s", line)
		}
		mons = append(mons, strings.TrimSpace(fields[1]))
	}
	return mons, nil
}

//IsPodRunning returns true if a Pod is running status or goes to Running status within 90s else returns false
func (k8sh *K8sHelper) IsPodRunning(name string) bool {
	args := []string{"get", "pod", name, "-o", "jsonpath='{.status.phase}'"}
	inc := 0
	for inc < RetryLoop {
		result, err := k8sh.Kubectl(args...)
		if err == nil {
			if strings.Contains(result, "Running") {
				return true
			}
			logger.Infof("Pod %s status: %s", name, result)
		}

		time.Sleep(RetryInterval * time.Second)
		inc++

	}
	logger.Infof("Giving up waiting for pod %s to be running", name)
	return false
}

//IsPodRunningInNamespace returns true if a Pod in a namespace is running status or goes to Running
// status within 90s else returns false
func (k8sh *K8sHelper) IsPodRunningInNamespace(name string) bool {
	args := []string{"get", "pods", "-n", "rook", name, "-o", "jsonpath='{.status.phase}'"}
	inc := 0
	for inc < RetryLoop {
		result, err := k8sh.Kubectl(args...)
		if err == nil {
			if strings.Contains(result, "Running") {
				return true
			}
			logger.Infof("Pod %s status: %s", name, result)
		}
		time.Sleep(RetryInterval * time.Second)
		inc++

	}
	logger.Infof("Giving up waiting for pod %s to be running", name)
	return false
}

//IsPodTerminated returns true if a Pod is terminated status or goes to Terminated  status
// within 90s else returns false\
func (k8sh *K8sHelper) IsPodTerminated(name string) bool {
	args := []string{"get", "pods", name}
	inc := 0
	for inc < RetryLoop {
		_, err := k8sh.Kubectl(args...)
		if err != nil {
			k8slogger.Infof("Pod in default namespace terminated: " + name)
			return true
		}
		time.Sleep(RetryInterval * time.Second)
		inc++

	}
	k8slogger.Infof("Pod in default namespace did not terminated: " + name)
	return false
}

//IsPodTerminatedInNamespace returns true if a Pod  in a namespace is terminated status
// or goes to Terminated  status within 90s else returns false\
func (k8sh *K8sHelper) IsPodTerminatedInNamespace(name string) bool {
	args := []string{"get", "-n", "rook", "pods", name}
	inc := 0
	for inc < RetryLoop {
		_, err := k8sh.Kubectl(args...)
		if err != nil {
			k8slogger.Infof("Pod in rook namespace terminated: " + name)
			return true
		}
		time.Sleep(RetryInterval * time.Second)
		inc++

	}
	k8slogger.Infof("Pod in rook namespace did not terminated: " + name)
	return false
}

//IsServiceUp returns true if a service is up or comes up within 40s, else returns false
func (k8sh *K8sHelper) IsServiceUp(name string) bool {
	args := []string{"get", "svc", name}
	inc := 0
	for inc < RetryLoop {
		_, err := k8sh.Kubectl(args...)
		if err == nil {
			k8slogger.Infof("Service in default namespace is up: " + name)
			return true
		}
		time.Sleep(RetryInterval * time.Second)
		inc++

	}
	k8slogger.Infof("Service in default namespace is not up: " + name)
	return false
}

//IsServiceUpInNameSpace returns true if a service  in a namespace is up or comes up within 40s, else returns false
func (k8sh *K8sHelper) IsServiceUpInNameSpace(name string) bool {
	args := []string{"get", "svc", "-n", "rook", name}
	inc := 0
	for inc < RetryLoop {
		_, err := k8sh.Kubectl(args...)
		if err == nil {
			return true
		}
		time.Sleep(RetryInterval * time.Second)
		inc++

	}
	k8slogger.Infof("Service in rook namespace is not up: " + name)
	return false
}

//GetService returns output from "kubectl get svc $NAME" command
func (k8sh *K8sHelper) GetService(servicename string) (string, error) {
	args := []string{"get", "svc", "-n", "rook", servicename}
	result, err := k8sh.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("Cannot find service %v -- %v", servicename, err)
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
	output, err := k8sh.GetPodDetails(podNamePattern, namespace)
	if err != nil {
		return "", err
	}

	podNames := strings.Split(output, "\n")
	if len(podNames) == 0 {
		return "", fmt.Errorf("pod %s not found", podNamePattern)
	}

	//get host Ip of the pod
	args := []string{"get", podNames[0], "-o", "jsonpath='{.status.hostIP}'"}
	if namespace != "" {
		args = append(args, []string{"-n", namespace}...)
	}
	result, err := k8sh.Kubectl(args...)
	if err == nil {
		hostIP := strings.Replace(result, "'", "", -1)
		return strings.TrimSpace(hostIP), nil
	}
	return "", fmt.Errorf("Error Getting Monitor IP -- %v", err)

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
	args := []string{"get", "pods", "-l", "app=" + podNamePattern, "-o", "jsonpath={.items[0].status.phase}", "--no-headers=true"}
	if namespace != "" {
		args = append(args, []string{"-n", namespace}...)
	}
	inc := 0
	for inc < RetryLoop {
		result, err := k8sh.Kubectl(args...)
		if err == nil {
			if result == state {
				return true
			}
		}
		inc++
		time.Sleep(RetryInterval * time.Second)
	}

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

//GetRGWServiceURL returns URL of ceph RGW service in the cluster
func (k8sh *K8sHelper) GetRGWServiceURL() (string, error) {
	hostip, err := k8sh.GetPodHostID("rook-ceph-rgw", "rook")
	if err != nil {
		panic(fmt.Errorf("RGW pods not found/object store possibly not started"))
	}

	//TODO - Get nodePort stop hardcoding
	endpoint := hostip + ":30001"
	return endpoint, err
}
