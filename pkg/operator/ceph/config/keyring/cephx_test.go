/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package keyring

import (
	"reflect"
	"testing"

	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/stretchr/testify/assert"
)

func TestShouldRotateCephxKeys(t *testing.T) {
	// commit IDs will ensure they are being ignored as part of comparison to keyCephVersion status
	v20_2_0 := version.CephVersion{Major: 20, Minor: 2, Extra: 0, CommitID: "ababababababa"}
	v20_2_2 := version.CephVersion{Major: 20, Minor: 2, Extra: 2, CommitID: "ababababababa"}

	type test struct {
		name             string
		cfg              v1.CephxConfig
		imageCephVersion version.CephVersion
		status           v1.CephxStatus
		want             bool
		wantErr          bool
	}

	tests := []test{
		{"policy unset", v1.CephxConfig{KeyRotationPolicy: "", KeyGeneration: 5}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 0, KeyCephVersion: "20.2.0-0"}, false, false},
		{"policy disabled", v1.CephxConfig{KeyRotationPolicy: "Disabled", KeyGeneration: 5}, v20_2_2, v1.CephxStatus{KeyGeneration: 6, KeyCephVersion: "20.2.0-0"}, false, false},
		{"policy disabled, keyType set", v1.CephxConfig{KeyRotationPolicy: "Disabled", KeyGeneration: 5, KeyType: "aes256k"}, v20_2_2, v1.CephxStatus{KeyGeneration: 6, KeyCephVersion: "20.2.0-0"}, false, false},
		//
		{"policy generation, 3<4", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 3}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0"}, false, false},
		{"policy generation, 5>4", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 5}, v20_2_0, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.2-0"}, true, false},
		{"policy generation, 4=4", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 4}, v20_2_2, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0"}, false, false},
		{"policy generation, 5>0", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 5}, v20_2_0, v1.CephxStatus{KeyGeneration: 0, KeyCephVersion: "20.2.2-0"}, true, false},
		{"policy generation, 0=0", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, v20_2_2, v1.CephxStatus{KeyGeneration: 0, KeyCephVersion: "20.2.0-0"}, false, false},
		{"policy generation, 1>0, uninit", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, v20_2_2, v1.CephxStatus{KeyGeneration: 0, KeyCephVersion: "Uninitialized"}, false, false},
		{"policy generation, 2>0, uninit", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, v20_2_2, v1.CephxStatus{KeyGeneration: 0, KeyCephVersion: "Uninitialized"}, false, false},
		{"policy generation, 3<4, keyType set", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 3, KeyType: "aes256k"}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0"}, true, false},
		{"policy generation, 5>4, keyType set", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 5, KeyType: "aes256k"}, v20_2_0, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.2-0"}, true, false},
		{"policy generation, 4=4, keyType set", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 4, KeyType: "aes256k"}, v20_2_2, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0"}, true, false},
		{"policy generation, 5>0, keyType set", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 5, KeyType: "aes256k"}, v20_2_0, v1.CephxStatus{KeyGeneration: 0, KeyCephVersion: "20.2.2-0"}, true, false},
		{"policy generation, 0=0, keyType set", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0, KeyType: "aes256k"}, v20_2_2, v1.CephxStatus{KeyGeneration: 0, KeyCephVersion: "20.2.0-0"}, true, false},
		{"policy generation, 1>0, keyType set, uninit", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0, KeyType: "aes256k"}, v20_2_2, v1.CephxStatus{KeyGeneration: 0, KeyCephVersion: "Uninitialized"}, false, false},
		{"policy generation, 2>0, keyType set, uninit", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0, KeyType: "aes256k"}, v20_2_2, v1.CephxStatus{KeyGeneration: 0, KeyCephVersion: "Uninitialized"}, false, false},
		//
		// in unlikely event ceph version in image is unknown, do nothing, even if existing key version is unknown
		{"policy ceph update, unk vs unk", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: ""}, false, false},
		{"policy ceph update, unk vs uninit", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "Uninitialized"}, false, false},
		{"policy ceph update, 20.2.0 vs unk", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0}, v20_2_0, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: ""}, true, false},
		{"policy ceph update, 20.2.2 vs unk", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0}, v20_2_2, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: ""}, true, false},
		{"policy ceph update, 20.2.0 vs uninit", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0}, v20_2_0, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "Uninitialized"}, false, false},
		{"policy ceph update, unk vs 20.20.0", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0"}, false, false},
		{"policy ceph update, 20.2.2 vs 20.2.0", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0}, v20_2_2, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0"}, true, false},
		{"policy ceph update, 20.2.0 vs 20.2.2", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0}, v20_2_0, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.2-0"}, false, false},
		{"policy ceph update, 20.2.2 vs 20.2.2", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0}, v20_2_2, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.2-0"}, false, false},
		{"policy ceph update, unk vs unk, keyType set", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0, KeyType: "aes256k"}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: ""}, true, false},
		{"policy ceph update, unk vs uninit, keyType set", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0, KeyType: "aes256k"}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "Uninitialized"}, false, false},
		{"policy ceph update, 20.2.0 vs unk, keyType set", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0, KeyType: "aes256k"}, v20_2_0, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: ""}, true, false},
		{"policy ceph update, 20.2.2 vs unk, keyType set", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0, KeyType: "aes256k"}, v20_2_2, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: ""}, true, false},
		{"policy ceph update, 20.2.0 vs uninit, keyType set", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0, KeyType: "aes256k"}, v20_2_0, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "Uninitialized"}, false, false},
		{"policy ceph update, unk vs 20.20.0, keyType set", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0, KeyType: "aes256k"}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0"}, true, false},
		{"policy ceph update, 20.2.2 vs 20.2.0, keyType set", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0, KeyType: "aes256k"}, v20_2_2, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0"}, true, false},
		{"policy ceph update, 20.2.0 vs 20.2.2, keyType set", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0, KeyType: "aes256k"}, v20_2_0, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.2-0"}, true, false},
		{"policy ceph update, 20.2.2 vs 20.2.2, keyType set", v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate", KeyGeneration: 0, KeyType: "aes256k"}, v20_2_2, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.2-0"}, true, false},
		//
		{"invalid policy", v1.CephxConfig{KeyRotationPolicy: "InVaLiD", KeyGeneration: 0}, v20_2_2, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0"}, false, true},
	}

	keyTypeTests := []test{
		{"keyType unset vs set", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration"}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0", KeyType: "anything"}, false, false},
		{"keyType set vs unset", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyType: "anything"}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0", KeyType: ""}, true, false},
		{"keyType set vs same", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyType: "something"}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0", KeyType: "something"}, false, false},
		{"keyType set vs other", v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyType: "something"}, version.CephVersion{}, v1.CephxStatus{KeyGeneration: 4, KeyCephVersion: "20.2.0-0", KeyType: "somethingelse"}, true, false},
	}

	// include keyType tests in main test list
	tests = append(tests, keyTypeTests...)

	t.Run("can support", func(t *testing.T) {
		for _, tt := range tests {
			// run all tests for case where ceph version does support rotation
			t.Run(tt.name, func(t *testing.T) {
				ignoreKeyType := false // for these tests, don't ignore key type
				got, err := ShouldRotateCephxKeys(tt.cfg, v20_2_2, tt.imageCephVersion, tt.status, ignoreKeyType)
				if (err != nil) != tt.wantErr {
					t.Errorf("ShouldRotateCephxKeys() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("ShouldRotateCephxKeys() = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("can support backports", func(t *testing.T) {
		for _, tt := range tests {
			// run all tests for case where ceph version does support rotation
			t.Run(tt.name, func(t *testing.T) {
				ignoreKeyType := false // for these tests, don't ignore key type
				got, err := ShouldRotateCephxKeys(tt.cfg, version.CephVersion{Major: 18, Minor: 2, Extra: 999}, tt.imageCephVersion, tt.status, ignoreKeyType)
				if (err != nil) != tt.wantErr {
					t.Errorf("ShouldRotateCephxKeys() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("ShouldRotateCephxKeys() = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("cannot support", func(t *testing.T) {
		for _, tt := range tests {
			// and run all tests for case where ceph version does not support rotation
			t.Run(tt.name, func(t *testing.T) {
				ignoreKeyType := false // for these tests, don't ignore key type
				got, err := ShouldRotateCephxKeys(tt.cfg, version.CephVersion{Major: 19, Minor: 2, Extra: 2}, tt.imageCephVersion, tt.status, ignoreKeyType)
				assert.NoError(t, err)
				assert.False(t, got)
			})
		}
	})

	t.Run("test ignore key type", func(t *testing.T) {
		for _, tt := range keyTypeTests {
			t.Run(tt.name, func(t *testing.T) {
				ignoreKeyType := true // test that ignoring key type for keyType tests
				got, err := ShouldRotateCephxKeys(tt.cfg, version.CephVersion{Major: 19, Minor: 2, Extra: 2}, tt.imageCephVersion, tt.status, ignoreKeyType)
				assert.NoError(t, err)
				assert.False(t, got) // should not rotate when ignoring keyType
			})
		}
	})
}

func Test_parseCephVersionFromStatusVersion(t *testing.T) {
	cephVer := version.CephVersion{Major: 21, Minor: 3, Extra: 0, Build: 664, CommitID: "abababababbababababa"}
	cephVerNoCommitID := cephVer
	cephVerNoCommitID.CommitID = ""

	tests := []struct {
		ver     string
		want    version.CephVersion
		wantErr bool
	}{
		{"20.2.1-5", version.CephVersion{Major: 20, Minor: 2, Extra: 1, Build: 5}, false},
		{"v20.2.1-5", version.CephVersion{}, true},
		{"20.2.1", version.CephVersion{}, true},
		{"20.2", version.CephVersion{}, true},
		{"20", version.CephVersion{}, true},
		{"20.2-5", version.CephVersion{}, true},

		// test round trip CephVersionToCephxStatusVersion() with comparison here
		{CephVersionToCephxStatusVersion(cephVer), cephVerNoCommitID, false},
	}
	for _, tt := range tests {
		t.Run(tt.ver, func(t *testing.T) {
			got, err := parseCephVersionFromStatusVersion(tt.ver)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCephVersionFromStatusVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseCephVersionFromStatusVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdatedCephxStatus(t *testing.T) {
	cephVer := version.CephVersion{Major: 20, Minor: 2, Extra: 0}
	verStr := "20.2.0-0"

	tests := []struct {
		name               string
		didRotate          bool
		cfg                v1.CephxConfig
		keyType            string
		runningCephVersion version.CephVersion
		status             v1.CephxStatus
		want               v1.CephxStatus
	}{
		// brownfield: unset remains unset when no rotation
		{"norotate, nopolicy - unset -> unset", false, v1.CephxConfig{KeyRotationPolicy: ""}, "", cephVer, v1.CephxStatus{}, v1.CephxStatus{}},
		{"norotate, disabled - unset -> unset", false, v1.CephxConfig{KeyRotationPolicy: "Disabled"}, "", cephVer, v1.CephxStatus{}, v1.CephxStatus{}},
		{"norotate, keygen - unset -> unset", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 2}, "", cephVer, v1.CephxStatus{}, v1.CephxStatus{}},
		{"norotate, cephver - unset -> unset", false, v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate"}, "", cephVer, v1.CephxStatus{}, v1.CephxStatus{}},
		{"norotate, nopolicy - unset -> unset, with keyType", false, v1.CephxConfig{KeyRotationPolicy: ""}, "set", cephVer, v1.CephxStatus{}, v1.CephxStatus{}},
		{"norotate, disabled - unset -> unset, with keyType", false, v1.CephxConfig{KeyRotationPolicy: "Disabled"}, "set", cephVer, v1.CephxStatus{}, v1.CephxStatus{}},
		{"norotate, keygen - unset -> unset, with keyType", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 2}, "set", cephVer, v1.CephxStatus{}, v1.CephxStatus{}},
		{"norotate, cephver - unset -> unset, with keyType", false, v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate"}, "set", cephVer, v1.CephxStatus{}, v1.CephxStatus{}},

		// brownfield: unset gets set when rotation happened
		{"rotate, nopolicy - unset -> set", true, v1.CephxConfig{KeyRotationPolicy: ""}, "", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"rotate, disabled - unset -> set", true, v1.CephxConfig{KeyRotationPolicy: "Disabled"}, "", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"rotate, keygen 0 - unset -> set", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, "", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"rotate, keygen 1 - unset -> set", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 1}, "", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"rotate, keygen 3 - unset -> set", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 3}, "", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 3}},
		{"rotate, cephver - unset -> set", true, v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate"}, "", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"rotate, nopolicy - unset -> set, with keyType", true, v1.CephxConfig{KeyRotationPolicy: ""}, "set", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1, KeyType: "set"}},
		{"rotate, disabled - unset -> set, with keyType", true, v1.CephxConfig{KeyRotationPolicy: "Disabled"}, "set", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1, KeyType: "set"}},
		{"rotate, keygen 0 - unset -> set, with keyType", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, "set", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1, KeyType: "set"}},
		{"rotate, keygen 1 - unset -> set, with keyType", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 1}, "set", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1, KeyType: "set"}},
		{"rotate, keygen 3 - unset -> set, with keyType", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 3}, "set", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 3, KeyType: "set"}},
		{"rotate, cephver - unset -> set, with keyType", true, v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate"}, "set", cephVer, v1.CephxStatus{}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1, KeyType: "set"}},

		// greenfield: uninit gets initialized
		{"greenfield, nopolicy", false, v1.CephxConfig{KeyRotationPolicy: ""}, "", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"greenfield, disabled", false, v1.CephxConfig{KeyRotationPolicy: "Disabled"}, "", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"greenfield, keygen 0", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, "", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"greenfield, keygen 1", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 1}, "", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"greenfield, keygen 3", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 3}, "", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 3}},
		{"greenfield, cephver", false, v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate"}, "", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"greenfield, nopolicy, with keyType", false, v1.CephxConfig{KeyRotationPolicy: ""}, "set", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1, KeyType: "set"}},
		{"greenfield, disabled, with keyType", false, v1.CephxConfig{KeyRotationPolicy: "Disabled"}, "set", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1, KeyType: "set"}},
		{"greenfield, keygen 0, with keyType", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, "set", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1, KeyType: "set"}},
		{"greenfield, keygen 1, with keyType", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 1}, "set", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1, KeyType: "set"}},
		{"greenfield, keygen 3, with keyType", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 3}, "set", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 3, KeyType: "set"}},
		{"greenfield, cephver, with keyType", false, v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate"}, "set", cephVer, v1.CephxStatus{KeyCephVersion: "Uninitialized"}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1, KeyType: "set"}},

		// norotate: status retained
		{"norotate, nopolicy - retain status", false, v1.CephxConfig{KeyRotationPolicy: ""}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, disabled - retain status", false, v1.CephxConfig{KeyRotationPolicy: "Disabled"}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, keygen 0 - retain status", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, keygen 1 - retain status", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 1}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, keygen 3 - retain status", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 3}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, cephver - retain status", false, v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate"}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, nopolicy, with keyType - retain status", false, v1.CephxConfig{KeyRotationPolicy: ""}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, disabled, with keyType - retain status", false, v1.CephxConfig{KeyRotationPolicy: "Disabled"}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, keygen 0, with keyType - retain status", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, keygen 1, with keyType - retain status", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 1}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, keygen 3, with keyType - retain status", false, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 3}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},
		{"norotate, cephver, with keyType - retain status", false, v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate"}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}},

		// rotate: status updated
		{"rotate, nopolicy - status 1 -> 2", true, v1.CephxConfig{KeyRotationPolicy: ""}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 2}},
		{"rotate, disabled - status 1 -> 2", true, v1.CephxConfig{KeyRotationPolicy: "Disabled"}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 2}},
		{"rotate, keygen 0 - status 1 -> 2", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 2}},
		{"rotate, keygen 1 - status 1 -> 2", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 1}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 2}},
		{"rotate, keygen 3 - status 1 -> 3", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 3}, "", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 3}},
		{"rotate, cephver - status 1 -> 2", true, v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate"}, "", cephVer, v1.CephxStatus{KeyCephVersion: "20.3.0-0", KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 2}},
		{"rotate, nopolicy, with keyType - status 1 -> 2", true, v1.CephxConfig{KeyRotationPolicy: ""}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 2, KeyType: "set"}},
		{"rotate, disabled, with keyType - status 1 -> 2", true, v1.CephxConfig{KeyRotationPolicy: "Disabled"}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 2, KeyType: "set"}},
		{"rotate, keygen 0, with keyType - status 1 -> 2", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 0}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 2, KeyType: "set"}},
		{"rotate, keygen 1, with keyType - status 1 -> 2", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 1}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 2, KeyType: "set"}},
		{"rotate, keygen 3, with keyType - status 1 -> 3", true, v1.CephxConfig{KeyRotationPolicy: "KeyGeneration", KeyGeneration: 3}, "set", cephVer, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 3, KeyType: "set"}},
		{"rotate, cephver, with keyType - status 1 -> 2", true, v1.CephxConfig{KeyRotationPolicy: "WithCephVersionUpdate"}, "set", cephVer, v1.CephxStatus{KeyCephVersion: "20.3.0-0", KeyGeneration: 1}, v1.CephxStatus{KeyCephVersion: verStr, KeyGeneration: 2, KeyType: "set"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UpdatedCephxStatus(tt.didRotate, tt.cfg, tt.runningCephVersion, tt.status, v1.CephxKeyType(tt.keyType)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UpdatedCephxStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}
