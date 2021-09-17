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

// Package cluster to manage Kubernetes storage.
package cluster

import (
	"context"
	"encoding/json"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
)

func TestDiffImageSpecAndClusterRunningVersion(t *testing.T) {

	// 1st test
	fakeImageVersion := cephver.Nautilus
	fakeRunningVersions := []byte(`
	{
		"mon": {
			"ceph version 16.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) pacific (stable)": 1,
			"ceph version 17.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) quincy (stable)": 2
		}
	}`)
	var dummyRunningVersions cephv1.CephDaemonsVersions
	err := json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions)
	assert.NoError(t, err)

	m, err := diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions)
	assert.Error(t, err) // Overall is absent
	assert.False(t, m)

	// 2nd test - more than 1 version means we should upgrade
	fakeRunningVersions = []byte(`
	{
		"overall": {
			"ceph version 16.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) pacific (stable)": 1,
			"ceph version 17.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) quincy (stable)": 2
		}
	}`)
	var dummyRunningVersions2 cephv1.CephDaemonsVersions
	err = json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions2)
	assert.NoError(t, err)

	m, err = diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions2)
	assert.NoError(t, err)
	assert.True(t, m)

	// 3rd test - spec version is lower than running cluster? what's going on?
	fakeRunningVersions = []byte(`
		{
			"overall": {
				"ceph version 15.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) octopus (stable)": 2
			}
		}`)
	var dummyRunningVersions3 cephv1.CephDaemonsVersions
	err = json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions3)
	assert.NoError(t, err)

	m, err = diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions3)
	assert.Error(t, err)
	assert.True(t, m)

	// 4 test - spec version is higher than running cluster --> we upgrade
	fakeImageVersion = cephver.Pacific
	fakeRunningVersions = []byte(`
	{
		"overall": {
			"ceph version 15.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) octopus (stable)": 2
		}
	}`)
	var dummyRunningVersions4 cephv1.CephDaemonsVersions
	err = json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions4)
	assert.NoError(t, err)

	m, err = diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions4)
	assert.NoError(t, err)
	assert.True(t, m)

	// 5 test - spec version and running cluster versions are identical --> we upgrade
	fakeImageVersion = cephver.CephVersion{Major: 16, Minor: 2, Extra: 2,
		CommitID: "3a54b2b6d167d4a2a19e003a705696d4fe619afc"}
	fakeRunningVersions = []byte(`
		{
			"overall": {
				"ceph version 16.2.2 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) pacific (stable)": 2
			}
		}`)
	var dummyRunningVersions5 cephv1.CephDaemonsVersions
	err = json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions5)
	assert.NoError(t, err)

	m, err = diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions5)
	assert.NoError(t, err)
	assert.False(t, m)

	// 6 test - spec version and running cluster have different commit ID
	fakeImageVersion = cephver.CephVersion{Major: 16, Minor: 2, Extra: 11, Build: 139,
		CommitID: "5c0dc966af809fd1d429ec7bac48962a746af243"}
	fakeRunningVersions = []byte(`
		{
			"overall": {
				"ceph version 16.2.11-139.el8cp (3a54b2b6d167d4a2a19e003a705696d4fe619afc) pacific (stable)": 2
			}
		}`)
	var dummyRunningVersions6 cephv1.CephDaemonsVersions
	err = json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions6)
	assert.NoError(t, err)

	m, err = diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions6)
	assert.NoError(t, err)
	assert.True(t, m)

	// 7 test - spec version and running cluster have same commit ID
	fakeImageVersion = cephver.CephVersion{Major: 16, Minor: 2, Extra: 11, Build: 139,
		CommitID: "3a54b2b6d167d4a2a19e003a705696d4fe619afc"}
	fakeRunningVersions = []byte(`
		{
			"overall": {
				"ceph version 16.2.11-139.el8cp (3a54b2b6d167d4a2a19e003a705696d4fe619afc) pacific (stable)": 2
			}
		}`)
	var dummyRunningVersions7 cephv1.CephDaemonsVersions
	err = json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions7)
	assert.NoError(t, err)

	m, err = diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions7)
	assert.NoError(t, err)
	assert.False(t, m)
}

func TestMinVersion(t *testing.T) {
	c := testSpec(t)
	c.Spec.CephVersion.AllowUnsupported = true
	c.ClusterInfo = &client.ClusterInfo{Context: context.TODO()}

	// All versions less than 14.2.5 are invalid
	v := &cephver.CephVersion{Major: 13, Minor: 2, Extra: 3}
	assert.Error(t, c.validateCephVersion(v))
	v = &cephver.CephVersion{Major: 14, Minor: 2, Extra: 1}
	assert.Error(t, c.validateCephVersion(v))
	v = &cephver.CephVersion{Major: 14}
	assert.Error(t, c.validateCephVersion(v))

	// All versions at least 14.2.5 are valid
	v = &cephver.CephVersion{Major: 14, Minor: 2, Extra: 5}
	assert.NoError(t, c.validateCephVersion(v))
	v = &cephver.CephVersion{Major: 15}
	assert.NoError(t, c.validateCephVersion(v))
}

func TestSupportedVersion(t *testing.T) {
	c := testSpec(t)
	c.ClusterInfo = &client.ClusterInfo{Context: context.TODO()}

	// Supported versions are valid
	v := &cephver.CephVersion{Major: 14, Minor: 2, Extra: 12}
	assert.NoError(t, c.validateCephVersion(v))

	// Supported versions are valid
	v = &cephver.CephVersion{Major: 15, Minor: 2, Extra: 5}
	assert.NoError(t, c.validateCephVersion(v))

	// Supported versions are valid
	v = &cephver.CephVersion{Major: 16, Minor: 2, Extra: 0}
	assert.NoError(t, c.validateCephVersion(v))

	// Unsupported versions are not valid
	v = &cephver.CephVersion{Major: 17, Minor: 2, Extra: 0}
	assert.Error(t, c.validateCephVersion(v))

	// Unsupported versions are now valid
	c.Spec.CephVersion.AllowUnsupported = true
	assert.NoError(t, c.validateCephVersion(v))
}

func testSpec(t *testing.T) *cluster {
	clientset := testop.New(t, 1)
	context := &clusterd.Context{
		Clientset: clientset,
	}
	return &cluster{Spec: &cephv1.ClusterSpec{}, context: context}
}
