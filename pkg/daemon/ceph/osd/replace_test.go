/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package osd

import (
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kexec "k8s.io/utils/exec"
)

func TestCrushHostFromLocation(t *testing.T) {
	assert.Equal(t, "node-1", crushHostFromLocation("root=default host=node-1"))
	assert.Equal(t, "node-1", crushHostFromLocation("root=default host=node-1 region=r1 zone=z1"))
	assert.Equal(t, "", crushHostFromLocation("root=default"))
	assert.Equal(t, "", crushHostFromLocation(""))
}

func TestAvailableDataDevices(t *testing.T) {
	t.Run("picks a blank data device", func(t *testing.T) {
		available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{
			"sdb": {Data: unassignedOSDID, DeviceInfo: &sys.LocalDisk{Name: "sdb"}},
		}}
		assert.Equal(t, []string{"sdb"}, availableDataDevices(available))
	})

	t.Run("skips metadata device entries", func(t *testing.T) {
		available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{
			"nvme0n1": {Data: unassignedOSDID, Metadata: []int{}, DeviceInfo: &sys.LocalDisk{Name: "nvme0n1"}},
		}}
		assert.Empty(t, availableDataDevices(available))
	})

	t.Run("skips already-assigned data devices", func(t *testing.T) {
		available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{
			"sdb": {Data: 5, DeviceInfo: &sys.LocalDisk{Name: "sdb"}},
		}}
		assert.Empty(t, availableDataDevices(available))
	})

	t.Run("empty mapping", func(t *testing.T) {
		assert.Empty(t, availableDataDevices(&DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}))
	})

	t.Run("multiple blank candidates sorted deterministically", func(t *testing.T) {
		available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{
			"sdc": {Data: unassignedOSDID, DeviceInfo: &sys.LocalDisk{Name: "sdc"}},
			"sdb": {Data: unassignedOSDID, DeviceInfo: &sys.LocalDisk{Name: "sdb"}},
		}}
		// Map iteration order is random; the sorted result must always be ["sdb", "sdc"].
		for i := 0; i < 20; i++ {
			assert.Equal(t, []string{"sdb", "sdc"}, availableDataDevices(available))
		}
	})
}

func TestBlankDeviceClasses(t *testing.T) {
	available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{
		"sda":     {DeviceInfo: &sys.LocalDisk{Name: "sda", Rotational: true}},
		"sdb":     {DeviceInfo: &sys.LocalDisk{Name: "sdb", Rotational: false}},
		"nvme0n1": {DeviceInfo: &sys.LocalDisk{Name: "nvme0n1", Rotational: false, RealPath: "/dev/nvme0n1"}},
	}}
	classes := getBlankDeviceClasses(available, []string{"sda", "sdb", "nvme0n1"})
	assert.Equal(t, map[string]string{"sda": "hdd", "sdb": "ssd", "nvme0n1": "nvme"}, classes)
}

// expectedLeadingMatches returns the optimal number of same-class pairs a correct
// matchBlankAndDestroyedByDeviceClasses must line up at the front of both slices: for each
// non-empty device class, the smaller of (destroyed slots of that class, blank devices of that
// class). Empty/unknown classes never match, matching isDeviceClassMatch, which is false when
// either side is empty.
func expectedLeadingMatches(ids []int, slotClass map[int]string, devices []string, devClass map[string]string) int {
	dCount := map[string]int{}
	for _, id := range ids {
		if c := slotClass[id]; c != "" {
			dCount[c]++
		}
	}
	bCount := map[string]int{}
	for _, d := range devices {
		if c := devClass[d]; c != "" {
			bCount[c]++
		}
	}
	total := 0
	for c, dc := range dCount {
		total += min(dc, bCount[c])
	}
	return total
}

func TestMatchBlankAndDestroyedByDeviceClasses(t *testing.T) {
	tests := []struct {
		name        string
		ids         []int
		slotClass   map[int]string
		devices     []string
		devClass    map[string]string
		wantMatches int // optimal same-class pairs that must be packed to the front
	}{
		{
			name:        "empty inputs",
			ids:         nil,
			slotClass:   map[int]string{},
			devices:     nil,
			devClass:    map[string]string{},
			wantMatches: 0,
		},
		{
			name:        "no destroyed slots",
			ids:         nil,
			slotClass:   map[int]string{},
			devices:     []string{"sda"},
			devClass:    map[string]string{"sda": "hdd"},
			wantMatches: 0,
		},
		{
			name:        "no blank devices",
			ids:         []int{0},
			slotClass:   map[int]string{0: "hdd"},
			devices:     nil,
			devClass:    map[string]string{},
			wantMatches: 0,
		},
		{
			name:        "all hdd exact match",
			ids:         []int{0, 1, 2},
			slotClass:   map[int]string{0: "hdd", 1: "hdd", 2: "hdd"},
			devices:     []string{"sda", "sdb", "sdc"},
			devClass:    map[string]string{"sda": "hdd", "sdb": "hdd", "sdc": "hdd"},
			wantMatches: 3,
		},
		{
			name:      "mixed hdd/ssd/nvme, given cross-ordered, must fully align",
			ids:       []int{0, 1, 2},
			slotClass: map[int]string{0: "hdd", 1: "ssd", 2: "nvme"},
			devices:   []string{"sda", "sdb", "sdc"},
			// devices in a different class order than the slots: only a reorder aligns them.
			devClass:    map[string]string{"sda": "nvme", "sdb": "hdd", "sdc": "ssd"},
			wantMatches: 3,
		},
		{
			name:        "more destroyed than blank of a class (imbalance, best-effort)",
			ids:         []int{0, 1},
			slotClass:   map[int]string{0: "hdd", 1: "hdd"},
			devices:     []string{"sda", "sdb"},
			devClass:    map[string]string{"sda": "hdd", "sdb": "ssd"},
			wantMatches: 1, // one hdd/hdd pair; the second hdd slot spills onto the ssd device
		},
		{
			name:        "more blank than destroyed, match sits past the slot count",
			ids:         []int{0},
			slotClass:   map[int]string{0: "ssd"},
			devices:     []string{"sda", "sdb"},
			devClass:    map[string]string{"sda": "hdd", "sdb": "ssd"},
			wantMatches: 1, // ssd slot must pair with sdb(ssd), not the leading sda(hdd)
		},
		{
			name:        "unknown/empty slot class never matches",
			ids:         []int{0},
			slotClass:   map[int]string{}, // osd.0 has no recorded class
			devices:     []string{"sda"},
			devClass:    map[string]string{"sda": "nvme"},
			wantMatches: 0,
		},
		{
			name:        "unknown/empty device class never matches",
			ids:         []int{0},
			slotClass:   map[int]string{0: "ssd"},
			devices:     []string{"sda"},
			devClass:    map[string]string{"sda": ""},
			wantMatches: 0,
		},
		{
			name:        "nvme vs ssd is a mismatch",
			ids:         []int{0},
			slotClass:   map[int]string{0: "ssd"},
			devices:     []string{"sda"},
			devClass:    map[string]string{"sda": "nvme"},
			wantMatches: 0,
		},
		{
			name:        "all different classes, no match",
			ids:         []int{0, 1},
			slotClass:   map[int]string{0: "hdd", 1: "ssd"},
			devices:     []string{"sda", "sdb"},
			devClass:    map[string]string{"sda": "nvme", "sdb": "nvme"},
			wantMatches: 0,
		},
		{
			name:      "duplicate classes needing reorder",
			ids:       []int{0, 1, 2, 3},
			slotClass: map[int]string{0: "ssd", 1: "hdd", 2: "ssd", 3: "hdd"},
			devices:   []string{"sda", "sdb", "sdc", "sdd"},
			// sda,sdd = ssd ; sdb,sdc = hdd -> two ssd + two hdd on each side, all four align.
			devClass:    map[string]string{"sda": "ssd", "sdb": "hdd", "sdc": "hdd", "sdd": "ssd"},
			wantMatches: 4,
		},
		{
			name:      "partial class overlap, best-effort maximizes matches",
			ids:       []int{0, 1, 2},
			slotClass: map[int]string{0: "hdd", 1: "ssd", 2: "nvme"},
			devices:   []string{"sda", "sdb", "sdc"},
			// one hdd, two ssd devices, no nvme device: only hdd and one ssd can align (2 matches).
			devClass:    map[string]string{"sda": "hdd", "sdb": "ssd", "sdc": "ssd"},
			wantMatches: 2,
		},
	}

	// clone helpers so each run starts from the caller's original input order.
	cloneInts := func(s []int) []int {
		if s == nil {
			return nil
		}
		return append([]int(nil), s...)
	}
	cloneStrs := func(s []string) []string {
		if s == nil {
			return nil
		}
		return append([]string(nil), s...)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// sanity: the table's wantMatches agrees with the class multisets.
			require.Equal(t, tc.wantMatches, expectedLeadingMatches(tc.ids, tc.slotClass, tc.devices, tc.devClass),
				"wantMatches in the table is inconsistent with the class maps")

			ids := cloneInts(tc.ids)
			devices := cloneStrs(tc.devices)
			matchBlankAndDestroyedByDeviceClasses(ids, tc.slotClass, devices, tc.devClass)

			// The function only reorders in place: the result must be a permutation of the inputs.
			assert.ElementsMatch(t, tc.ids, ids, "destroyed ids must be preserved as a permutation")
			assert.ElementsMatch(t, tc.devices, devices, "blank devices must be preserved as a permutation")

			// Same-class pairs must be packed contiguously at the front, exactly wantMatches of them,
			// and every position past that boundary must be a genuine cross-class (best-effort) pair.
			n := min(len(ids), len(devices))
			matched := 0
			for i := 0; i < n; i++ {
				if isDeviceClassMatch(tc.slotClass[ids[i]], tc.devClass[devices[i]]) {
					matched++
				}
			}
			assert.Equal(t, tc.wantMatches, matched, "wrong number of same-class pairs aligned")
			for i := 0; i < n; i++ {
				isMatch := isDeviceClassMatch(tc.slotClass[ids[i]], tc.devClass[devices[i]])
				if i < tc.wantMatches {
					assert.Truef(t, isMatch, "position %d expected a same-class pair (osd.%d vs %q)", i, ids[i], devices[i])
				} else {
					assert.Falsef(t, isMatch, "position %d expected a cross-class spillover pair (osd.%d vs %q)", i, ids[i], devices[i])
				}
			}

			// Determinism: a second run from the same starting order yields the identical arrangement,
			// so the slot<->device pairing is stable across reconciles.
			ids2 := cloneInts(tc.ids)
			devices2 := cloneStrs(tc.devices)
			matchBlankAndDestroyedByDeviceClasses(ids2, tc.slotClass, devices2, tc.devClass)
			assert.Equal(t, ids, ids2, "reordering of destroyed ids must be deterministic")
			assert.Equal(t, devices, devices2, "reordering of blank devices must be deterministic")
		})
	}
}

func TestFilterDestroyedOSDIdsForNode(t *testing.T) {
	treeJSON := `{
		"nodes": [
			{"id": -1, "name": "default", "type": "root", "type_id": 10, "children": [-3, -4]},
			{"id": -3, "name": "node1", "type": "host", "type_id": 1, "children": [0, 1]},
			{"id": -4, "name": "node2", "type": "host", "type_id": 1, "children": [2, 3]},
			{"id": 0, "name": "osd.0", "type": "osd", "type_id": 0, "exists": 1, "status": "up"},
			{"id": 1, "name": "osd.1", "type": "osd", "type_id": 0, "exists": 1, "status": "destroyed"},
			{"id": 2, "name": "osd.2", "type": "osd", "type_id": 0, "exists": 1, "status": "destroyed"},
			{"id": 3, "name": "osd.3", "type": "osd", "type_id": 0, "exists": 1, "status": "up"}
		],
		"stray": []
	}`
	var tree cephclient.OsdTree
	require.NoError(t, json.Unmarshal([]byte(treeJSON), &tree))

	// GetDestroyedIDs picks up only "destroyed" slots cluster-wide.
	destroyed := tree.GetDestroyedIDs()
	assert.ElementsMatch(t, []int{1, 2}, destroyed)

	// destroyed ids cluster-wide are [1, 2]; only osd.1 belongs to node1.
	ids, err := filterDestroyedOSDIdsForNode(tree, destroyed, "root=default host=node1")
	assert.NoError(t, err)
	assert.Equal(t, []int{1}, ids)

	// node2 owns osd.2 only.
	ids, err = filterDestroyedOSDIdsForNode(tree, destroyed, "root=default host=node2")
	assert.NoError(t, err)
	assert.Equal(t, []int{2}, ids)

	// a node with no destroyed slots returns empty.
	ids, err = filterDestroyedOSDIdsForNode(tree, destroyed, "root=default host=node-unknown")
	assert.NoError(t, err)
	assert.Empty(t, ids)

	// missing host token is an error.
	_, err = filterDestroyedOSDIdsForNode(tree, destroyed, "root=default")
	assert.Error(t, err)
}

func TestFilterDestroyedOSDIdsForNodeDeepHierarchy(t *testing.T) {
	// Realistic multi-level CRUSH tree as seen in mid-to-large clusters, where rack-level failure
	// domains are typical and racks/datacenters sit ABOVE the host bucket:
	//
	//   root(default) -> datacenter -> rack -> host -> osd
	//
	// type_id values are the Ceph defaults (osd=0, host=1, chassis=2, rack=3, datacenter=8,
	// root=11), per src/osd/OSDMap.cc OSDMap::build_simple_crush_map_from_conf
	// (crush.set_type_name(0,"osd")..(11,"root")) and doc/rados/operations/crush-map-edits.rst
	// ("type 0 osd" .. "type 11 root"). The per-node JSON fields (id/name/type/type_id/children for
	// buckets; id/name/type/type_id/crush_weight/depth/exists/status/reweight/primary_affinity for
	// osd leaves) match the `ceph osd tree --format json` shape emitted by
	// CrushTreeDumper::dump_item_fields + OSDTreeFormattingDumper::dump_item_fields, with status one
	// of "up"/"down"/"destroyed".
	//
	// Layout:
	//   dc1
	//     rack-a1 -> host node-a1  : osd.0 (up),        osd.1 (destroyed)
	//     rack-a2 -> host node-a2  : osd.2 (up),        osd.3 (up)
	//   dc2
	//     rack-b1 -> host node-b1  : osd.4 (destroyed), osd.5 (up)
	//     rack-b1 -> host node-b2  : chassis -> osd.6 (destroyed), osd.7 (up)
	//
	// The node-b2 host has a non-standard intermediate `chassis` bucket BELOW the host (standard
	// CRUSH puts chassis ABOVE the host). It is included only to exercise the recursive descent in
	// collectOSDsUnderBucket: osd.6 sits one level deeper than a host's direct children, so it is
	// only returned if the walk recurses through the chassis.
	treeJSON := `{
		"nodes": [
			{"id": -1, "name": "default", "type": "root", "type_id": 11, "children": [-2, -3]},

			{"id": -2, "name": "dc1", "type": "datacenter", "type_id": 8, "children": [-10, -11]},
			{"id": -10, "name": "rack-a1", "type": "rack", "type_id": 3, "children": [-100]},
			{"id": -11, "name": "rack-a2", "type": "rack", "type_id": 3, "children": [-101]},
			{"id": -100, "name": "node-a1", "type": "host", "type_id": 1, "children": [0, 1]},
			{"id": -101, "name": "node-a2", "type": "host", "type_id": 1, "children": [2, 3]},

			{"id": -3, "name": "dc2", "type": "datacenter", "type_id": 8, "children": [-20]},
			{"id": -20, "name": "rack-b1", "type": "rack", "type_id": 3, "children": [-200, -201]},
			{"id": -200, "name": "node-b1", "type": "host", "type_id": 1, "children": [4, 5]},
			{"id": -201, "name": "node-b2", "type": "host", "type_id": 1, "children": [-300]},
			{"id": -300, "name": "node-b2-chassis", "type": "chassis", "type_id": 2, "children": [6, 7]},

			{"id": 0, "name": "osd.0", "type": "osd", "type_id": 0, "crush_weight": 1.0, "depth": 4, "exists": 1, "status": "up", "reweight": 1.0, "primary_affinity": 1.0},
			{"id": 1, "name": "osd.1", "type": "osd", "type_id": 0, "crush_weight": 1.0, "depth": 4, "exists": 1, "status": "destroyed", "reweight": 0.0, "primary_affinity": 1.0},
			{"id": 2, "name": "osd.2", "type": "osd", "type_id": 0, "crush_weight": 1.0, "depth": 4, "exists": 1, "status": "up", "reweight": 1.0, "primary_affinity": 1.0},
			{"id": 3, "name": "osd.3", "type": "osd", "type_id": 0, "crush_weight": 1.0, "depth": 4, "exists": 1, "status": "up", "reweight": 1.0, "primary_affinity": 1.0},
			{"id": 4, "name": "osd.4", "type": "osd", "type_id": 0, "crush_weight": 1.0, "depth": 4, "exists": 1, "status": "destroyed", "reweight": 0.0, "primary_affinity": 1.0},
			{"id": 5, "name": "osd.5", "type": "osd", "type_id": 0, "crush_weight": 1.0, "depth": 4, "exists": 1, "status": "up", "reweight": 1.0, "primary_affinity": 1.0},
			{"id": 6, "name": "osd.6", "type": "osd", "type_id": 0, "crush_weight": 1.0, "depth": 5, "exists": 1, "status": "destroyed", "reweight": 0.0, "primary_affinity": 1.0},
			{"id": 7, "name": "osd.7", "type": "osd", "type_id": 0, "crush_weight": 1.0, "depth": 5, "exists": 1, "status": "up", "reweight": 1.0, "primary_affinity": 1.0}
		],
		"stray": []
	}`
	var tree cephclient.OsdTree
	require.NoError(t, json.Unmarshal([]byte(treeJSON), &tree))

	// Cluster-wide destroyed slots span multiple racks/datacenters: osd.1 (dc1), osd.4 and osd.6 (dc2).
	destroyed := tree.GetDestroyedIDs()
	assert.ElementsMatch(t, []int{1, 4, 6}, destroyed)

	t.Run("returns only the destroyed osd under the target host, deep in the tree", func(t *testing.T) {
		// node-a1 sits at root->dc1->rack-a1->host; only its own destroyed osd.1 must come back,
		// not osd.4/osd.6 destroyed under other hosts/racks/datacenters.
		ids, err := filterDestroyedOSDIdsForNode(tree, destroyed, "root=default datacenter=dc1 rack=rack-a1 host=node-a1")
		assert.NoError(t, err)
		assert.Equal(t, []int{1}, ids)
	})

	t.Run("host in a different datacenter returns only its own destroyed osd", func(t *testing.T) {
		ids, err := filterDestroyedOSDIdsForNode(tree, destroyed, "root=default datacenter=dc2 rack=rack-b1 host=node-b1")
		assert.NoError(t, err)
		assert.Equal(t, []int{4}, ids)
	})

	t.Run("host with no destroyed osds returns empty", func(t *testing.T) {
		// node-a2 (osd.2, osd.3) has no destroyed slots even though its datacenter dc1 does (osd.1).
		ids, err := filterDestroyedOSDIdsForNode(tree, destroyed, "root=default datacenter=dc1 rack=rack-a2 host=node-a2")
		assert.NoError(t, err)
		assert.Empty(t, ids)
	})

	t.Run("osds nested below the host bucket are not matched", func(t *testing.T) {
		// node-b2's children are a chassis bucket, not osds directly; osd.6 is one level deeper.
		// Rook always makes OSDs direct children of the host bucket, so we only read the host's
		// direct children: an osd nested under an intermediate bucket below the host is not matched.
		ids, err := filterDestroyedOSDIdsForNode(tree, destroyed, "root=default datacenter=dc2 rack=rack-b1 host=node-b2")
		assert.NoError(t, err)
		assert.Empty(t, ids)
	})
}

func TestRecoverDBLV(t *testing.T) {
	a := &OsdAgent{}

	t.Run("shared metadata: db lv is recovered as vg/lv", func(t *testing.T) {
		out := `{
			"3": [
				{"type": "block", "vg_name": "ceph-block-vg", "lv_name": "osd-block-x", "path": "/dev/ceph-block-vg/osd-block-x"},
				{"type": "db", "vg_name": "ceph-db-vg", "lv_name": "osd-db-y", "path": "/dev/ceph-db-vg/osd-db-y"}
			]
		}`
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return out, nil
			},
		}
		dbLV, err := a.recoverDBLVForOSDFromHost(&clusterd.Context{Executor: executor}, 3)
		assert.NoError(t, err)
		assert.Equal(t, "ceph-db-vg/osd-db-y", dbLV)
	})

	t.Run("single-disk lvm: no db lv", func(t *testing.T) {
		out := `{"3": [{"type": "block", "vg_name": "ceph-block-vg", "lv_name": "osd-block-x", "path": "/dev/ceph-block-vg/osd-block-x"}]}`
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return out, nil
			},
		}
		dbLV, err := a.recoverDBLVForOSDFromHost(&clusterd.Context{Executor: executor}, 3)
		assert.NoError(t, err)
		assert.Equal(t, "", dbLV)
	})

	t.Run("single-disk raw: empty list", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return `{}`, nil
			},
		}
		dbLV, err := a.recoverDBLVForOSDFromHost(&clusterd.Context{Executor: executor}, 3)
		assert.NoError(t, err)
		assert.Equal(t, "", dbLV)
	})
}

func TestBuildReplacementPrepareArgs(t *testing.T) {
	const logPath = "/tmp/ceph-log"
	entry := &DeviceOsdIDEntry{
		Data:       unassignedOSDID,
		Config:     DesiredDevice{Name: "vdb", DeviceClass: "hdd"},
		DeviceInfo: &sys.LocalDisk{Name: "vdb"},
	}

	tests := []struct {
		name     string
		store    config.StoreConfig
		dbLV     string
		useRaw   bool
		expected []string
	}{
		{
			name:   "raw single-disk",
			store:  config.StoreConfig{StoreType: "bluestore"},
			dbLV:   "",
			useRaw: true,
			expected: []string{
				"-oL", "ceph-volume", "--log-path", logPath, "raw", "prepare", "--bluestore",
				"--osd-id", "0", "--data", "/dev/vdb", "--crush-device-class", "hdd",
			},
		},
		{
			name:   "lvm single-disk (no --block.db)",
			store:  config.StoreConfig{StoreType: "bluestore"},
			dbLV:   "",
			useRaw: false,
			expected: []string{
				"-oL", "ceph-volume", "--log-path", logPath, "lvm", "prepare", "--bluestore",
				"--osd-id", "0", "--data", "/dev/vdb", "--crush-device-class", "hdd",
			},
		},
		{
			name:   "lvm shared-metadata plain",
			store:  config.StoreConfig{StoreType: "bluestore"},
			dbLV:   "ceph-db-vg/osd-db-y",
			useRaw: false,
			expected: []string{
				"-oL", "ceph-volume", "--log-path", logPath, "lvm", "prepare", "--bluestore",
				"--osd-id", "0", "--data", "/dev/vdb", "--block.db", "ceph-db-vg/osd-db-y",
				"--crush-device-class", "hdd",
			},
		},
		{
			name:   "lvm shared-metadata encrypted",
			store:  config.StoreConfig{StoreType: "bluestore", EncryptedDevice: true},
			dbLV:   "ceph-db-vg/osd-db-y",
			useRaw: false,
			expected: []string{
				"-oL", "ceph-volume", "--log-path", logPath, "lvm", "prepare", "--bluestore",
				"--osd-id", "0", "--data", "/dev/vdb", "--block.db", "ceph-db-vg/osd-db-y",
				"--dmcrypt", "--crush-device-class", "hdd",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &OsdAgent{storeConfig: tc.store}
			args := a.buildReplacementPrepareArgs(0, "/dev/vdb", tc.dbLV, entry, tc.useRaw, logPath)
			assert.Equal(t, tc.expected, args)
		})
	}
}

func TestEncryptedDMNamesForOSD(t *testing.T) {
	// JSON shape mirrors a real `ceph-volume lvm list <id>` for a shared-metadata encrypted OSD: the
	// dm-crypt mapping name is the LV's lv_uuid (verified on a live cluster), and only entries tagged
	// ceph.encrypted=1 carry a mapping to close.
	const encryptedOut = `{
		"5": [
			{"type": "block", "lv_uuid": "BLOCK-UUID", "vg_name": "ceph-bvg", "lv_name": "osd-block-x", "path": "/dev/ceph-bvg/osd-block-x", "tags": {"ceph.encrypted": "1", "ceph.osd_id": "5"}},
			{"type": "db", "lv_uuid": "DB-UUID", "vg_name": "ceph-dvg", "lv_name": "osd-db-y", "path": "/dev/ceph-dvg/osd-db-y", "tags": {"ceph.encrypted": "1", "ceph.osd_id": "5"}}
		]
	}`

	t.Run("encrypted shared-metadata returns block and db dm names", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				assert.Contains(t, args, "list")
				assert.Contains(t, args, "5")
				return encryptedOut, nil
			},
		}
		names, err := encryptedDMNamesForOSD(&clusterd.Context{Executor: executor}, 5)
		assert.NoError(t, err)
		assert.Equal(t, []string{"BLOCK-UUID", "DB-UUID"}, names)
	})

	t.Run("non-encrypted OSD returns nothing", func(t *testing.T) {
		out := `{"5": [{"type": "block", "lv_uuid": "BLOCK-UUID", "tags": {"ceph.encrypted": "0", "ceph.osd_id": "5"}}]}`
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return out, nil
			},
		}
		names, err := encryptedDMNamesForOSD(&clusterd.Context{Executor: executor}, 5)
		assert.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("id absent from list returns nothing", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return `{}`, nil
			},
		}
		names, err := encryptedDMNamesForOSD(&clusterd.Context{Executor: executor}, 5)
		assert.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("wal LV is also returned", func(t *testing.T) {
		out := `{
			"5": [
				{"type": "block", "lv_uuid": "BLOCK-UUID", "tags": {"ceph.encrypted": "1"}},
				{"type": "db", "lv_uuid": "DB-UUID", "tags": {"ceph.encrypted": "1"}},
				{"type": "wal", "lv_uuid": "WAL-UUID", "tags": {"ceph.encrypted": "1"}}
			]
		}`
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return out, nil
			},
		}
		names, err := encryptedDMNamesForOSD(&clusterd.Context{Executor: executor}, 5)
		assert.NoError(t, err)
		assert.Equal(t, []string{"BLOCK-UUID", "DB-UUID", "WAL-UUID"}, names)
	})

	t.Run("encrypted entry with no lv_uuid is skipped", func(t *testing.T) {
		out := `{
			"5": [
				{"type": "block", "lv_uuid": "", "tags": {"ceph.encrypted": "1"}},
				{"type": "db", "lv_uuid": "DB-UUID", "tags": {"ceph.encrypted": "1"}}
			]
		}`
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return out, nil
			},
		}
		names, err := encryptedDMNamesForOSD(&clusterd.Context{Executor: executor}, 5)
		assert.NoError(t, err)
		assert.Equal(t, []string{"DB-UUID"}, names)
	})
}

func TestCloseEncryptedDevicesForOSD(t *testing.T) {
	const encryptedOut = `{
		"5": [
			{"type": "block", "lv_uuid": "BLOCK-UUID", "tags": {"ceph.encrypted": "1", "ceph.osd_id": "5"}},
			{"type": "db", "lv_uuid": "DB-UUID", "tags": {"ceph.encrypted": "1", "ceph.osd_id": "5"}}
		]
	}`

	t.Run("closes each encrypted mapping with luksClose", func(t *testing.T) {
		closed := []string{}
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return encryptedOut, nil
			},
			MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
				require.Equal(t, "cryptsetup", command)
				assert.Equal(t, []string{"--verbose", "luksClose", args[2]}, args)
				closed = append(closed, args[2])
				return "", nil
			},
		}
		err := CloseEncryptedDevicesForOSD(&clusterd.Context{Executor: executor}, 5)
		assert.NoError(t, err)
		assert.Equal(t, []string{"BLOCK-UUID", "DB-UUID"}, closed)
	})

	t.Run("already-closed mapping (exit status 4) is treated as success", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return encryptedOut, nil
			},
			MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
				return "Device is not active.", errors.New("exit status 4")
			},
		}
		err := CloseEncryptedDevicesForOSD(&clusterd.Context{Executor: executor}, 5)
		assert.NoError(t, err)
	})

	t.Run("real close failure is returned", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return encryptedOut, nil
			},
			MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
				return "boom", errors.New("exit status 1")
			},
		}
		err := CloseEncryptedDevicesForOSD(&clusterd.Context{Executor: executor}, 5)
		assert.Error(t, err)
	})

	t.Run("non-encrypted OSD is a no-op", func(t *testing.T) {
		called := false
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return `{"5": [{"type": "block", "lv_uuid": "X", "tags": {"ceph.encrypted": "0"}}]}`, nil
			},
			MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
				called = true
				return "", nil
			},
		}
		err := CloseEncryptedDevicesForOSD(&clusterd.Context{Executor: executor}, 5)
		assert.NoError(t, err)
		assert.False(t, called)
	})
}

func TestIsCryptsetupNotActive(t *testing.T) {
	assert.True(t, isCryptsetupNotActive(&kexec.CodeExitError{Err: errors.New("exit status 4"), Code: 4}))
	assert.True(t, isCryptsetupNotActive(errors.Wrap(&kexec.CodeExitError{Err: errors.New("exit status 4"), Code: 4}, "failed to close encrypted device")))
	assert.True(t, isCryptsetupNotActive(errors.New("Device foo is not active.")))
	assert.False(t, isCryptsetupNotActive(&kexec.CodeExitError{Err: errors.New("exit status 1"), Code: 1}))
	assert.False(t, isCryptsetupNotActive(errors.New("exit status 1")))
	assert.False(t, isCryptsetupNotActive(nil))
}
