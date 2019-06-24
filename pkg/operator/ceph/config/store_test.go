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
	"bufio"
	"fmt"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
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

	var previousConfigText = ""
	var recentConfigText = ""
	assertConfigStore := func(ci *cephconfig.ClusterInfo) {
		c, e := clientset.CoreV1().ConfigMaps(ns).Get(storeName, metav1.GetOptions{})
		assert.NoError(t, e)

		previousConfigText = recentConfigText
		recentConfigText = c.Data[confFileName]
		assert.NotEqual(t, "", recentConfigText)

		sec, e := clientset.CoreV1().Secrets(ns).Get(storeName, metav1.GetOptions{})
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

	findLine := func(lineRegex, inText string) int {
		scr := bufio.NewScanner(strings.NewReader(inText))
		i := 1
		for scr.Scan() {
			m, e := regexp.Match(lineRegex, scr.Bytes())
			assert.NoError(t, e)
			if m {
				return i
			}
			i++
		}
		return 0 // ret 0 if not found
	}

	i1 := testop.CreateConfigDir(1) // cluster w/ one mon
	i3 := testop.CreateConfigDir(3) // same cluster w/ 3 mons

	s.CreateOrUpdate(i1)
	assertConfigStore(i1)

	s.CreateOrUpdate(i3)
	assertConfigStore(i3)
	// the config should be the same regardless of how many mons there are
	assert.Equal(t, previousConfigText, recentConfigText)

	//
	// test overrides
	//
	createOverrideMap(t, ctx, ns, &owner)
	s.CreateOrUpdate(i3)
	assertConfigStore(i3)
	// configs should still be equal since the created map doesn't have data in it
	assert.Equal(t, previousConfigText, recentConfigText)

	updateOverrideMap(t, "", ctx, ns, &owner)
	s.CreateOrUpdate(i3)
	assertConfigStore(i3)
	// configs should still be equal since the created map's data is blank
	assert.Equal(t, previousConfigText, recentConfigText)

	updateOverrideMap(t, "this is not valid ini file text", ctx, ns, &owner)
	s.CreateOrUpdate(i3)
	assertConfigStore(i3)
	// configs should still be equal since the created map's data is invalid ini
	assert.Equal(t, previousConfigText, recentConfigText)

	// complicated override with overrides, new keys, new section
	updateOverrideMap(t, `[global]
log_file     = notreal
new_key           = fake
log stderr prefix = '"debug  "'

[mon]
debug_mon = makehaste
`, ctx, ns, &owner)
	s.CreateOrUpdate(i3)
	assertConfigStore(i3)
	// Verify some simple truths about the overridden config vs the original
	assert.NotEqual(t, previousConfigText, recentConfigText)                       // the new config has changed (finally)
	l1 := findLine("log_file[[:space:]]+= ", previousConfigText)                   //
	assert.NotZero(t, l1)                                                          // orig 'log_file = <num>' exists
	l2 := findLine("log_file[[:space:]]+= notreal", recentConfigText)              //
	assert.Equal(t, l1, l2)                                                        // overridden debug_default still has same line num
	l3 := findLine("new_key[[:space:]]+= fake", recentConfigText)                  //
	assert.True(t, l3 > l2)                                                        // new_key should come after debug_default
	l4 := findLine("log stderr prefix[[:space:]]+= \"debug  \"", recentConfigText) // becomes "debug " w/o single quotes
	assert.Equal(t, l3+1, l4)                                                      // ... and is 1 line after new_key
	l5 := findLine("[[]mon[]]", recentConfigText)                                  //
	assert.Equal(t, l4+2, l5)                                                      // [mon] hdr comes 2 after
	l6 := findLine("debug_mon[[:space:]]+= makehaste", recentConfigText)           //
	assert.Equal(t, l5+1, l6)                                                      // debug_mon comes 1 after
	//
	lg := findLine("[[]global[]]", recentConfigText)                           //
	assert.Equal(t, 1, lg)                                                     // [global] hdr is first line
	ln := findLine("[[].*[]]", strings.TrimLeft(recentConfigText, "[global]")) //
	assert.Equal(t, l5, ln)                                                    // next hdr after [global] is [mon]

}

func createOverrideMap(t *testing.T,
	context *clusterd.Context, namespace string, ownerRef *metav1.OwnerReference,
) {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8sutil.ConfigOverrideName,
		},
	}
	k8sutil.SetOwnerRef(&cm.ObjectMeta, ownerRef)
	_, err := context.Clientset.CoreV1().ConfigMaps(namespace).Create(cm)
	assert.NoError(t, err)
}

func updateOverrideMap(t *testing.T, overrideText string,
	context *clusterd.Context, namespace string, ownerRef *metav1.OwnerReference,
) {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8sutil.ConfigOverrideName,
		},
		Data: map[string]string{k8sutil.ConfigOverrideVal: overrideText},
	}
	k8sutil.SetOwnerRef(&cm.ObjectMeta, ownerRef)
	_, err := context.Clientset.CoreV1().ConfigMaps(namespace).Update(cm)
	assert.NoError(t, err)
}

func TestFileVolumeAndMount(t *testing.T) {
	clientset := testop.New(1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ns := "rook-ceph"
	owner := metav1.OwnerReference{}

	s := GetStore(ctx, ns, &owner)
	s.CreateOrUpdate(testop.CreateConfigDir(3))

	v := StoredFileVolume()
	m := StoredFileVolumeMount()
	// Test that the configmapped file will make it into containers with the appropriate filename at
	// the location where it is expected.
	assert.Equal(t, v.Name, m.Name)
	assert.Equal(t, storeName, v.VolumeSource.ConfigMap.LocalObjectReference.Name)
	assert.Equal(t, "/etc/ceph/ceph.conf", path.Join(m.MountPath, confFileName))
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
	f := StoredMonHostEnvVarReferences().GlobalFlags()

	// make sure the env var names and flags are matching pairs
	mh := v[0].Name
	mim := v[1].Name
	assert.Contains(t, f, fmt.Sprintf("--mon-host=$(%s)", mh))
	assert.Contains(t, f, fmt.Sprintf("--mon-initial-members=$(%s)", mim))

	// make sure the env vars are sourced from the right place
	assert.Equal(t, storeName, v[0].ValueFrom.SecretKeyRef.LocalObjectReference.Name)
	assert.Equal(t, "mon_host", v[0].ValueFrom.SecretKeyRef.Key)
	assert.Equal(t, storeName, v[1].ValueFrom.SecretKeyRef.LocalObjectReference.Name)
	assert.Equal(t, "mon_initial_members", v[1].ValueFrom.SecretKeyRef.Key)
}
