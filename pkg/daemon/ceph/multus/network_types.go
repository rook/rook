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

	"github.com/vishvananda/netlink"
)

// Methods from github.com/containernetworking/plugins/pkg/ns we use which we can override with
// mocks for unit testing.
// The original definition contains more functions than this, but these are all that we use.
// We partly make the copy here also because github.com/containernetworking/plugins/pkg/ns can only
// be compiled for linux and makes unit testing on mac impossible.
type NetNS interface {
	// Executes the passed closure in this object's network namespace,
	// attempting to restore the original namespace before returning.
	// However, since each OS thread can have a different network namespace,
	// and Go's thread scheduling is highly variable, callers cannot
	// guarantee any specific namespace is set unless operations that
	// require that namespace are wrapped with Do().  Also, no code called
	// from Do() should call runtime.UnlockOSThread(), or the risk
	// of executing code in an incorrect namespace will be greater.  See
	// https://github.com/golang/go/wiki/LockOSThread for further details.
	//
	// NOTE: modified to make the toRun func take no argument since we don't use it and since it
	// makes it harder to fit the linux wrapper onto the type.
	Do(toRun func() error) error

	// Returns the filesystem path representing this object's network namespace
	Path() string

	// Returns a file descriptor representing this object's network namespace
	Fd() uintptr
}

// Create an executor that will allow us to mock all the commands we make for unit testing
type NetworkExecutor interface {
	// from "github.com/containernetworking/plugins/pkg/ns"
	// https://pkg.go.dev/github.com/containernetworking/plugins/pkg/ns#GetCurrentNS
	//
	// NOTE: when this is called, the NetNS object returned will have a Path() method that does not
	// return consistent results between different calls to GetCurrentNS(). This is because the
	// function gets the /proc path to whatever OS thread the goroutine is currently running on
	// which may change.
	GetCurrentNS() (NetNS, error)

	// from "github.com/containernetworking/plugins/pkg/ns"
	// https://pkg.go.dev/github.com/containernetworking/plugins/pkg/ns#GetNS
	GetNS(nspath string) (NetNS, error)

	// from "net"
	// https://pkg.go.dev/net?utm_source=gopls#Interfaces
	Interfaces() ([]net.Interface, error)

	// from "github.com/vishvananda/netlink"
	// https://pkg.go.dev/github.com/vishvananda/netlink#LinkByName
	LinkByName(name string) (netlink.Link, error)

	// from "github.com/vishvananda/netlink"
	// https://pkg.go.dev/github.com/vishvananda/netlink#LinkSetDown
	LinkSetDown(link netlink.Link) error

	// from "github.com/vishvananda/netlink"
	// https://pkg.go.dev/github.com/vishvananda/netlink#LinkSetUp
	LinkSetUp(link netlink.Link) error

	// from "github.com/vishvananda/netlink"
	// https://pkg.go.dev/github.com/vishvananda/netlink#LinkAdd
	LinkAdd(link netlink.Link) error

	// from "github.com/vishvananda/netlink"
	// https://pkg.go.dev/github.com/vishvananda/netlink#LinkDel
	LinkDel(link netlink.Link) error

	// from "github.com/vishvananda/netlink"
	// https://pkg.go.dev/github.com/vishvananda/netlink#LinkSetNsFd
	LinkSetNsFd(link netlink.Link, fd int) error

	// from "github.com/vishvananda/netlink"
	// https://pkg.go.dev/github.com/vishvananda/netlink#AddrList
	AddrList(link netlink.Link, family int) ([]netlink.Addr, error)

	// from "github.com/vishvananda/netlink"
	// https://pkg.go.dev/github.com/vishvananda/netlink#AddrAdd
	AddrAdd(link netlink.Link, addr *netlink.Addr) error

	// from "github.com/vishvananda/netlink"
	// https://pkg.go.dev/github.com/vishvananda/netlink#RouteList
	RouteList(link netlink.Link, family int) ([]netlink.Route, error)

	// from "github.com/vishvananda/netlink"
	// https://pkg.go.dev/github.com/vishvananda/netlink#RouteAdd
	RouteAdd(route *netlink.Route) error
}
