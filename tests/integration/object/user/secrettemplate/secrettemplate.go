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

package secrettemplate

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const Namespace = "test-usersecrettemplate"

// the mixed-case prefix exercises the CRD rule that lowercases annotation keys
const replicateAnnotation = "Example.com/replicate"

func TestObjectStoreUserSecretTemplate(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		objectStore = store.ObjectStore()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
			},
		}

		osu1 = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-user1",
				Namespace: ns.Name,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:            objectStore.Name,
				ClusterNamespace: objectStore.Namespace,
				SecretTemplate: cephv1.SecretTemplate{
					Labels:      map[string]cephv1.LabelValue{"team": "payments", "tier": "gold"},
					Annotations: map[string]cephv1.AnnotationValue{replicateAnnotation: "true"},
				},
			},
		}
		secretName = "rook-ceph-object-user-" + osu1.Spec.Store + "-" + osu1.Name

		osuClient    = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name)
		secretClient = k8sh.Clientset.CoreV1().Secrets(ns.Name)
	)

	t.Run("ObjectStoreUser secretTemplate", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			// user creation may be slow right after rgw start up
			wait4.RequireCreate(ctx, t, osuClient, &osu1, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		t.Run(fmt.Sprintf("secret %q carries the template labels and annotations", secretName), func(t *testing.T) {
			secret := wait4.RequireCondition(ctx, t, secretClient, secretName, func(s *corev1.Secret) bool {
				return s.Labels["team"] == "payments"
			}, wait4.TimeoutShort)

			assert.Equal(t, "gold", secret.Labels["tier"])
			assert.Equal(t, osu1.Name, secret.Labels["user"])
			assert.Equal(t, osu1.Spec.Store, secret.Labels["rook_object_store"])
			assert.Equal(t, "true", secret.Annotations[replicateAnnotation])
		})

		t.Run(fmt.Sprintf("secret %q is selected by label %q", secretName, "team=payments"), func(t *testing.T) {
			secrets, err := secretClient.List(ctx, metav1.ListOptions{LabelSelector: "team=payments"})
			require.NoError(t, err)

			require.Len(t, secrets.Items, 1)
			assert.Equal(t, secretName, secrets.Items[0].Name)
		})

		t.Run(fmt.Sprintf("change secretTemplate on CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			liveOsu, err := osuClient.Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.SecretTemplate = cephv1.SecretTemplate{
				Labels: map[string]cephv1.LabelValue{"team": "billing"},
			}

			_, err = osuClient.Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("secret %q drops the removed label and annotation", secretName), func(t *testing.T) {
			secret, ok := wait4.AssertCondition(ctx, t, secretClient, secretName, func(s *corev1.Secret) bool {
				return s.Labels["team"] == "billing"
			}, wait4.TimeoutShort)
			if !ok {
				return
			}

			assert.NotContains(t, secret.Labels, "tier")
			assert.NotContains(t, secret.Annotations, replicateAnnotation)
			assert.Equal(t, osu1.Name, secret.Labels["user"])
		})

		t.Run(fmt.Sprintf("reserved secretTemplate keys on CephObjectStoreUser %q are rejected", osu1.Name), func(t *testing.T) {
			// a merge patch carries no resourceVersion, so it cannot conflict with the
			// operator's status write and always reaches validation
			patch, err := json.Marshal(map[string]any{"spec": map[string]any{"secretTemplate": map[string]any{
				"labels": map[string]string{"app": "other", "do_not_reconcile": "true", "rook.io/x": "y", "bad key": "v"},
				"annotations": map[string]string{
					"csi.rook.io/RBDProvisionerSecret": "true",
					"cephx-keyring":                    "x",
					"bad key":                          "v",
					// 262146 bytes in only 131073 characters: over both limits only when bytes are counted
					"example.com/large": strings.Repeat("\u00e9", 131073),
				},
			}}})
			require.NoError(t, err)

			_, err = osuClient.Patch(ctx, osu1.Name, types.MergePatchType, patch, metav1.PatchOptions{})
			require.Error(t, err)
			assert.True(t, kerrors.IsInvalid(err), err)
			// the API server evaluates every rule, so each one reports its own message
			for _, msg := range []string{
				"label keys app, user, rook_cluster and rook_object_store are reserved by Rook",
				"label key do_not_reconcile is reserved by Rook",
				"label keys with a rook.io prefix are reserved by Rook",
				"label keys must be valid Kubernetes label keys",
				"annotation key cephx-keyring is reserved by Rook",
				"annotation keys with a rook.io prefix are reserved by Rook",
				"annotation keys must be valid Kubernetes annotation keys",
				"annotation values must be at most 256 KiB",
				"annotations must total at most 256 KiB",
			} {
				assert.ErrorContains(t, err, msg)
			}
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, osuClient, osu1.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("secret %q is deleted with its user", secretName), func(t *testing.T) {
			wait4.AssertAbsent(ctx, t, secretClient, secretName, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("no CephObjectStoreUsers in ns %q", ns.Name), func(t *testing.T) {
			osus, err := osuClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, osus.Items, 0)
		})
	})
}
