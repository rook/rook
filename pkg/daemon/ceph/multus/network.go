/*
Copyright 2022 The Rook Authors. All rights reserved.

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
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
)

var (
	// override this executor's methods in unit tests to mock behavior
	netExec NetworkExecutor
)

const (
	nsDir = "/var/run/netns"
)

// FindNetworkNamespaceWithIP finds the network namespace which has any interface with the given IP.
// Returns an error if no namespace meeting the condition was found.
func FindNetworkNamespaceWithIP(ip string) (NetNS, error) {
	nsFiles, err := ioutil.ReadDir(nsDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read network namespace files in dir %q", nsDir)
	}

	var retErr error
	for _, nsFile := range nsFiles {
		nsPath := filepath.Join(nsDir, nsFile.Name())
		netNS, err := netExec.GetNS(nsPath)
		if err != nil {
			retErr = errors.Wrap(retErr, err.Error())
			retErr = errors.Wrapf(retErr, "failed to get info about network namespace %q", nsPath)
			continue // keep looking for namespace
		}

		iface, err := FindInterfaceWithIP(netNS, ip)
		if err != nil {
			retErr = errors.Wrap(retErr, err.Error())
			// this error's returned context is useful enough without an added wrap
			continue // keep looking for namespace
		}

		if iface != "" {
			return netNS, nil
		}
	}

	return nil, errors.Wrapf(retErr, "failed to find network namespace with ip %q", ip)
}

// FindInterfaceWithIP returns the name of the interface that is assigned the given IP. If no
// interface is assigned the IP, the result will be an empty string (""). If there were errors while
// searching for the interface, it is impossible to tell for sure that no interface exists with the
// IP given, so an error is returned.
func FindInterfaceWithIP(netNS NetNS, ip string) (string, error) {
	foundInterface := ""
	err := netNS.Do(func() error {
		ifaces, err := netExec.Interfaces()
		if err != nil {
			return errors.Wrap(err, "failed to list interfaces")
		}

		iface, err := interfaceWithIP(ifaces, ip)
		if err != nil {
			return err
		}

		if iface != "" {
			foundInterface = iface
			return nil
		}

		return nil
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to determine if ip %q is assigned to an interface in net namespace %q", ip, netNS.Path())
	}

	return foundInterface, nil
}

// FindInterfaceWithHardwareAddr returns the name of the interface that has the given hardware addr.
// Returns empty string ("") if no interface has the given hardware addr. Returns an error if the
// interface wasn't found and errors occurred.
func FindInterfaceWithHardwareAddr(netNS NetNS, mac net.HardwareAddr) (string, error) {
	foundInterface := ""
	err := netNS.Do(func() error {
		ifaces, err := netExec.Interfaces()
		if err != nil {
			return errors.Wrap(err, "failed to list interfaces")
		}

		for _, iface := range ifaces {
			if iface.HardwareAddr.String() == mac.String() {
				foundInterface = iface.Name
				break
			}
		}
		return nil
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to determine if an interface with hardware addr %q exists in net namespace %q", mac.String(), netNS.Path())
	}

	return foundInterface, nil
}

type NetworkConfig struct {
	LinkAttrs netlink.LinkAttrs
	Addrs     []netlink.Addr
	Routes    []netlink.Route
}

// GetNetworkConfig gets network config info (ip address, etc.) of the interface given.
func GetNetworkConfig(netNS NetNS, ifaceName string) (NetworkConfig, error) {
	linkAttrs := netlink.LinkAttrs{}
	addrs := []netlink.Addr{}
	routes := []netlink.Route{}

	err := netNS.Do(func() error {
		link, err := netExec.LinkByName(ifaceName)
		if err != nil {
			return errors.Wrap(err, "failed to get interface")
		}
		linkAttrs = *link.Attrs()

		addrs, err = netExec.AddrList(link, 0)
		if err != nil {
			return errors.Wrap(err, "failed to get address from interface")
		}

		routes, err = netExec.RouteList(link, 0)
		if err != nil {
			return errors.Wrap(err, "failed to get routes from interface")
		}

		return nil
	})
	if err != nil {
		return NetworkConfig{},
			errors.Wrapf(err, "failed to get network configuration info for interface %q in net namespace %q", ifaceName, netNS.Path())
	}

	return NetworkConfig{
		LinkAttrs: linkAttrs,
		Addrs:     addrs,
		Routes:    routes,
	}, nil
}

// DisableInterface disables the interface by setting it "down".
func DisableInterface(netNS NetNS, ifaceName string) error {
	err := netNS.Do(func() error {
		link, err := netExec.LinkByName(ifaceName)
		if err != nil {
			return errors.Wrapf(err, "failed to get link info")
		}

		if err := netExec.LinkSetDown(link); err != nil {
			return errors.Wrapf(err, "failed to set interface 'down'")
		}

		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed to disable interface %q in net namespace %q", ifaceName, netNS.Path())
	}

	return nil
}

// CopyInterfaceToHostNamespace sets creates a copy of the holder interface (in the holder net
// namespace) in the host net namespace with a new interface name.
func CopyInterfaceToHostNamespace(holderNS, hostNS NetNS, holderIfaceName, newIfaceName string) error {
	baseErrMsg := fmt.Sprintf("failed to copy holder interface %q to host namespace as %q", holderIfaceName, newIfaceName)

	var link netlink.Link
	err := holderNS.Do(func() error {
		var err error
		link, err = netExec.LinkByName(holderIfaceName)
		if err != nil {
			return errors.Wrapf(err, "failed to get holder interface %q", holderIfaceName)
		}

		return nil
	})
	if err != nil {
		return errors.Wrapf(err, baseErrMsg)
	}

	logger.Infof("holder interface %q found as: [%T] %+v", holderIfaceName, link, link)

	// In testing of multus with macvlan, link.Attrs().NetNsID is returned by the kernel via
	// IFLA_LINK_NETNSID with a value of zero (0), indicating the parent interface's network
	// namespace is the host namespace. However, zero (0) is a valid network namespace ID also.
	// With the macvlan plugin it isn't possible to reference an interface that is not in the host
	// network namespace. Therefore, we ignore the link.Attrs().NetNsID return value and assume the
	// parent interface is in the host network namespace.
	if link.Attrs().NetNsID > 0 {
		return errors.Errorf("%s. unsupported config: parent interface must be in host net namespace. "+
			"IFLA_LINK_NETNSID reports the parent interface is in net namespace with ID %d",
			baseErrMsg, link.Attrs().NetNsID)
	}

	// overwrite the link's attributes because we can't easily make a deep copy of the link, and we
	// want to keep the link's underlying type (IPVlan{}, Macvlan{}, etc.) reset to default the
	// values that we don't want to keep for the copy
	link.Attrs().Index = 0
	link.Attrs().Name = newIfaceName
	// link.Attrs().Namespace causes a segfault; move the namespace later
	link.Attrs().TxQLen = -1
	link.Attrs().NetNsID = -1 // create iface in current net namespace

	logger.Infof("copy of interface to be created: %+v", link)

	err = hostNS.Do(func() error {
		if err := netExec.LinkAdd(link); err != nil {
			return errors.Wrapf(err, "failed to create interface copy as %q in host net namespace", newIfaceName)
		}

		return nil
	})
	if err != nil {
		return errors.Wrapf(err, baseErrMsg)
	}

	return nil
}

// ConfigureInterface configures the interface with the IP config given and then enables the
// interface by setting it "up".
func ConfigureInterface(netNS NetNS, ifaceName string, netConfig *NetworkConfig) error {
	existingConfig, err := GetNetworkConfig(netNS, ifaceName)
	if err != nil {
		return errors.Wrapf(err, "failed to get initial network config of interface")
	}

	err = netNS.Do(func() error {
		link, err := netExec.LinkByName(ifaceName)
		if err != nil {
			return errors.Wrap(err, "failed to get interface")
		}

		// NOTE: we don't ever remove or replace addrs or routes here under the assumption that the
		// pod will not have its multus connection info updated once it is created. Rather, a new
		// pod would appear with new connection info.

		addrs := netConfig.Addrs
		for i := range addrs {
			if addrExists(existingConfig.Addrs, addrs[i]) {
				// address is already added; adding it again would be an error
				continue
			}
			// label must be changed to the new interface name for the AddrAdd call to succeed.
			addrs[i].Label = ifaceName
			if err := netExec.AddrAdd(link, &addrs[i]); err != nil {
				return errors.Wrapf(err, "failed to configure ip address %q", addrs[i].IP.String())
			}
		}

		// TODO: this is untested but included as best-effort since multus has a route modification
		// plugin... hopefully this works
		routes := netConfig.Routes
		for i := range routes {
			if routeExists(existingConfig.Routes, routes[i]) {
				// route is already added; adding it again would be an error
				continue
			}
			if err := netExec.RouteAdd(&routes[i]); err != nil {
				return errors.Wrapf(err, "failed to configure route %q", routes[i].String())
			}
		}

		if err := netExec.LinkSetUp(link); err != nil {
			return errors.Wrap(err, "failed to set interface 'up'")
		}

		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed to configure interface %q on namespace %q", ifaceName, netNS.Path())
	}

	return nil
}

// DeleteInterface deletes the network interface with the given name. It returns success if the
// interface does not exist.
func DeleteInterface(netNS NetNS, ifaceName string) error {
	err := netNS.Do(func() error {
		link, err := netExec.LinkByName(ifaceName)
		if err != nil {
			switch err.(type) {
			case netlink.LinkNotFoundError, *netlink.LinkNotFoundError:
				logger.Infof("interface %q in namespace %q was already deleted", ifaceName, netNS.Path())
				return nil // link not found means it's already deleted
			default:
				return errors.Wrap(err, "failed to get interface")
			}
		}

		err = netExec.LinkDel(link)
		if err != nil {
			return errors.Wrap(err, "failed to delete interface")
		}

		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed to delete interface %q in namespace %q", ifaceName, netNS.Path())
	}

	return nil
}

// DetermineNewLink name determines the next possible interface name with the given prefix.
// Operates in the host network namespace.
// The maximum length of a network name in linux is 15 chars.
func DetermineNextInterface(prefix string) (string, error) {
	// operates in host net namespace and does not write any net configs, so it should be safe to
	// not use netNS.Do() here

	interfaces, err := net.Interfaces()
	if err != nil {
		return "", errors.Wrap(err, "failed to get interfaces in host net namespace")
	}

	newIndex := nextIndex(interfaces, prefix)

	return fmt.Sprintf("%s%d", prefix, newIndex), nil
}

func nextIndex(interfaces []net.Interface, prefix string) int {
	// find the indexes of all interfaces that have the given prefix
	indexesWithPrefix := []int{}
	for _, iface := range interfaces {
		if strings.HasPrefix(iface.Name, prefix) {
			idxStr := strings.TrimPrefix(iface.Name, prefix)
			idx, err := strconv.Atoi(idxStr)
			if err != nil {
				// string following the prefix is not a number, so we don't risk conflicting w/ it
				continue
			}
			indexesWithPrefix = append(indexesWithPrefix, idx)
		}
	}

	// get the lowest-indexed interface possible (try to avoid any corner cases where we have
	// created thoutsands of these interfaces)
	newIndex := 0
	for {
		if !indexIsInList(newIndex, indexesWithPrefix) {
			// if index is not in list, it's available; let's use it
			break
		}
		// index is not available, try the next index
		newIndex++
	}

	return newIndex
}

func indexIsInList(idx int, indexes []int) bool {
	for _, i := range indexes {
		if i == idx {
			return true
		}
	}
	return false
}

// return the name of the interface that has the ip if it was found (with nil error)
// return empty string ("") with a nil error if the interface wasn't found
// return empty string ("") with an error if there were any errors encountered during the search
func interfaceWithIP(interfaces []net.Interface, ip string) (string, error) {
	var retErr error
	for _, iface := range interfaces {
		link, err := netExec.LinkByName(iface.Name)
		if err != nil {
			retErr = errors.Wrap(retErr, err.Error())
			retErr = errors.Wrapf(retErr, "failed to get link info for interface %q", iface.Name)
			continue // keep searching
		}

		addrs, err := netExec.AddrList(link, 0)
		if err != nil {
			retErr = errors.Wrap(retErr, err.Error())
			retErr = errors.Wrapf(retErr, "failed to get address information from interface %q", iface.Name)
			continue // keep searching
		}

		for _, addr := range addrs {
			if addr.IP.String() == ip {
				return iface.Name, nil // we found the IP, so any previous errors don't matter
			}
		}
	}

	if retErr != nil {
		// it is impossible to say for sure that an interface with the IP doesn't exist in this case
		return "", errors.Wrapf(retErr, "failed (possibly multiple times) while searching for ip %q in interfaces", ip)
	}
	// did not find the interface but no errors looking means an interface with the ip doesn't exist
	return "", nil
}

func addrExists(addrs []netlink.Addr, addr netlink.Addr) bool {
	for _, a := range addrs {
		if a.Equal(addr) {
			return true
		}
	}
	return false
}

func routeExists(routes []netlink.Route, route netlink.Route) bool {
	for _, r := range routes {
		if r.Equal(route) {
			return true
		}
	}
	return false
}
