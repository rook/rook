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
package util

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrimMachineID(t *testing.T) {
	testTrimNodeID(t, " 123 		", "123")
	testTrimNodeID(t, " 1234567890", "1234567890")
	testTrimNodeID(t, " 123456789012", "123456789012")
	testTrimNodeID(t, " 1234567890123", "123456789012")
	testTrimNodeID(t, "1234567890123", "123456789012")
	testTrimNodeID(t, "123456789012345678", "123456789012")
}

func testTrimNodeID(t *testing.T, input, expected string) {
	result := trimNodeID(input)
	assert.Equal(t, expected, result)
}

func TestRandomID(t *testing.T) {
	id := randomID()
	assert.Equal(t, 12, len(id))
	assert.NotEqual(t, "000000000000", id)
}

// Test loading the IDs in reverse priority order
func TestLoadNodeID(t *testing.T) {
	dataDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(dataDir)
	cachedIDFile := path.Join(dataDir, idFilename)
	defer os.Remove(cachedIDFile)

	// generate a random id
	id, save, err := loadNodeID(dataDir)
	assert.Nil(t, err)
	assert.Equal(t, 12, len(id))
	assert.NotEqual(t, "000000000000", id)
	assert.True(t, save)

	// return an error if the cached id is empty
	err = ioutil.WriteFile(cachedIDFile, []byte(" "), 0644)
	assert.Nil(t, err)
	id, save, err = loadNodeID(dataDir)
	assert.NotNil(t, err)
	assert.False(t, save)

	// return the id in the cached file
	cachedID := "543211234567890"
	err = ioutil.WriteFile(cachedIDFile, []byte(cachedID), 0644)
	assert.Nil(t, err)
	id, save, err = loadNodeID(dataDir)
	assert.Nil(t, err)
	assert.False(t, save)
	assert.Equal(t, cachedID, id)
}

func TestSaveNodeID(t *testing.T) {
	dataDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(dataDir)
	cachedIDFile := path.Join(dataDir, idFilename)

	// confirm that the node id is persisted
	id, err := LoadPersistedNodeID(dataDir)
	assert.Nil(t, err)

	cachedID, err := ioutil.ReadFile(cachedIDFile)
	assert.Nil(t, err)
	assert.Equal(t, id, string(cachedID))
}
