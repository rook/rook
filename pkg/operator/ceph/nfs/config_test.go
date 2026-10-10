/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package nfs

import (
	"errors"
	"os"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// leftoverTempFiles returns the names of any files left in the TMPDIR used by the test. The functions
// under test create temp files via os.CreateTemp("", ...), which honors the TMPDIR env var, so
// pointing TMPDIR at t.TempDir() lets us assert that no temp files leak after a call.
func leftoverTempFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func newTestClusterInfo(t *testing.T) *cephclient.ClusterInfo {
	return &cephclient.ClusterInfo{
		Namespace: "rook-ceph",
		Context:   t.Context(),
	}
}

func TestAtomicPrependToConfigObjectDoesNotLeakTempFiles(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}
	clusterInfo := newTestClusterInfo(t)

	err := atomicPrependToConfigObject(context, clusterInfo, "mypool", "myns", "myobj", "# prepend block\n")
	require.NoError(t, err)

	leftovers := leftoverTempFiles(t)
	assert.Empty(t, leftovers, "temp files left in TMPDIR after successful prepend: %v", leftovers)
}

func TestAtomicPrependToConfigObjectCleansUpTempFileOnError(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	executor := exectest.MockExecutor{
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			return "", errors.New("simulated rados failure")
		},
	}
	context := &clusterd.Context{Executor: &executor}
	clusterInfo := newTestClusterInfo(t)

	err := atomicPrependToConfigObject(context, clusterInfo, "mypool", "myns", "myobj", "# prepend block\n")
	assert.Error(t, err)

	leftovers := leftoverTempFiles(t)
	assert.Empty(t, leftovers, "temp files left in TMPDIR after failed prepend: %v", leftovers)
}

func TestAtomicRemoveFromConfigObjectDoesNotLeakTempFiles(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}
	clusterInfo := newTestClusterInfo(t)

	err := atomicRemoveFromConfigObject(context, clusterInfo, "mypool", "myns", "myobj", "# remove block\n")
	require.NoError(t, err)

	leftovers := leftoverTempFiles(t)
	assert.Empty(t, leftovers, "temp files left in TMPDIR after successful remove: %v", leftovers)
}

func TestAtomicRemoveFromConfigObjectCleansUpTempFileOnError(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	executor := exectest.MockExecutor{
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			return "", errors.New("simulated rados failure")
		},
	}
	context := &clusterd.Context{Executor: &executor}
	clusterInfo := newTestClusterInfo(t)

	err := atomicRemoveFromConfigObject(context, clusterInfo, "mypool", "myns", "myobj", "# remove block\n")
	assert.Error(t, err)

	leftovers := leftoverTempFiles(t)
	assert.Empty(t, leftovers, "temp files left in TMPDIR after failed remove: %v", leftovers)
}

func TestSetKerberosRadosConfigDoesNotLeakTempFiles(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}
	clusterInfo := newTestClusterInfo(t)
	nfs := &cephv1.CephNFS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-nfs",
			Namespace: "rook-ceph",
		},
		Spec: cephv1.NFSGaneshaSpec{
			RADOS: cephv1.GaneshaRADOSSpec{
				Pool:      "mypool",
				Namespace: "myns",
			},
			Security: &cephv1.NFSSecuritySpec{
				Kerberos: &cephv1.KerberosSpec{
					PrincipalName: "nfs",
				},
			},
		},
	}

	err := setKerberosRadosConfig(context, clusterInfo, nfs)
	require.NoError(t, err)

	leftovers := leftoverTempFiles(t)
	assert.Empty(t, leftovers, "temp files left in TMPDIR after setting kerberos config: %v", leftovers)
}
