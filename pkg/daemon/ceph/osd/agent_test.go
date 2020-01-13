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

package osd

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestKeyring(t *testing.T, configRoot string, args []string) {
	var configDir string
	if len(args) > 5 && strings.HasPrefix(args[5], "--id") {
		configDir = filepath.Join(configRoot, "osd") + args[5][5:]
		err := os.MkdirAll(configDir, 0744)
		assert.Nil(t, err)
		err = ioutil.WriteFile(path.Join(configDir, "keyring"), []byte("mykeyring"), 0644)
		assert.Nil(t, err)
	}
}
