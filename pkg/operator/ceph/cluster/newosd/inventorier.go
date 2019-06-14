/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package newosd

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

type cephVolumeInventoryResult struct {
	Available       bool                      `json:"available"`
	LVs             []cephVolumeInventoryLV   `json:"lvs"`
	Path            string                    `json:"path"`
	RejectedReasons []string                  `json:"rejected_reasons"`
	SysAPI          cephVolumeInventorySysAPI `json:"sys_api"`
}

type cephVolumeInventorySysAPI struct {
	Locked     int                                     `json:"locked"` // int `0` or `1`
	Partitions map[string]cephVolumeInventoryPartition `json:"partitions"`
	Path       string                                  `json:"path"`
	Rotational string                                  `json:"rotational"` // string `"0"` or `"1"`
	Size       float64                                 `json:"size"`
}

type cephVolumeInventoryLV struct {
	BlockUUID   string `json:"block_uuid"`
	ClusterFSID string `json:"cluster_fsid"`
	ClusterName string `json:"cluster_name"`
	Name        string `json:"name"`
	OsdFSID     string `json:"osd_fsid"`
	OsdID       string `json:"osd_id"`
	LVType      string `json:"type"`
}

type cephVolumeInventoryPartition struct {
	Sectors    string `json:"sectors"` // yes this is actually reported as a string ... sigh
	SectorSize int    `json:"sectorsize"`
}

type inventoriedNode struct {
	resolvedNode     rookalpha.Node
	inventory        []cephVolumeInventoryResult
	rawInventoryJSON string
	err              error
}

// Run an inventorier routine for each node which comes in the nodes channel, and put the
// inventoried output on the inventoried nodes channel.
func (c *Controller) inventoryNodes(nodesCh <-chan rookalpha.Node, inventoriedNodesCh chan<- inventoriedNode) {
	var wg sync.WaitGroup

	for n := range nodesCh {
		logger.Debugf("Inventorying node %+v", n)

		jobName := k8sutil.TruncateNodeName(inventoryAppNameFmt, n.Name)

		jobConfig := &cephVolumeJobConfiguration{
			parentController: c,
			hostname:         n.Name,
			appName:          inventoryAppName,
			jobName:          jobName,
			cephVolumeArgs:   []string{"inventory", "--format=json-pretty"},
		}
		cmdReporter, err := jobConfig.cmdReporter()
		if err != nil {
			msg := fmt.Sprintf("failed to inventory node %s. %+v", n.Name, err)
			logger.Errorf(msg) // since error is being returned in channel, log it immediately
			in := inventoriedNode{
				resolvedNode:     n,
				inventory:        []cephVolumeInventoryResult{},
				rawInventoryJSON: "",
				err:              fmt.Errorf(msg),
			}
			inventoriedNodesCh <- in
			continue
		}

		wg.Add(1)
		go func(n rookalpha.Node) {
			defer wg.Done()
			stdout, stderr, retcode, err := cmdReporter.Run(osdInventoryTimeout)
			inventory, err := parseInventory(stdout, stderr, retcode, err, n.Name)
			in := inventoriedNode{
				resolvedNode:     n,
				inventory:        inventory,
				rawInventoryJSON: stdout,
				err:              err,
			}
			if in.err != nil {
				// since error is being returned in channel, log it immediately
				logger.Errorf("%+v", in.err)
			}
			inventoriedNodesCh <- in
		}(n)

		// wait a few seconds after each CmdReporter run to ease pressure on Kube API server
		<-time.After(3 * time.Second)
	}

	// closer
	go func() {
		wg.Wait()
		close(inventoriedNodesCh)
		logger.Debugf("all nodes have been inventoried")
	}()
}

func parseInventory(
	stdout, stderr string, retcode int, err error, nodeName string,
) ([]cephVolumeInventoryResult, error) {
	res := []cephVolumeInventoryResult{}
	if err != nil {
		return res, fmt.Errorf("failed to inventory node %s due to an unexpected error. %+v", nodeName, err)
	}
	if retcode != 0 {
		combinedOut := fmt.Sprintf(`
stdout: %s
stderr: %s
retval: %d`, stdout, stderr, retcode)
		return res, fmt.Errorf("failed to inventory node %s due to a ceph-volume error: %s", nodeName, combinedOut)
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		logger.Errorf("failed to parse inventory from node %s: %s", nodeName, stdout)
		return []cephVolumeInventoryResult{}, fmt.Errorf("failed to parse inventory from node %s. %+v", nodeName, err)
	}
	return res, nil
}
