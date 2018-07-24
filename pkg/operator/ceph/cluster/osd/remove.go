/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package osd for the Ceph OSDs.
package osd

import (
	"fmt"
	"syscall"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func removeOSD(context *clusterd.Context, namespace, deploymentName string, id int) error {
	// get a baseline for OSD usage so we can compare usage to it later on to know when migration has started
	initialUsage, err := client.GetOSDUsage(context, namespace)
	if err != nil {
		logger.Warningf("failed to get baseline OSD usage, but will still continue")
	}

	// first reweight the OSD to be 0.0, which will begin the data migration
	o, err := client.CrushReweight(context, namespace, id, 0.0)
	alreadyPurged := false
	if err != nil {
		// Special error handling for this initial step, to pick up
		// the case where the OSD is already removed from Ceph cluster,
		// and skip the next few steps if so.
		cmdErr, ok := err.(*exec.CommandError)
		if ok && cmdErr.ExitStatus() == int(syscall.ENOENT) {
			alreadyPurged = true
		} else {
			return fmt.Errorf("failed to reweight osd.%d to 0.0: %+v. %s", id, err, o)
		}
	}

	if !alreadyPurged {
		// mark the OSD as out
		if err := markOSDOut(context, namespace, id); err != nil {
			return fmt.Errorf("failed to mark osd.%d out: %+v", id, err)
		}

		// wait for the OSDs data to be migrated
		if err := waitForRebalance(context, namespace, id, initialUsage); err != nil {
			return fmt.Errorf("failed to wait for cluster rebalancing after removing osd.%d: %+v", id, err)
		}
	}

	// data is migrated off the osd, we can delete the deployment now
	if err := k8sutil.DeleteDeployment(context.Clientset, namespace, deploymentName); err != nil {
		return fmt.Errorf("failed to delete deployment %s: %+v", deploymentName, err)
	}

	// purge the OSD from the cluster
	if err := purgeOSD(context, namespace, id); err != nil {
		return fmt.Errorf("failed to purge osd.%d from the cluster: %+v", id, err)
	}

	// delete any backups of the OSD filesystem
	if err := deleteOSDFileSystem(context.Clientset, namespace, id); err != nil {
		logger.Warningf("failed to delete osd.%d filesystem, it may need to be cleaned up manually: %+v", id, err)
	}

	return nil
}

func waitForRebalance(context *clusterd.Context, namespace string, osdID int, initialUsage *client.OSDUsage) error {
	if initialUsage != nil {
		// start a retry loop to wait for rebalancing to start
		err := util.Retry(20, 5*time.Second, func() error {
			currUsage, err := client.GetOSDUsage(context, namespace)
			if err != nil {
				return err
			}

			init := initialUsage.ByID(osdID)
			curr := currUsage.ByID(osdID)

			if init == nil || curr == nil {
				return fmt.Errorf("initial OSD usage or current OSD usage for osd.%d not found. init: %+v, curr: %+v",
					osdID, initialUsage, currUsage)
			}

			var initUsedKB, initPGs int64
			if init.UsedKB != "" {
				initUsedKB, err = init.UsedKB.Int64()
				if err != nil {
					return fmt.Errorf("error converting init used KB to int64. %+v", err)
				}
			}
			if init.Pgs != "" {
				initPGs, err = init.Pgs.Int64()
				if err != nil {
					return fmt.Errorf("error converting init PGs to int64. %+v", err)
				}
			}

			// when the initial OSD PG count or used KB is zero there is nothing to do here
			if initPGs == 0 || initUsedKB == 0 {
				return nil
			}

			var currUsedKB, currPGs int64

			if curr.UsedKB != "" {
				currUsedKB, err = curr.UsedKB.Int64()
				if err != nil {
					return fmt.Errorf("error converting current used KB to int64. %+v", err)
				}
			}

			if curr.Pgs != "" {
				currPGs, err = curr.Pgs.Int64()
				if err != nil {
					return fmt.Errorf("error converting current PGs to int64. %+v", err)
				}
			}

			if currUsedKB == 0 && currPGs == 0 {
				return nil
			}

			if currUsedKB >= initUsedKB && currPGs >= initPGs {
				return fmt.Errorf("current used space and pg count for osd.%d has not decreased still. curr=%+v", osdID, curr)
			}

			// either the used space or the number of PGs has decreased for the OSD, data rebalancing has started
			return nil
		})
		if err != nil {
			return err
		}
	}

	// wait until the cluster gets fully rebalanced again
	err := util.Retry(3000, 15*time.Second, func() error {
		// get a dump of all placement groups
		pgDump, err := client.GetPGDumpBrief(context, namespace)
		if err != nil {
			return err
		}

		// ensure that the given OSD is no longer assigned to any placement groups
		for _, pg := range pgDump {
			if pg.UpPrimaryID == osdID {
				return fmt.Errorf("osd.%d is still up primary for pg %s", osdID, pg.ID)
			}
			if pg.ActingPrimaryID == osdID {
				return fmt.Errorf("osd.%d is still acting primary for pg %s", osdID, pg.ID)
			}
			for _, id := range pg.UpOsdIDs {
				if id == osdID {
					return fmt.Errorf("osd.%d is still up for pg %s", osdID, pg.ID)
				}
			}
			for _, id := range pg.ActingOsdIDs {
				if id == osdID {
					return fmt.Errorf("osd.%d is still acting for pg %s", osdID, pg.ID)
				}
			}
		}

		// finally, ensure the cluster gets back to a clean state, meaning rebalancing is complete
		return client.IsClusterClean(context, namespace)
	})
	if err != nil {
		return err
	}

	return nil
}

func markOSDOut(context *clusterd.Context, namespace string, id int) error {
	_, err := client.OSDOut(context, namespace, id)
	return err
}

func purgeOSD(context *clusterd.Context, namespace string, id int) error {
	// remove the OSD from the crush map
	_, err := client.CrushRemove(context, namespace, fmt.Sprintf("osd.%d", id))
	if err != nil {
		return fmt.Errorf("failed to remove osd.%d from crush map. %v", id, err)
	}

	// delete the auth for the OSD
	err = client.AuthDelete(context, namespace, fmt.Sprintf("osd.%d", id))
	if err != nil {
		return err
	}

	// delete the OSD from the cluster
	_, err = client.OSDRemove(context, namespace, id)
	if err != nil {
		return fmt.Errorf("failed to rm osd.%d. %v", id, err)
	}
	return nil
}

func deleteOSDFileSystem(clientset kubernetes.Interface, namespace string, id int) error {
	logger.Infof("Deleting OSD %d file system", id)
	storeName := fmt.Sprintf(config.OSDFSStoreNameFmt, id)
	err := clientset.CoreV1().ConfigMaps(namespace).Delete(storeName, &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (c *Cluster) cleanUpNodeResources(nodeName, nodeCrushName string) error {

	if nodeCrushName != "" {
		// we have the crush name for this node, meaning we should remove it from the crush map
		if o, err := client.CrushRemove(c.context, c.Namespace, nodeCrushName); err != nil {
			return fmt.Errorf("failed to remove node %s from crush map.  %+v.  %s", nodeCrushName, err, o)
		}
	}

	// clean up node config store
	configStoreName := config.GetConfigStoreName(nodeName)
	if err := c.kv.ClearStore(configStoreName); err != nil {
		logger.Warningf("failed to delete node config store %s, may need to be cleaned up manually: %+v", configStoreName, err)
	}

	return nil
}
