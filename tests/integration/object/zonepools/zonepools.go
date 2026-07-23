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

package zonepools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
)

// TestZonePools is a canary against the shared store's zone: it asserts every
// *_pool field RGW writes into zone.json is covered by Rook's zonePoolNSSuffix
// map. A pool field introduced by a newer Ceph version that Rook does not yet
// map would otherwise silently spawn ghost default pools when shared pools are
// configured — which is exactly the shared store's configuration.
func TestZonePools(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	t.Run("zone.json pool field coverage", func(t *testing.T) {
		name := store.ObjectStore().Name
		ns := store.ObjectStore().Namespace

		output, err := store.Installer().Execute("radosgw-admin",
			[]string{"zone", "get", fmt.Sprintf("--rgw-zone=%s", name), fmt.Sprintf("--rgw-realm=%s", name)}, ns)
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
}
