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
		PublicAddrIPv4:  "10.1.1.1",
		PublicNetwork:   "10.1.1.0/24",
		ClusterAddrIPv4: "10.1.2.2",
		ClusterNetwork:  "10.1.2.0/24",
	}
	err = VerifyNetworkInfo(networkInfo)
	assert.Nil(t, err)

	// malformed IP address is not OK
	networkInfo = NetworkInfo{
		PublicAddrIPv4:  "10.1.1.256",
		PublicNetwork:   "10.1.1.0/24",
		ClusterAddrIPv4: "10.1.2.256",
		ClusterNetwork:  "10.1.2.0/24",
	}
	err = VerifyNetworkInfo(networkInfo)
	assert.NotNil(t, err)

	// malformed network address is not OK
	networkInfo = NetworkInfo{
		PublicAddrIPv4:  "10.1.1.1",
		PublicNetwork:   "10.1.1.0/33",
		ClusterAddrIPv4: "10.1.2.2",
		ClusterNetwork:  "10.1.2.0/33",
	}
	err = VerifyNetworkInfo(networkInfo)
	assert.NotNil(t, err)
}
