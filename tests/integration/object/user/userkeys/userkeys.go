/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package userkeys

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreos/pkg/capnslog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	utiladmin "github.com/rook/rook/tests/integration/object/util/admin"
)

var (
	defaultName = "test-userkeys"

	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
	}

	objectStore = &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
			// the CephObjectstore must be in the same ns as the CephCluster
			Namespace: "object-ns",
		},
		Spec: cephv1.ObjectStoreSpec{
			MetadataPool: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
			},
			DataPool: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
			},
			Gateway: cephv1.GatewaySpec{
				Port:      80,
				Instances: 1,
			},
			AllowUsersInNamespaces: []string{ns.Name},
		},
	}

	objectStoreSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectStore.Name,
			Namespace: objectStore.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":               "rook-ceph-rgw",
				"rook_cluster":      objectStore.Namespace,
				"rook_object_store": objectStore.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			Type:            corev1.ServiceTypeNodePort,
		},
	}

	secret1 = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-secret1",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte("P1ANF2BP4K2LPGOR5SBB"),
			"AWS_SECRET_ACCESS_KEY": []byte("yfCHPhhOed0vJkqZsGEODyJrdKqHD09OMTWCnwjX"),
		},
	}

	secret2 = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-secret2",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte("2I6RPUTQFMNCSYEXZ6VM"),
			"AWS_SECRET_ACCESS_KEY": []byte("uY066SWPfaOVlDeYc7GJyOTfkDejyDdXrqehS6wx"),
		},
	}

	secret3 = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-secret3",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte("J4D0P20F3EDR51OSND7Y"),
			"AWS_SECRET_ACCESS_KEY": []byte("jn89OpkXNoDlIHVVQ23mE2DZgPmuDK3WVH5ExOvQ"),
		},
	}

	secret4 = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-secret4",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte("MPZX7DG5WJWQ6VPCSLYT"),
			"AWS_SECRET_ACCESS_KEY": []byte("phh7DIxnLPeSD2V6FUouhmnWrKlKRD5dBykyXozX"),
		},
	}

	secret5 = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-secret5",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte("7TNSSANCO5KXK23IPT91"),
			"AWS_SECRET_ACCESS_KEY": []byte("HksEDf0hEh3PtTvl7s9x6CyXfkWuY8eAMYAcvH5l"),
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
			Keys: []cephv1.ObjectUserKey{
				{
					AccessKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret1.Name,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret1.Name,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
				{
					AccessKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret2.Name,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret2.Name,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
				{
					AccessKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret3.Name,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret3.Name,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
			},
		},
	}
)

// generate the secret name the operator is expected to generate for the CephObjectStoreUser
func generateObjectStoreUserSecretName(osu cephv1.CephObjectStoreUser) string {
	return "rook-ceph-object-user-" + osu.Namespace + "-" + osu.Name
}

// find a UserKeySpec by AccessKey value
func findUserKeySpec(keys []admin.UserKeySpec, accessKey string) (admin.UserKeySpec, error) {
	for _, k := range keys {
		if k.AccessKey == accessKey {
			return k, nil
		}
	}
	return admin.UserKeySpec{}, fmt.Errorf("UserKeySpec with AccessKey %q not found", accessKey)
}

func checkStatusKeys(t *testing.T, k8sh *utils.K8sHelper, osu cephv1.CephObjectStoreUser, expectedSecrets []*corev1.Secret) {
	t.Run(fmt.Sprintf("cephObjectStoreUser %q has .status.keys set", osu.Name), func(t *testing.T) {
		ctx := context.TODO()

		liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu.Name, metav1.GetOptions{})
		require.NoError(t, err)

		require.NotNil(t, liveOsu.Status)
		assert.Len(t, liveOsu.Status.Keys, len(expectedSecrets))

		for _, secret := range expectedSecrets {
			secretRef, err := func(secretName string, keys []cephv1.SecretReference) (cephv1.SecretReference, error) {
				for _, secretRef := range keys {
					if secretRef.Name == secretName {
						return secretRef, nil
					}
				}
				return cephv1.SecretReference{}, fmt.Errorf("secretReference for secret %q not found in CephObjectStoreUser.status.keys", secret.Name)
			}(secret.Name, liveOsu.Status.Keys)
			require.NoError(t, err)

			// fetch the live secret for UID and ResourceVersion
			liveSecret, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Get(ctx, secret.Name, metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, liveSecret.Name, secretRef.Name)
			assert.Equal(t, liveSecret.Namespace, secretRef.Namespace)
			assert.Equal(t, liveSecret.UID, secretRef.UID)
			assert.Equal(t, liveSecret.ResourceVersion, secretRef.ResourceVersion)
		}
	})
}

func checkRgwUserKeys(t *testing.T, adminClient *admin.API, osu cephv1.CephObjectStoreUser, expectedSecrets []*corev1.Secret, accessKeyName, secretKeyName string) {
	t.Run(fmt.Sprintf("rgw user %q has keys set", osu.Name), func(t *testing.T) {
		ctx := context.TODO()

		for _, secret := range expectedSecrets {
			var keySpec admin.UserKeySpec

			// assume that the .Phase doesn't change when updating keys
			inSync := utils.Retry(40, time.Second, fmt.Sprintf("CephObjectStoreUser has key in sync with secret %q ", secret.Name), func() bool {
				liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu.Name})
				if err != nil {
					return false
				}
				keySpec, err = findUserKeySpec(liveUser.Keys, string(secret.Data[accessKeyName]))
				return err == nil
			})
			require.True(t, inSync)

			assert.Equal(t, string(secret.Data[accessKeyName]), keySpec.AccessKey)
			assert.Equal(t, string(secret.Data[secretKeyName]), keySpec.SecretKey)
		}

		// check that no extra keys are present
		liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu.Name})
		require.NoError(t, err)
		assert.Equal(t, len(expectedSecrets), len(liveUser.Keys))
	})
}

func TestObjectStoreUserKeys(t *testing.T, k8sh *utils.K8sHelper, installer *installer.CephInstaller, logger *capnslog.PackageLogger, tlsEnable bool) {
	t.Run("ObjectStoreUser keys", func(t *testing.T) {
		if tlsEnable {
			// Skip testing with and without TLS to reduce test time
			t.Skip("skipping test for TLS enabled clusters")
		}

		var adminClient *admin.API
		ctx := context.TODO()

		t.Run(fmt.Sprintf("create ns %q", ns.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create CephObjectStore %q", objectStore.Name), func(t *testing.T) {
			objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Create(ctx, objectStore, metav1.CreateOptions{})
			require.NoError(t, err)

			osReady := utils.Retry(180, time.Second, "CephObjectStore is Ready", func() bool {
				liveOs, err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Get(ctx, objectStore.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				if liveOs.Status == nil {
					return false
				}

				return liveOs.Status.Phase == cephv1.ConditionReady
			})
			require.True(t, osReady)
		})

		t.Run(fmt.Sprintf("create svc %q", objectStoreSvc.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Create(ctx, objectStoreSvc, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create secret %q", secret1.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Create(ctx, secret1, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create secret %q", secret2.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Create(ctx, secret2, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create secret %q", secret3.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Create(ctx, secret3, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create secret %q", secret4.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Create(ctx, secret4, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create secret %q", secret5.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Create(ctx, secret5, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			_, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Create(ctx, &osu1, metav1.CreateOptions{})
			require.NoError(t, err)

			// user creation may be slow right after rgw start up
			osuReady := utils.Retry(120, time.Second, "CephObjectStoreUser is Ready", func() bool {
				liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				if liveOsu.Status == nil {
					return false
				}

				return liveOsu.Status.Phase == string(cephv1.ConditionReady)
			})
			require.True(t, osuReady)
		})

		t.Run("setup rgw admin api client", func(t *testing.T) {
			var err error
			adminClient, err = utiladmin.NewAdminClient(objectStore, installer, k8sh, tlsEnable)
			require.NoError(t, err)
		})

		{
			secrets := []*corev1.Secret{secret1, secret2, secret3}

			checkRgwUserKeys(t, adminClient, osu1, secrets, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY")
			checkStatusKeys(t, k8sh, osu1, secrets)
		}

		t.Run(fmt.Sprintf("update keys on CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.Keys = []cephv1.ObjectUserKey{
				{
					AccessKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret4.Name,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret4.Name,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
				{
					AccessKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret5.Name,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret5.Name,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
			}

			_, err = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		{
			secrets := []*corev1.Secret{secret4, secret5}

			checkRgwUserKeys(t, adminClient, osu1, secrets, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY")
			checkStatusKeys(t, k8sh, osu1, secrets)
		}

		// test transition from explicit keys -> automatic secret creation

		// when all explicit keys are removed from CephObjectStoreUser, one should
		// be left in place and the operator should [still] create a k8s secret for it
		t.Run(fmt.Sprintf("remove all keys set on CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.Keys = nil

			_, err = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)

			// wait for the number of keys to drop to 1
			inSync := utils.Retry(40, time.Second, "CephObjectStoreUser has 1 key", func() bool {
				liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
				if err != nil {
					return false
				}
				return len(liveUser.Keys) == 1
			})
			require.True(t, inSync)
		})

		// keys updated on user
		{
			// fetch automatic secret as it should be the only key set on the rgw user
			secretName := generateObjectStoreUserSecretName(osu1)
			liveSecret, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Get(ctx, secretName, metav1.GetOptions{})
			require.NoError(t, err)

			secrets := []*corev1.Secret{liveSecret}

			checkRgwUserKeys(t, adminClient, osu1, secrets, "AccessKey", "SecretKey")
		}

		t.Run(fmt.Sprintf("cephObjectStoreUser %q .status.keys is unset", osu1.Name), func(t *testing.T) {
			liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			require.NotNil(t, liveOsu.Status)
			assert.Len(t, liveOsu.Status.Keys, 0)
		})

		// test transition automatic secret creation -> explicit keys
		t.Run(fmt.Sprintf("add keys to CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.Keys = []cephv1.ObjectUserKey{
				{
					AccessKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret1.Name,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret1.Name,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
				{
					AccessKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret2.Name,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret2.Name,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
				{
					AccessKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret3.Name,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret3.Name,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
				{
					AccessKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret4.Name,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret4.Name,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
				{
					AccessKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret5.Name,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret5.Name,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
			}

			_, err = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		{
			secrets := []*corev1.Secret{secret1, secret2, secret3, secret4, secret5}

			checkRgwUserKeys(t, adminClient, osu1, secrets, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY")
			checkStatusKeys(t, k8sh, osu1, secrets)
		}

		// updating a secret already referenced by a CephObjectStoreUser should trigger a reconcile
		t.Run(fmt.Sprintf("update secret %q data", secret1.Name), func(t *testing.T) {
			liveSecret, err := k8sh.Clientset.CoreV1().Secrets(secret1.Namespace).Get(ctx, secret1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveSecret.Data = map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("foo"),
				"AWS_SECRET_ACCESS_KEY": []byte("bar"),
			}

			_, err = k8sh.Clientset.CoreV1().Secrets(secret1.Namespace).Update(ctx, liveSecret, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		{
			// fetch secret that we modified
			liveSecret, err := k8sh.Clientset.CoreV1().Secrets(secret1.Namespace).Get(ctx, secret1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			secrets := []*corev1.Secret{liveSecret, secret2, secret3, secret4, secret5}

			checkRgwUserKeys(t, adminClient, osu1, secrets, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY")
			checkStatusKeys(t, k8sh, osu1, secrets)
		}

		// deleting a secret referenced by a CephObjectStoreUser should trigger a reconcile (which fails)
		t.Run(fmt.Sprintf("delete secret %q", secret1.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Delete(ctx, secret1.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("CephObjectStoreUser %q has phase ReconcileFailed", osu1.Name), func(t *testing.T) {
			// user creation may be slow right after rgw start up
			osuReady := utils.Retry(40, time.Second, "CephObjectStoreUser is ReconcileFailed", func() bool {
				liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				if liveOsu.Status == nil {
					return false
				}

				return liveOsu.Status.Phase == string(cephv1.ReconcileFailed)
			})
			require.True(t, osuReady)
		})

		t.Run("missing referenced secret should not block CephObjectStoreUser deletion", func(t *testing.T) {
			t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
				err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Delete(ctx, osu1.Name, metav1.DeleteOptions{})
				require.NoError(t, err)

				absent := utils.Retry(40, time.Second, "CephObjectStoreUser is absent", func() bool {
					_, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
					return err != nil
				})
				assert.True(t, absent)
			})
		})

		t.Run(fmt.Sprintf("no CephObjectStoreUsers in ns %q", ns.Name), func(t *testing.T) {
			osus, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, osus.Items, 0)
		})

		t.Run(fmt.Sprintf("user %q does not exist", osu1.Name), func(t *testing.T) {
			_, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			require.ErrorIs(t, err, admin.ErrNoSuchUser)
		})

		// check that CephObjectStoreUser removal did not delete any secret resources

		{
			secrets := []*corev1.Secret{
				// secret1 was deleted in a previous step
				secret2,
				secret3,
				secret4,
				secret5,
			}

			for _, secret := range secrets {
				t.Run(fmt.Sprintf("secret %q still exists", secret.Name), func(t *testing.T) {
					_, err := k8sh.Clientset.CoreV1().Secrets(secret.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
					require.NoError(t, err)
				})

				// cleanup the secrets created for the test
				t.Run(fmt.Sprintf("delete secret %q", secret.Name), func(t *testing.T) {
					err := k8sh.Clientset.CoreV1().Secrets(secret.Namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{})
					require.NoError(t, err)
				})
			}
		}

		t.Run(fmt.Sprintf("no secrets in ns %q", ns.Name), func(t *testing.T) {
			secrets, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, secrets.Items, 0)
		})

		t.Run(fmt.Sprintf("delete svc %q", objectStoreSvc.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Delete(ctx, objectStoreSvc.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete CephObjectStore %q", objectStore.Name), func(t *testing.T) {
			err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Delete(ctx, objectStore.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete ns %q", ns.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})
	})
}
