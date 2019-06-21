/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package cmdreporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// AppName is the app name reported by cmd-reporter, notably on the ConfigMap's application label.
	AppName = "cmd-reporter"

	// ConfigMapStdoutKey defines the key in the ConfigMap where stdout is reported.
	ConfigMapStdoutKey = "stdout"

	// ConfigMapStderrKey defines the key in the ConfigMap where stderr is reported.
	ConfigMapStderrKey = "stderr"

	// ConfigMapRetcodeKey defines the key in the ConfigMap where the return code is reported.
	ConfigMapRetcodeKey = "retcode"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "job-reporter-cmd")
)

// Runner is a process intended to be run in simple Kubernetes jobs. The Runner runs a
// command in a job and stores the results in a ConfigMap which can be read by the operator.
type Runner struct {
	clientset     kubernetes.Interface
	cmd           []string
	args          []string
	configMapName string
	namespace     string
}

// New creates a new Runner and returns an error if cmd, configMapName, or Namespace aren't specified.
func New(clientset kubernetes.Interface, cmd, args []string, configMapName, namespace string) (*Runner, error) {
	if clientset == nil {
		return nil, fmt.Errorf("Kubernetes client interface was not specified")
	}
	if len(cmd) == 0 || cmd[0] == "" {
		return nil, fmt.Errorf("cmd was not specified")
	}
	if configMapName == "" {
		return nil, fmt.Errorf("the config map name was not specified")
	}
	if namespace == "" {
		return nil, fmt.Errorf("the namespace must be specified")
	}
	return &Runner{
		clientset:     clientset,
		cmd:           cmd,
		args:          args,
		configMapName: configMapName,
		namespace:     namespace,
	}, nil
}

// Create a simple representation struct for a command and its args so that Go's native JSON
// (un)marshalling can be used to convert a Kubernetes representation of command+args into a string
// representation automatically without the user having to fiddle with specifying their command+args
// in string form manually.
type commandRepresentation struct {
	Cmd  []string `json:"cmd"`
	Args []string `json:"args"`
}

// CommandToFlagArgument converts a command and arguments in typical Kubernetes container format
// into a string representation of the command+args that is compatible with the job reporter's
// command line flag "--command".
// This only returns the argument to "--command" and not the "--command" text itself.
func CommandToFlagArgument(cmd []string, args []string) (string, error) {
	r := &commandRepresentation{Cmd: cmd, Args: args}
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal command+args into an argument string. %+v", err)
	}
	a := string(b)
	return a, nil
}

// FlagArgumentToCommand converts a string representation of a command compatible with the job
// reporter's command line flag "--command" into a command and arguments in typical Kubernetes
// container format, i.e., a list of command strings and a list of arguments.
// This function processes the argument to "--command" but not the "--command" text itself.
func FlagArgumentToCommand(flagArg string) (cmd []string, args []string, err error) {
	b := []byte(flagArg)
	r := &commandRepresentation{}
	if err := json.Unmarshal(b, r); err != nil {
		return []string{}, []string{}, fmt.Errorf("failed to unmarshal command from argument. %+v", err)
	}
	return r.Cmd, r.Args, nil
}

// Run a given command to completion, and store the Stdout, Stderr, and return code
// results of the command in a ConfigMap. If the ConfigMap already exists, the
// Stdout, Stderr, and return code data which may be present in the ConfigMap
// will be overwritten.
//
// If cmd-reporter succeeds in running the command to completion, no error is
// reported, even if the command's return code is nonzero (failure). Run will
// return an error if the command could not be run for any reason or if there was
// an error storing the command results into the ConfigMap. An application label
// is applied to the ConfigMap, and if the label already exists and has a
// different application's name name, this returns an error, as this may indicate
// that it is not safe for cmd-reporter to edit the ConfigMap.
func (r *Runner) Run() error {
	stdout, stderr, retcode, err := r.runCommand()
	if err != nil {
		return fmt.Errorf("system failed to run command. %+v", err)
	}

	if err := r.saveToConfigMap(stdout, stderr, retcode); err != nil {
		return fmt.Errorf("failed to save command output to ConfigMap. %+v", err)
	}

	return nil
}

var execCommand = exec.Command

func (r *Runner) runCommand() (stdout, stderr string, retcode int, err error) {
	retcode = -1 // default retcode to -1

	baseCmd := r.cmd[0]
	fullArgs := append(r.cmd[1:], r.args...)

	var capturedStdout bytes.Buffer
	var capturedStderr bytes.Buffer

	// Capture stdout and stderr, and also send both to the container stdout/stderr, similar to the
	// 'tee' command
	stdoutTee := io.MultiWriter(&capturedStdout, os.Stdout)
	stderrTee := io.MultiWriter(&capturedStderr, os.Stdout)

	c := execCommand(baseCmd, fullArgs...)
	c.Stdout = stdoutTee
	c.Stderr = stderrTee

	cmdStr := fmt.Sprintf("%s %s", c.Path, strings.Join(c.Args, " "))
	logger.Infof("running command: %s", cmdStr)

	if err := c.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// c.ProcessState.ExitCode is available with Go 1.12 and could replace if block below
			if stat, ok := exitError.Sys().(syscall.WaitStatus); ok {
				retcode = stat.ExitStatus()
			}
			// it's possible the above failed to parse the return code, so report the whole error
			logger.Infof("command finished with error: %+v", err)
		} else {
			return "", "", -1, fmt.Errorf("failed to run command [%s]. %+v", cmdStr, err)
		}
	} else {
		retcode = 0
	}

	return string(capturedStdout.Bytes()), string(capturedStderr.Bytes()), retcode, nil
}

func (r *Runner) saveToConfigMap(stdout, stderr string, retcode int) error {
	retcodeStr := fmt.Sprintf("%d", retcode)

	k8s := r.clientset
	cm, err := k8s.CoreV1().ConfigMaps(r.namespace).Get(r.configMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to determine if ConfigMap %s is preexisting. %+v", r.configMapName, err)
		}

		// the given config map doesn't exist yet, create it now
		cm = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.configMapName,
				Namespace: r.namespace,
				Labels: map[string]string{
					k8sutil.AppAttr: AppName,
				},
			},
			Data: map[string]string{
				ConfigMapStdoutKey:  stdout,
				ConfigMapStderrKey:  stderr,
				ConfigMapRetcodeKey: retcodeStr,
			},
		}

		if _, err := k8s.CoreV1().ConfigMaps(r.namespace).Create(cm); err != nil {
			return fmt.Errorf("failed to create ConfigMap %s. %+v", r.configMapName, err)
		}
		return nil
	}

	// if the operator has created the configmap with a different app name, we assume that we aren't
	// allowed to modify the ConfigMap
	if app, ok := cm.Labels[k8sutil.AppAttr]; !ok || (ok && app == "") {
		// label is unset or set to empty string
		cm.Labels[k8sutil.AppAttr] = AppName
	} else if ok && app != "" && app != AppName {
		// label is set and not equal to the cmd-reporter app name
		return fmt.Errorf("ConfigMap [%s] already has label [%s] that differs from cmd-reporter's "+
			"label [%s]; this may indicate that it is not safe for cmd-reporter to modify the ConfigMap.",
			r.configMapName, fmt.Sprintf("%s=%s", k8sutil.AppAttr, app), fmt.Sprintf("%s=%s", k8sutil.AppAttr, AppName))
	}

	for _, k := range []string{ConfigMapStdoutKey, ConfigMapStderrKey, ConfigMapRetcodeKey} {
		if v, ok := cm.Data[k]; ok {
			logger.Warningf("ConfigMap [%s] data key [%s] is already set to [%s] and will be overwritten.", r.configMapName, k, v)
		}
	}

	// given configmap already exists, update it
	cm.Data[ConfigMapStdoutKey] = stdout
	cm.Data[ConfigMapStderrKey] = stderr
	cm.Data[ConfigMapRetcodeKey] = retcodeStr

	if _, err := k8s.CoreV1().ConfigMaps(r.namespace).Update(cm); err != nil {
		return fmt.Errorf("failed to update ConfigMap %s. %+v", r.configMapName, err)
	}

	return nil
}
