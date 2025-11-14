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
	fakeImageVersion := cephver.Reef
	fakeRunningVersions := []byte(`
	{
		"mon": {
			"ceph version 19.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) squid (stable)": 1,
			"ceph version 18.1.0 (4be78cea2b4ae54a27b1049cffa1208df48bffae) reef (stable)": 2
		}
	}`)
	var dummyRunningVersions cephv1.CephDaemonsVersions
	err := json.Unmarshal(fakeRunningVersions, &dummyRunningVersions)
	assert.NoError(t, err)

	c := testSpec(t)
	m, err := c.diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions)
	assert.Error(t, err) // Overall is absent
	assert.False(t, m)

	// 2nd test - more than 1 version means we should upgrade
	fakeRunningVersions = []byte(`
	{
		"overall": {
			"ceph version 19.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) squid (stable)": 1,
			"ceph version 18.1.0 (4be78cea2b4ae54a27b1049cffa1208df48bffae) reef (stable)": 2
		}
	}`)
	var dummyRunningVersions2 cephv1.CephDaemonsVersions
	err = json.Unmarshal(fakeRunningVersions, &dummyRunningVersions2)
	assert.NoError(t, err)

	m, err = c.diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions2)
	assert.NoError(t, err)
	assert.True(t, m)

	// 3rd test - spec version is lower than running cluster? what's going on?
	fakeRunningVersions = []byte(`
		{
			"overall": {
				"ceph version 19.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) squid (stable)": 2
			}
		}`)
	var dummyRunningVersions3 cephv1.CephDaemonsVersions
	err = json.Unmarshal(fakeRunningVersions, &dummyRunningVersions3)
	assert.NoError(t, err)

	// Allow the downgrade
	m, err = c.diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions3)
	assert.NoError(t, err)
	assert.True(t, m)

	// 4 test - spec version is higher than running cluster --> we upgrade
	fakeImageVersion = cephver.Squid
	fakeRunningVersions = []byte(`
	{
		"overall": {
			"ceph version 19.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) squid (stable)": 2
		}
	}`)
	var dummyRunningVersions4 cephv1.CephDaemonsVersions
	err = json.Unmarshal(fakeRunningVersions, &dummyRunningVersions4)
	assert.NoError(t, err)

	m, err = c.diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions4)
	assert.NoError(t, err)
	assert.True(t, m)

	// 5 test - spec version and running cluster versions are identical --> we upgrade
	fakeImageVersion = cephver.CephVersion{
		Major: 19, Minor: 2, Extra: 0,
		CommitID: "3a54b2b6d167d4a2a19e003a705696d4fe619afc",
	}
	fakeRunningVersions = []byte(`
		{
			"overall": {
				"ceph version 19.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) squid (stable)": 2
			}
		}`)
	var dummyRunningVersions5 cephv1.CephDaemonsVersions
	err = json.Unmarshal(fakeRunningVersions, &dummyRunningVersions5)
	assert.NoError(t, err)

	m, err = c.diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions5)
	assert.NoError(t, err)
	assert.False(t, m)

	// 6 test - spec version and running cluster have different commit ID
	fakeImageVersion = cephver.CephVersion{
		Major: 19, Minor: 2, Extra: 0, Build: 139,
		CommitID: "3a54b2b6d167d4a2a19e003a705696d4fe619afc",
	}
	fakeRunningVersions = []byte(`
		{
			"overall": {
				"ceph version 19.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) squid (stable)": 2
			}
		}`)
	var dummyRunningVersions6 cephv1.CephDaemonsVersions
	err = json.Unmarshal(fakeRunningVersions, &dummyRunningVersions6)
	assert.NoError(t, err)

	m, err = c.diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions6)
	assert.NoError(t, err)
	assert.True(t, m)

	// 7 test - spec version and running cluster have same commit ID
	fakeImageVersion = cephver.CephVersion{
		Major: 19, Minor: 2, Extra: 0,
		CommitID: "3a54b2b6d167d4a2a19e003a705696d4fe619afc",
	}
	fakeRunningVersions = []byte(`
		{
			"overall": {
				"ceph version 19.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) squid (stable)": 2
			}
		}`)
	var dummyRunningVersions7 cephv1.CephDaemonsVersions
	err = json.Unmarshal(fakeRunningVersions, &dummyRunningVersions7)
	assert.NoError(t, err)

	m, err = c.diffImageSpecAndClusterRunningVersion(fakeImageVersion, dummyRunningVersions7)
	assert.NoError(t, err)
	assert.False(t, m)
}

func TestMinVersion(t *testing.T) {
	c := testSpec(t)
	c.Spec.CephVersion.AllowUnsupported = true
	c.ClusterInfo = &client.ClusterInfo{Context: context.TODO()}

	// All versions less than 18.2.0 or invalid tag are invalid
	v := &cephver.CephVersion{Major: 18, Minor: 1, Extra: 999}
	assert.Error(t, c.validateCephVersion(v))
	v = &cephver.CephVersion{Major: 16, Minor: 2, Extra: 11}
	assert.Error(t, c.validateCephVersion(v))

	// All versions at least 18.2.0 are valid
	v = &cephver.CephVersion{Major: 18, Minor: 2}
	assert.NoError(t, c.validateCephVersion(v))
	v = &cephver.CephVersion{Major: 19}
	assert.NoError(t, c.validateCephVersion(v))
}

func TestSupportedVersion(t *testing.T) {
	c := testSpec(t)
	c.ClusterInfo = &client.ClusterInfo{Context: context.TODO()}

	// lower version is not supported
	v := &cephver.CephVersion{Major: 17, Minor: 2, Extra: 7}
	assert.Error(t, c.validateCephVersion(v))

	// Reef is supported
	v = &cephver.CephVersion{Major: 18, Minor: 2, Extra: 0}
	assert.NoError(t, c.validateCephVersion(v))

	// Squid is supported
	v = &cephver.CephVersion{Major: 19, Minor: 2, Extra: 0}
	assert.NoError(t, c.validateCephVersion(v))

	// Tentacle is supported
	v = &cephver.CephVersion{Major: 20, Minor: 1, Extra: 0}
	assert.NoError(t, c.validateCephVersion(v))

	// Urchin release is not supported
	v = &cephver.CephVersion{Major: 21, Minor: 1, Extra: 0}
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
