package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxRetryCount = 600
	retryPeriod   = 10
)

type K8s interface {
	DeployExists(ns, deployName string) (bool, error)
	GetPodNames(ns string, label string) ([]string, error)
	WaitForPodCountChange(ns, label string, desiredCount int) error
	GetPVCNameOfDeploy(ns, deployName string) (string, error)
	PVCExists(ns, pvcName string) (bool, error)
}

type K8sImpl struct {
}

func (k8s *K8sImpl) DeployExists(ns, deployName string) (bool, error) {
	stdout, _, err := utilObj.Kubectl(ns, "get", "deploy", "-o", "json")
	if err != nil {
		return false, err
	}
	stdout, _, err = utilObj.Exec(stdout, "jq", "-r", fmt.Sprintf(`.items[] | select(.metadata.name == "%s") | .metadata.name`, deployName))
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(string(stdout)) != deployName {
		return false, nil
	}
	return true, nil
}

func (k8s *K8sImpl) GetPodNames(ns string, label string) ([]string, error) {
	stdout, _, err := utilObj.Kubectl(ns, "get", "pod", "-l", label, "-o", "jsonpath={.items[*].metadata.name}")
	if err != nil {
		return nil, err
	}
	ret := strings.Split(strings.TrimSpace(string(stdout)), " ")
	if len(ret) == 1 && ret[0] == "" {
		ret = nil
	}
	return ret, nil
}

func (k8s *K8sImpl) WaitForPodCountChange(ns, label string, desiredCount int) error {
	for retryCount := 0; retryCount < maxRetryCount; retryCount++ {
		fmt.Printf("Waiting for changing the number of labeled pods (%s) to the desired count (%d)\n", label, desiredCount)
		podNames, err := k8s.GetPodNames(ns, label)
		if err != nil {
			return err
		}
		if len(podNames) == desiredCount {
			fmt.Printf("The number of labeled pods (%s) has changed successfully.\n", label)
			return nil
		}
		time.Sleep(time.Second * retryPeriod)
	}
	return errors.New("WaitForPodCountChange timeout")
}

func (k8s *K8sImpl) GetPVCNameOfDeploy(ns, deployName string) (string, error) {
	stdout, _, err := utilObj.Kubectl(ns, "get", "deploy", deployName, "-o", `jsonpath={.metadata.labels.ceph\.rook\.io/pvc}`)
	if err != nil {
		return "", err
	}
	return string(stdout), nil
}

func (k8s *K8sImpl) PVCExists(ns, pvcName string) (bool, error) {
	stdout, _, err := utilObj.Kubectl(ns, "get", "pvc", pvcName, "--no-headers", "--ignore-not-found=true")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(stdout)) != "", nil
}
