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
package rook

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockCommand(t *testing.T) {
	assert.Equal(t, "block", blockCmd.Use)

	commands := []string{}
	for _, command := range blockCmd.Commands() {
		commands = append(commands, command.Name())
	}

	if runtime.GOOS == "linux" {
		assert.Equal(t, []string{"create", "ls", "mount", "unmount"}, commands)
	} else {
		assert.Equal(t, []string{"create", "ls"}, commands)
	}
}
