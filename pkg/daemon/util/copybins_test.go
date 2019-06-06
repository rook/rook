/*
Copyright 2019 The Rook Authors. All rights reserved.

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

func TestCopyBinaries(t *testing.T) {

	createTestBinary := func(binPath string) {
		f, err := os.Create(binPath)
		assert.NoError(t, err)
		// the binary should just be the text showing where it was copied from so we can verify it
		_, err = f.WriteString(binPath)
		assert.NoError(t, err)
		err = os.Chmod(binPath, 0755)
		assert.NoError(t, err)
		f.Close()
	}

	mkdir := func(dir string) {
		err := os.MkdirAll(dir, 0755)
		assert.NoError(t, err)
	}

	cleanDir := func(dir string) {
		err := os.RemoveAll(dir)
		assert.NoError(t, err)
		mkdir(dir)
	}

	fileText := func(binPath string) string {
		b, err := ioutil.ReadFile(binPath)
		assert.NoError(t, err)
		return string(b)
	}

	// set up the initial test directory tree without bins
	// testRootDir/
	//   bin/
	//   copy-to-dir/
	testRootDir, err := ioutil.TempDir("", "rook-cmd-reporter-copy-binaries-test")
	assert.NoError(t, err)
	defer os.RemoveAll(testRootDir)
	// create a test PATH="testRootDir/bin"
	envPath := path.Join(testRootDir, "bin")
	mkdir(envPath)
	oPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oPath)
	err = os.Setenv("PATH", envPath)
	assert.NoError(t, err)
	// create initial copy-to-dir
	copyToDir := path.Join(testRootDir, "copy-to-dir")
	mkdir(copyToDir)
	// set default rook dir for unit tests
	oDefaultRookDir := defaultRookDir
	defaultRookDir = path.Join(testRootDir, "/usr/local/bin")
	defer func() { defaultRookDir = oDefaultRookDir }()
	// set default tini dir for unit tests
	oDefaultTiniDir := defaultTiniDir
	defaultTiniDir = path.Join(testRootDir, "/")
	defer func() { defaultTiniDir = oDefaultTiniDir }()

	// expect failure if rook binary is available in path but not tini
	// testRootDir/
	//   bin/
	//     rook
	//   copy-to-dir/
	createTestBinary(path.Join(envPath, "rook"))

	err = CopyBinaries(copyToDir)
	assert.Error(t, err)
	if err == nil {
		panic(err)
	}

	err = os.Remove(path.Join(envPath, "rook"))
	assert.NoError(t, err)

	// expect failure if tini binary is available in path but not rook
	// testRootDir/
	//   bin/
	//     tini
	//   copy-to-dir/
	createTestBinary(path.Join(envPath, "tini"))

	err = CopyBinaries(copyToDir)
	assert.Error(t, err)
	if err == nil {
		panic(err)
	}

	err = os.Remove(path.Join(envPath, "tini"))
	assert.NoError(t, err)

	// expect success if both binaries are available in path
	// testRootDir/
	//   bin/
	//     rook
	//     tini
	//   copy-to-dir/
	createTestBinary(path.Join(envPath, "rook"))
	createTestBinary(path.Join(envPath, "tini"))
	cleanDir(copyToDir)

	err = CopyBinaries(copyToDir)
	assert.NoError(t, err)
	r := fileText(path.Join(copyToDir, "rook"))
	assert.Contains(t, r, path.Join(testRootDir, "bin/rook"))
	r = fileText(path.Join(copyToDir, "tini"))
	assert.Contains(t, r, path.Join(testRootDir, "bin/tini"))

	// expect success if both binaries are available in default locations AND in path
	// additionally expect that the binaries will be taken from the default location in this case
	// testRootDir/
	//   tini
	//   usr/local/bin/
	//     rook
	//   bin/
	//     rook
	//     tini
	//   copy-to-dir/
	mkdir(defaultRookDir)
	createTestBinary(path.Join(defaultRookDir, "rook"))
	createTestBinary(path.Join(defaultTiniDir, "tini"))
	cleanDir(copyToDir)

	err = CopyBinaries(copyToDir)
	assert.NoError(t, err)
	r = fileText(path.Join(copyToDir, "rook"))
	assert.Contains(t, r, path.Join(testRootDir, "usr/local/bin/rook"))
	r = fileText(path.Join(copyToDir, "tini"))
	assert.Contains(t, r, path.Join(testRootDir, "tini"))
}
