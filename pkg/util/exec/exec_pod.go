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
	"os"
	"strconv"
	"strings"
	"time"

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

// redactedForLogging returns a copy of the options whose Command is safe to log.
// This path formats the whole struct rather than going through FormatCommand, so
// it needs its own call into redactArgs; the receiver's own Command is the vector
// that gets executed and is left untouched.
func (o ExecOptions) redactedForLogging() ExecOptions {
	redacted := o
	redacted.Command = redactArgs(o.Command)

	return redacted
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
func (e *RemotePodCommandExecutor) ExecWithOptions(ctx context.Context, options ExecOptions) (string, string, error) {
	const tty = false

	logger.Debugf("ExecWithOptions %+v", options.redactedForLogging())
	logger.Tracef("ExecWithOptions %+v", options) // only trace(insecure) logged because Command carries the secret flag value verbatim

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
	err := execute(ctx, http.MethodPost, req.URL(), e.RestClient, options.Stdin, &stdout, &stderr, tty)

	if options.PreserveWhitespace {
		return stdout.String(), stderr.String(), err
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// ExecCommandInContainerWithFullOutput executes a command in the
// specified container and returns stdout, stderr and error
func (e *RemotePodCommandExecutor) ExecCommandInContainerWithFullOutput(ctx context.Context, appLabel, containerName, namespace string, cmd ...string) (string, string, error) {
	options := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appLabel)}
	pods, err := e.ClientSet.CoreV1().Pods(namespace).List(ctx, options)
	if err != nil {
		return "", "", err
	}

	if len(pods.Items) == 0 {
		return "", "", errors.Errorf("no pods found with selector %q", appLabel)
	}

	return e.ExecWithOptions(ctx, ExecOptions{
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

func execute(ctx context.Context, method string, url *url.URL, config *rest.Config, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
	exec, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		return sanitizeRequestError(url, err)
	}
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    tty,
	})

	return sanitizeRequestError(url, err)
}

// commandQueryParam is the query parameter that v1.PodExecOptions.Command, the
// command's whole argv, is marshalled into.
const commandQueryParam = "command"

// sanitizeRequestError keeps the exec request's argv out of err's message.
//
// A transport failure is reported by formatting the entire request URL into the
// error text, and that URL carries the argv in its "command" parameters. The
// resulting error is not logged at DEBUG but returned: it reaches logger.Errorf
// and a Warning Event through pkg/operator/ceph/reporting, so a failed
// 'realm pull' on a Multus cluster would publish the realm's secret key to
// anyone who can read Events. That is a weaker permission than pods/log.
//
// The argv cannot be removed from such an error structurally, because the URL is
// no longer a value by the time it gets here: k8s.io/client-go/transport/spdy
// formats the *url.Error with %v, which bakes its text into a plain error with
// nothing left to unwrap. The message itself has to be rewritten, which replaces
// the error rather than wrapping it, so that no %+v or errors.Cause can reach
// past the sanitized text to the original.
func sanitizeRequestError(u *url.URL, err error) error {
	if err == nil || u == nil {
		return err
	}

	raw := u.String()
	msg := err.Error()
	if !strings.Contains(msg, raw) {
		// nothing to rewrite, so keep the error as it is: callers including
		// ExtractExitCode depend on its type, and an exit-status error does not
		// name the URL
		return err
	}

	return errors.New(strings.ReplaceAll(msg, raw, redactQueryArgv(u)))
}

// redactQueryArgv renders u with its "command" parameters put through
// redactArgs. Those parameters are the argv in order, so the same redaction
// serves here as in a log line: only a secret flag's value is replaced, and the
// command that failed stays readable. The rest of the request - host, namespace,
// pod, container - is left alone, since diagnosing a failed exec needs all of it.
func redactQueryArgv(u *url.URL) string {
	query := u.Query()
	command, ok := query[commandQueryParam]
	if !ok {
		return u.String()
	}
	query[commandQueryParam] = redactArgs(command)

	redacted := *u
	redacted.RawQuery = query.Encode()

	return redacted.String()
}

func (e *RemotePodCommandExecutor) ExecCommandInContainerWithFullOutputWithTimeout(ctx context.Context, appLabel, containerName, namespace string, timeout time.Duration, cmd ...string) (string, string, error) {
	return e.ExecCommandInContainerWithFullOutput(ctx, appLabel, containerName, namespace, append([]string{"timeout", strconv.Itoa(int(timeout.Seconds()))}, cmd...)...)
}

func (e *RemotePodCommandExecutor) CopyLocalFileToContainer(ctx context.Context, appLabel, containerName, namespace string, srcPath, dstPath string) error {
	options := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appLabel)}
	pods, err := e.ClientSet.CoreV1().Pods(namespace).List(ctx, options)
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return errors.Errorf("no pods found with selector %q", appLabel)
	}
	file, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()
	stdOut, stdErr, err := e.ExecWithOptions(ctx, ExecOptions{
		Command:            []string{"sh", "-c", fmt.Sprintf("cat - > %s", quoteArg(dstPath))},
		Namespace:          namespace,
		PodName:            pods.Items[0].Name,
		ContainerName:      containerName,
		Stdin:              file,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: false,
	})
	if err != nil {
		return fmt.Errorf("%w: unable to copy file, stdOut=%q, stdErr=%q", err, stdOut, stdErr)
	}
	return nil
}
