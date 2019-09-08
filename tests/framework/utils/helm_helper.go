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
	"fmt"
	"strings"

	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
)

// HelmHelper is wrapper for running helm commands
type HelmHelper struct {
	executor *exec.CommandExecutor
	HelmPath string
}

// NewHelmHelper creates a instance of HelmHelper
func NewHelmHelper(helmPath string) *HelmHelper {
	executor := &exec.CommandExecutor{}
	return &HelmHelper{executor: executor, HelmPath: helmPath}

}

// Execute is wrapper for executing helm commands
func (h *HelmHelper) Execute(args ...string) (string, error) {
	result, err := h.executor.ExecuteCommandWithOutput(false, "", h.HelmPath, args...)
	if err != nil {
		logger.Errorf("Errors Encountered while executing helm command %v: %v", result, err)
		return result, fmt.Errorf("Failed to run helm commands on args %v : %v , err -> %v", args, result, err)

	}
	return result, nil

}

// GetLocalRookHelmChartVersion returns helm chart version for a given chart
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

	version := ""
	slice := strings.Fields(sys.Grep(result, chartName))
	if len(slice) >= 2 {
		version = slice[1]
	}
	if version == "" {
		return "", fmt.Errorf("Failed to find version for helm chart %v", chartName)
	}
	return version, nil
}

// InstallLocalRookHelmChart installs a give helm chart
func (h *HelmHelper) InstallLocalRookHelmChart(chartName string, deployName string, chartVersion string, namespace, chartSettings string) error {
	cmdArgs := []string{"install", chartName, "--name", deployName, "--version", chartVersion}
	if namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", namespace)
	}
	if chartSettings != "" {
		cmdArgs = append(cmdArgs, "--set", chartSettings)
	}
	var result string
	var err error

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

	logger.Errorf("cannot install helm chart with name : %v, version: %v, namespace: %v  - %v , err: %v", chartName, chartVersion, namespace, result, err)
	return fmt.Errorf("cannot install helm chart with name : %v, version: %v, namespace: %v - %v, err: %v", chartName, chartVersion, namespace, result, err)
}

// DeleteLocalRookHelmChart uninstalls a give helm deploy
func (h *HelmHelper) DeleteLocalRookHelmChart(deployName string) error {
	cmdArgs := []string{"delete", "--purge", deployName}
	_, err := h.Execute(cmdArgs...)
	if err != nil {
		logger.Errorf("cannot delete helm chart with name  %v : %v", deployName, err)
		return fmt.Errorf("Failed to delete helm chart with name  %v : %v", deployName, err)
	}

	return nil
}
