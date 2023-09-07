/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package k8sutil

import (
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApplyMultus(t *testing.T) {
	json1 := `[{"name": "macvlan", "interface": "net1"}]`
	json2 := `[{"name": "macvlan", "interface": "net2"}]`

	tests := []struct {
		name         string
		netSelectors map[cephv1.CephNetworkType]string
		labels       map[string]string
		want         string
	}{
		{
			name: "non-osd pods don't get unknown nets",
			netSelectors: map[cephv1.CephNetworkType]string{
				"aruba":  "macvlan@net1",
				"bahama": "macvlan@net2",
			},
			want: ``,
		},
		{
			name: "osd pods don't get unknown nets",
			netSelectors: map[cephv1.CephNetworkType]string{
				"key-largo": "macvlan@net1",
				"montego":   "macvlan@net2",
			},
			labels: map[string]string{"app": "rook-ceph-osd"},
			want:   ``,
		},

		{
			name: "non-osd pods get only public networks",
			netSelectors: map[cephv1.CephNetworkType]string{
				"public":  "macvlan@net1",
				"cluster": "macvlan@net2",
			},
			want: `[{"name":"macvlan","namespace":"ns","interface":"net1"}]`,
		},
		{
			name: "osd pods get both networks",
			netSelectors: map[cephv1.CephNetworkType]string{
				"public":  "macvlan@net1",
				"cluster": "macvlan@net2",
			},
			labels: map[string]string{"app": "rook-ceph-osd"},
			want:   `[{"name":"macvlan","namespace":"ns","interface":"net1"},{"name":"macvlan","namespace":"ns","interface":"net2"}]`,
		},

		{
			name: "osd pod network ordering is not reversed",
			netSelectors: map[cephv1.CephNetworkType]string{
				"cluster": "macvlan@net2",
				"public":  "macvlan@net1",
			},
			labels: map[string]string{"app": "rook-ceph-osd"},
			want:   `[{"name":"macvlan","namespace":"ns","interface":"net1"},{"name":"macvlan","namespace":"ns","interface":"net2"}]`, // should not change the order of output
		},

		{
			name: "non-osd pods take json input",
			netSelectors: map[cephv1.CephNetworkType]string{
				"public":  json1,
				"cluster": json2,
			},
			want: `[{"name":"macvlan","namespace":"ns","interface":"net1"}]`,
		},
		{
			name: "osd pods take json input",
			netSelectors: map[cephv1.CephNetworkType]string{
				"public":  json1,
				"cluster": json2,
			},
			labels: map[string]string{"app": "rook-ceph-osd"},
			want:   `[{"name":"macvlan","namespace":"ns","interface":"net1"},{"name":"macvlan","namespace":"ns","interface":"net2"}]`,
		},
		{
			name: "osd pod network ordering is not reversed with json input",
			netSelectors: map[cephv1.CephNetworkType]string{
				"cluster": json2,
				"public":  json1,
			},
			labels: map[string]string{"app": "rook-ceph-osd"},
			want:   `[{"name":"macvlan","namespace":"ns","interface":"net1"},{"name":"macvlan","namespace":"ns","interface":"net2"}]`,
		},

		{
			name: "mixed json-non-json format is allowed",
			netSelectors: map[cephv1.CephNetworkType]string{
				"public":  "macvlan@net1",
				"cluster": json2,
			},
			labels: map[string]string{"app": "rook-ceph-osd"},
			want:   `[{"name":"macvlan","namespace":"ns","interface":"net1"},{"name":"macvlan","namespace":"ns","interface":"net2"}]`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			netSpec := cephv1.NetworkSpec{
				Provider:  "multus",
				Selectors: test.netSelectors,
			}
			objMeta := metav1.ObjectMeta{}
			objMeta.Labels = test.labels
			err := ApplyMultus("ns", &netSpec, &objMeta)
			assert.NoError(t, err)
			assert.Equal(t, test.want, objMeta.Annotations["k8s.v1.cni.cncf.io/networks"])
		})
	}
}

func TestParseLinuxIpAddrOutput(t *testing.T) {
	tests := []struct {
		name            string
		ipAddrRawOutput string
		want            []LinuxIpAddrResult
		wantErr         bool
	}{
		{
			name:            "empty string",
			ipAddrRawOutput: "",
			want:            []LinuxIpAddrResult{},
			wantErr:         true,
		}, {
			name:            "full output",
			ipAddrRawOutput: ipAddrOutputMixedIPv4v6,
			want: []LinuxIpAddrResult{
				{
					InterfaceName: "public",
					AddrInfo: []LinuxIpAddrInfo{
						{
							Local:     "fd4e:7658:764f:15c4:8cec:4aff:fee3:1b96",
							PrefixLen: 64,
						}, {
							Local:     "2000::6",
							PrefixLen: 112,
						}, {
							Local:     "fe80::8cec:4aff:fee3:1b96",
							PrefixLen: 64,
						},
					},
				}, {
					InterfaceName: "net1",
					AddrInfo: []LinuxIpAddrInfo{
						{
							Local:     "192.168.20.8",
							PrefixLen: 24,
						},
						{
							Local:     "fd4e:7658:764f:15c4:9878:30ff:fec3:e504",
							PrefixLen: 64,
						},
						{
							Local:     "fe80::9878:30ff:fec3:e504",
							PrefixLen: 64,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:            "full output syntax error",
			ipAddrRawOutput: removeLastChar(ipAddrOutputMixedIPv4v6),
			want:            []LinuxIpAddrResult{},
			wantErr:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLinuxIpAddrOutput(tt.ipAddrRawOutput)
			if (err != nil) != tt.wantErr {
				t.Errorf("RuntimeLinuxIpAddrParser.Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

var ipAddrOutputMixedIPv4v6 = `[
    {
        "ifindex": 3,
        "link_index": 2,
        "ifname": "public",
        "flags": [
            "BROADCAST",
            "MULTICAST",
            "UP",
            "LOWER_UP"
        ],
        "mtu": 1500,
        "qdisc": "noqueue",
        "operstate": "UP",
        "group": "default",
        "link_type": "ether",
        "address": "8e:ec:4a:e3:1b:96",
        "broadcast": "ff:ff:ff:ff:ff:ff",
        "link_netnsid": 0,
        "addr_info": [
            {
                "family": "inet6",
                "local": "fd4e:7658:764f:15c4:8cec:4aff:fee3:1b96",
                "prefixlen": 64,
                "scope": "global",
                "dynamic": true,
                "mngtmpaddr": true,
                "valid_life_time": 2591998,
                "preferred_life_time": 604798
            },
            {
                "family": "inet6",
                "local": "2000::6",
                "prefixlen": 112,
                "scope": "global",
                "valid_life_time": 4294967295,
                "preferred_life_time": 4294967295
            },
            {
                "family": "inet6",
                "local": "fe80::8cec:4aff:fee3:1b96",
                "prefixlen": 64,
                "scope": "link",
                "valid_life_time": 4294967295,
                "preferred_life_time": 4294967295
            }
        ]
    },
    {
        "ifindex": 3,
        "link_index": 2,
        "ifname": "net1",
        "flags": [
          "BROADCAST",
          "MULTICAST",
          "UP",
          "LOWER_UP"
        ],
        "mtu": 1500,
        "qdisc": "noqueue",
        "operstate": "UP",
        "group": "default",
        "link_type": "ether",
        "address": "9a:78:30:c3:e5:04",
        "broadcast": "ff:ff:ff:ff:ff:ff",
        "link_netnsid": 0,
        "addr_info": [
          {
            "family": "inet",
            "local": "192.168.20.8",
            "prefixlen": 24,
            "broadcast": "192.168.20.255",
            "scope": "global",
            "label": "net1",
            "valid_life_time": 4294967295,
            "preferred_life_time": 4294967295
          },
          {
            "family": "inet6",
            "local": "fd4e:7658:764f:15c4:9878:30ff:fec3:e504",
            "prefixlen": 64,
            "scope": "global",
            "dynamic": true,
            "mngtmpaddr": true,
            "valid_life_time": 2591910,
            "preferred_life_time": 604710
          },
          {
            "family": "inet6",
            "local": "fe80::9878:30ff:fec3:e504",
            "prefixlen": 64,
            "scope": "link",
            "valid_life_time": 4294967295,
            "preferred_life_time": 4294967295
          }
        ]
      }
]`

func removeLastChar(in string) string {
	chars := []byte(strings.TrimSpace(in))
	len := len(chars)
	return string(chars[0 : len-1])
}
