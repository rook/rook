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

package util

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCommandMarshallingUnmarshalling(t *testing.T) {
	type args struct {
		cmd  []string
		args []string
	}
	tests := []struct {
		name string
		args args
	}{
		{name: "one command only", args: args{cmd: []string{"one"}, args: []string{}}},
		{name: "no command or args", args: args{
			cmd: []string{}, args: []string{},
		}},
		{name: "no command w/ args", args: args{
			cmd: []string{}, args: []string{"arg1", "arg2"},
		}},
		{name: "one command w/ args", args: args{
			cmd: []string{"one"}, args: []string{"arg1", "arg2"},
		}},
		{name: "multi command only", args: args{
			cmd: []string{"one", "two", "three"}, args: []string{},
		}},
		{name: "multi command and arg", args: args{
			cmd: []string{"one", "two", "three"}, args: []string{"arg1", "arg2", "--", "arg3"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagArg, err1 := CommandToCmdReporterFlagArgument(tt.args.cmd, tt.args.args)
			if err1 != nil {
				t.Errorf("CommandToFlagArgument() error = %+v, wanted no err", err1)
			}
			cmd, args, err2 := CmdReporterFlagArgumentToCommand(flagArg)
			if err2 != nil {
				t.Errorf("FlagArgumentToCommand() error = %+v, wanted no err", err2)
			}
			if err1 != nil || err2 != nil {
				return
			}
			assert.Equal(t, tt.args.cmd, cmd)
			assert.Equal(t, tt.args.args, args)
		})
	}
}

func TestNew(t *testing.T) {
	ctx := context.TODO()
	client := fake.NewSimpleClientset
	type fields struct {
		clientset     kubernetes.Interface
		cmd           []string
		args          []string
		configMapName string
		namespace     string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{"all is well", fields{client(), []string{"cmd"}, []string{"args"}, "myConfigMap", "myNamespace"}, false},
		{"no k8s client", fields{nil, []string{"cmd"}, []string{"args"}, "myConfigMap", "myNamespace"}, true},
		{"no command", fields{client(), []string{}, []string{"args"}, "myConfigMap", "myNamespace"}, true},
		{"empty command", fields{client(), []string{""}, []string{"args"}, "myConfigMap", "myNamespace"}, true},
		{"three commands", fields{client(), []string{"one", "two", "three"}, []string{"args"}, "myConfigMap", "myNamespace"}, false},
		{"no args", fields{client(), []string{"cmd"}, []string{}, "myConfigMap", "myNamespace"}, false},
		{"empty arg", fields{client(), []string{"cmd"}, []string{""}, "myConfigMap", "myNamespace"}, false}, // an empty arg can still be valid
		{"three args", fields{client(), []string{"cmd"}, []string{"arg1", "arg2", "arg3"}, "myConfigMap", "myNamespace"}, false},
		{"no configmap name", fields{client(), []string{"cmd"}, []string{"args"}, "", "myNamespace"}, true},
		{"no namespace", fields{client(), []string{"cmd"}, []string{"args"}, "myConfigMap", ""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewCmdReporter(
				ctx,
				tt.fields.clientset,
				tt.fields.cmd,
				tt.fields.args,
				tt.fields.configMapName,
				tt.fields.namespace,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("Runner.Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				assert.NotNil(t, r)
				assert.Equal(t, tt.fields.clientset, r.clientset)
				assert.Equal(t, tt.fields.cmd, r.cmd)
				assert.Equal(t, tt.fields.args, r.args)
				assert.Equal(t, tt.fields.configMapName, r.configMapName)
				assert.Equal(t, tt.fields.namespace, r.namespace)
			}
		})
	}
}

func TestRunner_Run(t *testing.T) {
	ctx := context.TODO()
	origExecCommand := execCommand
	execCommand = mockExecCommand
	defer func() { execCommand = origExecCommand }()

	newClient := fake.NewSimpleClientset

	verifyConfigMap := func(client kubernetes.Interface, stdout, stderr, retval, cmName, namespace string) {
		cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, cmName, metav1.GetOptions{})
		fmt.Println("configmap:", cm)
		assert.NoError(t, err)
		assert.Equal(t, stdout, cm.Data[CmdReporterConfigMapStdoutKey])
		assert.Equal(t, stderr, cm.Data[CmdReporterConfigMapStderrKey])
		assert.Equal(t, retval, cm.Data[CmdReporterConfigMapRetcodeKey])
	}

	verifyCommand := func(cmd, args []string, command string) {
		err := os.Setenv("GO_HELPER_PROCESS_PRINT_COMMAND", "1")
		assert.NoError(t, err)
		defer func() { os.Unsetenv("GO_HELPER_PROCESS_PRINT_COMMAND") }()

		k8s := newClient()
		r, err := NewCmdReporter(ctx, k8s, cmd, args, "command-configmap", "command-namespace")
		assert.NoError(t, err)
		assert.NoError(t, r.Run())

		verifyConfigMap(k8s, command, "", "0", "command-configmap", "command-namespace")
	}

	verifyCommand([]string{"grep"}, []string{"-e", ".*time"}, "grep -e .*time")
	verifyCommand([]string{"ceph-volume", "inventory"}, []string{"--format=json-pretty"}, "ceph-volume inventory --format=json-pretty")
	verifyCommand([]string{"ceph-volume", "lvm", "list"}, []string{}, "ceph-volume lvm list")

	verifyOutputs := func(stdout, stderr, retcode string) {
		err := os.Setenv("GO_HELPER_PROCESS_STDOUT", stdout)
		assert.NoError(t, err)
		defer func() { os.Unsetenv("GO_HELPER_PROCESS_STDOUT") }()
		err = os.Setenv("GO_HELPER_PROCESS_STDERR", stderr)
		assert.NoError(t, err)
		defer func() { os.Unsetenv("GO_HELPER_PROCESS_STDERR") }()
		err = os.Setenv("GO_HELPER_PROCESS_RETCODE", retcode)
		assert.NoError(t, err)
		defer func() { os.Unsetenv("GO_HELPER_PROCESS_RETCODE") }()

		k8s := newClient()
		r, err := NewCmdReporter(ctx, k8s, []string{"standin-cmd"}, []string{"--some", "arg"}, "outputs-configmap", "outputs-namespace")
		assert.NoError(t, err)
		assert.NoError(t, r.Run())

		verifyConfigMap(k8s, stdout, stderr, retcode, "outputs-configmap", "outputs-namespace")
	}

	verifyOutputs("", "", "0")
	verifyOutputs("", "", "11")
	verifyOutputs("this is my stdout", "", "0")
	verifyOutputs("", "this is my stderr", "23")
	verifyOutputs("this is ...", "... mixed outputs", "23")

	// Verify that cmd-reporter won't overwrite a preexisting configmap with a different app name
	k8s := newClient()
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "preexisting-configmap",
			Namespace: "preexisting-namespace",
			Labels: map[string]string{
				k8sutil.AppAttr: "some-other-application",
			},
		},
		Data: map[string]string{},
	}
	_, err := k8s.CoreV1().ConfigMaps("preexisting-namespace").Create(ctx, cm, metav1.CreateOptions{})
	assert.NoError(t, err)
	r, err := NewCmdReporter(ctx, k8s, []string{"some-command"}, []string{"some", "args"}, "preexisting-configmap", "preexisting-namespace")
	assert.NoError(t, err)
	assert.Error(t, r.Run())
	cm, err = k8s.CoreV1().ConfigMaps("preexisting-namespace").Get(ctx, "preexisting-configmap", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotContains(t, cm.Data, CmdReporterConfigMapStdoutKey)
	assert.NotContains(t, cm.Data, CmdReporterConfigMapStderrKey)
	assert.NotContains(t, cm.Data, CmdReporterConfigMapRetcodeKey)

	// Verify that cmd-reporter WILL overwrite a preexisting configmap with cmd-reporter's app name
	k8s = newClient()
	cm = &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "preexisting-configmap",
			Namespace: "preexisting-namespace",
			Labels: map[string]string{
				k8sutil.AppAttr: CmdReporterAppName,
			},
		},
		Data: map[string]string{},
	}
	_, err = k8s.CoreV1().ConfigMaps("preexisting-namespace").Create(ctx, cm, metav1.CreateOptions{})
	assert.NoError(t, err)
	r, err = NewCmdReporter(ctx, k8s, []string{"some-command"}, []string{"some", "args"}, "preexisting-configmap", "preexisting-namespace")
	assert.NoError(t, err)
	assert.NoError(t, r.Run())
	verifyConfigMap(k8s, "", "", "0", "preexisting-configmap", "preexisting-namespace")
}

// Inspired by: https://github.com/golang/go/blob/master/src/os/exec/exec_test.go
func mockExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestCmdReporterHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...) //nolint:gosec //Rook controls the input to the exec arguments
	// the existing environment will contain variables which define the desired return from the
	// fake command which will be run.
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

// TestCmdReporterHelperProcess isn't a real test. It's used as a helper process
// for TestParameterRun.
// Inspired by: https://github.com/golang/go/blob/master/src/os/exec/exec_test.go
func TestCmdReporterHelperProcess(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}
	userCommand := args

	// test should set these in its environment to control the output of the test commands
	stdout := os.Getenv("GO_HELPER_PROCESS_STDOUT")
	stderr := os.Getenv("GO_HELPER_PROCESS_STDERR")
	retcode := os.Getenv("GO_HELPER_PROCESS_RETCODE")

	if os.Getenv("GO_HELPER_PROCESS_PRINT_COMMAND") == "1" {
		stdout = strings.Join(userCommand, " ")
		stderr = ""
		retcode = ""
	}

	if stdout != "" {
		fmt.Fprint(os.Stdout, stdout)
	}
	if stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}
	if retcode != "" {
		rc, err := strconv.Atoi(retcode)
		if err != nil {
			panic(err)
		}
		os.Exit(rc)
	}
	os.Exit(0)
}
