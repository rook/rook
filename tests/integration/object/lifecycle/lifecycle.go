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

package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/client"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const storeName = "test-lifecycle"

func TestCephObjectStoreLifecycle(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		clusterNS = store.ObjectStore().Namespace

		rgwName        = "rook-ceph-rgw-" + storeName
		certSecretName = storeName + "-tls"

		// deleting a store is destructive, so these tests get a private store
		// rather than the shared fixture; it also exercises a second store
		// coexisting with the shared one
		privateStore = &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      storeName,
				Namespace: clusterNS,
			},
			Spec: cephv1.ObjectStoreSpec{
				MetadataPool: cephv1.PoolSpec{
					Replicated:      cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false},
					CompressionMode: "passive",
				},
				DataPool: cephv1.PoolSpec{
					Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false},
				},
				Gateway: cephv1.GatewaySpec{
					Port:      80,
					Instances: 1,
				},
			},
		}

		cosClient    = k8sh.RookClientset.CephV1().CephObjectStores(clusterNS)
		deployClient = k8sh.Clientset.AppsV1().Deployments(clusterNS)
		svcClient    = k8sh.Clientset.CoreV1().Services(clusterNS)
	)

	t.Run("CephObjectStore lifecycle", func(t *testing.T) {
		ctx := t.Context()

		// canary: catch new RGW pool fields added in newer Ceph versions that
		// rook's zonePoolNSSuffix map does not yet cover, which would cause
		// ghost default pools when shared pools are configured
		t.Run("all zone.json pool fields are covered by the shared pool mapping", func(t *testing.T) {
			sharedName := store.ObjectStore().Name
			output, err := store.Installer().Execute("radosgw-admin",
				[]string{"zone", "get", fmt.Sprintf("--rgw-zone=%s", sharedName), fmt.Sprintf("--rgw-realm=%s", sharedName)}, clusterNS)
			require.NoError(t, err, "failed to get zone config; output: %s", output)
			require.NotEmpty(t, output, "zone config is empty")

			var zoneConfig map[string]any
			err = json.Unmarshal([]byte(output), &zoneConfig)
			require.NoError(t, err, "failed to parse zone config JSON; output: %s", output)

			knownPools := rgw.ZoneJsonPoolKeys()
			for field, val := range zoneConfig {
				if _, ok := val.(string); !ok {
					continue
				}
				if !strings.HasSuffix(field, "_pool") {
					continue
				}
				assert.Contains(t, knownPools, field,
					"RGW zone.json contains unknown pool field %q — add it to zonePoolNSSuffix in pkg/operator/ceph/object/objectstore.go", field)
			}
		})

		if store.TLSEnabled() {
			client.GenerateRgwTLSCertSecret(t, k8sh, clusterNS, certSecretName, rgwName)
			t.Cleanup(func() {
				// the store consumes the secret only while it exists; cleanup
				// runs after the test context is canceled
				_ = k8sh.Clientset.CoreV1().Secrets(clusterNS).Delete(context.Background(), certSecretName, metav1.DeleteOptions{})
			})
			privateStore.Spec.Gateway = cephv1.GatewaySpec{
				SecurePort:        443,
				SSLCertificateRef: certSecretName,
				Instances:         1,
			}
		}

		t.Run(fmt.Sprintf("create CephObjectStore %q", privateStore.Name), func(t *testing.T) {
			// store creation races rgw startup; same bespoke timeout as the
			// shared store fixture
			live := wait4.RequireCreate(ctx, t, cosClient, privateStore, wait4.ObjectStore, 3*time.Minute)
			assert.NotEmpty(t, live.Status.Info["endpoint"])
		})

		t.Run(fmt.Sprintf("rgw deployment for store %q is ready", privateStore.Name), func(t *testing.T) {
			wait4.RequireCondition(ctx, t, deployClient, rgwName+"-a",
				func(d *appsv1.Deployment) bool { return d.Status.ReadyReplicas >= 1 }, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("rgw service for store %q exists", privateStore.Name), func(t *testing.T) {
			_, err := svcClient.Get(ctx, rgwName, metav1.GetOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete CephObjectStore %q", privateStore.Name), func(t *testing.T) {
			wait4.RequireDelete(ctx, t, cosClient, privateStore.Name, 3*time.Minute)
		})
	})
}
