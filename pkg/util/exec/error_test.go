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

package exec

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestExitStatus(t *testing.T) {
	e := &exec.ExitError{ProcessState: &os.ProcessState{}}
	c := &CephCLIError{err: e}

	type args struct {
		err error
	}
	tests := []struct {
		name  string
		args  args
		want  int
		want1 bool
	}{
		{"error type is unknown", args{err: errors.New("unknownerror")}, 0, false},
		{"error type is ExitError", args{err: e}, 0, true},
		{"error type is CephCLIError and contains ExitError ", args{err: c}, 0, true},
		{"error type is CephCLIError and does not contain ExitError", args{err: &CephCLIError{err: errors.New("foo")}}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := ExitStatus(tt.args.err)
			if got != tt.want {
				t.Errorf("ExitStatus() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("ExitStatus() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
