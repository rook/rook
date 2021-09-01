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
	"net"

	"github.com/stretchr/testify/mock"
	"github.com/vishvananda/netlink"
)

type MockNetNS struct {
	mock.Mock
}

// Do is a special snowflake that we always want to run the 'toRun()' func. We shouldn't need to
// ever mock the return value from Do directly but control the output of it via modifying the output
// of commands run from in the 'toRun()' func.
func (m *MockNetNS) Do(toRun func() error) error {
	m.Called(toRun)
	return toRun()
}

func (m *MockNetNS) Path() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockNetNS) Fd() uintptr {
	args := m.Called()
	return args.Get(0).(uintptr)
}

type MockNetworkExecutor struct {
	mock.Mock
}

func (e *MockNetworkExecutor) GetCurrentNS() (NetNS, error) {
	args := e.Called()
	ns, _ := args.Get(0).(NetNS)
	return ns, args.Error(1)
}

func (e *MockNetworkExecutor) GetNS(nspath string) (NetNS, error) {
	args := e.Called(nspath)
	ns, _ := args.Get(0).(NetNS)
	return ns, args.Error(1)
}

func (e *MockNetworkExecutor) Interfaces() ([]net.Interface, error) {
	args := e.Called()
	return args.Get(0).([]net.Interface), args.Error(1)
}

func (e *MockNetworkExecutor) LinkByName(name string) (netlink.Link, error) {
	args := e.Called(name)
	link, _ := args.Get(0).(netlink.Link)
	return link, args.Error(1)
}

func (e *MockNetworkExecutor) LinkSetDown(link netlink.Link) error {
	args := e.Called(link)
	return args.Error(0)
}

func (e *MockNetworkExecutor) LinkSetUp(link netlink.Link) error {
	args := e.Called(link)
	return args.Error(0)
}

func (e *MockNetworkExecutor) LinkAdd(link netlink.Link) error {
	args := e.Called(link)
	e.TestData().Set("LinkAdd(link)", link)
	return args.Error(0)
}

func (e *MockNetworkExecutor) LinkDel(link netlink.Link) error {
	args := e.Called(link)
	return args.Error(0)
}

func (e *MockNetworkExecutor) LinkSetNsFd(link netlink.Link, fd int) error {
	args := e.Called(link, fd)
	return args.Error(0)
}

func (e *MockNetworkExecutor) AddrList(link netlink.Link, family int) ([]netlink.Addr, error) {
	args := e.Called(link, family)
	addrs, _ := args.Get(0).([]netlink.Addr) // can be nil on error case
	return addrs, args.Error(1)
}

func (e *MockNetworkExecutor) AddrAdd(link netlink.Link, addr *netlink.Addr) error {
	args := e.Called(link, addr)
	return args.Error(0)
}

func (e *MockNetworkExecutor) RouteList(link netlink.Link, family int) ([]netlink.Route, error) {
	args := e.Called(link, family)
	routes, _ := args.Get(0).([]netlink.Route)
	return routes, args.Error(1)
}

func (e *MockNetworkExecutor) RouteAdd(route *netlink.Route) error {
	args := e.Called(route)
	return args.Error(0)
}
