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
package clusterd

import "testing"
import "github.com/stretchr/testify/assert"

func TestVerifyNetworkInfo(t *testing.T) {
	// empty network info is OK
	var networkInfo NetworkInfo
	err := VerifyNetworkInfo(networkInfo)
	assert.Nil(t, err)

	// well formed network info is OK
	networkInfo = NetworkInfo{
		PublicAddr:     "10.1.1.1",
		PublicNetwork:  "10.1.1.0/24",
		ClusterAddr:    "10.1.2.2",
		ClusterNetwork: "10.1.2.0/24",
	}
	err = VerifyNetworkInfo(networkInfo)
	assert.Nil(t, err)

	// malformed IP address is not OK
	networkInfo = NetworkInfo{
		PublicAddr:     "10.1.1.256",
		PublicNetwork:  "10.1.1.0/24",
		ClusterAddr:    "10.1.2.256",
		ClusterNetwork: "10.1.2.0/24",
	}
	err = VerifyNetworkInfo(networkInfo)
	assert.NotNil(t, err)

	// malformed network address is not OK
	networkInfo = NetworkInfo{
		PublicAddr:     "10.1.1.1",
		PublicNetwork:  "10.1.1.0/33",
		ClusterAddr:    "10.1.2.2",
		ClusterNetwork: "10.1.2.0/33",
	}
	err = VerifyNetworkInfo(networkInfo)
	assert.NotNil(t, err)
}

func TestNetworkInfoSimplify(t *testing.T) {

	out := NetworkInfo{
		PublicAddr:     "10.1.1.1",
		PublicNetwork:  "10.1.1.0/24",
		ClusterAddr:    "10.1.2.2",
		ClusterNetwork: "10.1.2.0/24",
	}

	// only has old fields
	in := NetworkInfo{
		PublicAddrIPv4:  "10.1.1.1",
		PublicNetwork:   "10.1.1.0/24",
		ClusterAddrIPv4: "10.1.2.2",
		ClusterNetwork:  "10.1.2.0/24",
	}
	assert.Equal(t, out, in.Simplify())

	// has both new and old fields
	in = NetworkInfo{
		PublicAddr:      "10.1.1.1",
		PublicAddrIPv4:  "10.9.1.1",
		PublicNetwork:   "10.1.1.0/24",
		ClusterAddr:     "10.1.2.2",
		ClusterAddrIPv4: "10.9.2.2",
		ClusterNetwork:  "10.1.2.0/24",
	}
	assert.Equal(t, out, in.Simplify())

}
