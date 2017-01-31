package clusterd

import (
	"fmt"
	"net"
)

type NetworkInfo struct {
	PublicAddrIPv4  string
	ClusterAddrIPv4 string
	PublicNetwork   string // public network and subnet mask in CIDR notation
	ClusterNetwork  string // cluster network and subnet mask in CIDR notation
}

func VerifyNetworkInfo(networkInfo NetworkInfo) error {
	if err := verifyIPAddr(networkInfo.PublicAddrIPv4); err != nil {
		return err
	}

	if err := verifyIPAddr(networkInfo.ClusterAddrIPv4); err != nil {
		return err
	}

	if err := verifyIPNetwork(networkInfo.PublicNetwork); err != nil {
		return err
	}

	if err := verifyIPNetwork(networkInfo.ClusterNetwork); err != nil {
		return err
	}

	return nil
}

func verifyIPAddr(addr string) error {
	if addr == "" {
		// empty strings are OK
		return nil
	}

	if net.ParseIP(addr) == nil {
		return fmt.Errorf("failed to parse IP address %s", addr)
	}

	return nil
}

func verifyIPNetwork(network string) error {
	if network == "" {
		// empty strings are OK
		return nil
	}

	_, _, err := net.ParseCIDR(network)
	return err
}
