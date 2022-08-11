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

package fake

import (
	"fmt"
	"strconv"
	"strings"
)

// OsdLsOutput returns JSON output from 'ceph osd ls' that can be used for unit tests. It
// returns output for a Ceph cluster with the number of OSDs given as input starting with ID 0.
// example:  numOSDs = 5  =>  return: "[0,1,2,3,4]"
func OsdLsOutput(numOSDs int) string {
	stringIDs := make([]string, 0, numOSDs)
	for id := 0; id < numOSDs; id++ {
		stringIDs = append(stringIDs, strconv.Itoa(id))
	}
	return fmt.Sprintf("[%s]", strings.Join(stringIDs, ","))
}

// OsdTreeOutput returns JSON output from 'ceph osd tree' that can be used for unit tests.
// It returns output for a Ceph cluster with the given number of nodes and the given number of OSDs
// per node with no complex configuration. This should work even for 0 nodes.
//
//	example:  OsdTreeOutput(3, 3) // returns JSON output for the Ceph cluster below
//	    node0:       node1:      node2:
//	      - osd0      - osd1      - osd2
//	      - osd3      - osd4      - osd5
//	      - osd6      - osd7      - osd8
func OsdTreeOutput(numNodes, numOSDsPerNode int) string {
	// JSON output taken from Ceph Pacific
	rootFormat := `		{
			"id": -1,
			"name": "default",
			"type": "root",
			"type_id": 11,
			"children": [%s]
		}` // format: negative node IDs as comma-delimited string (e.g., "-3,-4,-5")
	nodeFormat := `		{
			"id": %d,
			"name": "%s",
			"type": "host",
			"type_id": 1,
			"pool_weights": {},
			"children": [%s]
		}` // format: negative node ID, node name, OSD IDs as comma-delimited string (e.g., "0,3,6")
	osdFormat := `		{
			"id": %d,
			"device_class": "hdd",
			"name": "osd.%d",
			"type": "osd",
			"type_id": 0,
			"crush_weight": 0.009796142578125,
			"depth": 2,
			"pool_weights": {},
			"exists": 1,
			"status": "up",
			"reweight": 1,
			"primary_affinity": 1
		}` // format: OSD ID, OSD ID
	wrapperFormat := `{
	"nodes": [
%s
	],
	"stray": []
}` // format: <rendered root JSON, rendered nodes, rendered osds - with commas in between>
	nodesJSON := []string{}
	osdsJSON := []string{}
	nodes := []string{}
	for n := 0; n < numNodes; n++ {
		osds := []string{}
		nodeName := fmt.Sprintf("node%d", n)
		nodeID := -3 - n
		nodes = append(nodes, strconv.Itoa(nodeID))
		for i := 0; i < numOSDsPerNode; i++ {
			osdID := n + 3*i
			osds = append(osds, strconv.Itoa(osdID))
			osdsJSON = append(osdsJSON, fmt.Sprintf(osdFormat, osdID, osdID))
		}
		nodesJSON = append(nodesJSON, fmt.Sprintf(nodeFormat, nodeID, nodeName, strings.Join(osds, ",")))
	}
	rootJSON := fmt.Sprintf(rootFormat, strings.Join(nodes, ","))
	fullJSON := append(append([]string{rootJSON}, nodesJSON...), osdsJSON...)
	rendered := fmt.Sprintf(wrapperFormat, strings.Join(fullJSON, ",\n"))
	return rendered
}

// OsdOkToStopOutput returns JSON output from 'ceph osd ok-to-stop' that can be used for unit tests.
// queriedID should be given as the ID sent to the 'osd ok-to-stop <id> [--max=N]' command. It will
// be returned with relevant NOT ok-to-stop results.
// If returnOsdIds is empty, this returns a NOT ok-to-stop result. Otherwise, it returns an
// ok-to-stop result. returnOsdIds should include queriedID if the result should be successful.
func OsdOkToStopOutput(queriedID int, returnOsdIds []int) string {
	// For Pacific and up (Pacific+)
	okTemplate := `{"ok_to_stop":true,"osds":[%s],"num_ok_pgs":132,"num_not_ok_pgs":0,"ok_become_degraded":["1.0","1.2","1.3"]}`
	notOkTemplate := `{"ok_to_stop":false,"osds":[%d],"num_ok_pgs":161,"num_not_ok_pgs":50,"bad_become_inactive":["1.0","1.3","1.a"],"ok_become_degraded":["1.2","1.4","1.5"]}`

	// NOT ok-to-stop
	if len(returnOsdIds) == 0 {
		return fmt.Sprintf(notOkTemplate, queriedID)
	}

	// ok-to-stop
	osdIdsStr := make([]string, len(returnOsdIds))
	for i := 0; i < len(returnOsdIds); i++ {
		osdIdsStr[i] = strconv.Itoa(returnOsdIds[i])
	}
	return fmt.Sprintf(okTemplate, strings.Join(osdIdsStr, ","))
}

// OSDDeviceClassOutput returns JSON output from 'ceph osd crush get-device-class' that can be used for unit tests.
// osdId is a osd ID to get from crush map. If ID is empty raise a fake error.
func OSDDeviceClassOutput(osdId string) string {
	if osdId == "" {
		return "ERR: fake error from ceph cli"
	}
	okTemplate := `[{"osd":%s,"device_class":"hdd"}]`
	return fmt.Sprintf(okTemplate, osdId)
}
