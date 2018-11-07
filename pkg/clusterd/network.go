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

import (
	"fmt"
	"net"
)

type NetworkInfo struct {
	PublicAddr     string
	ClusterAddr    string
	PublicNetwork  string // public network and subnet mask in CIDR notation
	ClusterNetwork string // cluster network and subnet mask in CIDR notation

	// deprecated ipv4 format address
	// TODO: remove these legacy fields in the future
	PublicAddrIPv4  string
	ClusterAddrIPv4 string
}

// Simplify adapts deprecated fields
// TODO: remove this function in the future
func (in NetworkInfo) Simplify() NetworkInfo {
	out := NetworkInfo{
		PublicNetwork:  in.PublicNetwork,
		ClusterNetwork: in.ClusterNetwork,
	}
	if in.PublicAddr != "" {
		out.PublicAddr = in.PublicAddr
	} else {
		out.PublicAddr = in.PublicAddrIPv4
	}

	if in.ClusterAddr != "" {
		out.ClusterAddr = in.ClusterAddr
	} else {
		out.ClusterAddr = in.ClusterAddrIPv4
	}
	return out
}

func VerifyNetworkInfo(networkInfo NetworkInfo) error {
	if err := verifyIPAddr(networkInfo.PublicAddr); err != nil {
		return err
	}

	if err := verifyIPAddr(networkInfo.ClusterAddr); err != nil {
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
