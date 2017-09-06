package utils

import (
	"fmt"
	"strings"

	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
	"time"
)

//HelmHelper is wrapper for running helm commands
type HelmHelper struct {
	executor *exec.CommandExecutor
}

//NewHelmHelper creates a instance of HelmHelper
func NewHelmHelper() *HelmHelper {
	executor := &exec.CommandExecutor{}
	return &HelmHelper{executor: executor}

}

//Execute is wrapper for executing helm commands
func (h *HelmHelper) Execute(args ...string) (string, error) {
	result, err := h.executor.ExecuteCommandWithOutput(false, "", "helm", args...)
	if err != nil {
		logger.Errorf("Errors Encountered while executing helm command %v: %v", result, err)
		return result, fmt.Errorf("Failed to run helm commands on args %v : %v , err -> %v", args, result, err)

	}
	return result, nil

}

//GetLocalRookHelmChartVersion returns helm chart version for a give chart
func (h *HelmHelper) GetLocalRookHelmChartVersion(chartName string) (string, error) {
	cmdArgs := []string{"search", chartName}
	result, err := h.Execute(cmdArgs...)
	if err != nil {
		logger.Errorf("cannot find helm chart %v : %v", chartName, err)
		return "", fmt.Errorf("Failed to find helm chart  %v : %v", chartName, err)
	}

	if strings.Contains(result, "No results found") {
		return "", fmt.Errorf("Failed to find helm chart  %v ", chartName)
	}
	cd := strings.Replace(sys.Grep(result, chartName), "\t", " ", 2)

	return sys.Awk(cd, 2), nil
}

//InstallLocalRookHelmChart installs a give helm chart
func (h *HelmHelper) InstallLocalRookHelmChart(chartName string, deployName string, chartVersion string, namespace string) error {
	cmdArgs := []string{"install", chartName, "--name", deployName, "--version", chartVersion}
	if namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", namespace)
	}
	var result string
	var err error
	inc := 0
	for inc < RetryLoop {
		result, err = h.Execute(cmdArgs...)
		if err == nil {
			return nil
		}
		logger.Infof("helm install for %s failed %v, err ->%v", chartName, result, err)
		ls, _ := h.Execute([]string{"ls"}...)
		logger.Infof("Helm ls result : %v", ls)
		ss, _ := h.Execute([]string{"search"}...)
		logger.Infof("Helm search result : %v", ss)
		rl, _ := h.Execute([]string{"repo", "list"}...)
		logger.Infof("Helm repo list result : %v", rl)

		inc++
		time.Sleep(RetryInterval * time.Second)
	}

	logger.Errorf("cannot install helm chart with name : %v, version: %v, namespace: %v  - %v , err: %v", chartName, chartVersion, namespace, result, err)
	return fmt.Errorf("cannot install helm chart with name : %v, version: %v, namespace: %v - %v, err: %v", chartName, chartVersion, namespace, result, err)
}

//DeleteLocalRookHelmChart uninstalls a give helm deploy
func (h *HelmHelper) DeleteLocalRookHelmChart(deployName string) error {
	cmdArgs := []string{"delete", "--purge", deployName}
	_, err := h.Execute(cmdArgs...)
	if err != nil {
		logger.Errorf("cannot delete helm chart with name  %v : %v", deployName, err)
		return fmt.Errorf("Failed to delete helm chart with name  %v : %v", deployName, err)
	}

	return nil
}
