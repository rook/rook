package multus

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
)

func TestGetAddressRangeValidConfig(t *testing.T) {
	validConfig := `{
		"cniVersion": "0.3.1",
		"type": "macvlan",
		"master": "enp1s0",
		"mode": "bridge",
		"ipam":
		{ 
			"type": "whereabouts",
			"range": "192.168.21.0/24"
		}
	}`

	result, err := GetAddressRange(validConfig)
	assert.NoError(t, err)
	assert.Equal(t, result, "192.168.21.0/24")
}

func TestGetAddressRangeInvalidConfig(t *testing.T) {
	invalidConfig := `{
		"cniVersion": "0.3.1",
		"type": "macvlan",
		"master": "enp1s0",
		"mode": "bridge",
		"ipam":
		{
			"type": "dhcp"
		}
	}`

	result, err := GetAddressRange(invalidConfig)
	assert.ErrorIs(t, err, unsupportedIPAM)
	assert.Equal(t, result, "")
}

func TestInAddrRange(t *testing.T) {
	var tests = []struct {
		ip       string
		ipRange  string
		expected bool
	}{
		{"192.168.0.1", "192.168.0.0/24", true},
		{"192.168.0.255", "192.168.0.0/24", true},
		{"192.168.1.5", "192.168.1.0/24", true},
		{"192.168.1.1", "192.168.0.0/24", false},
		{"192.168.0.1", "192.168.1.0/24", false},
	}

	for _, test := range tests {
		inRange, err := inAddrRange(test.ip, test.ipRange)
		assert.NoError(t, err)
		assert.Equal(t, inRange, test.expected)
	}
}

func TestGetMultusConfs(t *testing.T) {
	var pod = corev1.Pod{}
	pod.Annotations = map[string]string{
		multusAnnotation: `[{
          "name": "openshift-sdn",
          "interface": "eth0",
          "ips": [
              "10.217.0.15"
          ],
          "default": true,
          "dns": {}
      },{
          "name": "rook-ceph/public-net",
          "interface": "net1",
          "ips": [
              "192.168.20.7"
          ],
          "mac": "5e:e4:74:63:d1:75",
          "dns": {}
      }]`,
	}

	multusConfs, err := getMultusConfs(pod)
	assert.NoError(t, err)
	assert.Equal(t, len(multusConfs), 2)
}

func TestFindMultusData(t *testing.T) {
	var multusConfs []multusNetConfiguration = []multusNetConfiguration{
		{
			NetworkName:   "openshift-sdn",
			InterfaceName: "eth0",
			Ips: []string{
				"10.217.0.15",
			},
		},
		{
			NetworkName:   "rook-ceph/public-net",
			InterfaceName: "net1",
			Ips: []string{
				"192.168.20.7",
			},
		},
	}

	multusData, err := findMultusData(multusConfs, "public-net", "rook-ceph", "192.168.20.0/24")
	assert.NoError(t, err)
	assert.Equal(t, multusData.InterfaceName, "net1")
	assert.Equal(t, multusData.IP, "192.168.20.7")
}

func TestDetermineNewLinkName(t *testing.T) {
	// When there are no mlink# interfaces present,
	// determineNewLinkName(interfaces) will return mlink0
	var interfaces []net.Interface = []net.Interface{
		{
			Name: "lo",
		},
		{
			Name: "eth0",
		},
	}

	newLinkName, err := determineNewLinkName(interfaces)
	assert.NoError(t, err)
	assert.Equal(t, newLinkName, "mlink0")

	// When there are mlink# interfaces present,
	// The function will return the next available interface.
	interfaces = append(interfaces, net.Interface{Name: "mlink0"})
	newLinkName, err = determineNewLinkName(interfaces)
	assert.NoError(t, err)
	assert.Equal(t, newLinkName, "mlink1")
}

func TestCheckMigration(t *testing.T) {
	interfaces, err := net.Interfaces()
	assert.NoError(t, err, `Error occurred prior to test while getting network interfaces`)

	// This test expects the loopback device present on the network namespace under test.
	migrated, linkName, err := checkMigration(interfaces, "127.0.0.1")
	assert.NoError(t, err)
	assert.True(t, migrated, `checkMigration(interfaces, "127.0.0.1") did not find loopback network interface in current network namespace`)
	assert.Equal(t, linkName, "lo")
}
