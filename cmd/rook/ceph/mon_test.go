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
package ceph

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareKeyring(t *testing.T) {
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)

	keyring := "mytestcomplicatedsecurekeyring"
	contents := fmt.Sprintf(`
	[mon.]
	  key = %s
	`, keyring)
	dataPath := path.Join(configDir, "data")
	keyringPath := path.Join(dataPath, "keyring")
	err := os.MkdirAll(dataPath, 0755)
	assert.Nil(t, err)
	err = ioutil.WriteFile(keyringPath, []byte(contents), 0644)
	assert.Nil(t, err)

	err = compareMonSecret(keyring, configDir)
	assert.Nil(t, err)

	err = compareMonSecret("badkeyring", configDir)
	assert.NotNil(t, err)
}
