package multus

import (
	"errors"
	"net"
	"testing"

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
	if err != nil {
		t.Errorf(`unexpected error occurred during call to GetAddressRange(%q): %v`, validConfig, err)
	}
	if result != "192.168.21.0/24" {
		t.Errorf(`GetAddressRange(%q) = %q, expected "192.168.21.0/24"`, validConfig, result)
	}
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
	if !errors.Is(err, unsupportedIPAM) {
		t.Errorf("unexpected error occurred during call to GetAddressRange(%q): %v", invalidConfig, err)
	}
	if result != "" {
		t.Errorf(`GetAddressRange(%q) = %q, expected ""`, invalidConfig, result)
	}
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
		if err != nil {
			t.Errorf(`unexpected error occurred during call to inAddrRange(%q, %q): %v`, test.ip, test.ipRange, err)
			continue
		}
		if inRange != test.expected {
			t.Errorf(`inAddrRange(%q, %q) = %v; expected: %v`, test.ip, test.ipRange, inRange, test.expected)
		}
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
	if err != nil {
		t.Errorf(`Unexpected error occurred during call to getMultusConfs(pod): %v`, err)
		return
	}

	if len(multusConfs) != 2 {
		t.Errorf(`getMultusConf(pod) returned %d multus configurations, expected 2.`, len(multusConfs))
		return
	}
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
	if err != nil {
		t.Errorf(`Unexpected error occurred during call to findMultusData(...): %v`, err)
		return
	}

	if multusData.InterfaceName != "net1" {
		t.Errorf(`findMultusData(...).InterfaceName = %q; expected: net1`, multusData.InterfaceName)
		return
	}
	if multusData.IP != "192.168.20.7" {
		t.Errorf(`findMultusData(...).IP = %q; expected: 192.168.20.7`, multusData.IP)
		return
	}
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
	if err != nil {
		t.Errorf(`Unexpected error occurred during call to determineNewLinkName(interfaces): %v`, err)
		return
	}
	if newLinkName != "mlink0" {
		t.Errorf(`determineNewLinkName(interfaces) = %q; expected: %q`, newLinkName, "mlink0")
		return
	}

	// When there are mlink# interfaces present,
	// The function will return the next available interface.
	interfaces = append(interfaces, net.Interface{Name: "mlink0"})
	newLinkName, err = determineNewLinkName(interfaces)
	if err != nil {
		t.Errorf(`Unexpected error occurred during call to determineNewLinkName(interfaces): %v`, err)
		return
	}
	if newLinkName != "mlink1" {
		t.Errorf(`determineNewLinkName(interfaces) = %q; expected: %q`, newLinkName, "mlink1")
		return
	}
}

func TestCheckMigration(t *testing.T) {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Errorf(`Error occurred prior to test while getting network interfaces`)
		return
	}

	// This test expects the loopback device present on the network namespace under test.
	migrated, linkName, err := checkMigration(interfaces, "127.0.0.1")
	if err != nil {
		t.Errorf(`Unexpected error occurred during call to checkMigration(interfaces, "127.0.0.1")`)
		return
	}
	if !migrated {
		t.Error(`checkMigration(interfaces, "127.0.0.1") did not find loopback network interface in current network namespace`)
		return
	}
	if linkName != "lo" {
		t.Errorf(`checkMigration(interfaces, "127.0.0.1").linkName = %q; expected: "lo"`, linkName)
		return
	}
}
