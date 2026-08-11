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
	"fmt"
	"math"
	"strconv"
	"sync"

	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"

	"github.com/pkg/errors"
)

const (
	mclockIopsHDD          = "osd_mclock_max_capacity_iops_hdd"
	mclockIopsSSD          = "osd_mclock_max_capacity_iops_ssd"
	disableOSDBenchmarkEnv = "ROOK_DISABLE_OSD_BENCHMARK"
)

var mclockCapacityUpdated sync.Map

// ensureMclockCapacityForOSDs sets osd_mclock_max_capacity_iops_[hdd|ssd] from an OSD bench.
func (c *Cluster) ensureMclockCapacityForOSDs() {
	if k8sutil.GetOperatorSetting(disableOSDBenchmarkEnv, "false") == "true" {
		return
	}

	osdDump, err := cephclient.GetOSDDump(c.context, c.clusterInfo)
	if err != nil {
		log.NamespacedError(c.clusterInfo.Namespace, logger, "failed to get osd dump for mclock capacity update: %v", err)
		return
	}

	monStore := config.GetMonStore(c.context, c.clusterInfo)
	for _, osd := range osdDump.OSDs {
		osdID, ok := osdIDIsUp(osd.Up, osd.OSD)
		if !ok {
			continue
		}

		// namespace/osdID is used as the key to store the updated mclock capacity
		// this is used to avoid updating the mclock capacity multiple times for the same osd
		updatedKey := c.clusterInfo.Namespace + "/" + strconv.Itoa(osdID)
		if _, ok := mclockCapacityUpdated.Load(updatedKey); ok {
			continue
		}

		if err := c.updateMclockCapacity(osdID, monStore, updatedKey); err != nil {
			log.NamespacedError(c.clusterInfo.Namespace, logger, "failed to update mclock capacity for osd.%d: %v", osdID, err)
		}
	}
}

func (c *Cluster) updateMclockCapacity(osdID int, monStore *config.MonStore, updatedKey string) error {
	meta, err := cephclient.GetOSDMetadataByID(c.context, c.clusterInfo, osdID)
	if err != nil {
		return err
	}
	option := mclockIopsSSD
	if meta.BluestoreBdevRotational == "1" || meta.BluestoreBdevType == "hdd" {
		option = mclockIopsHDD
	}

	who := fmt.Sprintf("osd.%d", osdID)
	currentValue, err := monStore.Get(who, option)
	if err != nil {
		return errors.Wrapf(err, "failed to get current iops for osd.%d", osdID)
	}

	currentIOPS, err := strconv.ParseFloat(currentValue, 64)
	if err != nil {
		return errors.Wrapf(err, "failed to parse current iops for osd.%d", osdID)
	}

	if err := cephclient.OSDCacheDrop(c.context, c.clusterInfo, osdID); err != nil {
		return err
	}

	result, err := cephclient.OSDBench(c.context, c.clusterInfo, osdID)
	if err != nil {
		return err
	}

	resultIOPS, err := result.IOPS.Float64()
	if err != nil {
		return errors.Wrapf(err, "failed to parse bench iops for osd.%d", osdID)
	}

	if resultIOPS <= currentIOPS {
		mclockCapacityUpdated.Store(updatedKey, struct{}{})
		return nil
	}

	resultIOPSValue := strconv.Itoa(int(math.Round(resultIOPS)))
	if err := monStore.Set(who, option, resultIOPSValue); err != nil {
		return err
	}

	mclockCapacityUpdated.Store(updatedKey, struct{}{})
	log.NamespacedInfo(c.clusterInfo.Namespace, logger, "set %s=%s for osd.%d from osd bench", option, resultIOPSValue, osdID)
	return nil
}

func osdIDIsUp(up, osd json.Number) (int, bool) {
	upVal, err := up.Int64()
	if err != nil || upVal != 1 {
		return 0, false
	}
	id, err := osd.Int64()
	if err != nil {
		return 0, false
	}
	return int(id), true
}
