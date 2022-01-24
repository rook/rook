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

package client

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-ini/ini"
	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/stretchr/testify/assert"
)

func TestCreateDefaultCephConfig(t *testing.T) {
	clusterInfo := &ClusterInfo{
		FSID:          "id",
		MonitorSecret: "monsecret",
		Namespace:     "foo-cluster",
		Monitors: map[string]*MonInfo{
			"node0": {Name: "mon0", Endpoint: "10.0.0.1:6789"},
			"node1": {Name: "mon1", Endpoint: "10.0.0.2:6789"},
		},
		CephVersion: cephver.Octopus,
	}

	// start with INFO level logging
	context := &clusterd.Context{}

	cephConfig, err := CreateDefaultCephConfig(context, clusterInfo)
	if err != nil {
		t.Fatalf("failed to create default ceph config. %+v", err)
	}
	verifyConfig(t, cephConfig, clusterInfo, 0)

	cephConfig, err = CreateDefaultCephConfig(context, clusterInfo)
	if err != nil {
		t.Fatalf("failed to create default ceph config. %+v", err)
	}
	verifyConfig(t, cephConfig, clusterInfo, 10)
}

func TestGenerateConfigFile(t *testing.T) {
	ctx := context.TODO()
	// set up a temporary config directory that will be cleaned up after test
	configDir := t.TempDir()

	// create mocked cluster context and info
	clientset := test.New(t, 3)

	context := &clusterd.Context{
		ConfigDir: configDir,
		Clientset: clientset,
	}

	ns := "foo-cluster"
	data := make(map[string]string, 1)
	data["config"] = "[global]\n    bluestore_min_alloc_size_hdd = 4096"
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sutil.ConfigOverrideName,
			Namespace: ns,
		},
		Data: data,
	}
	_, err := clientset.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
	assert.NoError(t, err)

	clusterInfo := &ClusterInfo{
		FSID:          "myfsid",
		MonitorSecret: "monsecret",
		Namespace:     ns,
		Monitors: map[string]*MonInfo{
			"node0": {Name: "mon0", Endpoint: "10.0.0.1:6789"},
		},
		CephVersion: cephver.Octopus,
		CephCred:    CephCred{Username: "admin", Secret: "mysecret"},
		Context:     ctx,
	}

	isInitialized := clusterInfo.IsInitialized(true)
	assert.True(t, isInitialized)

	// generate the config file to disk now
	configFilePath, err := generateConfigFile(context, clusterInfo, configDir, filepath.Join(configDir, "mykeyring"), nil, nil)
	assert.Nil(t, err)
	assert.Equal(t, filepath.Join(configDir, "foo-cluster.config"), configFilePath)

	// verify some of the contents of written config file by loading it from disk
	actualConf, err := ini.Load(configFilePath)
	assert.Nil(t, err)
	verifyConfigValue(t, actualConf, "global", "fsid", clusterInfo.FSID)
	verifyConfigValue(t, actualConf, "global", "bluestore_min_alloc_size_hdd", "4096")
}

func verifyConfig(t *testing.T, cephConfig *CephConfig, cluster *ClusterInfo, loggingLevel int) {
	monMembers := make([]string, len(cluster.Monitors))
	i := 0
	for _, expectedMon := range cluster.Monitors {
		contained := false
		monMembers[i] = expectedMon.Name
		for _, actualMon := range strings.Split(cephConfig.MonMembers, " ") {
			if expectedMon.Name == actualMon {
				contained = true
				break
			}
		}

		assert.True(t, contained)
	}

	// Testing mon_host

	expectedMons := "[v2:10.0.0.1:3300,v1:10.0.0.1:6789],[v2:10.0.0.2:3300,v1:10.0.0.2:6789]"

	for _, expectedMon := range strings.Split(expectedMons, ",") {
		contained := false
		for _, actualMon := range strings.Split(cephConfig.MonHost, ",") {
			if expectedMon == actualMon {
				contained = true
				break
			}
		}

		assert.True(t, contained, "expectedMons: %+v, actualMons: %+v", expectedMons, cephConfig.MonHost)
	}
}

func verifyConfigValue(t *testing.T, actualConf *ini.File, section, key, expectedVal string) {
	s, err := actualConf.GetSection(section)
	if !assert.Nil(t, err) {
		return
	}

	k := s.Key(key)
	if !assert.NotNil(t, k) {
		return
	}

	actualVal := k.Value()
	assert.Equal(t, expectedVal, actualVal)
}
