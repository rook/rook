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

// NOTE: because of the _linux.go extension, this file will only compile for linux.
// github.com/containernetworking/plugins/pkg/ns only has a linux implementation, so this multus
// code will not work for any other OSes (possible windows support in future?)

package multus

import (
	"net"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

func init() {
	netExec = LinuxNetworkExecutor{}
}

// LinuxNetNS wraps ns.NetNS in a way such that we can mock the NetNS without compilation issues.
// Because ns.NetNS's Do() method takes a ns.NetNS type as an argument, we cannot simply using
// ns.NetNS as our NetNS because we will get compile-time errors like these:
// 	ns.NetNS does not implement NetNS (wrong type for Do method)
// 		have Do(func(ns.NetNS) error) error
// 		want Do(func(NetNS) error) error
type LinuxNetNS struct {
	NetNS ns.NetNS
}

func (l LinuxNetNS) Do(toRun func() error) error {
	return l.NetNS.Do(func(ns.NetNS) error {
		return toRun()
	})
}

func (l LinuxNetNS) Path() string {
	return l.NetNS.Path()
}

func (l LinuxNetNS) Fd() uintptr {
	return l.NetNS.Fd()
}

type LinuxNetworkExecutor struct{}

func (e LinuxNetworkExecutor) GetCurrentNS() (NetNS, error) {
	// wrap ns.GetCurrentNS()'s return in our LinuxNetNS to match type
	netNS, err := ns.GetCurrentNS()
	if err != nil {
		return nil, err
	}
	return LinuxNetNS{
		NetNS: netNS,
	}, nil
}

func (e LinuxNetworkExecutor) GetNS(nspath string) (NetNS, error) {
	// wrap ns.GetNS()'s return in our LinuxNetNS to match type
	netNS, err := ns.GetNS(nspath)
	if err != nil {
		return nil, err
	}
	return LinuxNetNS{
		NetNS: netNS,
	}, nil
}

func (e LinuxNetworkExecutor) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

func (e LinuxNetworkExecutor) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

func (e LinuxNetworkExecutor) LinkSetDown(link netlink.Link) error {
	return netlink.LinkSetDown(link)
}

func (e LinuxNetworkExecutor) LinkSetUp(link netlink.Link) error {
	return netlink.LinkSetUp(link)
}

func (e LinuxNetworkExecutor) LinkAdd(link netlink.Link) error {
	return netlink.LinkAdd(link)
}

func (e LinuxNetworkExecutor) LinkDel(link netlink.Link) error {
	return netlink.LinkDel(link)
}

func (e LinuxNetworkExecutor) LinkSetNsFd(link netlink.Link, fd int) error {
	return netlink.LinkSetNsFd(link, fd)
}

func (e LinuxNetworkExecutor) AddrList(link netlink.Link, family int) ([]netlink.Addr, error) {
	return netlink.AddrList(link, family)
}

func (e LinuxNetworkExecutor) AddrAdd(link netlink.Link, addr *netlink.Addr) error {
	return netlink.AddrAdd(link, addr)
}

func (e LinuxNetworkExecutor) RouteList(link netlink.Link, family int) ([]netlink.Route, error) {
	return netlink.RouteList(link, family)
}

func (e LinuxNetworkExecutor) RouteAdd(route *netlink.Route) error {
	return netlink.RouteAdd(route)
}
