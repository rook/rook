/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package object

import (
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func newConfig(t *testing.T) *clusterConfig {
	clusterInfo := &cephclient.ClusterInfo{
		CephVersion: cephver.Squid,
	}
	clusterSpec := &cephv1.ClusterSpec{
		Network: cephv1.NetworkSpec{
			HostNetwork: false,
		},
	}
	return &clusterConfig{
		store: &cephv1.CephObjectStore{
			Spec: cephv1.ObjectStoreSpec{
				Gateway: cephv1.GatewaySpec{},
			},
		},
		clusterInfo: clusterInfo,
		clusterSpec: clusterSpec,
		context:     &clusterd.Context{Clientset: test.New(t, 3)},
	}
}

func TestPortString(t *testing.T) {
	// No port or secure port on beast
	cfg := newConfig(t)
	result := cfg.portString()
	assert.Equal(t, "", result)

	// Insecure port on beast
	cfg = newConfig(t)
	// Set host networking
	cfg.clusterSpec.Network.HostNetwork = true
	cfg.store.Spec.Gateway.Port = 80
	result = cfg.portString()
	assert.Equal(t, "port=80", result)

	// Secure port on beast
	cfg = newConfig(t)
	cfg.store.Spec.Gateway.SecurePort = 443
	cfg.store.Spec.Gateway.SSLCertificateRef = "some-k8s-key-secret"
	result = cfg.portString()
	assert.Equal(t, "ssl_port=443 ssl_certificate=/etc/ceph/private/rgw-cert.pem", result)

	// Both ports on beast
	cfg = newConfig(t)
	// Set host networking
	cfg.clusterSpec.Network.HostNetwork = true
	cfg.store.Spec.Gateway.Port = 80
	cfg.store.Spec.Gateway.SecurePort = 443
	cfg.store.Spec.Gateway.SSLCertificateRef = "some-k8s-key-secret"
	result = cfg.portString()
	assert.Equal(t, "port=80 ssl_port=443 ssl_certificate=/etc/ceph/private/rgw-cert.pem", result)

	// Secure port requires the cert on beast
	cfg = newConfig(t)
	cfg.store.Spec.Gateway.SecurePort = 443
	result = cfg.portString()
	assert.Equal(t, "", result)

	// Using SDN, no host networking so the rgw port internal is not the same
	cfg = newConfig(t)
	cfg.store.Spec.Gateway.Port = 80
	result = cfg.portString()
	assert.Equal(t, "port=8080", result)
}

func TestRgwFrontendStr(t *testing.T) {
	// No Security set: default ssl_options always applied
	cfg := newConfig(t)
	cfg.clusterSpec.Network.HostNetwork = true
	cfg.store.Spec.Gateway.Port = 80
	result := cfg.rgwFrontendStr()
	assert.Equal(t, "beast port=80 ssl_options=no_sslv2:no_sslv3:no_tlsv1:no_tlsv1_1", result, "case 1")

	// Empty SSLOptions struct: same as default
	cfg = newConfig(t)
	cfg.clusterSpec.Network.HostNetwork = true
	cfg.store.Spec.Gateway.Port = 80
	cfg.store.Spec.Security = &cephv1.ObjectStoreSecuritySpec{
		SSLOptions: &cephv1.SSLOptionsSpec{},
	}
	result = cfg.rgwFrontendStr()
	assert.Equal(t, "beast port=80 ssl_options=no_sslv2:no_sslv3:no_tlsv1:no_tlsv1_1", result, "case 2")

	// Restrict to TLS 1.3 only (all old protocols + TLS 1.2 disabled)
	boolPtr := func(b bool) *bool { return &b }
	cfg = newConfig(t)
	cfg.clusterSpec.Network.HostNetwork = true
	cfg.store.Spec.Gateway.Port = 80
	cfg.store.Spec.Security = &cephv1.ObjectStoreSecuritySpec{
		SSLOptions: &cephv1.SSLOptionsSpec{
			SSLv2:   boolPtr(false),
			SSLv3:   boolPtr(false),
			TLSv1_0: boolPtr(false),
			TLSv1_1: boolPtr(false),
			TLSv1_2: boolPtr(false),
		},
	}
	result = cfg.rgwFrontendStr()
	assert.Equal(t, "beast port=80 ssl_options=no_sslv2:no_sslv3:no_tlsv1:no_tlsv1_1:no_tlsv1_2", result, "case 3")

	// With ciphers only (default ssl_options still applied)
	cfg = newConfig(t)
	cfg.clusterSpec.Network.HostNetwork = true
	cfg.store.Spec.Gateway.Port = 80
	cfg.store.Spec.Security = &cephv1.ObjectStoreSecuritySpec{
		Ciphers: []cephv1.OpenSslCipher{"AES256-SHA", "AES128-SHA"},
	}
	result = cfg.rgwFrontendStr()
	assert.Equal(t, "beast port=80 ssl_options=no_sslv2:no_sslv3:no_tlsv1:no_tlsv1_1 ssl_ciphers=AES256-SHA:AES128-SHA", result, "case 4")

	// With both SSL options and ciphers
	cfg = newConfig(t)
	cfg.clusterSpec.Network.HostNetwork = true
	cfg.store.Spec.Gateway.Port = 80
	cfg.store.Spec.Security = &cephv1.ObjectStoreSecuritySpec{
		SSLOptions: &cephv1.SSLOptionsSpec{},
		Ciphers:    []cephv1.OpenSslCipher{"AES256-SHA"},
	}
	result = cfg.rgwFrontendStr()
	assert.Equal(t, "beast port=80 ssl_options=no_sslv2:no_sslv3:no_tlsv1:no_tlsv1_1 ssl_ciphers=AES256-SHA", result, "case 5")
}

func TestBuildSSLOptions(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	// All nil (nothing set): Ceph default applied
	result := buildSSLOptions(&cephv1.SSLOptionsSpec{})
	assert.Equal(t, "no_sslv2:no_sslv3:no_tlsv1:no_tlsv1_1", result, "case 1")

	// All protocols explicitly disabled
	result = buildSSLOptions(&cephv1.SSLOptionsSpec{
		SSLv2:   boolPtr(false),
		SSLv3:   boolPtr(false),
		TLSv1_0: boolPtr(false),
		TLSv1_1: boolPtr(false),
		TLSv1_2: boolPtr(false),
	})
	assert.Equal(t, "no_sslv2:no_sslv3:no_tlsv1:no_tlsv1_1:no_tlsv1_2", result, "case 2")

	// Enable TLSv1.1, rest disabled, TLS 1.2 enabled
	result = buildSSLOptions(&cephv1.SSLOptionsSpec{
		SSLv2:   boolPtr(false),
		SSLv3:   boolPtr(false),
		TLSv1_0: boolPtr(false),
		TLSv1_1: boolPtr(true),
		TLSv1_2: boolPtr(true),
	})
	assert.Equal(t, "no_sslv2:no_sslv3:no_tlsv1", result, "case 3")

	// Enable all protocols — no "no_" flags
	result = buildSSLOptions(&cephv1.SSLOptionsSpec{
		SSLv2:   boolPtr(true),
		SSLv3:   boolPtr(true),
		TLSv1_0: boolPtr(true),
		TLSv1_1: boolPtr(true),
		TLSv1_2: boolPtr(true),
	})
	assert.Equal(t, "", result, "case 4")

	// Non-protocol options only (protocol fields nil → Ceph default)
	result = buildSSLOptions(&cephv1.SSLOptionsSpec{
		DefaultWorkarounds:     true,
		NoCompression:          true,
		SingleDiffieHellmanUse: boolPtr(true),
	})
	assert.Equal(t, "default_workarounds:no_compression:single_dh_use:no_sslv2:no_sslv3:no_tlsv1:no_tlsv1_1", result, "case 5")

	// Non-protocol options with explicit protocol config
	result = buildSSLOptions(&cephv1.SSLOptionsSpec{
		DefaultWorkarounds: true,
		TLSv1_2:            boolPtr(true),
	})
	assert.Equal(t, "default_workarounds:no_sslv2:no_sslv3:no_tlsv1:no_tlsv1_1", result, "case 6")
}

func TestGenerateCephXUser(t *testing.T) {
	fakeUser := generateCephXUser("rook-ceph-rgw-fake-store-fake-user")
	assert.Equal(t, "client.rgw.fake.store.fake.user", fakeUser)
}

// general testing theory here is that this easy unit test covers almost all of the RGW configs
// applied via mon store for all the various user input configurations. e2e tests will verify that
// RGW configs determined here are applied to the running RGW
func Test_clusterConfig_generateMonConfigOptions(t *testing.T) {
	defaultConfigs := map[string]string{
		"rgw_enable_usage_log":       "true",
		"rgw_log_nonexistent_bucket": "true",
		"rgw_log_object_name_utc":    "true",
		"rgw_run_sync_thread":        "true",
		"rgw_zone":                   "zone",
		"rgw_zonegroup":              "zone-group",
	}

	// overlay string slice as map KVs on top of default configs (can append or modify)
	overlayOnDefaultConfigs := func(kv ...string) map[string]string {
		out := map[string]string{}
		for k, v := range defaultConfigs {
			out[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			out[kv[i]] = kv[i+1]
		}
		return out
	}

	tests := []struct {
		name            string // test name
		objectStoreSpec *cephv1.ObjectStoreSpec
		want            map[string]string
		wantErr         bool
	}{
		{"empty spec", &cephv1.ObjectStoreSpec{}, defaultConfigs, false},
		{"multisite sync enabled", &cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{DisableMultisiteSyncTraffic: true},
		}, overlayOnDefaultConfigs("rgw_run_sync_thread", "false"), false},
		{"empty rgwConfig", &cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{RgwConfig: map[string]string{}},
		}, defaultConfigs, false},
		{"one add rgwConfig", &cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{RgwConfig: map[string]string{"one": "add"}},
		}, overlayOnDefaultConfigs("one", "add"), false},
		{"two add rgwConfig", &cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{RgwConfig: map[string]string{"one": "add", "two": "add"}},
		}, overlayOnDefaultConfigs("one", "add", "two", "add"), false},
		{"one add one modify rgwConfig", &cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{RgwConfig: map[string]string{"one": "add", "rgw_enable_usage_log": "false"}},
		}, overlayOnDefaultConfigs("one", "add", "rgw_enable_usage_log", "false"), false},
		{"rgwCommandFlags set", &cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{RgwCommandFlags: map[string]string{"one": "add", "rgw_enable_usage_log": "false"}},
		}, defaultConfigs, false}, // verifies rgwCommandFlags don't affect mon config store
		{"test all configs", &cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{
				DisableMultisiteSyncTraffic: true,
				RgwConfig:                   map[string]string{"one": "add", "rgw_enable_usage_log": "false"},
				RgwCommandFlags:             map[string]string{"two": "add", "rgw_zone": "bob"},
			},
		}, overlayOnDefaultConfigs("rgw_run_sync_thread", "false", "one", "add", "rgw_enable_usage_log", "false"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos := &cephv1.CephObjectStore{
				Spec: *tt.objectStoreSpec,
			}
			cos.Namespace = "ns"
			cos.Name = "my-store"
			rgwConfig := &rgwConfig{
				ResourceName: "rook-ceph-rgw-my-store-a",
				DaemonID:     "my-store-a",
				Realm:        "realm",
				ZoneGroup:    "zone-group",
				Zone:         "zone",
			}

			c := &clusterConfig{
				store:       cos,
				clusterInfo: &cephclient.ClusterInfo{Namespace: "ns"},
			}

			got, err := c.generateMonConfigOptions(rgwConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("clusterConfig.generateMonConfigOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRgwConfigFromSecret(t *testing.T) {
	objContext := &Context{
		Context: &clusterd.Context{
			Clientset: test.New(t, 3),
		},
		clusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"),
	}

	objectStore := simpleStore()
	objectStore.Spec.Gateway.RgwConfigFromSecret = map[string]v1.SecretKeySelector{
		"rgw_secret_conf_name": {
			LocalObjectReference: v1.LocalObjectReference{
				Name: "my-secret",
			},
			Key: "secKey",
		},
	}

	c := &clusterConfig{
		store:       objectStore,
		context:     objContext.Context,
		clusterInfo: objContext.clusterInfo,
		ownerInfo:   k8sutil.NewOwnerInfo(objectStore, runtime.NewScheme()),
	}

	rgwConfig := &rgwConfig{}

	_, err := c.generateMonConfigOptions(rgwConfig)
	assert.Error(t, err, "error if secret not exists")

	s := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "rook-ceph",
		},
		Data: map[string][]byte{
			"secKey": []byte("secVal"),
		},
	}
	_, err = objContext.Context.Clientset.CoreV1().Secrets(objContext.clusterInfo.Namespace).Create(context.TODO(), s, metav1.CreateOptions{})
	assert.NoError(t, err, "create secret")

	got, err := c.generateMonConfigOptions(rgwConfig)
	assert.NoError(t, err)
	assert.EqualValues(t, "secVal", got["rgw_secret_conf_name"])
}
