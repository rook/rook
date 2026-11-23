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
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	enforceSecretResourceVersions(clientset)
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

// enforceSecretResourceVersions makes the fake clientset version Secrets and reject an update that
// carries a stale resourceVersion, as the API server does; the fake does neither on its own.
func enforceSecretResourceVersions(clientset *fake.Clientset) {
	version := 0
	clientset.PrependReactor("*", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		var secret *v1.Secret
		switch action.GetVerb() {
		case "create":
			secret = action.(k8stesting.CreateAction).GetObject().(*v1.Secret)
		case "update":
			secret = action.(k8stesting.UpdateAction).GetObject().(*v1.Secret)
			stored, err := clientset.Tracker().Get(v1.SchemeGroupVersion.WithResource("secrets"), action.GetNamespace(), secret.Name)
			if err == nil && secret.ResourceVersion != "" && secret.ResourceVersion != stored.(*v1.Secret).ResourceVersion {
				return true, nil, kerrors.NewConflict(v1.Resource("secrets"), secret.Name, errors.New("stale resourceVersion"))
			}
		default:
			return false, nil, nil
		}
		version++
		secret.ResourceVersion = strconv.Itoa(version)
		return false, nil, nil
	})
}

// assignJobUIDs gives each created Job a distinct UID, which the fake clientset leaves empty.
func assignJobUIDs(clientset *fake.Clientset) {
	n := 0
	clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		n++
		action.(k8stesting.CreateAction).GetObject().(*batchv1.Job).UID = types.UID(fmt.Sprintf("job-uid-%d", n))
		return false, nil, nil
	})
}

func getCleanupSecret(t *testing.T, clientset *fake.Clientset) *v1.Secret {
	t.Helper()
	secret, err := clientset.CoreV1().Secrets(cleanupTestNamespace).Get(t.Context(), "rook-ceph-cleanup", metav1.GetOptions{})
	require.NoError(t, err)
	return secret
}

// assertOwnedByJobs checks that the Secret's owners are exactly the Jobs in the namespace, as
// non-controller references that do not block the Jobs' deletion.
func assertOwnedByJobs(t *testing.T, clientset *fake.Clientset, secret *v1.Secret) {
	t.Helper()
	jobs, err := clientset.BatchV1().Jobs(cleanupTestNamespace).List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	want := []string{}
	for _, job := range jobs.Items {
		require.NotEmpty(t, job.UID)
		want = append(want, job.Name+"/"+string(job.UID))
	}
	got := []string{}
	for _, ref := range secret.OwnerReferences {
		assert.Equal(t, "batch/v1", ref.APIVersion)
		assert.Equal(t, "Job", ref.Kind)
		assert.False(t, ref.Controller != nil && *ref.Controller)
		assert.False(t, ref.BlockOwnerDeletion != nil && *ref.BlockOwnerDeletion)
		got = append(got, ref.Name+"/"+string(ref.UID))
	}
	assert.ElementsMatch(t, want, got)
}

func TestCleanupSecretOwnedByCreatedJobs(t *testing.T) {
	controller, clientset, cluster := newCleanupTest(t)
	assignJobUIDs(clientset)

	controller.startCleanUpJobs(cluster, []string{"node0", "node1"}, cleanupTestMonSecret, cleanupTestFSID)

	secret := getCleanupSecret(t, clientset)
	require.Len(t, secret.OwnerReferences, 2)
	assertOwnedByJobs(t, clientset, secret)
}

func TestCleanupSecretOwnersClearedBeforeJobsReplaced(t *testing.T) {
	controller, clientset, cluster := newCleanupTest(t)
	assignJobUIDs(clientset)
	hosts := []string{"node0", "node1"}

	controller.startCleanUpJobs(cluster, hosts, cleanupTestMonSecret, cleanupTestFSID)
	firstOwners := getCleanupSecret(t, clientset).OwnerReferences
	require.Len(t, firstOwners, 2)

	// The second run deletes the first run's Jobs before creating their replacements. If the
	// Secret still listed them as owners then, the garbage collector could delete it.
	var ownersAtJobChange [][]metav1.OwnerReference
	recordOwners := func(k8stesting.Action) (bool, runtime.Object, error) {
		obj, err := clientset.Tracker().Get(v1.SchemeGroupVersion.WithResource("secrets"), cleanupTestNamespace, "rook-ceph-cleanup")
		if err != nil {
			return true, nil, err
		}
		ownersAtJobChange = append(ownersAtJobChange, obj.(*v1.Secret).OwnerReferences)
		return false, nil, nil
	}
	clientset.PrependReactor("delete", "jobs", recordOwners)
	clientset.PrependReactor("create", "jobs", recordOwners)

	controller.startCleanUpJobs(cluster, hosts, cleanupTestMonSecret, cleanupTestFSID)

	require.Len(t, ownersAtJobChange, 4, "two deletes and two creates")
	for _, owners := range ownersAtJobChange {
		assert.Empty(t, owners)
	}
	secret := getCleanupSecret(t, clientset)
	assertOwnedByJobs(t, clientset, secret)
	for _, old := range firstOwners {
		for _, ref := range secret.OwnerReferences {
			assert.NotEqual(t, old.UID, ref.UID)
		}
	}
}

func TestCleanupSecretLeftUnownedWhenOwnersCannotBeSet(t *testing.T) {
	for _, tc := range []struct{ verb, resource string }{
		{verb: "update", resource: "secrets"},
		{verb: "get", resource: "jobs"},
	} {
		t.Run(tc.verb+" "+tc.resource, func(t *testing.T) {
			controller, clientset, cluster := newCleanupTest(t)
			assignJobUIDs(clientset)
			clientset.PrependReactor(tc.verb, tc.resource, func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("injected failure")
			})

			controller.startCleanUpJobs(cluster, []string{"node0", "node1"}, cleanupTestMonSecret, cleanupTestFSID)

			jobs, err := clientset.BatchV1().Jobs(cleanupTestNamespace).List(t.Context(), metav1.ListOptions{})
			require.NoError(t, err)
			assert.Len(t, jobs.Items, 2)
			secret := getCleanupSecret(t, clientset)
			assert.Equal(t, []byte(cleanupTestMonSecret), secret.Data["mon-secret"])
			assert.Empty(t, secret.OwnerReferences)
		})
	}
}

func TestCleanupSecretUnownedWhenOneJobCannotBeRead(t *testing.T) {
	controller, clientset, cluster := newCleanupTest(t)
	assignJobUIDs(clientset)
	clientset.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.(k8stesting.GetAction).GetName() == "cluster-cleanup-job-node1" {
			return true, nil, errors.New("injected failure")
		}
		return false, nil, nil
	})

	controller.startCleanUpJobs(cluster, []string{"node0", "node1"}, cleanupTestMonSecret, cleanupTestFSID)

	jobs, err := clientset.BatchV1().Jobs(cleanupTestNamespace).List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, jobs.Items, 2)
	// Owned by node0's Job alone, deleting it would take the Secret from node1's Job.
	assert.Empty(t, getCleanupSecret(t, clientset).OwnerReferences)
}

func TestCleanupStaleRunLeavesSecretToLaterRun(t *testing.T) {
	controller, clientset, cluster := newCleanupTest(t)
	assignJobUIDs(clientset)

	// Run A saves the Secret and starts its Job.
	secretA, err := controller.saveCleanupSecret(cluster, cleanupTestMonSecret)
	require.NoError(t, err)
	jobName, err := controller.runCleanUpJob(cluster, "node0", cleanupTestFSID)
	require.NoError(t, err)

	// Run B saves the Secret before A sets its owners.
	secretB, err := controller.saveCleanupSecret(cluster, cleanupTestMonSecret)
	require.NoError(t, err)

	// B is about to replace A's Job; owned by it, the Secret would be collected with it.
	require.NoError(t, controller.setCleanupSecretOwners(secretA, []string{jobName}))
	assert.Empty(t, getCleanupSecret(t, clientset).OwnerReferences)

	// B replaces A's Job and sets its owners.
	_, err = controller.runCleanUpJob(cluster, "node0", cleanupTestFSID)
	require.NoError(t, err)
	require.NoError(t, controller.setCleanupSecretOwners(secretB, []string{jobName}))
	secret := getCleanupSecret(t, clientset)
	require.Len(t, secret.OwnerReferences, 1)
	assert.Equal(t, types.UID("job-uid-2"), secret.OwnerReferences[0].UID)
	assertOwnedByJobs(t, clientset, secret)
}

func TestCleanupSecretNotOwnedByJobThatFailedToCreate(t *testing.T) {
	controller, clientset, cluster := newCleanupTest(t)
	assignJobUIDs(clientset)
	clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if strings.HasSuffix(action.(k8stesting.CreateAction).GetObject().(*batchv1.Job).Name, "node1") {
			return true, nil, errors.New("injected failure")
		}
		return false, nil, nil
	})

	controller.startCleanUpJobs(cluster, []string{"node0", "node1"}, cleanupTestMonSecret, cleanupTestFSID)

	secret := getCleanupSecret(t, clientset)
	require.Len(t, secret.OwnerReferences, 1)
	assert.Equal(t, "cluster-cleanup-job-node0", secret.OwnerReferences[0].Name)
	assertOwnedByJobs(t, clientset, secret)
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
