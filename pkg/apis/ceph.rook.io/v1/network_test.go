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

package v1

import (
	"encoding/json"
	"fmt"
	"testing"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestNetworkCephSpecLegacy(t *testing.T) {
	netSpecYAML := []byte(`hostNetwork: true`)

	rawJSON, err := yaml.ToJSON(netSpecYAML)
	assert.Nil(t, err)

	var net NetworkSpec

	err = json.Unmarshal(rawJSON, &net)
	assert.Nil(t, err)

	expected := NetworkSpec{HostNetwork: true}

	assert.Equal(t, expected, net)
}

func TestValidateNetworkSpec(t *testing.T) {
	net := NetworkSpec{
		HostNetwork: true,
		Provider:    NetworkProviderDefault,
	}
	err := ValidateNetworkSpec("", net)
	assert.NoError(t, err)

	net = NetworkSpec{
		HostNetwork: true,
		Provider:    NetworkProviderHost,
	}
	err = ValidateNetworkSpec("", net)
	assert.Error(t, err)

	net = NetworkSpec{
		HostNetwork: false,
		Provider:    NetworkProviderDefault,
	}
	err = ValidateNetworkSpec("", net)
	assert.NoError(t, err)

	net = NetworkSpec{
		HostNetwork: false,
		Provider:    NetworkProviderHost,
	}
	err = ValidateNetworkSpec("", net)
	assert.NoError(t, err)
}

// test the NetworkSpec.IsHost method with different network providers
// Also test it in combination with the legacy
// "HostNetwork" setting.
// Also test the effect of the operator config setting
// ROOK_ENFORCE_HOST_NETWORK.
func TestNetworkCephIsHost(t *testing.T) {
	net := NetworkSpec{HostNetwork: false}

	net.Provider = NetworkProviderHost
	assert.True(t, net.IsHost())

	net.Provider = NetworkProviderDefault
	net.HostNetwork = true
	assert.True(t, net.IsHost())

	// enforcing does not change the result if host network is selected
	// anyway in the cluster.
	SetEnforceHostNetwork(true)
	assert.True(t, net.IsHost())

	SetEnforceHostNetwork(false)
	assert.True(t, net.IsHost())

	net = NetworkSpec{}
	net.Provider = NetworkProviderDefault
	net.HostNetwork = false
	assert.False(t, net.IsHost())

	net = NetworkSpec{}
	net.Provider = NetworkProviderMultus
	net.HostNetwork = false
	assert.False(t, net.IsHost())

	// test that not enforcing does not change the result.
	SetEnforceHostNetwork(false)
	assert.False(t, net.IsHost())

	// test enforcing of host network
	SetEnforceHostNetwork(true)
	assert.True(t, net.IsHost())

	SetEnforceHostNetwork(false)
	net = NetworkSpec{}
	net.Provider = NetworkProviderMultus
	net.HostNetwork = true
	assert.False(t, net.IsHost())

	// test with nonempty but invalid provider
	net = NetworkSpec{}
	net.HostNetwork = true
	net.Provider = "foo"
	SetEnforceHostNetwork(false)
	assert.False(t, net.IsHost())
	SetEnforceHostNetwork(true)
	assert.True(t, net.IsHost())

}

func TestNetworkSpec(t *testing.T) {
	netSpecYAML := []byte(`
provider: host
selectors:
  server: enp2s0f0
  broker: enp2s0f0`)

	rawJSON, err := yaml.ToJSON(netSpecYAML)
	assert.Nil(t, err)

	var net NetworkSpec

	err = json.Unmarshal(rawJSON, &net)
	assert.Nil(t, err)

	expected := NetworkSpec{
		Provider: "host",
		Selectors: map[CephNetworkType]string{
			"server": "enp2s0f0",
			"broker": "enp2s0f0",
		},
	}

	assert.Equal(t, expected, net)
}

func TestAddressRangesSpec_IsEmpty(t *testing.T) {
	var specNil *AddressRangesSpec
	assert.True(t, specNil.IsEmpty())

	empty := &AddressRangesSpec{}
	assert.True(t, empty.IsEmpty())

	someCIDR := CIDR("1.1.1.1/16")
	nonEmptyTests := []AddressRangesSpec{
		{Public: []CIDR{someCIDR}},
		{Public: []CIDR{someCIDR, someCIDR}},
		{Cluster: []CIDR{someCIDR}},
		{Cluster: []CIDR{someCIDR, someCIDR}},
		{Public: []CIDR{someCIDR}, Cluster: []CIDR{someCIDR}},
		{Public: []CIDR{someCIDR, someCIDR}, Cluster: []CIDR{someCIDR, someCIDR}},
	}
	for _, spec := range nonEmptyTests {
		assert.False(t, spec.IsEmpty())
	}

}

func TestAddressRangesSpec_Validate(t *testing.T) {
	// only test a small subset of CIDRs since Rook should definitely use the Go stdlib underneath
	v1 := CIDR("123.123.123.123/24")
	v2 := CIDR("1.0.0.1/24")
	v3 := CIDR("2000::/64")
	v4 := CIDR("2000:2000:2000:2000:2000:2000:2000:2000/64")
	v5 := CIDR("2000::128.128.128.128/96") // ipv4 expressed as subnet of ipv6 is valid

	// invalid CIDRs
	i1 := CIDR("123.123.123/24")
	i2 := CIDR("123.123.123.123/33")
	i4 := CIDR("2000/64")
	i3 := CIDR("2000:/64")
	i5 := CIDR("2000::128.128.128.128/129")

	tests := []struct {
		name    string
		spec    AddressRangesSpec
		numErrs int
	}{
		{"empty", AddressRangesSpec{}, 0},
		{"all valid", AddressRangesSpec{
			Public:  []CIDR{v1},
			Cluster: []CIDR{v2, v3, v4, v5},
		}, 0},
		{"all invalid", AddressRangesSpec{
			Public:  []CIDR{i1},
			Cluster: []CIDR{i2, i3, i4, i5},
		}, 5},
		{"public only, valid", AddressRangesSpec{Public: []CIDR{v1}}, 0},
		{"public only, invalid", AddressRangesSpec{Public: []CIDR{i1}}, 1},
		{"cluster only, valid", AddressRangesSpec{Cluster: []CIDR{v2}}, 0},
		{"cluster only, invalid", AddressRangesSpec{Cluster: []CIDR{i2}}, 1},
		{"public valid, cluster valid", AddressRangesSpec{
			Public:  []CIDR{v1},
			Cluster: []CIDR{v2},
		}, 0},
		{"public valid, cluster invalid", AddressRangesSpec{
			Public:  []CIDR{v2},
			Cluster: []CIDR{i2},
		}, 1},
		{"public invalid, cluster valid", AddressRangesSpec{
			Public:  []CIDR{i3},
			Cluster: []CIDR{v2},
		}, 1},
		{"public invalid, cluster invalid", AddressRangesSpec{
			Public:  []CIDR{i3},
			Cluster: []CIDR{i4},
		}, 2},
		{"both, valid and invalid", AddressRangesSpec{
			Public:  []CIDR{v1, i2},
			Cluster: []CIDR{v3, i4},
		}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.numErrs > 0 {
				assert.Error(t, err)
				t.Log(err)
				assert.ErrorContains(t, err, fmt.Sprintf("%d network ranges are invalid", tt.numErrs))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// these two functions are should almost always used together and can be unit tested together more
// easily than apart
func TestNetworkSpec_GetNetworkSelection_NetworkSelectionsToAnnotationValue(t *testing.T) {
	// inputs are the same definition expressed in json format or non-json format
	input1 := func(json bool) string {
		if json {
			return `[{"name": "macvlan", "interface": "net1"}]`
		}
		return "macvlan@net1"
	}
	input2 := func(json bool) string {
		if json {
			return `[{"name": "macvlan", "interface": "net2"}]`
		}
		return "macvlan@net2"
	}

	// allow running the test suite with json-format or non-json-format inputs
	testGetNetworkAnnotationValue := func(t *testing.T, json bool) {
		t.Helper()

		tests := []struct {
			name          string
			specSelectors map[CephNetworkType]string
			cephNets      []CephNetworkType
			want          string
			wantErr       bool
		}{
			{
				name: "public want public",
				specSelectors: map[CephNetworkType]string{
					"public": input1(json),
				},
				cephNets: []CephNetworkType{CephNetworkPublic},
				want:     `[{"name":"macvlan","namespace":"ns","interface":"net1"}]`,
				wantErr:  false,
			},
			{
				name: "cluster want cluster",
				specSelectors: map[CephNetworkType]string{
					"cluster": input1(json),
				},
				cephNets: []CephNetworkType{CephNetworkCluster},
				want:     `[{"name":"macvlan","namespace":"ns","interface":"net1"}]`,
				wantErr:  false,
			},
			{
				name: "public want cluster",
				specSelectors: map[CephNetworkType]string{
					"public": input1(json),
				},
				cephNets: []CephNetworkType{CephNetworkCluster},
				want:     ``,
				wantErr:  false,
			},
			{
				name: "cluster want public",
				specSelectors: map[CephNetworkType]string{
					"cluster": input1(json),
				},
				cephNets: []CephNetworkType{CephNetworkPublic},
				want:     ``,
				wantErr:  false,
			},
			{
				name:          "nothing want public",
				specSelectors: map[CephNetworkType]string{},
				cephNets:      []CephNetworkType{CephNetworkPublic},
				want:          ``,
				wantErr:       false,
			},
			{
				name:          "nothing want cluster",
				specSelectors: map[CephNetworkType]string{},
				cephNets:      []CephNetworkType{CephNetworkCluster},
				want:          ``,
				wantErr:       false,
			},
			{
				name: "unknown want public",
				specSelectors: map[CephNetworkType]string{
					"uncleKnown": input1(json),
				},
				cephNets: []CephNetworkType{CephNetworkPublic},
				want:     ``,
				wantErr:  false,
			},
			{
				name: "unknown want cluster",
				specSelectors: map[CephNetworkType]string{
					"uncleKnown": input1(json),
				},
				cephNets: []CephNetworkType{CephNetworkCluster},
				want:     ``,
				wantErr:  false,
			},
			{
				name: "public want public and cluster",
				specSelectors: map[CephNetworkType]string{
					"public": input1(json),
				},
				cephNets: []CephNetworkType{CephNetworkPublic, CephNetworkCluster},
				want:     `[{"name":"macvlan","namespace":"ns","interface":"net1"}]`,
				wantErr:  false,
			},
			{
				name: "cluster want public and cluster",
				specSelectors: map[CephNetworkType]string{
					"cluster": input1(json),
				},
				cephNets: []CephNetworkType{CephNetworkPublic, CephNetworkCluster},
				want:     `[{"name":"macvlan","namespace":"ns","interface":"net1"}]`,
				wantErr:  false,
			},
			{
				name: "public and cluster want public and cluster",
				specSelectors: map[CephNetworkType]string{
					"public":  input1(json),
					"cluster": input2(json),
				},
				cephNets: []CephNetworkType{CephNetworkPublic, CephNetworkCluster},
				want:     `[{"name":"macvlan","namespace":"ns","interface":"net1"},{"name":"macvlan","namespace":"ns","interface":"net2"}]`,
				wantErr:  false,
			},
			{
				name: "support mixed json-non-json spec",
				specSelectors: map[CephNetworkType]string{
					"public":  input1(json),
					"cluster": input2(!json), // invert json-ness of this one
				},
				cephNets: []CephNetworkType{CephNetworkPublic, CephNetworkCluster},
				want:     `[{"name":"macvlan","namespace":"ns","interface":"net1"},{"name":"macvlan","namespace":"ns","interface":"net2"}]`,
				wantErr:  false,
			},
			{
				name: "public and cluster want nothing",
				specSelectors: map[CephNetworkType]string{
					"public":  input1(json),
					"cluster": input2(json),
				},
				cephNets: []CephNetworkType{},
				want:     ``,
				wantErr:  false,
			},
			{
				name: "legacy single json object support",
				specSelectors: map[CephNetworkType]string{
					"public": `{"name": "legacyJsonObject"}`,
				},
				cephNets: []CephNetworkType{CephNetworkPublic, CephNetworkCluster},
				want:     `[{"name":"legacyJsonObject","namespace":"ns"}]`,
				wantErr:  false,
			},
			{
				name: "invalid network selections",
				specSelectors: map[CephNetworkType]string{
					"public":  `[{"name": "jsonWithNoClosingBracket"}`,
					"cluster": "multus%net",
				},
				cephNets: []CephNetworkType{CephNetworkPublic, CephNetworkCluster},
				want:     ``,
				wantErr:  true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				n := &NetworkSpec{
					Selectors: tt.specSelectors,
				}

				selections := []*nadv1.NetworkSelectionElement{}
				errs := []error{}
				for _, net := range tt.cephNets {
					s, err := n.GetNetworkSelection("ns", net)
					if err != nil {
						errs = append(errs, err)
					}
					selections = append(selections, s)
				}
				got, err := NetworkSelectionsToAnnotationValue(selections...)
				if err != nil {
					errs = append(errs, err)
				}

				assert.Equal(t, tt.wantErr, len(errs) > 0, "wantErr %v but got errs %v", tt.wantErr, errs)
				assert.Equal(t, tt.want, got)
			})
		}
	}

	// Actual subtests
	t.Run("non-JSON input", func(t *testing.T) {
		testGetNetworkAnnotationValue(t, false)
	})
	t.Run("JSON input", func(t *testing.T) {
		testGetNetworkAnnotationValue(t, true)
	})
}
