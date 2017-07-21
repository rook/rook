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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/jmoiron/jsonq"
	"github.com/rook/rook/pkg/util/exec"
)

//K8sHelper is a helper for common kubectl commads
type K8sHelper struct {
	executor *exec.CommandExecutor
}

//CreatK8sHelper creates a instance of k8sHelper
func CreatK8sHelper() *K8sHelper {
	return &K8sHelper{&exec.CommandExecutor{}}
}

var k8slogger = capnslog.NewPackageLogger("github.com/rook/rook", "k8sutil")

//Kubectl is wrapper for executing kubectl commands
func (k8sh *K8sHelper) Kubectl(args ...string) string {
	result, error := k8sh.executor.ExecuteCommandWithOutput("", "kubectl", args...)
	if error != nil {
		k8slogger.Errorf("Errors Encounterd while executing kubectl command : %v", error)
		panic(error)

	}
	return result
}

//GetMonIP returns IP address for a ceph mon pod
func (k8sh *K8sHelper) GetMonIP(mon string) (string, error) {
	//kubectl -n rook get pod mon0 -o json|jq ".status.podIP"|
	cmdArgs := []string{"-n", "rook", "get", "pod", mon, "-o", "json"}
	out, err, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		data := map[string]interface{}{}
		dec := json.NewDecoder(strings.NewReader(out))
		dec.Decode(&data)
		jq := jsonq.NewQuery(data)
		ip, _ := jq.String("status", "podIP")
		return ip + ":6790", nil
	}
	return err, fmt.Errorf("Error Getting Monitor IP")
}

//ResourceOperationFromTemplate performs a kubectl action from a template file after replacing its context
func (k8sh *K8sHelper) ResourceOperationFromTemplate(action string, poddefPath string, config map[string]string) (string, error) {

	t, _ := template.ParseFiles(poddefPath)
	file, _ := ioutil.TempFile(os.TempDir(), "prefix")
	t.Execute(file, config)
	dir, _ := filepath.Abs(file.Name())

	cmdArgs := []string{action, "-f", dir}
	stdout, stderr, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		return stdout, nil
	}
	return stdout + " : " + stderr, fmt.Errorf("Could Not create resource in k8s. status=%d, stdout=%s, stderr=%s", status, stdout, stderr)

}

//ResourceOperation performs a kubectl action on a yaml file
func (k8sh *K8sHelper) ResourceOperation(action string, poddefPath string) (string, error) {

	cmdArgs := []string{action, "-f", poddefPath}
	out, err, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		return out, nil
	}
	return out + " : " + err, fmt.Errorf("Could Not create resource in k8s")

}

//DeleteResource performs a kubectl delete on give args
func (k8sh *K8sHelper) DeleteResource(args []string) (string, error) {
	cmdArgs := append([]string{"delete"}, args...)
	out, err, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		return out, nil
	}
	return out + " : " + err, fmt.Errorf("Could Not delete resource in k8s")

}

//GetResource performs a kubectl get on give args
func (k8sh *K8sHelper) GetResource(args []string) (string, error) {
	cmdArgs := append([]string{"get"}, args...)
	out, err, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		return out, nil
	}
	return out + " : " + err, fmt.Errorf("Could Not Get resource in k8s")

}

//GetMonitorPods returns all ceph mon pod names
func (k8sh *K8sHelper) GetMonitorPods() ([]string, error) {
	mons := []string{}
	monIdx := 0
	moncount := 0

	for moncount < 3 {
		m := fmt.Sprintf("rook-ceph-mon%d", monIdx)
		selector := fmt.Sprintf("mon=%s", m)
		cmdArgs := []string{"-n", "rook", "get", "pod", "-l", selector}
		stdout, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			// Get the first word of the second line of the output for the mon pod
			lines := strings.Split(stdout, "\n")
			if len(lines) > 1 {
				name := strings.Split(lines[1], " ")[0]
				mons = append(mons, name)
				moncount++
			} else {
				return mons, fmt.Errorf("did not recognize mon pod output %s", m)
			}
		}
		monIdx++
		if monIdx > 100 {
			return mons, fmt.Errorf("failed to find monitors")
		}
	}

	return mons, nil
}

//IsPodRunning retuns true if a Pod is running status or goes to Running status within 90s else returns false
func (k8sh *K8sHelper) IsPodRunning(name string) bool {
	cmdArgs := []string{"get", "pod", name}
	inc := 0
	for inc < 20 {
		out, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			lines := strings.Split(out, "\n")
			if len(lines) == 3 {
				lines = lines[1 : len(lines)-1]
				bktsrawdata := strings.Split(lines[0], "  ")
				var r []string
				for _, str := range bktsrawdata {
					if str != "" {
						r = append(r, strings.TrimSpace(str))
					}
				}
				if r[2] == "Running" {
					return true
				}
			}
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	return false
}

//IsPodRunningInNamespace retuns true if a Pod in a namespace is running status or goes to Running
// status within 90s else returns false
func (k8sh *K8sHelper) IsPodRunningInNamespace(name string) bool {
	cmdArgs := []string{"get", "pods", "-n", "rook", name}
	inc := 0
	for inc < 20 {
		out, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			lines := strings.Split(out, "\n")
			if len(lines) == 3 {
				lines = lines[1 : len(lines)-1]
				bktsrawdata := strings.Split(lines[0], "  ")
				var r []string
				for _, str := range bktsrawdata {
					if str != "" {
						r = append(r, strings.TrimSpace(str))
					}
				}
				if r[2] == "Running" {
					return true
				}
			}
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	return false
}

//IsPodTerminated retuns true if a Pod is terminated status or goes to Terminated  status
// within 90s else returns false\
func (k8sh *K8sHelper) IsPodTerminated(name string) bool {
	cmdArgs := []string{"get", "pods", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status != 0 {
			k8slogger.Infof("Pod in default namespace terminated: " + name)
			return true
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	k8slogger.Infof("Pod in default namespace did not terminated: " + name)
	return false
}

//IsPodTerminatedInNamespace retuns true if a Pod  in a namespace is terminated status
// or goes to Terminated  status within 90s else returns false\
func (k8sh *K8sHelper) IsPodTerminatedInNamespace(name string) bool {
	cmdArgs := []string{"get", "-n", "rook", "pods", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status != 0 {
			k8slogger.Infof("Pod in rook namespace terminated: " + name)
			return true
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	k8slogger.Infof("Pod in rook namespace did not terminated: " + name)
	return false
}

//IsServiceUp returns true if a service is up or comes up within 40s, else returns false
func (k8sh *K8sHelper) IsServiceUp(name string) bool {
	cmdArgs := []string{"get", "svc", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			k8slogger.Infof("Service in default namespace is up: " + name)
			return true
		}
		time.Sleep(2 * time.Second)
		inc++

	}
	k8slogger.Infof("Service in default namespace is not up: " + name)
	return false
}

//IsServiceUpInNameSpace returns true if a service  in a namespace is up or comes up within 40s, else returns false
func (k8sh *K8sHelper) IsServiceUpInNameSpace(name string) bool {

	cmdArgs := []string{"get", "svc", "-n", "rook", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			return true
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	k8slogger.Infof("Service in rook namespace is not up: " + name)
	return false
}

//GetService returns output from "kubectl get svc $NAME" command
func (k8sh *K8sHelper) GetService(servicename string) (string, error) {
	cmdArgs := []string{"get", "svc", "-n", "rook", servicename}
	sout, serr, status := ExecuteCmd("kubectl", cmdArgs)
	if status != 0 {
		return serr, fmt.Errorf("Cannot find service")
	}
	return sout, nil
}

//IsThirdPartyResourcePresent returns true if Third party resource is present
func (k8sh *K8sHelper) IsThirdPartyResourcePresent(tprname string) bool {

	cmdArgs := []string{"get", "thirdpartyresources", tprname}
	inc := 0
	for inc < 20 {
		_, _, exitCode := ExecuteCmd("kubectl", cmdArgs)
		if exitCode == 0 {
			k8slogger.Infof("Found the thirdparty resource: " + tprname)
			return true
		}
		time.Sleep(5 * time.Second)
		inc++
	}

	return false
}

//GetPodDetails returns details about a  pod
func (k8sh *K8sHelper) GetPodDetails(podNamePattern string, namespace string) (string, error) {
	cmdArgs := []string{"get", "pods", "-l", "app=" + podNamePattern, "-o", "wide", "--no-headers=true"}
	if namespace != "" {
		cmdArgs = append(cmdArgs, []string{"-n", namespace}...)
	}
	sout, serr, status := ExecuteCmd("kubectl", cmdArgs)
	if status != 0 || strings.Contains(sout, "No resources found") {
		return serr, fmt.Errorf("Cannot find pod in with name like %s in namespace : %s", podNamePattern, namespace)
	}
	return sout, nil
}

//GetPodHostID returns HostIP address of a pod
func (k8sh *K8sHelper) GetPodHostID(podNamePattern string, namespace string) (string, error) {
	data, err := k8sh.GetPodDetails(podNamePattern, namespace)
	if err != nil {
		return data, err
	}

	// Handle case when no data is returned
	lines := strings.Split(data, "\n")

	//extract name of the pod
	lineRawdata := strings.Split(lines[0], "  ")
	var r []string
	for _, str := range lineRawdata {
		if str != "" {
			r = append(r, strings.TrimSpace(str))
		}
	}

	//get host Ip of the pod
	cmdArgs := []string{"get", "pods", r[0], "-o", "jsonpath='{.status.hostIP}'"}
	if namespace != "" {
		cmdArgs = append(cmdArgs, []string{"-n", namespace}...)
	}
	sout, serr, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		hostIP := strings.Replace(sout, "'", "", -1)
		return strings.TrimSpace(hostIP), nil
	}
	return serr, fmt.Errorf("Error Getting Monitor IP")

}

//IsStorageClassPresent returns true if storageClass is present, if not false
func (k8sh *K8sHelper) IsStorageClassPresent(name string) (bool, error) {
	cmdArgs := []string{"get", "storageclass", "-o", "jsonpath='{.items[*].metadata.name}'"}
	sout, serr, _ := ExecuteCmd("kubectl", cmdArgs)
	if strings.Contains(sout, name) {
		return true, nil
	}
	return false, fmt.Errorf("Storageclass %s not found, err ->%s", name, serr)

}

//GetPVCStatus returns status of PVC
func (k8sh *K8sHelper) GetPVCStatus(name string) (string, error) {
	cmdArgs := []string{"get", "pvc", "-o", "jsonpath='{.items[*].metadata.name}'"}
	sout, serr, _ := ExecuteCmd("kubectl", cmdArgs)
	if strings.Contains(sout, name) {
		cmdArgs := []string{"get", "pvc", name, "-o", "jsonpath='{.status.phase}'"}
		res, _, _ := ExecuteCmd("kubectl", cmdArgs)
		return res, nil
	}
	return "PVC NOT FOUND", fmt.Errorf("PVC %s not found,err->%s", name, serr)
}

//IsPodInExpectedState waits for 90s for a pod to be an expected state
//If the pod is in expected state within 90s true is returned,  if not false
func (k8sh *K8sHelper) IsPodInExpectedState(podNamePattern string, namespace string, state string) bool {
	cmdArgs := []string{"get", "pods", "-l", "app=" + podNamePattern, "-o", "jsonpath={.items[0].status.phase}", "--no-headers=true"}
	if namespace != "" {
		cmdArgs = append(cmdArgs, []string{"-n", namespace}...)
	}
	inc := 0
	for inc < 30 {
		res, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			if res == state {
				return true
			}
		}
		inc++
		time.Sleep(3 * time.Second)
	}

	return false
}

//WaitUntilPodInNamespaceIsDeleted waits for 90s for a pod  in a namespace to be terminated
//If the pod disappears within 90s true is returned,  if not false
func (k8sh *K8sHelper) WaitUntilPodInNamespaceIsDeleted(podNamePattern string, namespace string) bool {
	args := []string{"-n", namespace, "pods", "-l", "app=" + podNamePattern}
	inc := 0
	for inc < 30 {
		out, _ := k8sh.GetResource(args)
		if !strings.Contains(out, podNamePattern) {
			return true
		}

		inc++
		time.Sleep(3 * time.Second)
	}
	panic(fmt.Errorf("Rook not uninstalled"))
}

//WaitUntilPodIsDeleted waits for 90s for a pod to be terminated
//If the pod disappears within 90s true is returned,  if not false
func (k8sh *K8sHelper) WaitUntilPodIsDeleted(podNamePattern string) bool {
	args := []string{"pods", "-l", "app=" + podNamePattern}
	inc := 0
	for inc < 30 {
		out, _ := k8sh.GetResource(args)
		if !strings.Contains(out, podNamePattern) {
			return true
		}

		inc++
		time.Sleep(3 * time.Second)
	}
	return false
}

//WaitUntilPVCIsBound waits for a PVC to be in bound state for 90 seconds
//if PVC goes to Bound state within 90s True is returned, if not false
func (k8sh *K8sHelper) WaitUntilPVCIsBound(pvcname string) bool {

	inc := 0
	for inc < 30 {
		out, _ := k8sh.GetPVCStatus(pvcname)
		if strings.Contains(out, "Bound") {
			return true
		}

		inc++
		time.Sleep(3 * time.Second)
	}
	return false
}
