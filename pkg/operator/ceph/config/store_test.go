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
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStore(t *testing.T) {
	ctxt := context.TODO()
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ns := "rook-ceph"
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()

	s := GetStore(ctx, ns, ownerInfo)
	mon1EndpointsEnabled := false
	assertConfigStore := func(ci *cephclient.ClusterInfo) {
		sec, e := clientset.CoreV1().Secrets(ns).Get(ctxt, StoreName, metav1.GetOptions{})
		assert.NoError(t, e)
		mh := strings.Split(sec.StringData["mon_host"], ",") // list of mon ip:port pairs in cluster
		expectedEndpoints := len(ci.InternalMonitors)
		if mon1EndpointsEnabled {
			expectedEndpoints *= 2
		}
		assert.Equal(t, expectedEndpoints, len(mh), ci.InternalMonitors["a"].Endpoint) // we need to pass x2 since we split on "," above and that returns msgr1 and msgr2 addresses
		mim := strings.Split(sec.StringData["mon_initial_members"], ",")               // list of mon ids in cluster
		assert.Equal(t, len(ci.InternalMonitors), len(mim))
		// make sure every mon has its id/ip:port in mon_initial_members/mon_host
		for _, id := range mim {
			// cannot use "assert.Contains(t, mh, ci.Monitors[id].Endpoint)"
			// it looks like the value is not found but if present, it might be confused by the brackets
			contains := false
			for _, c := range mh {
				if strings.Contains(c, ci.InternalMonitors[id].Endpoint) {
					contains = true
				}
			}
			assert.True(t, contains)
			assert.Contains(t, mim, ci.InternalMonitors[id].Name)
		}
	}

	i1 := clienttest.CreateTestClusterInfo(1) // cluster w/ one mon
	i3 := clienttest.CreateTestClusterInfo(3) // same cluster w/ 3 mons

	err := s.CreateOrUpdate(i1)
	assert.NoError(t, err)
	assertConfigStore(i1)

	err = s.CreateOrUpdate(i3)
	assert.NoError(t, err)
	assertConfigStore(i3)

	// Now run the same test for v1 endpoints
	mon1EndpointsEnabled = true
	for _, mon := range i1.InternalMonitors {
		mon.Endpoint = "1.2.3.4:6789"
	}
	err = s.CreateOrUpdate(i1)
	assert.NoError(t, err)
	assertConfigStore(i1)

	for _, mon := range i3.InternalMonitors {
		mon.Endpoint = "1.2.3.4:6789"
	}
	err = s.CreateOrUpdate(i3)
	assert.NoError(t, err)
	assertConfigStore(i3)
}

func TestEnvVarsAndFlags(t *testing.T) {
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ns := "rook-ceph"
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()

	s := GetStore(ctx, ns, ownerInfo)
	err := s.CreateOrUpdate(clienttest.CreateTestClusterInfo(3))
	assert.NoError(t, err)

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
