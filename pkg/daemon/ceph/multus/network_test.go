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
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vishvananda/netlink"
)

// realistic macvlan struct returned from LinkByName() taken from a simple test env
func holderMacvlanLink() *netlink.Macvlan {
	return &netlink.Macvlan{
		LinkAttrs: netlink.LinkAttrs{
			Index:        5,
			MTU:          1500,
			TxQLen:       0,
			Name:         "net1",
			HardwareAddr: net.HardwareAddr([]byte{214, 47, 43, 63, 143, 51}), // d6:2f:2b:3f:8f:33
			Flags:        net.FlagBroadcast | net.FlagMulticast,
			RawFlags:     4098,
			ParentIndex:  2,
			MasterIndex:  0,
			Namespace:    nil,
			Alias:        "",
			Statistics:   nil, // usually filled in but ignore for unit test
			Promisc:      0,
			Xdp:          nil, // usually filled in but ignore for unit test
			EncapType:    "ether",
			Protinfo:     nil,
			OperState:    netlink.OperUp,
			NetNsID:      0, // parent link is in host net (-1 for current net)
			NumTxQueues:  1,
			NumRxQueues:  1,
			GSOMaxSize:   65536,
			GSOMaxSegs:   65535,
			Vfs:          []netlink.VfInfo{},
			Group:        0,
			Slave:        nil,
		},
		Mode:     netlink.MACVLAN_MODE_BRIDGE,
		MACAddrs: []net.HardwareAddr{},
	}
}

func TestCopyInterfaceToHostNamespace(t *testing.T) {
	holderNs := func() *MockNetNS {
		holderNs := new(MockNetNS)
		holderNs.On("Path").Return("/var/netns/holdernsid")
		holderNs.On("Fd").Return(uintptr(2))
		// note: does not set up expectations for "Do"
		return holderNs
	}

	hostNs := func() *MockNetNS {
		hostNs := new(MockNetNS)
		hostNs.On("Path").Return("/proc/some/path")
		hostNs.On("Fd").Return(uintptr(1))
		// note: does not set up expectations for "Do"
		return hostNs
	}

	holderIface := "net1"
	newIface := "rookm0"

	netExecOrig := netExec
	defer func() { netExec = netExecOrig }()

	t.Run("fail to get holder link", func(t *testing.T) {
		holderNs := holderNs()
		// Do should be called once in the holder ns when getting the link
		holderNs.On("Do", mock.Anything).Return().Once()

		// Do should not be called in the host ns
		hostNs := hostNs()

		mExec := new(MockNetworkExecutor)
		mExec.On("LinkByName", holderIface).Return(nil, errors.New("induced failure"))

		netExec = mExec

		err := CopyInterfaceToHostNamespace(holderNs, hostNs, holderIface, newIface)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get holder interface")
		assert.Contains(t, err.Error(), "induced failure")
		holderNs.AssertNumberOfCalls(t, "Do", 1)
		mExec.AssertCalled(t, "LinkByName", holderIface)
		mExec.AssertNotCalled(t, "LinkAdd", mock.Anything)
	})

	t.Run("fail to create copy", func(t *testing.T) {
		holderNs := holderNs()
		// do should be called once in the holder ns when getting the link
		holderNs.On("Do", mock.Anything).Return().Once()

		hostNs := hostNs()
		// do should be called once in the host ns when creating the copy
		hostNs.On("Do", mock.Anything).Return().Once()

		mExec := new(MockNetworkExecutor)
		mExec.On("LinkByName", holderIface).Return(holderMacvlanLink(), nil)
		mExec.On("LinkAdd", mock.Anything).Return(errors.New("induced failure"))

		netExec = mExec

		err := CopyInterfaceToHostNamespace(holderNs, hostNs, holderIface, newIface)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create interface copy")
		assert.Contains(t, err.Error(), "induced failure")
		holderNs.AssertNumberOfCalls(t, "Do", 1)
		mExec.AssertCalled(t, "LinkByName", holderIface)
		hostNs.AssertNumberOfCalls(t, "Do", 1)
		mExec.AssertCalled(t, "LinkAdd", mock.Anything)
	})

	t.Run("with parent in host namespace", func(t *testing.T) {
		holderNs := holderNs()
		// do should be called once in the holder ns when getting the link
		holderNs.On("Do", mock.Anything).Return().Once()

		hostNs := hostNs()
		// do should be called once in the host ns when creating the copy
		hostNs.On("Do", mock.Anything).Return().Once()

		mExec := new(MockNetworkExecutor)
		mExec.On("LinkByName", holderIface).Return(holderMacvlanLink(), nil)
		mExec.On("LinkAdd", mock.Anything).Return(nil)

		netExec = mExec

		err := CopyInterfaceToHostNamespace(holderNs, hostNs, holderIface, newIface)
		assert.NoError(t, err)
		holderNs.AssertNumberOfCalls(t, "Do", 1)
		mExec.AssertCalled(t, "LinkByName", holderIface)
		hostNs.AssertNumberOfCalls(t, "Do", 1)
		mExec.AssertCalled(t, "LinkAdd", mock.Anything)

		// for one of our tests, we should verify that certain values are un-set for our copy so we
		// get default kernel-supplied values upon creation
		logger.Info("TestData()", mExec.TestData())
		newLink := mExec.TestData().Get("LinkAdd(link)").Data().(netlink.Link)
		assert.Equal(t, 0, newLink.Attrs().Index)
		assert.Equal(t, -1, newLink.Attrs().NetNsID)
		assert.Equal(t, -1, newLink.Attrs().TxQLen)
		assert.Nil(t, newLink.Attrs().Namespace) // causes segfault if this is set
	})

	t.Run("cannot handle parent interface that isn't in host namespace", func(t *testing.T) {
		holderNs := holderNs()
		// do should be called once in the holder ns when getting the link
		holderNs.On("Do", mock.Anything).Return().Once()

		hostNs := hostNs()

		mExec := new(MockNetworkExecutor)
		holderMacvlanLink := holderMacvlanLink()
		holderMacvlanLink.NetNsID = 1
		mExec.On("LinkByName", holderIface).Return(holderMacvlanLink, nil)

		netExec = mExec

		err := CopyInterfaceToHostNamespace(holderNs, hostNs, holderIface, newIface)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "IFLA_LINK_NETNSID reports the parent interface is in net namespace with ID 1")
		holderNs.AssertNumberOfCalls(t, "Do", 1)
		mExec.AssertCalled(t, "LinkByName", holderIface)
	})
}

func Test_nextIndex(t *testing.T) {
	prefix0 := net.Interface{
		Name: "prefix0",
	}
	prefix1 := net.Interface{
		Name: "prefix1",
	}
	prefix2 := net.Interface{
		Name: "prefix2",
	}
	diff1 := net.Interface{
		Name: "diff1",
	}

	t.Run("no interfaces exist", func(t *testing.T) {
		next := nextIndex([]net.Interface{}, "prefix")
		assert.Equal(t, 0, next)
	})
	t.Run("interface 0 exists", func(t *testing.T) {
		next := nextIndex([]net.Interface{prefix0}, "prefix")
		assert.Equal(t, 1, next)
	})
	t.Run("interfaces 0 and 2 exist", func(t *testing.T) {
		next := nextIndex([]net.Interface{prefix0, prefix2}, "prefix")
		assert.Equal(t, 1, next)
	})
	t.Run("interfaces 0 and 1 exist", func(t *testing.T) {
		next := nextIndex([]net.Interface{prefix0, prefix1}, "prefix")
		assert.Equal(t, 2, next)
	})
	t.Run("interfaces 2 exists", func(t *testing.T) {
		next := nextIndex([]net.Interface{prefix2}, "prefix")
		assert.Equal(t, 0, next)
	})
	t.Run("interface diff1 exists", func(t *testing.T) {
		next := nextIndex([]net.Interface{diff1}, "prefix")
		assert.Equal(t, 0, next)
	})
	t.Run("interfaces 0 and diff1 exist", func(t *testing.T) {
		next := nextIndex([]net.Interface{prefix0, diff1}, "prefix")
		assert.Equal(t, 1, next)
	})
}
