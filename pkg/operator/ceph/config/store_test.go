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

package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStore(t *testing.T) {
	clientset := testop.New(1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ns := "rook-ceph"
	owner := metav1.OwnerReference{}

	s := GetStore(ctx, ns, &owner)

	assertConfigStore := func(ci *cephconfig.ClusterInfo) {
		sec, e := clientset.CoreV1().Secrets(ns).Get(StoreName, metav1.GetOptions{})
		assert.NoError(t, e)
		mh := strings.Split(sec.StringData["mon_host"], ",") // list of mon ip:port pairs in cluster
		assert.Equal(t, len(ci.Monitors), len(mh))
		mim := strings.Split(sec.StringData["mon_initial_members"], ",") // list of mon ids in cluster
		assert.Equal(t, len(ci.Monitors), len(mim))
		// make sure every mon has its id/ip:port in mon_initial_members/mon_host
		for _, id := range mim {
			assert.Contains(t, mh, ci.Monitors[id].Endpoint)
			assert.Contains(t, mim, ci.Monitors[id].Name)
		}
	}

	i1 := testop.CreateConfigDir(1) // cluster w/ one mon
	i3 := testop.CreateConfigDir(3) // same cluster w/ 3 mons

	s.CreateOrUpdate(i1)
	assertConfigStore(i1)

	s.CreateOrUpdate(i3)
	assertConfigStore(i3)
}

func TestEnvVarsAndFlags(t *testing.T) {
	clientset := testop.New(1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ns := "rook-ceph"
	owner := metav1.OwnerReference{}

	s := GetStore(ctx, ns, &owner)
	s.CreateOrUpdate(testop.CreateConfigDir(3))

	v := StoredMonHostEnvVars()
	f := StoredMonHostEnvVarFlags()

	// make sure the env var names and flags are matching pairs
	mh := v[0].Name
	mim := v[1].Name
	assert.Contains(t, f, fmt.Sprintf("--mon-host=$(%s)", mh))
	assert.Contains(t, f, fmt.Sprintf("--mon-initial-members=$(%s)", mim))

	// make sure the env vars are sourced from the right place
	assert.Equal(t, StoreName, v[0].ValueFrom.SecretKeyRef.LocalObjectReference.Name)
	assert.Equal(t, "mon_host", v[0].ValueFrom.SecretKeyRef.Key)
	assert.Equal(t, StoreName, v[1].ValueFrom.SecretKeyRef.LocalObjectReference.Name)
	assert.Equal(t, "mon_initial_members", v[1].ValueFrom.SecretKeyRef.Key)
}
