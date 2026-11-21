/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package cluster

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestCleanupJobSpec(t *testing.T) {
	expectedHostPath := "var/lib/rook"
	expectedNamespace := "test-rook-ceph"
	cluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: expectedNamespace,
		},
		Spec: cephv1.ClusterSpec{
			DataDirHostPath: expectedHostPath,
			CleanupPolicy: cephv1.CleanupPolicySpec{
				Confirmation: "yes-really-destroy-data",
			},
		},
	}
	clientset := testop.New(t, 3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookfake.NewSimpleClientset(),
	}
	controller := NewClusterController(context, "")
	podTemplateSpec := controller.cleanUpJobTemplateSpec(cluster, "28b87851-8dc1-46c8-b1ec-90ec51a47c89")
	assert.Equal(t, expectedHostPath, podTemplateSpec.Spec.Containers[0].Env[0].Value)
	assert.Equal(t, expectedNamespace, podTemplateSpec.Spec.Containers[0].Env[1].Value)
}

const (
	cleanupTestNamespace = "test-rook-ceph"
	cleanupTestMonSecret = "AQBnot-a-real-mon-secret-for-tests==" //nolint:gosec // G101: fake value for the test
	cleanupTestFSID      = "28b87851-8dc1-46c8-b1ec-90ec51a47c89"
)

func newCleanupTest(t *testing.T) (*ClusterController, *fake.Clientset, *cephv1.CephCluster) {
	t.Helper()
	clientset := testop.New(t, 1)
	controller := NewClusterController(&clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookfake.NewSimpleClientset(),
	}, "rook/ceph:test")
	controller.OpManagerCtx = t.Context()
	cluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: cleanupTestNamespace},
		Spec: cephv1.ClusterSpec{
			DataDirHostPath: "/var/lib/rook",
			CleanupPolicy: cephv1.CleanupPolicySpec{
				Confirmation: "yes-really-destroy-data",
			},
		},
	}
	return controller, clientset, cluster
}

func TestCleanupJobReferencesMonSecret(t *testing.T) {
	controller, clientset, cluster := newCleanupTest(t)

	controller.startCleanUpJobs(cluster, []string{"node0"}, cleanupTestMonSecret, cleanupTestFSID)

	jobs, err := clientset.BatchV1().Jobs(cleanupTestNamespace).List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, jobs.Items, 1)

	// Anyone allowed to read Jobs or Pods in the namespace can read the Job spec.
	raw, err := json.Marshal(jobs.Items[0])
	require.NoError(t, err)
	assert.NotContains(t, string(raw), cleanupTestMonSecret)

	env := map[string]v1.EnvVar{}
	for _, e := range jobs.Items[0].Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e
	}
	require.Contains(t, env, "ROOK_MON_SECRET")
	monSecretEnv := env["ROOK_MON_SECRET"]
	assert.Empty(t, monSecretEnv.Value)
	require.NotNil(t, monSecretEnv.ValueFrom)
	require.NotNil(t, monSecretEnv.ValueFrom.SecretKeyRef)
	assert.Equal(t, "rook-ceph-cleanup", monSecretEnv.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "mon-secret", monSecretEnv.ValueFrom.SecretKeyRef.Key)

	secret, err := clientset.CoreV1().Secrets(cleanupTestNamespace).Get(t.Context(), "rook-ceph-cleanup", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []byte(cleanupTestMonSecret), secret.Data["mon-secret"])
	assert.Equal(t, "rook-ceph-cleanup", secret.Labels["app"])
	assert.Equal(t, "true", secret.Labels["rook-ceph-cleanup"])
	assert.Equal(t, cleanupTestNamespace, secret.Labels["rook_cluster"])
	// It must outlive the CephCluster, as the cleanup Jobs do.
	assert.Empty(t, secret.OwnerReferences)
}

func TestCleanupMonSecretUpdatedOnRerun(t *testing.T) {
	controller, clientset, cluster := newCleanupTest(t)

	controller.startCleanUpJobs(cluster, []string{"node0"}, "first-secret", cleanupTestFSID)
	controller.startCleanUpJobs(cluster, []string{"node0"}, "second-secret", cleanupTestFSID)

	secrets, err := clientset.CoreV1().Secrets(cleanupTestNamespace).List(t.Context(), metav1.ListOptions{LabelSelector: "app=rook-ceph-cleanup"})
	require.NoError(t, err)
	require.Len(t, secrets.Items, 1)
	assert.Equal(t, []byte("second-secret"), secrets.Items[0].Data["mon-secret"])

	jobs, err := clientset.BatchV1().Jobs(cleanupTestNamespace).List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, jobs.Items, 1)
}

func TestCleanupJobsSkippedWhenMonSecretWriteFails(t *testing.T) {
	controller, clientset, cluster := newCleanupTest(t)
	clientset.PrependReactor("create", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("injected failure")
	})

	controller.startCleanUpJobs(cluster, []string{"node0"}, cleanupTestMonSecret, cleanupTestFSID)

	// A Job referencing a missing Secret would never start its pod.
	jobs, err := clientset.BatchV1().Jobs(cleanupTestNamespace).List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, jobs.Items)
}

func TestCleanupMonSecretNotWrittenWithoutHosts(t *testing.T) {
	controller, clientset, cluster := newCleanupTest(t)

	controller.startCleanUpJobs(cluster, nil, cleanupTestMonSecret, cleanupTestFSID)

	_, err := clientset.CoreV1().Secrets(cleanupTestNamespace).Get(t.Context(), "rook-ceph-cleanup", metav1.GetOptions{})
	assert.True(t, kerrors.IsNotFound(err))
}

func TestCleanupSecretOmitsClusterLabelsAndAnnotations(t *testing.T) {
	controller, clientset, cluster := newCleanupTest(t)
	cluster.Spec.Labels = cephv1.LabelsSpec{
		cephv1.KeyAll:     {"all-label": "a"},
		cephv1.KeyCleanup: {"cleanup-label": "c"},
	}
	cluster.Spec.Annotations = cephv1.AnnotationsSpec{
		cephv1.KeyAll:     {"all-annotation": "a"},
		cephv1.KeyCleanup: {"cleanup-annotation": "c"},
	}

	controller.startCleanUpJobs(cluster, []string{"node0"}, cleanupTestMonSecret, cleanupTestFSID)

	// The Job takes them, so the spec above is in effect.
	jobs, err := clientset.BatchV1().Jobs(cleanupTestNamespace).List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, jobs.Items, 1)
	assert.Equal(t, "a", jobs.Items[0].Labels["all-label"])
	assert.Equal(t, "c", jobs.Items[0].Annotations["cleanup-annotation"])

	// User-declared labels or annotations could opt the Secret into a replication tool.
	secret, err := clientset.CoreV1().Secrets(cleanupTestNamespace).Get(t.Context(), "rook-ceph-cleanup", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotContains(t, secret.Labels, "all-label")
	assert.NotContains(t, secret.Labels, "cleanup-label")
	assert.Empty(t, secret.Annotations)
}

func TestCleanupPlacement(t *testing.T) {
	// no tolerations end up in an empty list of tolerations
	c := cephv1.ClusterSpec{}
	p := getCleanupPlacement(c)
	assert.Equal(t, cephv1.Placement{}, p)

	// add tolerations for each of the daemons
	c.Placement = cephv1.PlacementSpec{}
	c.Placement[cephv1.KeyAll] = cephv1.Placement{Tolerations: []v1.Toleration{{Key: "allToleration"}}}
	p = getCleanupPlacement(c)
	assert.Equal(t, c.Placement[cephv1.KeyAll], p)

	c.Placement[cephv1.KeyMon] = cephv1.Placement{Tolerations: []v1.Toleration{{Key: "monToleration"}}}
	p = getCleanupPlacement(c)
	assert.Equal(t, 2, len(p.Tolerations))

	c.Placement[cephv1.KeyMgr] = cephv1.Placement{Tolerations: []v1.Toleration{{Key: "mgrToleration"}}}
	p = getCleanupPlacement(c)
	assert.Equal(t, 3, len(p.Tolerations))

	c.Placement[cephv1.KeyMonArbiter] = cephv1.Placement{Tolerations: []v1.Toleration{{Key: "monArbiterToleration"}}}
	p = getCleanupPlacement(c)
	assert.Equal(t, 4, len(p.Tolerations))

	c.Placement[cephv1.KeyOSD] = cephv1.Placement{Tolerations: []v1.Toleration{{Key: "osdToleration"}}}
	p = getCleanupPlacement(c)
	assert.Equal(t, 5, len(p.Tolerations))

	c.Storage.StorageClassDeviceSets = []cephv1.StorageClassDeviceSet{
		{Placement: cephv1.Placement{Tolerations: []v1.Toleration{{Key: "deviceSetToleration"}}}},
	}
	p = getCleanupPlacement(c)
	assert.Equal(t, 6, len(p.Tolerations))
}
