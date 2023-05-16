package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	HealthOK    = "HEALTH_OK"
	HealthWarn  = "HEALTH_WARN"
	HealthError = "HEALTH_ERROR"
)

type Rook interface {
	SetNamespace(ns string)
	ValidateOSDDeploy(osdID int, osdDeployName string) error
	GetHealthStatus() (string, error)
	GetPrepareJobName(pvcName string) (string, error)
	GetSameDeviceClassOSDs(osdID int, deviceClass string) ([]*OSD, error)
	GetOSDInfo(osdID int) (*OSD, error)
	IsOSDIn(osdID int) (bool, error)
	StartOperator() error
	StopOperator() error
	StopOSD(osdID int) error
	DeleteSubsidiaryResources(deployName string, deletePVC bool) error
	WaitForAllOSDsToStart() error
	DeleteCephInternalOSDInfo(osdID int) error
}

type RookImpl struct {
	Namespace string
}

func (rook *RookImpl) SetNamespace(ns string) {
	rook.Namespace = ns
}

func (rook *RookImpl) ValidateOSDDeploy(osdID int, osdDeployName string) error {
	stdout, _, err := utilObj.Kubectl(rook.Namespace, "get", "deploy", osdDeployName, "-o", "jsonpath={.metadata.labels.ceph-osd-id}")
	if err != nil {
		return err
	}
	res, err := strconv.Atoi(string(stdout))
	if err != nil {
		return err
	}
	if res != osdID {
		return fmt.Errorf("OSD ID is not same: expected %d, got %d", osdID, res)
	}

	return nil
}

func (rook *RookImpl) GetHealthStatus() (string, error) {
	stdout, _, err := utilObj.Ceph(rook.Namespace, "status", "-f", "json")
	if err != nil {
		return "", err
	}
	stdout, _, err = utilObj.Exec(stdout, "jq", "-r", ".health.status")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

func (rook *RookImpl) GetOSDInfo(osdID int) (*OSD, error) {
	stdout, _, err := utilObj.Ceph(rook.Namespace, "osd", "df", fmt.Sprintf("osd.%d", osdID), "-f", "json")
	if err != nil {
		return nil, err
	}
	osdDfResult, _, err := utilObj.Exec(stdout, "jq", ".nodes[]")
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(string(osdDfResult)) == "" {
		return nil, ErrOsdNotFoundError
	}

	return rook.getOSDInfoHelper(osdID, osdDfResult)
}

func (rook *RookImpl) getOSDInfoHelper(osdID int, osdDfResult []byte) (*OSD, error) {
	fmt.Printf("osdID = %d\n", osdID)
	stdout, _, err := utilObj.Exec(osdDfResult, "jq", "-r", ".device_class")
	if err != nil {
		return nil, err
	}
	deviceClass := strings.TrimSpace(string(stdout))
	fmt.Printf("deviceClass = %s\n", deviceClass)

	capacity, err := getValFromOSDDfResultFloat64(osdDfResult, "kb")
	if err != nil {
		return nil, err
	}
	fmt.Printf("capacity = %v\n", capacity)

	usedCapacity, err := getValFromOSDDfResultFloat64(osdDfResult, "kb_used")
	if err != nil {
		return nil, err
	}
	fmt.Printf("usedCapacity = %v\n", usedCapacity)

	nearfullRatio, err := rook.getNearfullRatio(osdID)
	if err != nil {
		return nil, err
	}
	fmt.Printf("nearfullRatio = %v\n", nearfullRatio)

	weight, err := getValFromOSDDfResultFloat64(osdDfResult, "crush_weight")
	if err != nil {
		return nil, err
	}
	fmt.Printf("weight = %v\n", weight)

	return &OSD{
		ID:            osdID,
		DeviceClass:   deviceClass,
		Capacity:      capacity,
		UsedCapacity:  usedCapacity,
		NearfullRatio: nearfullRatio,
		Weight:        weight,
	}, nil
}

func (rook *RookImpl) GetSameDeviceClassOSDs(osdID int, deviceClass string) ([]*OSD, error) {
	stdout, _, err := utilObj.Ceph(rook.Namespace, "osd", "df", "-f", "json")
	if err != nil {
		return nil, err
	}
	osdDfResults, _, err := utilObj.Exec(stdout, "jq", ".nodes[]")
	if err != nil {
		return nil, err
	}
	sameDeviceClassOSDIDs, err := getSameDeviceClassOSDIDs(osdDfResults, deviceClass, osdID)
	if err != nil {
		return nil, err
	}
	sameDeviceClassOSDIDs, err = rook.filterOutUnhealthyOSDs(osdDfResults, sameDeviceClassOSDIDs)
	if err != nil {
		return nil, err
	}
	sameDeviceClassOSDs := make([]*OSD, len(sameDeviceClassOSDIDs))
	for i, sameDevOSDID := range sameDeviceClassOSDIDs {
		osdDfResult, err := getOSDDfResult(osdDfResults, sameDevOSDID)
		if err != nil {
			return nil, err
		}
		osd, err := rook.getOSDInfoHelper(sameDevOSDID, osdDfResult)
		if err != nil {
			return nil, err
		}
		sameDeviceClassOSDs[i] = osd
	}
	return sameDeviceClassOSDs, nil
}

func (rook *RookImpl) filterOutUnhealthyOSDs(osdDfResults []byte, osdIDs []int) ([]int, error) {
	result := make([]int, 0)
	for _, osdID := range osdIDs {
		osdDfResult, err := getOSDDfResult(osdDfResults, osdID)
		if err != nil {
			return nil, err
		}
		osdStatus, _, err := utilObj.Exec(osdDfResult, "jq", "-r", ".status")
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(osdStatus)) == "up" {
			result = append(result, osdID)
		}
	}
	return result, nil
}

func getOSDDfResult(osdDfResults []byte, osdID int) ([]byte, error) {
	osdDfResult, _, err := utilObj.Exec(osdDfResults, "jq", ". | select(.id == "+strconv.Itoa(osdID)+")")
	return osdDfResult, err
}

func (rook *RookImpl) DownOSD(osdID int) error {
	_, _, err := utilObj.Ceph(rook.Namespace, "osd", "down", strconv.Itoa(osdID))
	if err != nil {
		return err
	}
	stdout, _, err := utilObj.Ceph(rook.Namespace, "osd", "dump", "-f", "json")
	if err != nil {
		return err
	}
	stdout, _, err = utilObj.Exec(stdout, "jq", "-r", fmt.Sprintf(".osds[] | select(.osd == %s) | .up", strconv.Itoa(osdID)))
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(stdout)) != "0" {
		return fmt.Errorf("failed to down osd.%d", osdID)
	}

	return nil
}

func contains(data []int, key int) bool {
	for _, v := range data {
		if v == key {
			return true
		}
	}
	return false
}

func (rook *RookImpl) IsOSDIn(osdID int) (bool, error) {
	stdout, _, err := utilObj.Ceph(rook.Namespace, "osd", "ls", "-f", "json")
	if err != nil {
		return false, err
	}
	osdIDs := []int{}
	err = json.Unmarshal(stdout, &osdIDs)
	if err != nil {
		return false, err
	}
	if !contains(osdIDs, osdID) {
		return false, nil
	}

	stdout, _, err = utilObj.Ceph(rook.Namespace, "osd", "info", fmt.Sprintf("osd.%d", osdID), "-f", "json")
	if err != nil {
		return false, err
	}
	stdout, _, err = utilObj.Exec(stdout, "jq", ".in")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(stdout)) == "1", nil
}

func (rook *RookImpl) OutOSD(osdID int) error {
	_, _, err := utilObj.Ceph(rook.Namespace, "osd", "out", strconv.Itoa(osdID))
	if err != nil {
		return err
	}
	stdout, _, err := utilObj.Ceph(rook.Namespace, "osd", "dump", "-f", "json")
	if err != nil {
		return err
	}
	stdout, _, err = utilObj.Exec(stdout, "jq", "-r", ".osds[] | select(.osd ==  "+strconv.Itoa(osdID)+") | .in")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(stdout)) != "0" {
		return fmt.Errorf("failed to out osd.%d", osdID)
	}

	return nil
}

func (rook *RookImpl) StartOperator() error {
	_, _, err := utilObj.Kubectl(rook.Namespace, "scale", "deployments", "rook-ceph-operator", "--replicas=1")
	if err != nil {
		return err
	}
	err = k8sObj.WaitForPodCountChange(rook.Namespace, "app=rook-ceph-operator", 1)
	if err != nil {
		return err
	}
	return nil
}

func (rook *RookImpl) StopOperator() error {
	_, _, err := utilObj.Kubectl(rook.Namespace, "scale", "deployments", "rook-ceph-operator", "--replicas=0")
	if err != nil {
		return err
	}
	err = k8sObj.WaitForPodCountChange(rook.Namespace, "app=rook-ceph-operator", 0)
	if err != nil {
		return err
	}
	return nil
}

func (rook *RookImpl) StopOSD(osdID int) error {
	_, _, err := utilObj.Kubectl(rook.Namespace, "scale", "deployments", fmt.Sprintf("rook-ceph-osd-%d", osdID), "--replicas=0")
	if err != nil {
		return err
	}
	err = k8sObj.WaitForPodCountChange(rook.Namespace, fmt.Sprintf("app=rook-ceph-osd,ceph-osd-id=%d", osdID), 0)
	if err != nil {
		return err
	}

	err = rook.DownOSD(osdID)
	if err != nil {
		return err
	}

	err = rook.OutOSD(osdID)
	if err != nil {
		return err
	}

	return nil
}

func (rook *RookImpl) WaitForAllOSDsToStart() error {
	stdout, _, err := utilObj.Kubectl(rook.Namespace, "get", "cephcluster", rook.Namespace, "-o", "json")
	if err != nil {
		return err
	}
	stdout, _, err = utilObj.Exec(stdout, "jq", ".spec.storage.storageClassDeviceSets[].count")
	if err != nil {
		return err
	}
	osdCountsStr := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	desiredOSDCount := 0
	for _, osdCountStr := range osdCountsStr {
		osdCount, err := strconv.Atoi(osdCountStr)
		if err != nil {
			return err
		}
		desiredOSDCount += osdCount
	}

	err = k8sObj.WaitForPodCountChange(rook.Namespace, "app=rook-ceph-osd", desiredOSDCount)
	if err != nil {
		return err
	}
	return nil
}

func (rook *RookImpl) GetOSDCount() (int, error) {
	stdout, _, err := utilObj.Ceph(rook.Namespace, "status", "-f", "json")
	if err != nil {
		return 0, err
	}
	stdout, _, err = utilObj.Exec(stdout, "jq", ".osdmap.num_osds")
	if err != nil {
		return 0, err
	}
	osdCount, err := strconv.Atoi(strings.TrimSpace(string(stdout)))
	if err != nil {
		return 0, err
	}
	return osdCount, nil
}

func (rook *RookImpl) GetPrepareJobName(pvcName string) (string, error) {
	stdout, _, err := utilObj.Kubectl(rook.Namespace, "get", "jobs", "-l", "ceph.rook.io/pvc="+pvcName, "-o", "jsonpath={.items[0].metadata.name}")
	if err != nil {
		return "", err
	}
	return string(stdout), nil
}

func (rook *RookImpl) getNearfullRatio(osdID int) (float64, error) {
	stdout, _, err := utilObj.Ceph(rook.Namespace, "config", "show-with-defaults", "osd."+strconv.Itoa(osdID), "-f", "json")
	if err != nil {
		return 0.0, err
	}
	stdout, _, err = utilObj.Exec(stdout, "jq", "-r", `.[] | select(.name == "mon_osd_nearfull_ratio") | .value`)
	if err != nil {
		return 0.0, err
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(string(stdout)), 64)
	if err != nil {
		return 0.0, err
	}
	return val, nil
}

func (rook *RookImpl) DeleteCephInternalOSDInfo(osdID int) error {
	_, _, err := utilObj.Ceph(rook.Namespace, "auth", "del", fmt.Sprintf("osd.%d", osdID))
	if err != nil {
		return err
	}
	stdout, _, err := utilObj.Ceph(rook.Namespace, "auth", "ls", "-f", "json")
	if err != nil {
		return err
	}
	stdout, _, err = utilObj.Exec(stdout,
		"jq", fmt.Sprintf(`.auth_dump[] | select(.entity == "osd.%d")`, osdID))
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(stdout)) != "" {
		return errors.New("failed to delete authentication record of the target OSD")
	}

	_, _, err = utilObj.Ceph(rook.Namespace, "osd", "purge", strconv.Itoa(osdID), "--yes-i-really-mean-it")
	if err != nil {
		return err
	}
	stdout, _, err = utilObj.Ceph(rook.Namespace, "osd", "tree", "-f", "json")
	if err != nil {
		return err
	}
	stdout, _, err = utilObj.Exec(stdout,
		"jq", fmt.Sprintf(`.nodes[] | select(.name == "osd.%d")`, osdID))
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(stdout)) != "" {
		return errors.New("failed to purge OSD")
	}

	return nil
}

func (rook *RookImpl) DeleteSubsidiaryResources(deployName string, deletePVC bool) error {
	pvcName, err := k8sObj.GetPVCNameOfDeploy(rook.Namespace, deployName)
	if err != nil {
		return err
	}
	fmt.Printf("pvcName = %s\n", pvcName)
	prepareJobName, err := rook.GetPrepareJobName(pvcName)
	if err != nil {
		return err
	}
	fmt.Printf("prepareJobName = %s\n", prepareJobName)

	pvcExists, err := k8sObj.PVCExists(rook.Namespace, pvcName)
	if err != nil {
		return err
	}
	if pvcExists {
		_, _, err = utilObj.Kubectl(rook.Namespace, "label", "pvc", pvcName, "ceph.rook.io/DeviceSetPVCId-")
		if err != nil {
			return err
		}

		if deletePVC {
			_, _, err = utilObj.Kubectl(rook.Namespace, "delete", "pvc", pvcName, "--wait=false")
			if err != nil {
				return err
			}
			fmt.Println("Deleted PVC successfully.")
		}
	}

	_, _, err = utilObj.Kubectl(rook.Namespace, "delete", "job", prepareJobName)
	if err != nil {
		return err
	}
	_, _, err = utilObj.Kubectl(rook.Namespace, "delete", "deployment", deployName)
	if err != nil {
		return err
	}

	return nil
}

func GetOSDDeploymentName(osdID int) string {
	return "rook-ceph-osd-" + strconv.Itoa(osdID)
}

func getSameDeviceClassOSDIDs(osdDfResults []byte, deviceClass string, osdIDOfDeviceClass int) ([]int, error) {
	stdout, _, err := utilObj.Exec(osdDfResults, "jq", fmt.Sprintf(`. | select(.device_class == "%s") | .id`, deviceClass))
	if err != nil {
		return nil, err
	}
	ret := make([]int, 0)
	osdIDsStr := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	for _, osdIDStr := range osdIDsStr {
		osdID, err := strconv.Atoi(osdIDStr)
		if err != nil {
			return nil, err
		}
		if osdID == osdIDOfDeviceClass {
			continue
		}
		ret = append(ret, osdID)
	}
	return ret, nil
}

func getValFromOSDDfResultFloat64(osdDfResult []byte, key string) (float64, error) {
	stdout, _, err := utilObj.Exec(osdDfResult, "jq", "."+key)
	if err != nil {
		return 0.0, err
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(string(stdout)), 64)
	if err != nil {
		return 0.0, err
	}
	return val, nil
}
