/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package test provides common resources useful for testing many Ceph daemons. This includes
// functions for testing that resources match what is expected.
package test

import (
	"strings"
	"testing"

	"github.com/coreos/pkg/capnslog"
	optest "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
)

// ContainerTestDefinition defines which k8s container values to test and what those values should
// be. Any definition item may be nil, and a nil value will prevent the item from being tested in
// the container test.
type ContainerTestDefinition struct {
	// Image is the name of the container image
	Image *string
	// Command is the container command
	Command []string
	// Args is a list of expected arguments in the same format as the expected arguments from
	// the ArgumentsMatchExpected() function.
	Args [][]string
	// InOrderArgs is a map of argument position (int) to the argument itself (string). If the
	// "third" arg must be exactly the third argument this should be: InOrderArgs[2]="third"
	InOrderArgs map[int]string
	// VolumeMountNames is a list of volume mount names which must be mounted in the container
	VolumeMountNames []string
	// EnvCount is the number of 'Env' variables the container should define
	EnvCount *int
	// Ports is a list of ports the container must define. This list is in order, and each port's
	// 'ContainerPort' and 'Protocol' are tested for equality.
	// Note: port's in general aren't order-dependent, but there is not yet a method to test for
	//       the existence of a port in a list of ports without caring about order.
	Ports []v1.ContainerPort
	// IsPrivileged tests if the container is privileged (true) or unprivileged (false)
	IsPrivileged *bool
}

// TestContainer tests that a container matches the container test definition. Moniker is a name
// given to the container for identifying it in tests. Cont is the container to be tested.
// Logger is the logger to output logs to.
func (d *ContainerTestDefinition) TestContainer(
	t *testing.T,
	moniker string,
	cont *v1.Container,
	logger *capnslog.PackageLogger,
) {

	if d.Image != nil {
		assert.Equal(t, *d.Image, cont.Image)
	}
	logCommandWithArgs(moniker, cont.Command, cont.Args, logger)
	if d.Command != nil {
		assert.Equal(t, len(d.Command), len(cont.Command))
		assert.Equal(t, strings.Join(d.Command, " "), strings.Join(cont.Command, " "))
	}
	if d.Args != nil {
		assert.Nil(t, optest.ArgumentsMatchExpected(cont.Args, d.Args))
	}
	if d.InOrderArgs != nil {
		for argNum, arg := range d.InOrderArgs {
			assert.Equal(t, cont.Args[argNum], arg)
		}
	}
	if d.VolumeMountNames != nil {
		assert.Equal(t, len(d.VolumeMountNames), len(cont.VolumeMounts))
		for _, n := range d.VolumeMountNames {
			assert.Nil(t, optest.VolumeMountExists(n, cont.VolumeMounts))
		}
	}
	if d.EnvCount != nil {
		assert.Equal(t, *d.EnvCount, len(cont.Env))
	}
	if d.Ports != nil {
		assert.Equal(t, len(d.Ports), len(cont.Ports))
		for i, p := range d.Ports {
			assert.Equal(t, p.ContainerPort, cont.Ports[i].ContainerPort)
			assert.Equal(t, p.Protocol, cont.Ports[i].Protocol)
		}
	}
	if d.IsPrivileged != nil {
		assert.Equal(t, *d.IsPrivileged, *cont.SecurityContext.Privileged)
	}
}

// logCommandWithArgs writes a command and its arguments to the logger with a moniker to identify it
func logCommandWithArgs(moniker string, command, args []string, logger *capnslog.PackageLogger) {
	logger.Infof("%s command : %s %s", moniker, strings.Join(command, " "), strings.Join(args, " "))
}
