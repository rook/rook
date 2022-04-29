/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

var (
	luksDump = `LUKS header information
Version:        2
Epoch:          13
Metadata area:  12288 bytes
UUID:           a97525ee-7c30-4f70-89ac-e56d48907cc5
Label:          pvc_name=set1-data-0lmdjp
Subsystem:      ceph_fsid=811e7dc0-ea13-4951-b000-24a8565d0735
Flags:          (no flags)

Data segments:
  0: crypt
        offset: 2097152 [bytes]
        length: (whole device)
        cipher: aes-xts-plain64
        sector: 512 [bytes]

Keyslots:
  0: luks2
        Key:        256 bits
        Priority:   normal
        Cipher:     aes-xts-plain64
        PBKDF:      pbkdf2
        Hash:       sha256
        Iterations: 583190
        Salt:       4f 9d 0d 0b 83 41 2f 47 b4 1f 6b 35 df 89 e0 33
                    c8 bd 27 60 22 a5 f5 02 62 94 a9 92 12 2a 4f c0
        AF stripes: 4000
        Area offset:32768 [bytes]
        Area length:131072 [bytes]
        Digest ID:  0
Tokens:
Digests:
  0: pbkdf2
        Hash:       sha256
        Iterations: 36127
        Salt:       db 98 33 3a d4 15 b6 6c 48 63 6d 7b 33 b0 7e cd
                    ef 90 8d 81 46 37 78 b4 82 37 3b 84 e8 e7 d8 1b
        Digest:     6d 86 96 05 99 4f a9 48 87 54
                    5c ef 4b 99 3b 9d fa 0b 8f 8a`
)

func TestCloseEncryptedDevice(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if command == "cryptsetup" && args[0] == "--verbose" && args[1] == "luksClose" {
			return "success", nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	err := CloseEncryptedDevice(context, "/dev/mapper/ceph-43e9efed-0676-4731-b75a-a4c42ece1bb1-xvdbr-block-dmcrypt")
	assert.NoError(t, err)
}

func TestDmsetupVersion(t *testing.T) {
	dmsetupOutput := `
Library version:   1.02.154 (2018-12-07)
Driver version:    4.40.0
`
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if command == "dmsetup" && args[0] == "version" {
			return dmsetupOutput, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	err := dmsetupVersion(context)
	assert.NoError(t, err)
}

func TestIsCephEncryptedBlock(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if command == cryptsetupBinary && args[0] == "luksDump" {
			return luksDump, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}
	context := &clusterd.Context{Executor: executor}

	t.Run("different fsid", func(t *testing.T) {
		isCephEncryptedBlock := isCephEncryptedBlock(context, "foo", "/dev/sda1")
		assert.False(t, isCephEncryptedBlock)
	})
	t.Run("same cluster", func(t *testing.T) {
		isCephEncryptedBlock := isCephEncryptedBlock(context, "811e7dc0-ea13-4951-b000-24a8565d0735", "/dev/sda1")
		assert.True(t, isCephEncryptedBlock)
	})
}
