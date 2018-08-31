/*
Copyright 2017 The Rook Authors. All rights reserved.

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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindDevicePath(t *testing.T) {
	// set up a mock RBD sys bus file system
	mockRBDSysBusPath, err := ioutil.TempDir("", "TestFindDevicePath")
	if err != nil {
		t.Fatalf("failed to create temp rbd sys bus dir: %+v", err)
	}
	defer os.RemoveAll(mockRBDSysBusPath)
	dev0Path := filepath.Join(mockRBDSysBusPath, "devices", "3")
	os.MkdirAll(dev0Path, 0777)
	ioutil.WriteFile(filepath.Join(dev0Path, "name"), []byte("myimage1"), 0777)
	ioutil.WriteFile(filepath.Join(dev0Path, "pool"), []byte("mypool1"), 0777)
	mappedImageFile, _ := FindRBDMappedFile("myimage1", "mypool1", mockRBDSysBusPath)
	assert.Equal(t, "3", mappedImageFile)
}
