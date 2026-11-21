/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package leakguard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	cephv1client "github.com/rook/rook/pkg/client/clientset/versioned/typed/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// conditionMessage holds characters that JSON or YAML would escape, so the
// test proves values are written raw, as an exact-match scan needs them.
const conditionMessage = `failed to prepare osd: key "AQBexample+key/value==" & <more>`

func readSnapshot(t *testing.T, dir, name string) string {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, "snapshots", name))
	require.NoError(t, err)
	return string(got)
}

func TestSnapshotWritesConfigMapsEventsAndStatus(t *testing.T) {
	cs := fake.NewClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Name: "osd-status"},
			Data:       map[string]string{"status": "line one\nline two\n", "node": "worker-0"},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: "elsewhere", Name: "unguarded"},
			Data:       map[string]string{"k": "v"},
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Namespace: "operator", Name: "pod.17a"},
			Message:    "Back-off restarting failed container",
		},
	)
	rcs := rookfake.NewSimpleClientset(
		&cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Name: "my-cluster"},
			Status: cephv1.ClusterStatus{
				Phase:      cephv1.ConditionProgressing,
				Conditions: []cephv1.Condition{{Type: cephv1.ConditionProgressing, Message: conditionMessage}},
			},
		},
		&cephv1.CephBlockPool{
			ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Name: "replicapool"},
			Status:     &cephv1.CephBlockPoolStatus{Phase: cephv1.ConditionReady},
		},
		&cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Name: "no-status"},
		},
	)
	dir := t.TempDir()

	require.NoError(t, Snapshot(context.Background(), cs, rcs, []string{"operator", "rook-ceph"}, dir))

	entries, err := os.ReadDir(filepath.Join(dir, "snapshots"))
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{
		"rook-ceph_ConfigMap_osd-status.txt",
		"operator_Event_pod.17a.txt",
		"rook-ceph_CephCluster_my-cluster.txt",
		"rook-ceph_CephBlockPool_replicapool.txt",
	}, names, "an object without status, or outside the namespaces, must not be written")

	assert.Equal(t, "data.node: worker-0\ndata.status: line one\nline two\n",
		readSnapshot(t, dir, "rook-ceph_ConfigMap_osd-status.txt"))
	assert.Equal(t, "message: Back-off restarting failed container\n",
		readSnapshot(t, dir, "operator_Event_pod.17a.txt"))
	cluster := readSnapshot(t, dir, "rook-ceph_CephCluster_my-cluster.txt")
	assert.Contains(t, cluster, "status.conditions[0].message: "+conditionMessage+"\n")
	assert.Contains(t, cluster, "status.phase: Progressing\n")
	assert.Equal(t, "status.phase: Ready\n", readSnapshot(t, dir, "rook-ceph_CephBlockPool_replicapool.txt"))
}

func TestSnapshotContinuesPastAListError(t *testing.T) {
	cs := fake.NewClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Name: "cm"},
		Data:       map[string]string{"k": "v"},
	})
	cs.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("events unavailable")
	})
	rcs := rookfake.NewSimpleClientset(&cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Name: "my-cluster"},
		Status:     cephv1.ClusterStatus{Phase: cephv1.ConditionReady},
	})
	dir := t.TempDir()

	err := Snapshot(context.Background(), cs, rcs, []string{"rook-ceph"}, dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "events unavailable")
	assert.FileExists(t, filepath.Join(dir, "snapshots", "rook-ceph_ConfigMap_cm.txt"))
	assert.FileExists(t, filepath.Join(dir, "snapshots", "rook-ceph_CephCluster_my-cluster.txt"))
}

// Every getter of CephV1Interface except RESTClient lists one namespaced kind,
// so a kind added to the clientset without a table entry fails here.
func TestSnapshotCoversEveryCephKind(t *testing.T) {
	getters := reflect.TypeFor[cephv1client.CephV1Interface]().NumMethod() - 1
	c := rookfake.NewSimpleClientset().CephV1()
	kinds := map[string]bool{}
	for _, k := range cephKinds {
		kinds[k.kind] = true
		list, err := k.list(context.Background(), c, "rook-ceph")
		require.NoError(t, err)
		assert.Equal(t, "*v1."+k.kind+"List", fmt.Sprintf("%T", list), "the getter of %s lists another kind", k.kind)
	}
	assert.Len(t, kinds, len(cephKinds), "a kind is listed twice")
	assert.Len(t, cephKinds, getters)
}
