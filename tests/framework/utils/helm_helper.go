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
	result, err := h.executor.ExecuteCommandWithOutput(h.HelmPath, args...)
	if err != nil {
		logger.Errorf("Errors Encountered while executing helm command %v: %v", result, err)
		return result, fmt.Errorf("Failed to run helm commands on args %v : %v , err -> %v", args, result, err)

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

// InstallLocalRookHelmChart installs a give helm chart
func (h *HelmHelper) InstallLocalRookHelmChart(namespace, chart string, values map[string]interface{}) error {
	rootDir, err := FindRookRoot()
	if err != nil {
		return errors.Wrap(err, "failed to find rook root")
	}
	chartDir := path.Join(rootDir, fmt.Sprintf("deploy/charts/%s/", chart))
	cmdArgs := []string{"install", "--create-namespace", chart, chartDir}
	if namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", namespace)
	}

	if values != nil {
		testValuesPath := path.Join(chartDir, "values-test.yaml")
		if err := createValuesFile(testValuesPath, values); err != nil {
			return fmt.Errorf("error creating values file: %v", err)
		}
		defer func() {
			_ = os.Remove(testValuesPath)
		}()

		cmdArgs = append(cmdArgs, "-f", testValuesPath)
	}

	var result string
	result, err = h.Execute(cmdArgs...)
	if err == nil {
		return nil
	}

	logger.Errorf("could not install helm chart with name : %v, namespace: %v  - %v , err: %v", chart, namespace, result, err)
	return fmt.Errorf("could not install helm chart with name : %v, namespace: %v - %v, err: %v", chart, namespace, result, err)
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
