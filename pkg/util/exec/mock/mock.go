/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package mock

import (
	"context"
	"testing"

	"github.com/rook/rook/pkg/util/exec"
	"github.com/stretchr/testify/mock"
)

type RemotePodCommandExecutor struct {
	mock.Mock

	// this is optional. if this is given, it is used to output cmd logs to the test output
	T *testing.T
}

func (m *RemotePodCommandExecutor) ExecCommandInContainerWithFullOutput(ctx context.Context, appLabel string, containerName string, namespace string, cmd ...string) (string, string, error) {
	p := []interface{}{ctx, appLabel, containerName, namespace}
	for _, c := range cmd {
		p = append(p, c)
	}
	args := m.Called(p...)
	if m.T != nil {
		m.T.Log("cmd:", p, "out:", args)
	}
	return args.String(0), args.String(1), args.Error(2)
}

func (m *RemotePodCommandExecutor) ExecCommandInContainerWithFullOutputWithTimeout(ctx context.Context, appLabel string, containerName string, namespace string, cmd ...string) (string, string, error) {
	p := []interface{}{ctx, appLabel, containerName, namespace}
	for _, c := range cmd {
		p = append(p, c)
	}
	args := m.Called(p...)
	if m.T != nil {
		m.T.Log("cmd:", p, "out:", args)
	}
	return args.String(0), args.String(1), args.Error(2)
}

func (m *RemotePodCommandExecutor) ExecWithOptions(ctx context.Context, options exec.ExecOptions) (string, string, error) {
	args := m.Called(ctx, options)
	if m.T != nil {
		m.T.Log("cmd:", ctx, options, "out:", args)
	}
	return args.String(0), args.String(1), args.Error(2)
}
