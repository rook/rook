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
			"ceph version 13.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) mimic (stable)": 1,
			"ceph version 14.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) nautilus (stable)": 2
		}
	}`)
	var dummyRunningVersions client.CephDaemonsVersions
	err := json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions)
	assert.NoError(t, err)

	m, err := diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions)
	assert.Error(t, err) // Overall is absent
	assert.False(t, m)

	// 2nd test - more than 1 version means we should upgrade
	fakeRunningVersions = []byte(`
	{
		"overall": {
			"ceph version 13.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) mimic (stable)": 1,
			"ceph version 14.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) nautilus (stable)": 2
		}
	}`)
	var dummyRunningVersions2 client.CephDaemonsVersions
	err = json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions2)
	assert.NoError(t, err)

	m, err = diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions2)
	assert.NoError(t, err)
	assert.True(t, m)

	// 3nd test - spec version is lower than running cluster? what's going on?
	fakeRunningVersions = []byte(`
		{
			"overall": {
				"ceph version 15.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) octopus (stable)": 2
			}
		}`)
	var dummyRunningVersions3 client.CephDaemonsVersions
	err = json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions3)
	assert.NoError(t, err)

	m, err = diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions3)
	assert.Error(t, err)
	assert.True(t, m)

	// 4 test - spec version is higher than running cluster --> we upgrade
	fakeImageVersion = cephver.Nautilus
	fakeRunningVersions = []byte(`
	{
		"overall": {
			"ceph version 13.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) mimic (stable)": 2
		}
	}`)
	var dummyRunningVersions4 client.CephDaemonsVersions
	err = json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions4)
	assert.NoError(t, err)

	m, err = diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions4)
	assert.NoError(t, err)
	assert.True(t, m)

	// 5 test - spec version and running cluster versions are identical --> we upgrade
	fakeImageVersion = cephver.CephVersion{Major: 14, Minor: 2, Extra: 2}
	fakeRunningVersions = []byte(`
		{
			"overall": {
				"ceph version 14.2.2 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) nautilus (stable)": 2
			}
		}`)
	var dummyRunningVersions5 client.CephDaemonsVersions
	err = json.Unmarshal([]byte(fakeRunningVersions), &dummyRunningVersions5)
	assert.NoError(t, err)

	m, err = diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions5)
	assert.NoError(t, err)
	assert.False(t, m)
}

func TestMinVersion(t *testing.T) {
	c := testSpec()
	c.Spec.CephVersion.AllowUnsupported = true

	// All versions less than 13.2.4 are invalid
	v := &cephver.CephVersion{Major: 12, Minor: 2, Extra: 10}
	assert.Error(t, c.validateCephVersion(v))
	v = &cephver.CephVersion{Major: 13, Minor: 2, Extra: 3}
	assert.Error(t, c.validateCephVersion(v))

	// All versions at least 13.2.4 are valid
	v = &cephver.CephVersion{Major: 13, Minor: 2, Extra: 4}
	assert.NoError(t, c.validateCephVersion(v))
	v = &cephver.CephVersion{Major: 14}
	assert.NoError(t, c.validateCephVersion(v))
	v = &cephver.CephVersion{Major: 15}
	assert.NoError(t, c.validateCephVersion(v))
}

func TestSupportedVersion(t *testing.T) {
	c := testSpec()

	// Supported versions are valid
	v := &cephver.CephVersion{Major: 14, Minor: 2, Extra: 0}
	assert.NoError(t, c.validateCephVersion(v))

	// Unsupported versions are not valid
	v = &cephver.CephVersion{Major: 15, Minor: 2, Extra: 0}
	assert.Error(t, c.validateCephVersion(v))

	// Unsupported versions are now valid
	c.Spec.CephVersion.AllowUnsupported = true
	assert.NoError(t, c.validateCephVersion(v))
}

func testSpec() cluster {
	clientset := testop.New(1)
	context := &clusterd.Context{
		Clientset: clientset,
	}
	return cluster{Spec: &cephv1.ClusterSpec{}, context: context}
}
