/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package exec

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kexec "k8s.io/utils/exec"
)

func Test_assertErrorType(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"unknown error type", args{err: errors.New("i don't know this error")}, ""},
		{"exec.exitError type", args{err: &exec.ExitError{Stderr: []byte("this is an error")}}, "this is an error"},
		{"exec.Error type", args{err: &exec.Error{Name: "my error", Err: errors.New("this is an error")}}, "exec: \"my error\": this is an error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assertErrorType(tt.args.err); got != tt.want {
				t.Errorf("assertErrorType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractExitCode(t *testing.T) {
	mockExecExitError := func(retcode int) *exec.ExitError {
		// we can't create an exec.ExitError directly, but we can get one by running a command that fails
		// use go's type assertion to be sure we are returning exactly *exec.ExitError
		cmd := mockExecCommandReturns("stdout", "stderr", retcode)
		err := cmd.Run()

		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to create an *exec.ExitError. instead %T", err)
		}
		return ee
	}

	expectError := true
	noError := false

	tests := []struct {
		name     string
		inputErr error
		want     int
		wantErr  bool
	}{
		{"*exec.ExitError",
			mockExecExitError(3),
			3, noError},
		/* {"exec.ExitError", // non-pointer case is impossible (won't compile) */
		{"*kexec.CodeExitError (pointer)",
			&kexec.CodeExitError{Err: errors.New("some error"), Code: 4},
			4, noError},
		{"kexec.CodeExitError (non-pointer)",
			kexec.CodeExitError{Err: errors.New("some error"), Code: 5},
			5, noError},
		{"*kerrors.StatusError",
			&kerrors.StatusError{ErrStatus: metav1.Status{Code: 6}},
			6, noError},
		/* {"kerrors.StatusError", // non-pointer case is impossible (won't compile) */
		{"unknown error type with error code extractable from error message",
			errors.New("command terminated with exit code 7"),
			7, noError},
		{"unknown error type with no extractable error code",
			errors.New("command with no extractable error code even with an int here: 8"),
			-1, expectError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractExitCode(tt.inputErr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractExitCode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractExitCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Mock an exec command where we really only care about the return values
// Inspired by: https://github.com/golang/go/blob/master/src/os/exec/exec_test.go
func mockExecCommandReturns(stdout, stderr string, retcode int) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestExecHelperProcess") //nolint:gosec //Rook controls the input to the exec arguments
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		fmt.Sprintf("GO_HELPER_PROCESS_STDOUT=%s", stdout),
		fmt.Sprintf("GO_HELPER_PROCESS_STDERR=%s", stderr),
		fmt.Sprintf("GO_HELPER_PROCESS_RETCODE=%d", retcode),
	)
	return cmd
}

// TestHelperProcess isn't a real test. It's used as a helper process.
// Inspired by: https://github.com/golang/go/blob/master/src/os/exec/exec_test.go
func TestExecHelperProcess(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	// test should set these in its environment to control the output of the test commands
	fmt.Fprint(os.Stdout, os.Getenv("GO_HELPER_PROCESS_STDOUT"))
	fmt.Fprint(os.Stderr, os.Getenv("GO_HELPER_PROCESS_STDERR"))
	rc, err := strconv.Atoi(os.Getenv("GO_HELPER_PROCESS_RETCODE"))
	if err != nil {
		panic(err)
	}
	os.Exit(rc)
}
