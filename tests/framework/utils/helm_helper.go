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
	"os"
	"path"
	"path/filepath"

	"gopkg.in/yaml.v2"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/util/exec"
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
	result, err := h.executor.ExecuteCommandWithOutput(exec.CephCommandsTimeout, h.HelmPath, args...)
	if err != nil {
		logger.Errorf("Errors Encountered while executing helm command %v: %v", result, err)
		return result, fmt.Errorf("Failed to run helm command on args %v : %v , err -> %v", args, result, err)

	}
	return result, nil

}

func createValuesFile(path string, values map[string]interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create values file: %v", err)
	}
	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}()

	output, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("could not serialize values file: %v", err)
	}

	logger.Debugf("Writing values file %v: \n%v", path, string(output))
	if _, err := f.Write(output); err != nil {
		return fmt.Errorf("could not write values file: %v", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("could not flush values file")
	}

	return nil
}

// InstallLocalHelmChart installs a give helm chart
func (h *HelmHelper) InstallLocalHelmChart(upgrade bool, namespace, chart string, values map[string]interface{}) error {
	rootDir, err := FindRookRoot()
	if err != nil {
		return errors.Wrap(err, "failed to find rook root")
	}
	var cmdArgs []string
	chartDir := path.Join(rootDir, fmt.Sprintf("deploy/charts/%s/", chart))
	if upgrade {
		cmdArgs = []string{"upgrade"}
	} else {
		cmdArgs = []string{"install", "--create-namespace"}
	}
	cmdArgs = append(cmdArgs, chart, chartDir)
	if namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", namespace)
	}

	err = h.installChart(cmdArgs, values)
	if err != nil {
		return fmt.Errorf("failed to install local helm chart %s with in namespace: %v, err=%v", chart, namespace, err)
	}
	return nil
}

func (h *HelmHelper) InstallVersionedChart(namespace, chart, version string, values map[string]interface{}) error {

	logger.Infof("adding rook-release helm repo")
	cmdArgs := []string{"repo", "add", "rook-release", "https://charts.rook.io/release"}
	_, err := h.Execute(cmdArgs...)
	if err != nil {
		// Continue on error in case the repo already was added
		logger.Warningf("failed to add repo rook-release, err=%v", err)
	}

	logger.Infof("installing helm chart %s with version %s", chart, version)
	cmdArgs = []string{"install", "--create-namespace", chart, "rook-release/" + chart, "--version=" + version}
	if namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", namespace)
	}

	err = h.installChart(cmdArgs, values)
	if err != nil {
		return fmt.Errorf("failed to install helm chart %s with version %s in namespace: %v, err=%v", chart, version, namespace, err)
	}
	return nil
}

func (h *HelmHelper) installChart(cmdArgs []string, values map[string]interface{}) error {
	if values != nil {
		testValuesPath := "values-test.yaml"
		if err := createValuesFile(testValuesPath, values); err != nil {
			return fmt.Errorf("error creating values file: %v", err)
		}
		defer func() {
			_ = os.Remove(testValuesPath)
		}()

		cmdArgs = append(cmdArgs, "-f", testValuesPath)
	}

	result, err := h.Execute(cmdArgs...)
	if err != nil {
		logger.Errorf("failed to install chart. result=%s, err=%v", result, err)
		return err
	}
	return nil
}

// DeleteLocalRookHelmChart uninstalls a give helm deploy
func (h *HelmHelper) DeleteLocalRookHelmChart(namespace, deployName string) error {
	cmdArgs := []string{"delete", "-n", namespace, deployName}
	_, err := h.Execute(cmdArgs...)
	if err != nil {
		logger.Errorf("could not delete helm chart with name  %v : %v", deployName, err)
		return fmt.Errorf("Failed to delete helm chart with name  %v : %v", deployName, err)
	}

	return nil
}

func FindRookRoot() (string, error) {
	const folderToFind = "tests"
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to find current working directory. %v", err)
	}
	parentPath := workingDirectory
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to find user home directory. %v", err)
	}
	for parentPath != userHome {
		fmt.Printf("parent path = %s\n", parentPath)
		_, err := os.Stat(path.Join(parentPath, folderToFind))
		if os.IsNotExist(err) {
			parentPath = filepath.Dir(parentPath)
			continue
		}
		return parentPath, nil
	}

	return "", fmt.Errorf("rook root not found above directory %s", workingDirectory)
}
