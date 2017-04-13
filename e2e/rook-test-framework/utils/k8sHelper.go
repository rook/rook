package utils

import (
	"encoding/json"
	"errors"
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
		return err, errors.New("Error Getting Monitor IP")
	}
}

func (k8sh *K8sHelper) CreatePodFromTemplate(poddefPath string) (string, error) {

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

	cmdArgs := []string{"create", "-f", dir}
	out, err, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		return out, nil
	} else {
		return out + " : " + err, errors.New("Could Not create resource in k8s")
	}
}

func (k8sh *K8sHelper) DeletePodFromTemplate(poddefPath string) (string, error) {

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

	cmdArgs := []string{"delete", "-f", dir}
	out, err, status := ExecuteCmd("kubectl", cmdArgs)
	if status == 0 {
		return out, nil
	} else {
		return out + " : " + err, errors.New("Could Not create resource in k8s")
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
	cmdArgs := []string{"logs", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			return true
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	return false
}
func (k8sh *K8sHelper) IsPodRunningInNamespace(name string) bool {
	cmdArgs := []string{"logs", "-n", "rook", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			return true
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	return false
}

func (k8sh *K8sHelper) IsPodTerminated(name string) bool {
	cmdArgs := []string{"get", "pods", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status != 0 {
			return true
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	return false
}
func (k8sh *K8sHelper) IsPodTerminatedInNamespace(name string) bool {
	cmdArgs := []string{"get", "-n", "rook", "pods", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status != 0 {
			return true
		}
		time.Sleep(5 * time.Second)
		inc++

	}
	return false
}

func (k8sh *K8sHelper) IsServiceUp(name string) bool {
	cmdArgs := []string{"get", "svc", name}
	inc := 0
	for inc < 20 {
		_, _, status := ExecuteCmd("kubectl", cmdArgs)
		if status == 0 {
			return true
		}
		time.Sleep(5 * time.Second)
		inc++

	}
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
	return false
}

func (k8sh *K8sHelper) GetService(servicename string) (string, error) {
	cmdArgs := []string{"get", "svc", "-n", "rook", servicename}
	sout, serr, status := ExecuteCmd("kubectl", cmdArgs)
	if status != 0 {
		return serr, errors.New("Cannot find rgw service")
	}
	return sout, nil
}
