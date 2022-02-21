/*
Copyright 2021 The Rook Authors. All rights reserved.

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

// Heavily inspired by https://github.com/kubernetes/kubernetes/blob/master/test/e2e/framework/exec_util.go

package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecOptions passed to ExecWithOptions
type ExecOptions struct {
	Command       []string
	Namespace     string
	PodName       string
	ContainerName string
	Stdin         io.Reader
	CaptureStdout bool
	CaptureStderr bool
	// If false, whitespace in std{err,out} will be removed.
	PreserveWhitespace bool
}

// RemotePodCommandExecutor is an exec.Executor that execs every command in a remote container
// This is especially useful when the CephCluster networking type is Multus and when the Operator pod
// does not have the right network annotations.
type RemotePodCommandExecutor struct {
	ClientSet  kubernetes.Interface
	RestClient *rest.Config
}

// ExecWithOptions executes a command in the specified container,
// returning stdout, stderr and error. `options` allowed for
// additional parameters to be passed.
func (e *RemotePodCommandExecutor) ExecWithOptions(options ExecOptions) (string, string, error) {
	const tty = false

	logger.Debugf("ExecWithOptions %+v", options)

	req := e.ClientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(options.PodName).
		Namespace(options.Namespace).
		SubResource("exec").
		Param("container", options.ContainerName)
	req.VersionedParams(&v1.PodExecOptions{
		Container: options.ContainerName,
		Command:   options.Command,
		Stdin:     options.Stdin != nil,
		Stdout:    options.CaptureStdout,
		Stderr:    options.CaptureStderr,
		TTY:       tty,
	}, scheme.ParameterCodec)

	var stdout, stderr bytes.Buffer
	err := execute(http.MethodPost, req.URL(), e.RestClient, options.Stdin, &stdout, &stderr, tty)

	if options.PreserveWhitespace {
		return stdout.String(), stderr.String(), err
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// ExecCommandInContainerWithFullOutput executes a command in the
// specified container and return stdout, stderr and error
func (e *RemotePodCommandExecutor) ExecCommandInContainerWithFullOutput(ctx context.Context, appLabel, containerName, namespace string, cmd ...string) (string, string, error) {
	options := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appLabel)}
	pods, err := e.ClientSet.CoreV1().Pods(namespace).List(ctx, options)
	if err != nil {
		return "", "", err
	}

	if len(pods.Items) == 0 {
		return "", "", errors.Errorf("no pods found with selector %q", appLabel)
	}

	return e.ExecWithOptions(ExecOptions{
		Command:   cmd,
		Namespace: namespace,
		// Always pick the first pod, it's always 1 unless stretched cluster is enabled
		// TODO: if we have 2 pods we could try each result if the command fails to run due to a network partition-related error.
		PodName:            pods.Items[0].Name,
		ContainerName:      containerName,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: false,
	})
}

func execute(method string, url *url.URL, config *rest.Config, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
	exec, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		return err
	}
	return exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    tty,
	})
}

func (e *RemotePodCommandExecutor) ExecCommandInContainerWithFullOutputWithTimeout(ctx context.Context, appLabel, containerName, namespace string, cmd ...string) (string, string, error) {
	return e.ExecCommandInContainerWithFullOutput(ctx, appLabel, containerName, namespace, append([]string{"timeout", strconv.Itoa(int(CephCommandsTimeout.Seconds()))}, cmd...)...)
}
