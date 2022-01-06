/*
Copyright 2021 The Rook Authors. All rights reserved.

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
package multus

import (
	"io/ioutil"
	"net"
	"path/filepath"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
)

const (
	nsDir = "/var/run/netns"
)

var InterfaceNotFound = errors.New("Interface with matching IP not found")

func findInterface(interfaces []net.Interface, ipStr string) (string, error) {
	var ifaceName string

	for _, iface := range interfaces {
		link, err := netlink.LinkByName(iface.Name)
		if err != nil {
			return ifaceName, errors.Wrap(err, "failed to get link")
		}
		if link == nil {
			return ifaceName, errors.New("failed to find link")
		}

		addrs, err := netlink.AddrList(link, 0)
		if err != nil {
			return ifaceName, errors.Wrap(err, "failed to get address from link")
		}

		for _, addr := range addrs {
			if addr.IP.String() == ipStr {
				linkAttrs := link.Attrs()
				if linkAttrs != nil {
					ifaceName = linkAttrs.Name
				}
				return ifaceName, nil
			}
		}
	}

	return ifaceName, InterfaceNotFound
}

func DetermineNetNS(ip string) (ns.NetNS, error) {
	var netNS ns.NetNS
	nsFiles, err := ioutil.ReadDir(nsDir)
	if err != nil {
		return netNS, errors.Wrap(err, "failed to read netns files")
	}

	for _, nsFile := range nsFiles {
		var foundNS bool

		netNS, err := ns.GetNS(filepath.Join(nsDir, nsFile.Name()))
		if err != nil {
			return netNS, errors.Wrap(err, "failed to get network namespace")
		}

		err = netNS.Do(func(ns ns.NetNS) error {
			interfaces, err := net.Interfaces()
			if err != nil {
				return errors.Wrap(err, "failed to list interfaces")
			}

			iface, err := findInterface(interfaces, ip)
			if err != nil && !errors.Is(err, InterfaceNotFound) {
				return errors.Wrap(err, "failed to find needed interface")
			}
			if iface != "" {
				foundNS = true
				return nil
			}
			return nil
		})

		if err != nil {
			// Don't quit, just keep looking.
			logger.Infof("error occurred while looking for network namespace: %v; continuing search", err)
			continue
		}

		if foundNS {
			return netNS, nil
		}
	}

	return netNS, errors.New("failed to find network namespace")
}

type netConfig struct {
	Addrs  []netlink.Addr
	Routes []netlink.Route
}

func GetNetworkConfig(netNS ns.NetNS, linkName string) (netConfig, error) {
	var conf netConfig

	err := netNS.Do(func(ns ns.NetNS) error {
		link, err := netlink.LinkByName(linkName)
		if err != nil {
			return errors.Wrap(err, "failed to get link")
		}

		conf.Addrs, err = netlink.AddrList(link, 0)
		if err != nil {
			return errors.Wrap(err, "failed to get address from link")
		}

		conf.Routes, err = netlink.RouteList(link, 0)
		if err != nil {
			return errors.Wrap(err, "failed to get routes from link")
		}

		return nil
	})

	if err != nil {
		return conf, errors.Wrap(err, "failed to get network namespace")
	}

	return conf, nil
}

func MigrateInterface(holderNS, hostNS ns.NetNS, multusLinkName, newLinkName string) error {
	err := holderNS.Do(func(ns.NetNS) error {

		link, err := netlink.LinkByName(multusLinkName)
		if err != nil {
			return errors.Wrap(err, "failed to get multus link")
		}

		if err := netlink.LinkSetDown(link); err != nil {
			return errors.Wrap(err, "failed to set link down")
		}

		if err := netlink.LinkSetName(link, newLinkName); err != nil {
			return errors.Wrap(err, "failed to rename link")
		}

		// After renaming the link, the link object must be updated or netlink will get confused.
		link, err = netlink.LinkByName(newLinkName)
		if err != nil {
			return errors.Wrap(err, "failed to get link")
		}

		if err = netlink.LinkSetNsFd(link, int(hostNS.Fd())); err != nil {
			return errors.Wrap(err, "failed to move interface to host namespace")
		}

		return nil
	})

	if err != nil {
		return errors.Wrap(err, "failed to migrate multus interface")
	}
	return nil
}

func ConfigureInterface(linkName string, conf netConfig) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return errors.Wrap(err, "failed to get interface on host namespace")
	}
	for i := range conf.Addrs {
		// The IP address label must be changed to the new interface name
		// for the AddrAdd call to succeed.
		conf.Addrs[i].Label = linkName
		if err := netlink.AddrAdd(link, &conf.Addrs[i]); err != nil {
			return errors.Wrap(err, "failed to configure ip address on interface")
		}
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return errors.Wrap(err, "failed to set link up")
	}

	return nil
}

func DeleteInterface(linkName string) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return errors.Wrap(err, "failed to get multus network interface")
	}

	err = netlink.LinkDel(link)
	if err != nil {
		return errors.Wrap(err, "failed to delete multus network interface")
	}
	return nil
}
