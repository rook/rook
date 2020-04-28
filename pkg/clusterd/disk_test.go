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
package clusterd

import (
	"testing"

	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestDiscoverDevices(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommand: func(command string, arg ...string) error {
			logger.Infof("mock execute. %s", command)
			return nil
		},
		MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
			logger.Infof("mock execute with output. %s", command)
			return "", nil
		},
	}
	devices, err := DiscoverDevices(executor)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(devices))
}

func TestIgnoreDevice(t *testing.T) {
	cases := map[string]bool{
		"rbd0":    true,
		"rbd2":    true,
		"rbd9913": true,
		"rbd32p1": true,
		"rbd0a2":  false,
		"rbd":     false,
		"arbd0":   false,
		"rbd0x":   false,
	}
	for dev, expected := range cases {
		assert.Equal(t, expected, ignoreDevice(dev), dev)
	}
}
