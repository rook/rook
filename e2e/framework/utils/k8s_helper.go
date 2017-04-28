package utils

import (
	"encoding/json"
	"fmt"
	"github.com/jmoiron/jsonq"
	"html/template"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type K8sHelper struct {
}

func CreatK8sHelper() *K8sHelper {
	return &K8sHelper{}
}

func (k8sh *K8sHelper) getMonIp(mon string) (string, error) {
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

func (k8sh *K8sHelper) ResourceOperationFromTemplate(action string, poddefPath string) (string, error) {

	t, _ := template.ParseFiles(poddefPath)
	mons := k8sh.getMonitorPods()
	ip1, _ := k8sh.getMonIp(mons[0])
	ip2, _ := k8sh.getMonIp(mons[1])
	ip3, _ := k8sh.getMonIp(mons[2])

	config := map[string]string{
		"mon0": ip1,
		"mon1": ip2,
		"mon2": ip3,
	}
	file, _ := ioutil.TempFile(os.TempDir(), "prefix")
	t.Execute(file, config)
	dir, _ := filepath.Abs(file.Name())

	cmdArgs := []string{action, "-f", dir}
	out, err, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		return out, nil
	} else {
		return out + " : " + err, fmt.Errorf("Could Not create resource in k8s")
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

func (k8sh *K8sHelper) CleanUpDymaincCreatedPVC(blockList map[string]blockListData) {

	for _, v := range blockList {
		cmdArgs := []string{"exec", "-n", "rook", "rook-client", "--", "rook", "block", "delete",
			"--name", v.name, "--pool-name", v.pool}
		ExecuteCmd("kubectl", cmdArgs)
	}
}

func (k8sh *K8sHelper) getMonitorPods() []string {
	mons := []string{}
	monIdx := 0
	moncount := 0

	for moncount < 3 {
		m := "mon" + strconv.Itoa(monIdx)
		cmdArgs := []string{"-n", "rook", "get", "pods", m}
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			mons = append(mons, m)
			moncount++
		}
		monIdx++
	}

	return mons
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
