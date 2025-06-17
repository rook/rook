/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package cleanup

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (

	// ClientBlocklistDuration is the duration (in seconds) for which the client IP will be blocklisted
	ClientBlocklistDuration = "1200"
)

func RadosNamespaceCleanup(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, poolName, radosNamespace string) error {
	logger.Infof("starting clean up of CephBlockPoolRadosNamespace %q resources in cephblockpool %q", radosNamespace, poolName)

	err := cleanupImages(context, clusterInfo, poolName, radosNamespace)
	if err != nil {
		logger.Errorf("failed to clean up CephBlockPoolRadosNamespace %q resources in cephblockpool %q", radosNamespace, poolName)
		return err
	}

	logger.Infof("successfully cleaned up CephBlockPoolRadosNamespace %q resources in cephblockpool %q", radosNamespace, poolName)
	return nil
}

func cleanupImages(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, poolName, radosNamespace string) error {
	msg := fmt.Sprintf("cephblockpool %q", poolName)
	if radosNamespace != "" {
		msg = fmt.Sprintf("%s in rados namespace %q", msg, radosNamespace)
	}
	images, err := cephclient.ListImagesInRadosNamespace(context, clusterInfo, poolName, radosNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to list images in %s", msg)
	}

	err = blocklistClientIPs(context, clusterInfo, images, poolName, radosNamespace)
	if err != nil {
		return errors.Wrap(err, "failed to add client IPs to the blocklist")
	}

	var retErr error
	for _, image := range images {
		snaps, err := cephclient.ListSnapshotsInRadosNamespace(context, clusterInfo, poolName, image.Name, radosNamespace)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to list snapshots for the image %q in %s", image.Name, msg)
			logger.Error(retErr)
		}

		for _, snap := range snaps {
			err := cephclient.DeleteSnapshotInRadosNamespace(context, clusterInfo, poolName, image.Name, snap.Name, radosNamespace)
			if err != nil {
				retErr = errors.Wrapf(err, "failed to delete snapshot %q of the image %q in %s", snap.Name, image.Name, msg)
				logger.Error(retErr)
			} else {
				logger.Infof("successfully deleted snapshot %q of image %q in %s", snap.Name, image.Name, msg)
			}
		}

		err = cephclient.MoveImageToTrashInRadosNamespace(context, clusterInfo, poolName, image.Name, radosNamespace)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to move image %q to trash in %s", image.Name, msg)
			logger.Error(retErr)
		}
		err = cephclient.DeleteImageFromTrashInRadosNamespace(context, clusterInfo, poolName, image.ID, radosNamespace)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to add task to remove image %q from trash in %s", image.Name, msg)
			logger.Error(retErr)
		}
	}
	return retErr
}

func BlockPoolCleanup(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, poolName string) error {
	logger.Infof("starting clean up of CephBlockPool %q resource", poolName)

	err := cleanupImages(context, clusterInfo, poolName, "")
	if err != nil {
		logger.Errorf("failed to clean up CephBlockPool %q resource", poolName)
		return err
	}

	logger.Infof("successfully cleaned up CephBlockPool %q resource", poolName)
	return nil
}

func blocklistClientIPs(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, images []cephclient.CephBlockImage, poolName, radosNamespace string) error {
	ips, err := getClientIPs(context, clusterInfo, images, poolName, radosNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to get client IPs for all the images in pool %q in namespace %q", poolName, radosNamespace)
	}

	if len(ips) == 0 {
		logger.Info("no client IPs found for images in pool %q in rados namespace %q", poolName, radosNamespace)
	}

	for ip := range ips {
		logger.Infof("blocklist client IP %q for images in pool %q in namespace %q", ip, poolName, radosNamespace)
		err = cephclient.BlocklistIP(context, clusterInfo, ip, ClientBlocklistDuration)
		if err != nil {
			return errors.Wrapf(err, "failed to blocklist IP  %q in pool %q in namespace %q", ip, poolName, radosNamespace)
		}
		logger.Infof("successfully blocklisted client IP %q in pool %q in namespace %q", ip, poolName, radosNamespace)
	}

	return nil
}

func getClientIPs(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, images []cephclient.CephBlockImage, poolName, radosNamespace string) (sets.Set[string], error) {
	clientIPs := sets.New[string]()
	for _, image := range images {
		rbdStatus, err := cephclient.GetRBDImageStatus(context, clusterInfo, poolName, image.Name, radosNamespace)
		if err != nil {
			return clientIPs, errors.Wrapf(err, "failed to list watchers for the image %q in %s", image.Name, radosNamespace)
		}
		ips := rbdStatus.GetWatcherIPs()
		if len(ips) == 0 {
			logger.Infof("no watcher IPs found for image %q in pool %q in namespace %q", image.Name, poolName, radosNamespace)
			continue
		}

		logger.Infof("watcher IPs for image %q in pool %q in namespace %q: %v", image.Name, poolName, radosNamespace, ips)
		for _, ip := range ips {
			clientIPs.Insert(ip)
		}
	}

	logger.Infof("client IPs : %v", clientIPs)
	return clientIPs, nil
}
