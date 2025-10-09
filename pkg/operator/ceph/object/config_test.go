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

func TestBeastConfig(t *testing.T) {
	tests := []struct {
		name      string
		beastOpts cephv1.BeastOptions
		output    string
	}{
		{
			name: "Sets max backlog and so_reuseport",
			beastOpts: cephv1.BeastOptions{
				MaxConnectionBacklog: 100,
				SoReusePort:          true,
			},
			output: "port=80 max_connection_backlog=100 so_reuseport=1",
		},
		{
			name: "Sets max backlog and so_reuse port to false",
			beastOpts: cephv1.BeastOptions{
				MaxConnectionBacklog: 100,
				SoReusePort:          false,
			},
			output: "port=80 max_connection_backlog=100",
		},
		{
			name:      "Do not set any beast options",
			beastOpts: cephv1.BeastOptions{},
			output:    "port=80",
		},
		{
			name: "Set all available options",
			beastOpts: cephv1.BeastOptions{
				TcpNoDelay:           true,
				MaxConnectionBacklog: 10,
				RequestTimeoutMs:     2000,
				MaxHeaderSize:        10100,
				SoReusePort:          true,
			},
			output: "port=80 tcp_nodelay=1 max_connection_backlog=10 request_timeout_ms=2000 max_header_size=10100 so_reuseport=1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(t)
			cfg.clusterSpec.Network.HostNetwork = true
			cfg.store.Spec.Gateway.Port = 80
			cfg.store.Spec.Gateway.BeastOpts = tt.beastOpts
			result := cfg.beastConfig()
			assert.Equal(t, tt.output, result)
		})
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
				store: cos,
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
