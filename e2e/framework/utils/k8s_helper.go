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

	"github.com/jmoiron/jsonq"
)

type K8sHelper struct {
}

func CreatK8sHelper() *K8sHelper {
	return &K8sHelper{}
}

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
	} else {
		return err, fmt.Errorf("Error Getting Monitor IP")
	}
}

func (k8sh *K8sHelper) ResourceOperationFromTemplate(action string, poddefPath string, config map[string]string) (string, error) {

	t, _ := template.ParseFiles(poddefPath)
	file, _ := ioutil.TempFile(os.TempDir(), "prefix")
	t.Execute(file, config)
	dir, _ := filepath.Abs(file.Name())

	cmdArgs := []string{action, "-f", dir}
	stdout, stderr, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		return stdout, nil
	} else {
		return stdout + " : " + stderr, fmt.Errorf("Could Not create resource in k8s. status=%d, stdout=%s, stderr=%s", status, stdout, stderr)
	}
}
func (k8sh *K8sHelper) ResourceOperation(action string, poddefPath string) (string, error) {

	cmdArgs := []string{action, "-f", poddefPath}
	out, err, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		return out, nil
	} else {
		return out + " : " + err, fmt.Errorf("Could Not create resource in k8s")
	}
}

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

//TODO: Combine these like functions
func (k8sh *K8sHelper) IsPodTerminated(name string) bool {
	cmdArgs := []string{"get", "pods", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status != 0 {
			fmt.Println("Pod in default namespace terminated: " + name)
			return true
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	fmt.Println("Pod in default namespace did not terminated: " + name)
	return false
}

//TODO: Combine these like functions
func (k8sh *K8sHelper) IsPodTerminatedInNamespace(name string) bool {
	cmdArgs := []string{"get", "-n", "rook", "pods", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status != 0 {
			fmt.Println("Pod in rook namespace terminated: " + name)
			return true
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	fmt.Println("Pod in rook namespace did not terminated: " + name)
	return false
}

func (k8sh *K8sHelper) IsServiceUp(name string) bool {
	cmdArgs := []string{"get", "svc", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			fmt.Println("Service in default namespace is up: " + name)
			return true
		}
		time.Sleep(2 * time.Second)
		inc++

	}
	fmt.Println("Service in default namespace is not up: " + name)
	return false
}

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
	fmt.Println("Service in rook namespace is not up: " + name)
	return false
}

func (k8sh *K8sHelper) GetService(servicename string) (string, error) {
	cmdArgs := []string{"get", "svc", "-n", "rook", servicename}
	sout, serr, status := ExecuteCmd("kubectl", cmdArgs)
	if status != 0 {
		return serr, fmt.Errorf("Cannot find service")
	}
	return sout, nil
}

func (k8sh *K8sHelper) IsThirdPartyResourcePresent(tprname string) bool {

	cmdArgs := []string{"get", "thirdpartyresources", tprname}
	inc := 0
	for inc < 20 {
		_, _, exitCode := ExecuteCmd("kubectl", cmdArgs)
		if exitCode == 0 {
			fmt.Println("Found the thirdparty resource: " + tprname)
			return true
		}
		time.Sleep(5 * time.Second)
		inc++
	}

	return false
}

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

func (k8sh *K8sHelper) GetPodHostId(podNamePattern string, namespace string) (string, error) {
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
		hostIp := strings.Replace(sout, "'", "", -1)
		return strings.TrimSpace(hostIp), nil
	} else {
		return serr, fmt.Errorf("Error Getting Monitor IP")
	}
}

func (k8sh *K8sHelper) IsStorageClassPresent(name string) (bool, error) {
	cmdArgs := []string{"get", "storageclass", "-o", "jsonpath='{.items[*].metadata.name}'"}
	sout, serr, _ := ExecuteCmd("kubectl", cmdArgs)
	if strings.Contains(sout, name) {
		return true, nil
	}
	return false, fmt.Errorf("Storageclass %s not found, err ->%s", name, serr)

}

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
